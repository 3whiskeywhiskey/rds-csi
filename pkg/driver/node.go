package driver

import (
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/mount"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/nvme"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
)

const (
	// Default filesystem type if not specified
	defaultFSType = "ext4"

	// Device connection timeout (reserved for future use)
	// deviceTimeout = 30 * time.Second

	// VolumeContext keys
	volumeContextNQN         = "nqn"
	volumeContextAddress     = "rdsAddress"
	volumeContextNVMEAddress = "nvmeAddress"
	volumeContextPort        = "nvmePort"
	volumeContextFSType      = "fsType"
)

// NodeServer implements the CSI Node service
type NodeServer struct {
	csi.UnimplementedNodeServer
	driver   *Driver
	nvmeConn nvme.Connector
	mounter  mount.Mounter
	nodeID   string
}

// NewNodeServer creates a new Node service
func NewNodeServer(driver *Driver, nodeID string) *NodeServer {
	return &NodeServer{
		driver:   driver,
		nvmeConn: nvme.NewConnector(),
		mounter:  mount.NewMounter(),
		nodeID:   nodeID,
	}
}

// NodeStageVolume stages a volume to a staging path on the node
// This involves:
// 1. Connecting to the NVMe/TCP target
// 2. Waiting for the block device to appear
// 3. Formatting the filesystem if needed
// 4. Mounting the filesystem to the staging path
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()

	klog.V(2).Infof("NodeStageVolume called for volume: %s, staging path: %s", volumeID, stagingPath)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path is required")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}

	// Extract volume context
	volumeContext := req.GetVolumeContext()
	nqn := volumeContext[volumeContextNQN]
	nvmeAddress := volumeContext[volumeContextNVMEAddress]
	// Fall back to rdsAddress if nvmeAddress not set (backward compatibility)
	if nvmeAddress == "" {
		nvmeAddress = volumeContext[volumeContextAddress]
	}
	nvmePort := volumeContext[volumeContextPort]

	if nqn == "" || nvmeAddress == "" || nvmePort == "" {
		return nil, status.Errorf(codes.InvalidArgument,
			"missing required volume context: nqn=%s, nvmeAddress=%s, nvmePort=%s",
			nqn, nvmeAddress, nvmePort)
	}

	// Parse port
	var port int
	if _, err := fmt.Sscanf(nvmePort, "%d", &port); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid nvmePort: %s", nvmePort)
	}

	// Get filesystem type from capability or use default
	fsType := defaultFSType
	if mnt := req.GetVolumeCapability().GetMount(); mnt != nil {
		if mnt.FsType != "" {
			fsType = mnt.FsType
		}
	}

	klog.V(2).Infof("Staging volume %s: NQN=%s, Address=%s:%d, FSType=%s",
		volumeID, nqn, nvmeAddress, port, fsType)

	// Step 1: Connect to NVMe/TCP target
	target := nvme.Target{
		Transport:     "tcp",
		NQN:           nqn,
		TargetAddress: nvmeAddress,
		TargetPort:    port,
	}

	devicePath, err := ns.nvmeConn.Connect(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to connect to NVMe target: %v", err)
	}

	klog.V(2).Infof("Connected to NVMe target, device: %s", devicePath)

	// Step 2: Format filesystem if needed
	if err := ns.mounter.Format(devicePath, fsType); err != nil {
		// Cleanup on failure
		_ = ns.nvmeConn.Disconnect(nqn)
		return nil, status.Errorf(codes.Internal, "failed to format device: %v", err)
	}

	// Step 3: Mount to staging path
	mountOptions := []string{}
	if mnt := req.GetVolumeCapability().GetMount(); mnt != nil {
		mountOptions = mnt.MountFlags
	}

	if err := ns.mounter.Mount(devicePath, stagingPath, fsType, mountOptions); err != nil {
		// Cleanup on failure
		_ = ns.nvmeConn.Disconnect(nqn)
		return nil, status.Errorf(codes.Internal, "failed to mount device: %v", err)
	}

	klog.V(2).Infof("Successfully staged volume %s to %s", volumeID, stagingPath)
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unstages a volume from the staging path
// This involves:
// 1. Unmounting the filesystem from the staging path
// 2. Disconnecting from the NVMe/TCP target
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()

	klog.V(2).Infof("NodeUnstageVolume called for volume: %s, staging path: %s", volumeID, stagingPath)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path is required")
	}

	// Step 1: Unmount from staging path
	if err := ns.mounter.Unmount(stagingPath); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staging path: %v", err)
	}

	klog.V(2).Infof("Unmounted volume %s from %s", volumeID, stagingPath)

	// Step 2: Disconnect from NVMe/TCP target
	// Derive NQN from volume ID (same as what was used during CreateVolume)
	nqn, err := volumeIDToNQN(volumeID)
	if err != nil {
		// Log but don't fail - volume might have been disconnected already
		klog.Warningf("Failed to derive NQN from volume ID %s: %v", volumeID, err)
	} else {
		if err := ns.nvmeConn.Disconnect(nqn); err != nil {
			// Log but don't fail - disconnection issues shouldn't block unstaging
			klog.Warningf("Failed to disconnect NVMe device for volume %s: %v", volumeID, err)
		} else {
			klog.V(2).Infof("Disconnected NVMe device for volume %s (NQN: %s)", volumeID, nqn)
		}
	}

	klog.V(2).Infof("Successfully unstaged volume %s", volumeID)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume publishes a volume to the target path
// This involves bind-mounting from the staging path to the target path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()

	klog.V(2).Infof("NodePublishVolume called for volume: %s, target path: %s", volumeID, targetPath)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path is required")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}

	// Check if staging path is mounted
	mounted, err := ns.mounter.IsLikelyMountPoint(stagingPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check staging path: %v", err)
	}
	if !mounted {
		return nil, status.Errorf(codes.FailedPrecondition,
			"staging path %s is not mounted", stagingPath)
	}

	// Build mount options
	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	// Add any additional mount flags from capability
	if mnt := req.GetVolumeCapability().GetMount(); mnt != nil {
		mountOptions = append(mountOptions, mnt.MountFlags...)
	}

	// Bind mount from staging to target
	if err := ns.mounter.Mount(stagingPath, targetPath, "", mountOptions); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to bind mount: %v", err)
	}

	klog.V(2).Infof("Successfully published volume %s to %s", volumeID, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unpublishes a volume from the target path
func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	klog.V(2).Infof("NodeUnpublishVolume called for volume: %s, target path: %s", volumeID, targetPath)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}

	// Unmount from target path
	if err := ns.mounter.Unmount(targetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
	}

	klog.V(2).Infof("Successfully unpublished volume %s from %s", volumeID, targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats returns volume usage statistics
func (ns *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeID := req.GetVolumeId()
	volumePath := req.GetVolumePath()

	klog.V(4).Infof("NodeGetVolumeStats called for volume: %s, path: %s", volumeID, volumePath)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path is required")
	}

	// Get device statistics
	stats, err := ns.mounter.GetDeviceStats(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get volume stats: %v", err)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Total:     stats.TotalBytes,
				Used:      stats.UsedBytes,
				Available: stats.AvailableBytes,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Total:     stats.TotalInodes,
				Used:      stats.UsedInodes,
				Available: stats.AvailableInodes,
			},
		},
	}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node service
func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(5).Info("NodeGetCapabilities called")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.driver.nscaps,
	}, nil
}

// NodeGetInfo returns information about the node
func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo called for node: %s", ns.nodeID)

	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
		// MaxVolumesPerNode: 0 means unlimited
		MaxVolumesPerNode: 0,
	}, nil
}

// NodeExpandVolume expands a volume (not yet implemented)
func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not yet implemented")
}

// Helper functions

// volumeIDToNQN converts a volume ID to an NVMe Qualified Name
func volumeIDToNQN(volumeID string) (string, error) {
	return utils.VolumeIDToNQN(volumeID)
}
