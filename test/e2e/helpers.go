package e2e

import (
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Constants for test configuration
const (
	GiB                  = 1024 * 1024 * 1024
	MiB                  = 1024 * 1024
	defaultTimeout       = 2 * time.Minute
	pollInterval         = 200 * time.Millisecond
	testVolumeBasePath   = "/storage-pool/metal-csi"
)

// testVolumeName creates a unique volume name for the current test
// by prepending the test run ID to ensure isolation between test runs
func testVolumeName(name string) string {
	return fmt.Sprintf("%s-%s", testRunID, name)
}

// mountVolumeCapability returns a mount volume capability with SINGLE_NODE_WRITER access mode
func mountVolumeCapability(fsType string) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				FsType: fsType,
			},
		},
	}
}

// blockVolumeCapability returns a block volume capability with SINGLE_NODE_WRITER access mode
func blockVolumeCapability() *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}
}

// createVolumeWithCleanup creates a volume and automatically registers cleanup
// using Ginkgo's DeferCleanup to ensure the volume is deleted even if the test fails
func createVolumeWithCleanup(name string, sizeBytes int64, capability *csi.VolumeCapability) string {
	resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name:               name,
		CapacityRange:      &csi.CapacityRange{RequiredBytes: sizeBytes},
		VolumeCapabilities: []*csi.VolumeCapability{capability},
	})
	Expect(err).NotTo(HaveOccurred(), "CreateVolume should succeed")
	Expect(resp.Volume).NotTo(BeNil())

	volumeID := resp.Volume.VolumeId
	DeferCleanup(func() {
		_, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
	})

	return volumeID
}

// waitForVolumeOnMockRDS waits for a volume to appear on the mock RDS server
// and be exported via NVMe/TCP
func waitForVolumeOnMockRDS(volumeID string) {
	Eventually(func() bool {
		vol, exists := mockRDS.GetVolume(volumeID)
		return exists && vol.Exported
	}, defaultTimeout, pollInterval).Should(BeTrue(),
		"Volume %s should exist and be exported on mock RDS", volumeID)
}

// waitForVolumeDeletedFromMockRDS waits for a volume to be deleted from the mock RDS server
func waitForVolumeDeletedFromMockRDS(volumeID string) {
	Eventually(func() bool {
		_, exists := mockRDS.GetVolume(volumeID)
		return !exists
	}, defaultTimeout, pollInterval).Should(BeTrue(),
		"Volume %s should be deleted from mock RDS", volumeID)
}
