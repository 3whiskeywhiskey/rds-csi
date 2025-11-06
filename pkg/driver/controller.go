package driver

import (
	"context"
	"fmt"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	// Default values for storage parameters
	defaultVolumeBasePath = "/storage-pool/metal-csi"
	defaultNVMETCPPort    = 4420

	// Parameter keys for StorageClass
	paramRDSAddress  = "rdsAddress"
	paramNVMEAddress = "nvmeAddress"
	paramNVMEPort    = "nvmePort"
	paramSSHPort     = "sshPort"
	paramFSType      = "fsType"
	paramVolumePath  = "volumePath"
	paramNQNPrefix   = "nqnPrefix"

	// Minimum/maximum volume sizes
	minVolumeSizeBytes = 1 * 1024 * 1024 * 1024         // 1 GiB
	maxVolumeSizeBytes = 16 * 1024 * 1024 * 1024 * 1024 // 16 TiB
)

// ControllerServer implements the CSI Controller service
type ControllerServer struct {
	csi.UnimplementedControllerServer
	driver *Driver
}

// NewControllerServer creates a new Controller service
func NewControllerServer(driver *Driver) *ControllerServer {
	return &ControllerServer{
		driver: driver,
	}
}

// CreateVolume provisions a new volume on RDS
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(2).Infof("CreateVolume called with name: %s", req.GetName())

	// Validate request
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is required")
	}

	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}

	// Validate volume capabilities
	if err := cs.validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capabilities: %v", err)
	}

	// Get required capacity
	requiredBytes := req.GetCapacityRange().GetRequiredBytes()
	if requiredBytes == 0 {
		requiredBytes = minVolumeSizeBytes
	}

	// Enforce size limits
	if requiredBytes < minVolumeSizeBytes {
		requiredBytes = minVolumeSizeBytes
	}

	limitBytes := req.GetCapacityRange().GetLimitBytes()
	if limitBytes > 0 && requiredBytes > limitBytes {
		return nil, status.Errorf(codes.OutOfRange, "required bytes %d exceeds limit bytes %d", requiredBytes, limitBytes)
	}

	if requiredBytes > maxVolumeSizeBytes {
		return nil, status.Errorf(codes.OutOfRange, "required bytes %d exceeds maximum %d", requiredBytes, maxVolumeSizeBytes)
	}

	// Generate deterministic volume ID from volume name (for idempotency)
	volumeID := utils.VolumeNameToID(req.GetName())
	klog.V(2).Infof("Generated volume ID: %s for volume name: %s", volumeID, req.GetName())

	// Check if volume already exists (idempotency)
	existingVolume, err := cs.driver.rdsClient.GetVolume(volumeID)
	if err == nil {
		// Volume already exists, verify it matches requirements
		klog.V(2).Infof("Volume %s already exists, returning existing volume", volumeID)

		// Get parameters from StorageClass for response context
		params := req.GetParameters()

		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      volumeID,
				CapacityBytes: existingVolume.FileSizeBytes,
				VolumeContext: map[string]string{
					"rdsAddress":  cs.getRDSAddress(params),
					"nvmeAddress": cs.getNVMEAddress(params),
					"nvmePort":    fmt.Sprintf("%d", existingVolume.NVMETCPPort),
					"nqn":         existingVolume.NVMETCPNQN,
					"volumePath":  existingVolume.FilePath,
				},
			},
		}, nil
	}

	// Volume doesn't exist, create it
	// Get parameters from StorageClass
	params := req.GetParameters()
	volumeBasePath := defaultVolumeBasePath
	if path, ok := params[paramVolumePath]; ok {
		volumeBasePath = path
	}

	nvmePort := defaultNVMETCPPort
	if portStr, ok := params[paramNVMEPort]; ok {
		// Parse port number
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
			nvmePort = port
		}
	}

	// Generate NQN
	nqn, err := utils.VolumeIDToNQN(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate NQN: %v", err)
	}

	// Generate file path
	filePath, err := utils.VolumeIDToFilePath(volumeID, volumeBasePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate file path: %v", err)
	}

	// Create volume on RDS
	klog.V(2).Infof("Creating volume %s on RDS (size: %d bytes, path: %s, nqn: %s)", volumeID, requiredBytes, filePath, nqn)

	createOpts := rds.CreateVolumeOptions{
		Slot:          volumeID,
		FilePath:      filePath,
		FileSizeBytes: requiredBytes,
		NVMETCPPort:   nvmePort,
		NVMETCPNQN:    nqn,
	}

	if err := cs.driver.rdsClient.CreateVolume(createOpts); err != nil {
		// Check if this is a capacity error
		if containsString(err.Error(), "not enough space") {
			return nil, status.Errorf(codes.ResourceExhausted, "insufficient storage on RDS: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to create volume on RDS: %v", err)
	}

	klog.V(2).Infof("Successfully created volume %s on RDS", volumeID)

	// Return volume information
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: requiredBytes,
			VolumeContext: map[string]string{
				"rdsAddress":  cs.getRDSAddress(params),
				"nvmeAddress": cs.getNVMEAddress(params),
				"nvmePort":    fmt.Sprintf("%d", nvmePort),
				"nqn":         nqn,
				"volumePath":  filePath,
			},
		},
	}, nil
}

// DeleteVolume removes a volume from RDS
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	klog.V(2).Infof("DeleteVolume called for volume: %s", volumeID)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	// Validate volume ID format
	if err := utils.ValidateVolumeID(volumeID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume ID: %v", err)
	}

	// Delete volume from RDS (idempotent)
	if err := cs.driver.rdsClient.DeleteVolume(volumeID); err != nil {
		klog.Errorf("Failed to delete volume %s: %v", volumeID, err)
		return nil, status.Errorf(codes.Internal, "failed to delete volume: %v", err)
	}

	klog.V(2).Infof("Successfully deleted volume %s", volumeID)

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities validates that the requested capabilities are supported
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	klog.V(4).Infof("ValidateVolumeCapabilities called for volume: %s", volumeID)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}

	// Check if volume exists
	if _, err := cs.driver.rdsClient.GetVolume(volumeID); err != nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
	}

	// Validate capabilities
	if err := cs.validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{
			Message: err.Error(),
		}, nil
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

// GetCapacity returns the available storage capacity on RDS
func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.V(4).Info("GetCapacity called")

	// Get parameters
	params := req.GetParameters()
	volumeBasePath := defaultVolumeBasePath
	if path, ok := params[paramVolumePath]; ok {
		volumeBasePath = path
	}

	// Query capacity from RDS
	capacity, err := cs.driver.rdsClient.GetCapacity(volumeBasePath)
	if err != nil {
		klog.Errorf("Failed to get capacity from RDS: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query capacity: %v", err)
	}

	klog.V(4).Infof("RDS capacity: total=%d, free=%d, used=%d", capacity.TotalBytes, capacity.FreeBytes, capacity.UsedBytes)

	return &csi.GetCapacityResponse{
		AvailableCapacity: capacity.FreeBytes,
	}, nil
}

// ControllerGetCapabilities returns the capabilities of the controller service
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Info("ControllerGetCapabilities called")

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.driver.cscaps,
	}, nil
}

// ControllerPublishVolume is not supported (node-local attachment)
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerPublishVolume is not supported")
}

// ControllerUnpublishVolume is not supported (node-local attachment)
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerUnpublishVolume is not supported")
}

// CreateSnapshot is not yet implemented
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not yet implemented")
}

// DeleteSnapshot is not yet implemented
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not yet implemented")
}

// ListSnapshots is not yet implemented
func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListSnapshots is not yet implemented")
}

// ControllerExpandVolume is not yet implemented
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not yet implemented")
}

// ListVolumes lists all volumes on RDS
func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.V(4).Info("ListVolumes called")

	// Query all volumes from RDS
	volumes, err := cs.driver.rdsClient.ListVolumes()
	if err != nil {
		klog.Errorf("Failed to list volumes from RDS: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to list volumes: %v", err)
	}

	// Convert to CSI format
	var entries []*csi.ListVolumesResponse_Entry
	for _, vol := range volumes {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol.Slot,
				CapacityBytes: vol.FileSizeBytes,
			},
		})
	}

	return &csi.ListVolumesResponse{
		Entries: entries,
	}, nil
}

// ControllerGetVolume is not yet implemented
func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is not yet implemented")
}

// ControllerModifyVolume is not yet implemented
func (cs *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerModifyVolume is not yet implemented")
}

// Helper functions

// validateVolumeCapabilities checks if the requested capabilities are supported
func (cs *ControllerServer) validateVolumeCapabilities(caps []*csi.VolumeCapability) error {
	for _, cap := range caps {
		// Check access mode
		accessMode := cap.GetAccessMode().GetMode()
		supported := false
		for _, supportedMode := range cs.driver.vcaps {
			if accessMode == supportedMode.GetMode() {
				supported = true
				break
			}
		}

		if !supported {
			return fmt.Errorf("access mode %v is not supported", accessMode)
		}

		// Check access type (must be block or mount)
		if cap.GetBlock() == nil && cap.GetMount() == nil {
			return fmt.Errorf("volume capability must specify either block or mount")
		}
	}

	return nil
}

// getRDSAddress extracts RDS address from parameters
func (cs *ControllerServer) getRDSAddress(params map[string]string) string {
	if addr, ok := params[paramRDSAddress]; ok {
		return addr
	}
	// Fall back to driver's RDS client address
	return cs.driver.rdsClient.GetAddress()
}

// getNVMEAddress gets the NVMe/TCP target address from params or falls back to RDS address
func (cs *ControllerServer) getNVMEAddress(params map[string]string) string {
	// Prefer nvmeAddress if specified (for separate storage network)
	if addr, ok := params[paramNVMEAddress]; ok {
		return addr
	}
	// Fall back to RDS address if nvmeAddress not specified
	return cs.getRDSAddress(params)
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexString(s, substr) >= 0)
}

// indexString finds the index of substr in s
func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
