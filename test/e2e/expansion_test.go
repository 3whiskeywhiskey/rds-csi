package e2e

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"
)

var _ = Describe("Volume Expansion [E2E-03]", func() {
	It("should expand volume via ControllerExpandVolume", func() {
		volumeName := testVolumeName("expansion")
		initialSize := int64(5 * GiB)
		expandedSize := int64(10 * GiB)

		By("Creating initial volume")
		createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      &csi.CapacityRange{RequiredBytes: initialSize},
			VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
		})
		Expect(err).NotTo(HaveOccurred())
		volumeID := createResp.Volume.VolumeId
		DeferCleanup(func() {
			_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
		})

		By("Verifying initial size on mock RDS")
		waitForVolumeOnMockRDS(volumeID)
		vol, _ := mockRDS.GetVolume(volumeID)
		Expect(vol.FileSizeBytes).To(Equal(initialSize))

		By("Expanding volume via ControllerExpandVolume")
		expandResp, err := controllerClient.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
			VolumeId: volumeID,
			CapacityRange: &csi.CapacityRange{
				RequiredBytes: expandedSize,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(expandResp.CapacityBytes).To(Equal(expandedSize))

		By("Verifying expanded size on mock RDS")
		Eventually(func() int64 {
			vol, exists := mockRDS.GetVolume(volumeID)
			if !exists {
				return 0
			}
			return vol.FileSizeBytes
		}, defaultTimeout, pollInterval).Should(Equal(expandedSize))

		klog.Infof("Volume expansion test passed: %s expanded from %d to %d bytes",
			volumeID, initialSize, expandedSize)
	})

	It("should handle expansion to same size (idempotent)", func() {
		volumeName := testVolumeName("expansion-idempotent")
		size := int64(5 * GiB)

		By("Creating volume")
		createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      &csi.CapacityRange{RequiredBytes: size},
			VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
		})
		Expect(err).NotTo(HaveOccurred())
		volumeID := createResp.Volume.VolumeId
		DeferCleanup(func() {
			_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
		})

		waitForVolumeOnMockRDS(volumeID)

		By("Expanding to same size (idempotent no-op)")
		expandResp, err := controllerClient.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
			VolumeId:      volumeID,
			CapacityRange: &csi.CapacityRange{RequiredBytes: size},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(expandResp.CapacityBytes).To(BeNumerically(">=", size))

		klog.Infof("Expansion idempotency test passed for %s", volumeID)
	})

	It("should report NodeExpansionRequired for filesystem volumes", func() {
		volumeName := testVolumeName("expansion-node-required")

		By("Creating filesystem volume")
		createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      &csi.CapacityRange{RequiredBytes: int64(5 * GiB)},
			VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
		})
		Expect(err).NotTo(HaveOccurred())
		volumeID := createResp.Volume.VolumeId
		DeferCleanup(func() {
			_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
		})

		waitForVolumeOnMockRDS(volumeID)

		By("Expanding volume and checking NodeExpansionRequired")
		expandResp, err := controllerClient.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
			VolumeId:         volumeID,
			CapacityRange:    &csi.CapacityRange{RequiredBytes: int64(10 * GiB)},
			VolumeCapability: mountVolumeCapability("ext4"),
		})
		Expect(err).NotTo(HaveOccurred())
		// For mount volumes, node expansion is needed to resize filesystem
		Expect(expandResp.NodeExpansionRequired).To(BeTrue(),
			"Mount volume expansion should require node expansion for filesystem resize")

		klog.Infof("NodeExpansionRequired test passed for %s", volumeID)
	})

	It("should expand block volume without NodeExpansionRequired", func() {
		volumeName := testVolumeName("expansion-block")

		By("Creating block volume")
		createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      &csi.CapacityRange{RequiredBytes: int64(5 * GiB)},
			VolumeCapabilities: []*csi.VolumeCapability{blockVolumeCapability()},
		})
		Expect(err).NotTo(HaveOccurred())
		volumeID := createResp.Volume.VolumeId
		DeferCleanup(func() {
			_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
		})

		waitForVolumeOnMockRDS(volumeID)

		By("Expanding block volume")
		expandResp, err := controllerClient.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
			VolumeId:         volumeID,
			CapacityRange:    &csi.CapacityRange{RequiredBytes: int64(10 * GiB)},
			VolumeCapability: blockVolumeCapability(),
		})
		Expect(err).NotTo(HaveOccurred())
		// Block volumes don't need filesystem expansion
		Expect(expandResp.NodeExpansionRequired).To(BeFalse(),
			"Block volume expansion should not require node expansion")

		klog.Infof("Block volume expansion test passed for %s", volumeID)
	})
})
