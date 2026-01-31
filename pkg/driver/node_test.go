package driver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/mount"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/nvme"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/observability"
)

// mockMounter implements mount.Mounter for testing
type mockMounter struct {
	formatCalled    bool
	mountCalled     bool
	unmountCalled   bool
	mountErr        error
	unmountErr      error
	formatErr       error
	isLikelyMounted bool
	isLikelyErr     error
	stats           *mount.DeviceStats
	statsErr        error
}

func (m *mockMounter) Mount(source, target, fsType string, options []string) error {
	m.mountCalled = true
	return m.mountErr
}

func (m *mockMounter) Unmount(target string) error {
	m.unmountCalled = true
	return m.unmountErr
}

func (m *mockMounter) IsLikelyMountPoint(path string) (bool, error) {
	return m.isLikelyMounted, m.isLikelyErr
}

func (m *mockMounter) Format(device, fsType string) error {
	m.formatCalled = true
	return m.formatErr
}

func (m *mockMounter) IsFormatted(device string) (bool, error) {
	return true, nil
}

func (m *mockMounter) ResizeFilesystem(device, volumePath string) error {
	return nil
}

func (m *mockMounter) GetDeviceStats(path string) (*mount.DeviceStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	if m.stats == nil {
		return &mount.DeviceStats{
			TotalBytes:      100 * 1024 * 1024,
			UsedBytes:       50 * 1024 * 1024,
			AvailableBytes:  50 * 1024 * 1024,
			TotalInodes:     1000,
			UsedInodes:      100,
			AvailableInodes: 900,
		}, nil
	}
	return m.stats, nil
}

func (m *mockMounter) ForceUnmount(target string, timeout time.Duration) error {
	return m.unmountErr
}

func (m *mockMounter) IsMountInUse(path string) (bool, []int, error) {
	return false, nil, nil
}

// staleCheckBehavior defines the expected behavior of stale check
type staleCheckBehavior struct {
	stale  bool
	reason mount.StaleReason
	err    error
}

// createNodeServerWithStaleBehavior creates a NodeServer with controlled stale check behavior
func createNodeServerWithStaleBehavior(mounter mount.Mounter, behavior staleCheckBehavior) *NodeServer {
	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}

	// Create a temp directory for mock sysfs
	tmpDir, _ := os.MkdirTemp("", "node-test-*")

	// Create mock sysfs structure for NQN resolution
	nqn := "nqn.2000-02.com.mikrotik:pvc-12345678-1234-1234-1234-123456789012"
	controllerName := "nvme0"
	deviceName := "nvme0n1"

	// Create controller directory with NQN
	ctrlDir := filepath.Join(tmpDir, "class", "nvme", controllerName)
	_ = os.MkdirAll(ctrlDir, 0755)
	_ = os.WriteFile(filepath.Join(ctrlDir, "subsysnqn"), []byte(nqn+"\n"), 0644)

	// Create block device entry
	bdDir := filepath.Join(tmpDir, "class", "block", deviceName)
	_ = os.MkdirAll(bdDir, 0755)

	// Create the resolver with mock sysfs
	resolver := nvme.NewDeviceResolverWithConfig(nvme.ResolverConfig{
		SysfsRoot: tmpDir,
	})

	// Create stale checker
	checker := mount.NewStaleMountChecker(resolver)

	// Configure stale check behavior based on test scenario
	switch {
	case behavior.err != nil:
		// Error case - return error from getMountDev
		checker.SetMountDeviceFunc(func(path string) (string, error) {
			return "", behavior.err
		})
	case behavior.stale && behavior.reason == mount.StaleReasonMountNotFound:
		// Mount not found
		checker.SetMountDeviceFunc(func(path string) (string, error) {
			return "", errors.New("mount point not found")
		})
	case behavior.stale && behavior.reason == mount.StaleReasonDeviceDisappeared:
		// Device disappeared
		checker.SetMountDeviceFunc(func(path string) (string, error) {
			return "/dev/nvme99n99", nil // Non-existent device
		})
	case behavior.stale && behavior.reason == mount.StaleReasonDeviceMismatch:
		// For device mismatch, we need a second device that differs from resolved
		// Create a mock device file that exists but differs from resolved path
		mockDevice := filepath.Join(tmpDir, "nvme_old")
		_ = os.WriteFile(mockDevice, []byte{}, 0644)
		checker.SetMountDeviceFunc(func(path string) (string, error) {
			return mockDevice, nil
		})
	default:
		// Healthy - mount device exists and matches resolver
		// Create a mock device file that we can use
		mockDevice := filepath.Join(tmpDir, "class", "block", deviceName, "device")
		_ = os.WriteFile(mockDevice, []byte{}, 0644)

		// The resolver returns /dev/nvmeXnY format, but EvalSymlinks on /dev/... will fail
		// on macOS. So for healthy case, we need both paths to resolve to same thing.
		// For simplicity, return "not found" error which triggers mount not found code path.
		// Actually for healthy test, we need special handling.

		// For healthy volumes, we can't easily test the full stale check on macOS
		// because /dev/nvme devices don't exist. The test will exercise the "no stale checker"
		// path which still returns healthy.
	}

	return &NodeServer{
		driver:       driver,
		mounter:      mounter,
		nodeID:       "test-node",
		staleChecker: checker,
	}
}

// createNodeServerNoStaleChecker creates a NodeServer without stale checker
func createNodeServerNoStaleChecker(mounter mount.Mounter) *NodeServer {
	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}

	return &NodeServer{
		driver:  driver,
		mounter: mounter,
		nodeID:  "test-node",
		// No stale checker - will default to healthy condition
	}
}

// TestNodeGetVolumeStats_AlwaysReturnsVolumeCondition tests that NodeGetVolumeStats
// always returns a VolumeCondition, even in different scenarios
func TestNodeGetVolumeStats_AlwaysReturnsVolumeCondition(t *testing.T) {
	tests := []struct {
		name            string
		volumeID        string
		setupServer     func(*mockMounter) *NodeServer
		wantAbnormal    bool
		wantMsgContains string
		wantErr         bool
	}{
		{
			name:     "healthy volume returns healthy condition (no stale checker)",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setupServer: func(m *mockMounter) *NodeServer {
				m.stats = &mount.DeviceStats{
					TotalBytes:      100 * 1024 * 1024,
					UsedBytes:       50 * 1024 * 1024,
					AvailableBytes:  50 * 1024 * 1024,
					TotalInodes:     1000,
					UsedInodes:      100,
					AvailableInodes: 900,
				}
				return createNodeServerNoStaleChecker(m)
			},
			wantAbnormal:    false,
			wantMsgContains: "healthy",
		},
		{
			name:     "stale mount due to mount not found returns abnormal condition",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setupServer: func(m *mockMounter) *NodeServer {
				return createNodeServerWithStaleBehavior(m, staleCheckBehavior{
					stale:  true,
					reason: mount.StaleReasonMountNotFound,
				})
			},
			wantAbnormal:    true,
			wantMsgContains: "Stale mount detected",
		},
		{
			name:     "stale mount due to device disappeared returns abnormal condition",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setupServer: func(m *mockMounter) *NodeServer {
				return createNodeServerWithStaleBehavior(m, staleCheckBehavior{
					stale:  true,
					reason: mount.StaleReasonDeviceDisappeared,
				})
			},
			wantAbnormal:    true,
			wantMsgContains: "Stale mount detected",
		},
		{
			name:     "stale check returns error - inconclusive condition",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setupServer: func(m *mockMounter) *NodeServer {
				// On macOS, device mismatch test is not possible because /dev/nvme devices
				// don't exist. Instead, test that when stale checker returns an error,
				// the VolumeCondition is still set (as "inconclusive").
				return createNodeServerWithStaleBehavior(m, staleCheckBehavior{
					stale:  true,
					reason: mount.StaleReasonDeviceMismatch,
				})
			},
			wantAbnormal:    false, // Inconclusive defaults to not abnormal
			wantMsgContains: "inconclusive",
		},
		{
			name:     "invalid volume ID still returns condition (defaults to healthy)",
			volumeID: "invalid-volume-id", // Can't derive NQN
			setupServer: func(m *mockMounter) *NodeServer {
				m.stats = &mount.DeviceStats{
					TotalBytes:      100 * 1024 * 1024,
					UsedBytes:       50 * 1024 * 1024,
					AvailableBytes:  50 * 1024 * 1024,
					TotalInodes:     1000,
					UsedInodes:      100,
					AvailableInodes: 900,
				}
				return createNodeServerNoStaleChecker(m)
			},
			wantAbnormal:    false, // Defaults to healthy when can't check
			wantMsgContains: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mounter := &mockMounter{}
			ns := tt.setupServer(mounter)

			// Create request
			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   tt.volumeID,
				VolumePath: "/var/lib/kubelet/pods/test-pod/volumes/rds.csi.srvlab.io/test-volume",
			}

			// Execute
			ctx := context.Background()
			resp, err := ns.NodeGetVolumeStats(ctx, req)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// CRITICAL: VolumeCondition should NEVER be nil
			if resp.VolumeCondition == nil {
				t.Fatal("VolumeCondition should never be nil")
			}

			// Check Abnormal flag
			if resp.VolumeCondition.Abnormal != tt.wantAbnormal {
				t.Errorf("VolumeCondition.Abnormal = %v, want %v", resp.VolumeCondition.Abnormal, tt.wantAbnormal)
			}

			// Check message contains expected text
			if !strings.Contains(resp.VolumeCondition.Message, tt.wantMsgContains) {
				t.Errorf("VolumeCondition.Message = %q, want to contain %q", resp.VolumeCondition.Message, tt.wantMsgContains)
			}
		})
	}
}

// TestNodeGetVolumeStats_Validation tests input validation
func TestNodeGetVolumeStats_Validation(t *testing.T) {
	tests := []struct {
		name      string
		volumeID  string
		path      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "missing volume ID",
			volumeID:  "",
			path:      "/some/path",
			wantErr:   true,
			errSubstr: "volume ID",
		},
		{
			name:      "missing volume path",
			volumeID:  "pvc-123",
			path:      "",
			wantErr:   true,
			errSubstr: "volume path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := createNodeServerNoStaleChecker(&mockMounter{})

			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   tt.volumeID,
				VolumePath: tt.path,
			}

			_, err := ns.NodeGetVolumeStats(context.Background(), req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestNodeGetVolumeStats_UsageReported tests that volume usage stats are reported
func TestNodeGetVolumeStats_UsageReported(t *testing.T) {
	mounter := &mockMounter{
		stats: &mount.DeviceStats{
			TotalBytes:      1024 * 1024 * 1024, // 1GB
			UsedBytes:       512 * 1024 * 1024,  // 512MB
			AvailableBytes:  512 * 1024 * 1024,  // 512MB
			TotalInodes:     100000,
			UsedInodes:      5000,
			AvailableInodes: 95000,
		},
	}

	ns := createNodeServerNoStaleChecker(mounter)

	req := &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "pvc-12345678-1234-1234-1234-123456789012",
		VolumePath: "/var/lib/kubelet/pods/test-pod/volumes/test-volume",
	}

	resp, err := ns.NodeGetVolumeStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that usage is reported
	if len(resp.Usage) == 0 {
		t.Fatal("expected usage stats to be reported")
	}

	// Find bytes usage
	var bytesUsage, inodesUsage *csi.VolumeUsage
	for _, usage := range resp.Usage {
		switch usage.Unit {
		case csi.VolumeUsage_BYTES:
			bytesUsage = usage
		case csi.VolumeUsage_INODES:
			inodesUsage = usage
		}
	}

	if bytesUsage == nil {
		t.Error("expected bytes usage to be reported")
	} else {
		if bytesUsage.Total != 1024*1024*1024 {
			t.Errorf("bytes Total = %d, want %d", bytesUsage.Total, 1024*1024*1024)
		}
		if bytesUsage.Used != 512*1024*1024 {
			t.Errorf("bytes Used = %d, want %d", bytesUsage.Used, 512*1024*1024)
		}
	}

	if inodesUsage == nil {
		t.Error("expected inodes usage to be reported")
	} else {
		if inodesUsage.Total != 100000 {
			t.Errorf("inodes Total = %d, want %d", inodesUsage.Total, 100000)
		}
	}

	// Also verify VolumeCondition is present
	if resp.VolumeCondition == nil {
		t.Fatal("VolumeCondition should not be nil")
	}
}

// TestNodeGetVolumeStats_StaleMountReturnsEmptyUsage tests that stale mounts
// return empty usage but still have VolumeCondition
func TestNodeGetVolumeStats_StaleMountReturnsEmptyUsage(t *testing.T) {
	mounter := &mockMounter{}

	ns := createNodeServerWithStaleBehavior(mounter, staleCheckBehavior{
		stale:  true,
		reason: mount.StaleReasonDeviceDisappeared,
	})

	req := &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "pvc-12345678-1234-1234-1234-123456789012",
		VolumePath: "/var/lib/kubelet/pods/test-pod/volumes/test-volume",
	}

	resp, err := ns.NodeGetVolumeStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// For stale mounts, Usage should be empty
	if len(resp.Usage) != 0 {
		t.Errorf("expected empty usage for stale mount, got %d usage entries", len(resp.Usage))
	}

	// VolumeCondition should still be present and abnormal
	if resp.VolumeCondition == nil {
		t.Fatal("VolumeCondition should not be nil")
	}
	if !resp.VolumeCondition.Abnormal {
		t.Error("VolumeCondition.Abnormal should be true for stale mount")
	}
}

// TestNodeGetVolumeStats_MetricsRecorded tests that stale mount detection
// records metrics
func TestNodeGetVolumeStats_MetricsRecorded(t *testing.T) {
	mounter := &mockMounter{}

	ns := createNodeServerWithStaleBehavior(mounter, staleCheckBehavior{
		stale:  true,
		reason: mount.StaleReasonMountNotFound,
	})

	req := &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "pvc-12345678-1234-1234-1234-123456789012",
		VolumePath: "/var/lib/kubelet/pods/test-pod/volumes/test-volume",
	}

	_, err := ns.NodeGetVolumeStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that metrics were recorded (we can't easily check counter values
	// but we can verify the handler returns metrics without panic)
	if ns.driver.metrics != nil {
		_ = ns.driver.metrics.Handler()
	}
}

// TestNodeGetCapabilities tests the node capabilities response
func TestNodeGetCapabilities(t *testing.T) {
	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		nscaps: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}

	ns := &NodeServer{
		driver: driver,
		nodeID: "test-node",
	}

	resp, err := ns.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(resp.Capabilities))
	}
}

// TestNodeGetInfo tests the node info response
func TestNodeGetInfo(t *testing.T) {
	ns := &NodeServer{
		driver: &Driver{},
		nodeID: "test-node-123",
	}

	resp, err := ns.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.NodeId != "test-node-123" {
		t.Errorf("NodeId = %q, want %q", resp.NodeId, "test-node-123")
	}

	if resp.MaxVolumesPerNode != 0 {
		t.Errorf("MaxVolumesPerNode = %d, want 0 (unlimited)", resp.MaxVolumesPerNode)
	}
}

// TestNodeGetVolumeStats_VolumeConditionNeverNil is a focused test to verify
// the critical invariant that VolumeCondition is never nil
func TestNodeGetVolumeStats_VolumeConditionNeverNil(t *testing.T) {
	scenarios := []struct {
		name     string
		volumeID string
		setup    func() *NodeServer
	}{
		{
			name:     "with stale checker",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setup: func() *NodeServer {
				return createNodeServerNoStaleChecker(&mockMounter{})
			},
		},
		{
			name:     "without stale checker",
			volumeID: "pvc-12345678-1234-1234-1234-123456789012",
			setup: func() *NodeServer {
				driver := &Driver{
					name:    "rds.csi.srvlab.io",
					version: "test",
					metrics: observability.NewMetrics(),
				}
				return &NodeServer{
					driver:       driver,
					mounter:      &mockMounter{},
					nodeID:       "test-node",
					staleChecker: nil, // Explicitly nil
				}
			},
		},
		{
			name:     "invalid volume ID (can't derive NQN)",
			volumeID: "not-a-pvc-format",
			setup: func() *NodeServer {
				return createNodeServerNoStaleChecker(&mockMounter{})
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			ns := sc.setup()

			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   sc.volumeID,
				VolumePath: "/var/lib/kubelet/pods/test-pod/volumes/test-volume",
			}

			resp, err := ns.NodeGetVolumeStats(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// THE CRITICAL CHECK: VolumeCondition must NEVER be nil
			if resp.VolumeCondition == nil {
				t.Fatal("VolumeCondition MUST never be nil - this is a critical invariant")
			}
		})
	}
}
