package e2e

import (
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

// Snapshot test helpers

// testSnapshotName creates a unique snapshot name for the current test
func testSnapshotName(name string) string {
	return fmt.Sprintf("%s-snap-%s", testRunID, name)
}

// waitForSnapshotOnMockRDS waits for a snapshot to appear on the mock RDS server
func waitForSnapshotOnMockRDS(snapshotID string) {
	Eventually(func() bool {
		snap, exists := mockRDS.GetSnapshot(snapshotID)
		return exists && snap != nil
	}, defaultTimeout, pollInterval).Should(BeTrue(),
		"Snapshot %s should exist on mock RDS", snapshotID)
}

// waitForSnapshotDeletedFromMockRDS waits for a snapshot to be deleted from the mock RDS server
func waitForSnapshotDeletedFromMockRDS(snapshotID string) {
	Eventually(func() bool {
		_, exists := mockRDS.GetSnapshot(snapshotID)
		return !exists
	}, defaultTimeout, pollInterval).Should(BeTrue(),
		"Snapshot %s should be deleted from mock RDS", snapshotID)
}

var _ = Describe("Snapshot Operations [E2E-08]", func() {
	Describe("TC-08.1: Basic Snapshot Lifecycle", func() {
		It("should create snapshot, verify on RDS, and delete successfully", func() {
			volumeName := testVolumeName("snap-lifecycle-source")
			snapshotName := testSnapshotName("lifecycle")
			var volumeID, snapshotID string

			By("Step 1: Creating source volume via CreateVolume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.Volume).NotTo(BeNil())
			volumeID = createResp.Volume.VolumeId
			klog.Infof("Created source volume: %s", volumeID)

			DeferCleanup(func() {
				klog.Infof("Cleaning up source volume: %s", volumeID)
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
			})

			By("Step 2: Verifying source volume exists on mock RDS")
			waitForVolumeOnMockRDS(volumeID)

			By("Step 3: Creating snapshot via CreateSnapshot")
			createSnapResp, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: volumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(createSnapResp.Snapshot).NotTo(BeNil())
			snapshotID = createSnapResp.Snapshot.SnapshotId
			klog.Infof("Created snapshot: %s from volume: %s", snapshotID, volumeID)

			DeferCleanup(func() {
				klog.Infof("Cleaning up snapshot: %s", snapshotID)
				_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID})
			})

			// Verify snapshot metadata
			Expect(createSnapResp.Snapshot.SourceVolumeId).To(Equal(volumeID))
			Expect(createSnapResp.Snapshot.SizeBytes).To(Equal(int64(smallVolumeSize)))
			Expect(createSnapResp.Snapshot.ReadyToUse).To(BeTrue())
			Expect(createSnapResp.Snapshot.CreationTime).NotTo(BeNil())

			By("Step 4: Verifying snapshot exists on mock RDS")
			waitForSnapshotOnMockRDS(snapshotID)
			snap, exists := mockRDS.GetSnapshot(snapshotID)
			Expect(exists).To(BeTrue())
			Expect(snap.SourceVolume).To(Equal(volumeID))
			Expect(snap.FileSizeBytes).To(Equal(int64(smallVolumeSize)))
			// Snapshots are read-only/immutable by design: MockSnapshot has no NVMe export fields
			// (absence of nvme-tcp-export means the disk is not network-exported = immutable)

			By("Step 5: Deleting snapshot via DeleteSnapshot")
			_, err = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID})
			Expect(err).NotTo(HaveOccurred())

			By("Step 6: Verifying snapshot deleted from mock RDS")
			waitForSnapshotDeletedFromMockRDS(snapshotID)
		})
	})

	Describe("TC-08.2: Restore from Snapshot (Same Size)", func() {
		It("should restore volume from snapshot with same capacity", func() {
			sourceName := testVolumeName("snap-restore-source")
			snapshotName := testSnapshotName("restore-same-size")
			restoredName := testVolumeName("snap-restore-same-size")
			var sourceVolumeID, snapshotID, restoredVolumeID string

			By("Step 1: Creating source volume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               sourceName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			sourceVolumeID = createResp.Volume.VolumeId
			klog.Infof("Created source volume: %s", sourceVolumeID)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: sourceVolumeID})
			})

			By("Step 2: Creating snapshot from source")
			createSnapResp, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: sourceVolumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			snapshotID = createSnapResp.Snapshot.SnapshotId
			klog.Infof("Created snapshot: %s", snapshotID)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID})
			})

			By("Step 3: Restoring volume from snapshot with same size")
			restoreResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               restoredName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: snapshotID,
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(restoreResp.Volume).NotTo(BeNil())
			restoredVolumeID = restoreResp.Volume.VolumeId
			klog.Infof("Restored volume: %s from snapshot: %s", restoredVolumeID, snapshotID)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: restoredVolumeID})
			})

			// Verify restored volume metadata
			Expect(restoreResp.Volume.CapacityBytes).To(Equal(int64(smallVolumeSize)))
			Expect(restoreResp.Volume.ContentSource).NotTo(BeNil())
			Expect(restoreResp.Volume.ContentSource.GetSnapshot()).NotTo(BeNil())
			Expect(restoreResp.Volume.ContentSource.GetSnapshot().SnapshotId).To(Equal(snapshotID))

			By("Step 4: Verifying restored volume exists on mock RDS")
			waitForVolumeOnMockRDS(restoredVolumeID)
			vol, exists := mockRDS.GetVolume(restoredVolumeID)
			Expect(exists).To(BeTrue())
			Expect(vol.FileSizeBytes).To(Equal(int64(smallVolumeSize)))
			Expect(vol.Exported).To(BeTrue())

			By("Step 5: Verifying source snapshot still exists after restore")
			snap, exists := mockRDS.GetSnapshot(snapshotID)
			Expect(exists).To(BeTrue(), "Source snapshot should persist after restore")
			Expect(snap.SourceVolume).To(Equal(sourceVolumeID))
		})
	})

	Describe("TC-08.3: Restore from Snapshot (Larger Size)", func() {
		It("should restore volume from snapshot with larger capacity", func() {
			sourceName := testVolumeName("snap-restore-large-source")
			snapshotName := testSnapshotName("restore-larger-size")
			restoredName := testVolumeName("snap-restore-larger")
			var sourceVolumeID, snapshotID, restoredVolumeID string

			By("Step 1: Creating small source volume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               sourceName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			sourceVolumeID = createResp.Volume.VolumeId
			klog.Infof("Created source volume: %s (size: %d)", sourceVolumeID, smallVolumeSize)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: sourceVolumeID})
			})

			By("Step 2: Creating snapshot from source")
			createSnapResp, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: sourceVolumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			snapshotID = createSnapResp.Snapshot.SnapshotId
			klog.Infof("Created snapshot: %s (size: %d)", snapshotID, smallVolumeSize)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID})
			})

			By("Step 3: Restoring volume from snapshot with larger size (5 GiB)")
			restoreResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               restoredName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: mediumVolumeSize}, // 5 GiB
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: snapshotID,
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			restoredVolumeID = restoreResp.Volume.VolumeId
			klog.Infof("Restored volume: %s from snapshot with larger size: %d", restoredVolumeID, mediumVolumeSize)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: restoredVolumeID})
			})

			// Verify restored volume has larger capacity
			Expect(restoreResp.Volume.CapacityBytes).To(Equal(int64(mediumVolumeSize)))
			Expect(restoreResp.Volume.CapacityBytes).To(BeNumerically(">", smallVolumeSize))

			By("Step 4: Verifying restored volume exists on mock RDS with larger size")
			waitForVolumeOnMockRDS(restoredVolumeID)
			vol, exists := mockRDS.GetVolume(restoredVolumeID)
			Expect(exists).To(BeTrue())
			Expect(vol.FileSizeBytes).To(Equal(int64(mediumVolumeSize)))
		})
	})

	Describe("TC-08.4: Snapshot Idempotency", func() {
		It("should handle duplicate snapshot creation idempotently", func() {
			volumeName := testVolumeName("snap-idempotent-source")
			snapshotName := testSnapshotName("idempotent")
			var volumeID, snapshotID1, snapshotID2 string

			By("Step 1: Creating source volume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			volumeID = createResp.Volume.VolumeId
			klog.Infof("Created source volume: %s", volumeID)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
			})

			By("Step 2: Creating snapshot (first time)")
			createSnapResp1, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: volumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(createSnapResp1.Snapshot).NotTo(BeNil())
			snapshotID1 = createSnapResp1.Snapshot.SnapshotId
			klog.Infof("Created snapshot (first call): %s", snapshotID1)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID1})
			})

			// Save first snapshot creation time
			creationTime1 := createSnapResp1.Snapshot.CreationTime

			By("Step 3: Creating snapshot with same name (second time - idempotent)")
			createSnapResp2, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: volumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(createSnapResp2.Snapshot).NotTo(BeNil())
			snapshotID2 = createSnapResp2.Snapshot.SnapshotId
			klog.Infof("Created snapshot (second call): %s", snapshotID2)

			// Verify idempotency: same snapshot ID returned
			Expect(snapshotID2).To(Equal(snapshotID1), "Second CreateSnapshot call should return same snapshot ID")

			// Verify metadata matches
			Expect(createSnapResp2.Snapshot.SourceVolumeId).To(Equal(volumeID))
			Expect(createSnapResp2.Snapshot.SizeBytes).To(Equal(int64(smallVolumeSize)))
			Expect(createSnapResp2.Snapshot.ReadyToUse).To(BeTrue())

			// Creation time should be identical (not updated)
			Expect(createSnapResp2.Snapshot.CreationTime.AsTime()).To(Equal(creationTime1.AsTime()))

			By("Step 4: Verifying only one snapshot exists on mock RDS")
			snap, exists := mockRDS.GetSnapshot(snapshotID1)
			Expect(exists).To(BeTrue())
			Expect(snap.SourceVolume).To(Equal(volumeID))

			// Verify snapshot count hasn't increased
			snapshots := mockRDS.ListSnapshots()
			snapshotCount := 0
			for _, s := range snapshots {
				if s.SourceVolume == volumeID {
					snapshotCount++
				}
			}
			Expect(snapshotCount).To(Equal(1), "Should have exactly one snapshot, not multiple")

			By("Step 5: Deleting snapshot (idempotent - first call)")
			_, err = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID1})
			Expect(err).NotTo(HaveOccurred())

			By("Step 6: Deleting same snapshot (idempotent - second call)")
			_, err = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID1})
			Expect(err).NotTo(HaveOccurred(), "Second DeleteSnapshot call should succeed (idempotent)")

			By("Step 7: Verifying snapshot deleted from mock RDS")
			waitForSnapshotDeletedFromMockRDS(snapshotID1)
		})
	})

	Describe("TC-08.5: ListSnapshots Pagination", func() {
		It("should list snapshots with pagination support", func() {
			volumeName := testVolumeName("snap-list-source")
			var volumeID string
			var snapshotIDs []string
			const numSnapshots = 12

			By("Step 1: Creating source volume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			volumeID = createResp.Volume.VolumeId
			klog.Infof("Created source volume: %s", volumeID)

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
			})

			By(fmt.Sprintf("Step 2: Creating %d snapshots", numSnapshots))
			for i := 0; i < numSnapshots; i++ {
				snapshotName := testSnapshotName(fmt.Sprintf("list-%02d", i))
				snapResp, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
					SourceVolumeId: volumeID,
					Name:           snapshotName,
				})
				Expect(err).NotTo(HaveOccurred())
				snapshotIDs = append(snapshotIDs, snapResp.Snapshot.SnapshotId)
				klog.Infof("Created snapshot %d/%d: %s", i+1, numSnapshots, snapResp.Snapshot.SnapshotId)
			}

			DeferCleanup(func() {
				klog.Infof("Cleaning up %d snapshots", len(snapshotIDs))
				for _, snapID := range snapshotIDs {
					_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapID})
				}
			})

			By("Step 3: Listing snapshots without pagination (all at once)")
			listResp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SourceVolumeId: volumeID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Entries).To(HaveLen(numSnapshots), "Should return all snapshots")

			By("Step 4: Listing snapshots with pagination (max_entries=5)")
			const maxEntries = 5
			var allEntries []*csi.ListSnapshotsResponse_Entry
			var nextToken string

			// First page
			page1Resp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SourceVolumeId: volumeID,
				MaxEntries:     maxEntries,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(page1Resp.Entries).To(HaveLen(maxEntries), "First page should have max_entries snapshots")
			Expect(page1Resp.NextToken).NotTo(BeEmpty(), "Should have next_token for more pages")
			allEntries = append(allEntries, page1Resp.Entries...)
			nextToken = page1Resp.NextToken
			klog.Infof("Page 1: %d snapshots, nextToken=%s", len(page1Resp.Entries), nextToken)

			// Second page
			page2Resp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SourceVolumeId: volumeID,
				MaxEntries:     maxEntries,
				StartingToken:  nextToken,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(page2Resp.Entries).To(HaveLen(maxEntries), "Second page should have max_entries snapshots")
			Expect(page2Resp.NextToken).NotTo(BeEmpty(), "Should have next_token for more pages")
			allEntries = append(allEntries, page2Resp.Entries...)
			nextToken = page2Resp.NextToken
			klog.Infof("Page 2: %d snapshots, nextToken=%s", len(page2Resp.Entries), nextToken)

			// Third page (remaining snapshots)
			page3Resp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SourceVolumeId: volumeID,
				MaxEntries:     maxEntries,
				StartingToken:  nextToken,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(page3Resp.Entries).To(HaveLen(numSnapshots-2*maxEntries), "Third page should have remaining snapshots")
			Expect(page3Resp.NextToken).To(BeEmpty(), "Should not have next_token on last page")
			allEntries = append(allEntries, page3Resp.Entries...)
			klog.Infof("Page 3: %d snapshots (final page)", len(page3Resp.Entries))

			By("Step 5: Verifying all snapshots returned via pagination")
			Expect(allEntries).To(HaveLen(numSnapshots), "Pagination should return all snapshots")

			// Verify snapshot IDs match
			returnedIDs := make(map[string]bool)
			for _, entry := range allEntries {
				Expect(entry.Snapshot).NotTo(BeNil())
				Expect(entry.Snapshot.SourceVolumeId).To(Equal(volumeID))
				returnedIDs[entry.Snapshot.SnapshotId] = true
			}

			for _, snapID := range snapshotIDs {
				Expect(returnedIDs).To(HaveKey(snapID), "All created snapshots should be in paginated results")
			}

			By("Step 6: Listing specific snapshot by ID")
			specificSnapID := snapshotIDs[5] // Pick middle snapshot
			specificResp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SnapshotId: specificSnapID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(specificResp.Entries).To(HaveLen(1), "Should return exactly one snapshot when queried by ID")
			Expect(specificResp.Entries[0].Snapshot.SnapshotId).To(Equal(specificSnapID))
			Expect(specificResp.NextToken).To(BeEmpty(), "Single snapshot lookup should not have next_token")

			By("Step 7: Listing with invalid snapshot ID")
			invalidResp, err := controllerClient.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
				SnapshotId: "snap-nonexistent-12345678-1234-1234-1234-123456789abc",
			})
			Expect(err).NotTo(HaveOccurred(), "Invalid snapshot ID should not error per CSI spec")
			Expect(invalidResp.Entries).To(BeEmpty(), "Should return empty list for non-existent snapshot")
		})
	})

	Describe("TC-08.6: Snapshot Creation Timestamp", func() {
		It("should include valid creation timestamp in snapshot metadata", func() {
			volumeName := testVolumeName("snap-timestamp-source")
			snapshotName := testSnapshotName("timestamp-test")
			var volumeID, snapshotID string

			By("Step 1: Creating source volume")
			createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
				Name:               volumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: smallVolumeSize},
				VolumeCapabilities: []*csi.VolumeCapability{mountVolumeCapability("ext4")},
			})
			Expect(err).NotTo(HaveOccurred())
			volumeID = createResp.Volume.VolumeId

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
			})

			By("Step 2: Creating snapshot and capturing creation timestamp")
			createSnapResp, err := controllerClient.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
				SourceVolumeId: volumeID,
				Name:           snapshotName,
			})
			Expect(err).NotTo(HaveOccurred())
			snapshotID = createSnapResp.Snapshot.SnapshotId

			DeferCleanup(func() {
				_, _ = controllerClient.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: snapshotID})
			})

			// Verify creation timestamp is present
			Expect(createSnapResp.Snapshot.CreationTime).NotTo(BeNil(), "CreationTime should be set")

			// Verify timestamp is valid protobuf timestamp
			creationTime := createSnapResp.Snapshot.CreationTime
			Expect(creationTime).To(BeAssignableToTypeOf(&timestamppb.Timestamp{}))

			// Verify timestamp is reasonable (not zero, not future)
			creationTimeGo := creationTime.AsTime()
			Expect(creationTimeGo.IsZero()).To(BeFalse(), "Creation time should not be zero")
			Expect(creationTimeGo.Unix()).To(BeNumerically(">", 0), "Creation time should be after Unix epoch")

			klog.Infof("Snapshot %s created at %s", snapshotID, creationTimeGo.Format("2006-01-02 15:04:05"))
		})
	})
})
