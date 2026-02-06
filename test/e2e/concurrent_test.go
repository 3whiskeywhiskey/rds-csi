package e2e

import (
	"fmt"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"
)

var _ = Describe("Concurrent Operations [E2E-04]", func() {
	const numConcurrentVolumes = 5

	It("should handle concurrent CreateVolume operations without conflicts", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, numConcurrentVolumes)
		volumeIDs := make([]string, numConcurrentVolumes)
		var volumeIDsMu sync.Mutex

		By(fmt.Sprintf("Creating %d volumes concurrently", numConcurrentVolumes))
		for i := 0; i < numConcurrentVolumes; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				volumeName := testVolumeName(fmt.Sprintf("concurrent-%d", idx))
				resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
					Name:               volumeName,
					CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
					VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
				})
				if err != nil {
					errChan <- fmt.Errorf("CreateVolume for volume %d failed: %w", idx, err)
					return
				}

				volumeIDsMu.Lock()
				volumeIDs[idx] = resp.Volume.VolumeId
				volumeIDsMu.Unlock()

				klog.V(2).Infof("Created concurrent volume %d: %s", idx, resp.Volume.VolumeId)
				errChan <- nil
			}(i)
		}

		wg.Wait()
		close(errChan)

		// Collect errors
		var errors []error
		for err := range errChan {
			if err != nil {
				errors = append(errors, err)
			}
		}
		Expect(errors).To(BeEmpty(), "All concurrent CreateVolume operations should succeed")

		// Register cleanup for all volumes
		DeferCleanup(func() {
			var cleanupWg sync.WaitGroup
			for _, volID := range volumeIDs {
				if volID != "" {
					cleanupWg.Add(1)
					go func(id string) {
						defer cleanupWg.Done()
						_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: id})
					}(volID)
				}
			}
			cleanupWg.Wait()
		})

		By("Verifying all volumes exist on mock RDS")
		Eventually(func() int {
			count := 0
			volumeIDsMu.Lock()
			defer volumeIDsMu.Unlock()
			for _, volID := range volumeIDs {
				if volID != "" {
					if _, exists := mockRDS.GetVolume(volID); exists {
						count++
					}
				}
			}
			return count
		}, defaultTimeout, pollInterval).Should(Equal(numConcurrentVolumes),
			"All %d volumes should exist on mock RDS", numConcurrentVolumes)

		klog.Infof("Concurrent create test passed: %d volumes created", numConcurrentVolumes)
	})

	It("should handle concurrent DeleteVolume operations without conflicts", func() {
		// First create volumes sequentially for reliability
		volumeIDs := make([]string, numConcurrentVolumes)

		By("Creating volumes for concurrent delete test")
		for i := 0; i < numConcurrentVolumes; i++ {
			volumeName := testVolumeName(fmt.Sprintf("concurrent-delete-%d", i))
			resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			volumeIDs[i] = resp.Volume.VolumeId
			waitForVolumeOnMockRDS(volumeIDs[i])
		}

		By(fmt.Sprintf("Deleting %d volumes concurrently", numConcurrentVolumes))
		var wg sync.WaitGroup
		errChan := make(chan error, numConcurrentVolumes)

		for i := 0; i < numConcurrentVolumes; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				_, err := controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
					VolumeId: volumeIDs[idx],
				})
				if err != nil {
					errChan <- fmt.Errorf("DeleteVolume for volume %d failed: %w", idx, err)
					return
				}
				klog.V(2).Infof("Deleted concurrent volume %d: %s", idx, volumeIDs[idx])
				errChan <- nil
			}(i)
		}

		wg.Wait()
		close(errChan)

		// Collect errors
		var errors []error
		for err := range errChan {
			if err != nil {
				errors = append(errors, err)
			}
		}
		Expect(errors).To(BeEmpty(), "All concurrent DeleteVolume operations should succeed")

		By("Verifying all volumes deleted from mock RDS")
		Eventually(func() int {
			count := 0
			for _, volID := range volumeIDs {
				if _, exists := mockRDS.GetVolume(volID); exists {
					count++
				}
			}
			return count
		}, defaultTimeout, pollInterval).Should(Equal(0), "All volumes should be deleted from mock RDS")

		klog.Infof("Concurrent delete test passed: %d volumes deleted", numConcurrentVolumes)
	})

	It("should handle mixed concurrent create and delete operations", func() {
		const numOperations = 10 // 5 creates + 5 deletes
		var wg sync.WaitGroup
		errChan := make(chan error, numOperations)

		// Pre-create volumes to delete
		deleteVolumes := make([]string, 5)
		for i := 0; i < 5; i++ {
			volumeName := testVolumeName(fmt.Sprintf("mixed-delete-%d", i))
			resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			deleteVolumes[i] = resp.Volume.VolumeId
			waitForVolumeOnMockRDS(deleteVolumes[i])
		}

		createVolumes := make([]string, 5)
		var createVolumesMu sync.Mutex

		By("Running mixed concurrent creates and deletes")
		// Start creates
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				volumeName := testVolumeName(fmt.Sprintf("mixed-create-%d", idx))
				resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
					Name:               volumeName,
					CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
					VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
				})
				if err != nil {
					errChan <- fmt.Errorf("mixed create %d failed: %w", idx, err)
					return
				}
				createVolumesMu.Lock()
				createVolumes[idx] = resp.Volume.VolumeId
				createVolumesMu.Unlock()
				errChan <- nil
			}(i)
		}

		// Start deletes
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				_, err := controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
					VolumeId: deleteVolumes[idx],
				})
				if err != nil {
					errChan <- fmt.Errorf("mixed delete %d failed: %w", idx, err)
					return
				}
				errChan <- nil
			}(i)
		}

		wg.Wait()
		close(errChan)

		// Cleanup created volumes
		DeferCleanup(func() {
			for _, volID := range createVolumes {
				if volID != "" {
					_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volID})
				}
			}
		})

		// Collect errors
		var errors []error
		for err := range errChan {
			if err != nil {
				errors = append(errors, err)
			}
		}
		Expect(errors).To(BeEmpty(), "All mixed operations should succeed")

		klog.Infof("Mixed concurrent operations test passed")
	})
})
