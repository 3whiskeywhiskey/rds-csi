package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/security"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
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

	// Use the volume name directly as the volume ID
	// The external-provisioner passes the PV name (pvc-<uuid>) which is already unique and deterministic
	volumeID := req.GetName()

	// Validate the volume ID format
	if err := utils.ValidateVolumeID(volumeID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume name format: %v", err)
	}
	klog.V(2).Infof("Using volume ID: %s (from volume name: %s)", volumeID, req.GetName())

	// Check if volume already exists (idempotency)
	existingVolume, err := cs.driver.rdsClient.GetVolume(volumeID)
	if err == nil {
		// Volume already exists, verify it matches requirements
		klog.V(2).Infof("Volume %s already exists, returning existing volume", volumeID)

		// Get parameters from StorageClass for response context
		params := req.GetParameters()

		// Parse NVMe connection parameters from StorageClass
		nvmeParams, err := ParseNVMEConnectionParams(params)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid NVMe connection parameters: %v", err)
		}

		// Parse migration timeout
		migrationTimeout := ParseMigrationTimeout(params)

		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      volumeID,
				CapacityBytes: existingVolume.FileSizeBytes,
				VolumeContext: map[string]string{
					"rdsAddress":              cs.getRDSAddress(params),
					"nvmeAddress":             cs.getNVMEAddress(params),
					"nvmePort":                fmt.Sprintf("%d", existingVolume.NVMETCPPort),
					"nqn":                     existingVolume.NVMETCPNQN,
					"volumePath":              existingVolume.FilePath,
					"ctrlLossTmo":             fmt.Sprintf("%d", nvmeParams.CtrlLossTmo),
					"reconnectDelay":          fmt.Sprintf("%d", nvmeParams.ReconnectDelay),
					"keepAliveTmo":            fmt.Sprintf("%d", nvmeParams.KeepAliveTmo),
					"migrationTimeoutSeconds": fmt.Sprintf("%.0f", migrationTimeout.Seconds()),
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

	// Parse NVMe connection parameters from StorageClass
	nvmeParams, err := ParseNVMEConnectionParams(params)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid NVMe connection parameters: %v", err)
	}

	// Parse migration timeout
	migrationTimeout := ParseMigrationTimeout(params)

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

	// Log volume create request
	secLogger := security.GetLogger()
	secLogger.LogVolumeCreate(volumeID, req.GetName(), security.OutcomeUnknown, nil, 0)

	createOpts := rds.CreateVolumeOptions{
		Slot:          volumeID,
		FilePath:      filePath,
		FileSizeBytes: requiredBytes,
		NVMETCPPort:   nvmePort,
		NVMETCPNQN:    nqn,
	}

	startTime := time.Now()
	if err := cs.driver.rdsClient.CreateVolume(createOpts); err != nil {
		// Log volume create failure
		secLogger.LogVolumeCreate(volumeID, req.GetName(), security.OutcomeFailure, err, time.Since(startTime))

		// Check if this is a capacity error
		if containsString(err.Error(), "not enough space") {
			return nil, status.Errorf(codes.ResourceExhausted, "insufficient storage on RDS: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to create volume on RDS: %v", err)
	}

	klog.V(2).Infof("Successfully created volume %s on RDS", volumeID)

	// Log volume create success
	secLogger.LogVolumeCreate(volumeID, req.GetName(), security.OutcomeSuccess, nil, time.Since(startTime))

	// Return volume information
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: requiredBytes,
			VolumeContext: map[string]string{
				"rdsAddress":              cs.getRDSAddress(params),
				"nvmeAddress":             cs.getNVMEAddress(params),
				"nvmePort":                fmt.Sprintf("%d", nvmePort),
				"nqn":                     nqn,
				"volumePath":              filePath,
				"ctrlLossTmo":             fmt.Sprintf("%d", nvmeParams.CtrlLossTmo),
				"reconnectDelay":          fmt.Sprintf("%d", nvmeParams.ReconnectDelay),
				"keepAliveTmo":            fmt.Sprintf("%d", nvmeParams.KeepAliveTmo),
				"migrationTimeoutSeconds": fmt.Sprintf("%.0f", migrationTimeout.Seconds()),
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

	// Safety check: verify volume exists before attempting deletion
	// This helps catch force-deletion scenarios where the volume might still be in use
	volume, err := cs.driver.rdsClient.GetVolume(volumeID)
	if err != nil {
		// If volume doesn't exist, deletion is idempotent - return success
		klog.V(3).Infof("Volume %s not found on RDS, assuming already deleted", volumeID)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// Log volume details for audit trail
	klog.V(3).Infof("Deleting volume %s (path=%s, size=%d bytes, nvme_export=%v)",
		volumeID, volume.FilePath, volume.FileSizeBytes, volume.NVMETCPExport)

	// Log volume delete request
	secLogger := security.GetLogger()
	secLogger.LogVolumeDelete(volumeID, "", security.OutcomeUnknown, nil, 0)

	// Delete volume from RDS (idempotent)
	startTime := time.Now()
	if err := cs.driver.rdsClient.DeleteVolume(volumeID); err != nil {
		klog.Errorf("Failed to delete volume %s: %v", volumeID, err)

		// Log volume delete failure
		secLogger.LogVolumeDelete(volumeID, "", security.OutcomeFailure, err, time.Since(startTime))

		return nil, status.Errorf(codes.Internal, "failed to delete volume: %v", err)
	}

	klog.V(2).Infof("Successfully deleted volume %s", volumeID)

	// Log volume delete success
	secLogger.LogVolumeDelete(volumeID, "", security.OutcomeSuccess, nil, time.Since(startTime))

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

// validateBlockingNodeExists checks if a node still exists in Kubernetes.
// Returns (exists, error) - if node is deleted, exists=false, error=nil.
// Used for self-healing when blocking node is deleted without cleanup.
func (cs *ControllerServer) validateBlockingNodeExists(ctx context.Context, nodeID string) (bool, error) {
	if cs.driver.k8sClient == nil {
		// No k8s client = can't validate, assume node exists (fail-closed)
		return true, nil
	}
	_, err := cs.driver.k8sClient.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil // Node deleted
		}
		return false, err // API error
	}
	return true, nil
}

// buildPublishContext creates the publish_context map with NVMe connection parameters.
// Uses snake_case keys to match existing volumeContext conventions.
func (cs *ControllerServer) buildPublishContext(volume *rds.VolumeInfo, params map[string]string) map[string]string {
	fsType := "ext4"
	if fs, ok := params[paramFSType]; ok && fs != "" {
		fsType = fs
	}

	return map[string]string{
		"nvme_address": cs.getNVMEAddress(params),
		"nvme_port":    fmt.Sprintf("%d", volume.NVMETCPPort),
		"nvme_nqn":     volume.NVMETCPNQN,
		"fs_type":      fsType,
	}
}

// postAttachmentConflictEvent posts a K8s event for an attachment conflict.
// Best effort - failures are logged but don't affect the main operation.
func (cs *ControllerServer) postAttachmentConflictEvent(ctx context.Context, req *csi.ControllerPublishVolumeRequest, attachedNode string) {
	// Extract PVC info from volume context if available
	volCtx := req.GetVolumeContext()
	pvcNamespace := volCtx["csi.storage.k8s.io/pvc/namespace"]
	pvcName := volCtx["csi.storage.k8s.io/pvc/name"]

	if pvcNamespace == "" || pvcName == "" {
		klog.V(3).Infof("Cannot post attachment conflict event: PVC info not in volume context")
		return
	}

	// Create temporary EventPoster if we have k8s client
	if cs.driver.k8sClient == nil {
		return
	}

	poster := NewEventPoster(cs.driver.k8sClient)
	if err := poster.PostAttachmentConflict(ctx, pvcNamespace, pvcName, req.GetVolumeId(), req.GetNodeId(), attachedNode); err != nil {
		klog.Warningf("Failed to post attachment conflict event: %v", err)
	}
}

// postVolumeAttachedEvent posts a K8s event when a volume is attached.
// Best effort - failures are logged but don't affect the main operation.
func (cs *ControllerServer) postVolumeAttachedEvent(ctx context.Context, req *csi.ControllerPublishVolumeRequest, duration time.Duration) {
	volCtx := req.GetVolumeContext()
	pvcNamespace := volCtx["csi.storage.k8s.io/pvc/namespace"]
	pvcName := volCtx["csi.storage.k8s.io/pvc/name"]

	if pvcNamespace == "" || pvcName == "" {
		klog.V(3).Infof("Cannot post volume attached event: PVC info not in volume context")
		return
	}

	if cs.driver.k8sClient == nil {
		return
	}

	poster := NewEventPoster(cs.driver.k8sClient)
	if err := poster.PostVolumeAttached(ctx, pvcNamespace, pvcName, req.GetVolumeId(), req.GetNodeId(), duration); err != nil {
		klog.Warningf("Failed to post volume attached event: %v", err)
	}
}

// postVolumeDetachedEvent posts a K8s event when a volume is detached.
// Best effort - failures are logged but don't affect the main operation.
func (cs *ControllerServer) postVolumeDetachedEvent(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) {
	if cs.driver.k8sClient == nil {
		return
	}

	// For unpublish, we don't have volume context with PVC info
	// We need to look up the PV to get the claimRef
	pv, err := cs.driver.k8sClient.CoreV1().PersistentVolumes().Get(ctx, req.GetVolumeId(), metav1.GetOptions{})
	if err != nil {
		klog.V(3).Infof("Cannot get PV %s for detached event: %v", req.GetVolumeId(), err)
		return
	}

	claimRef := pv.Spec.ClaimRef
	if claimRef == nil {
		klog.V(3).Infof("PV %s has no claimRef for detached event", req.GetVolumeId())
		return
	}

	poster := NewEventPoster(cs.driver.k8sClient)
	if err := poster.PostVolumeDetached(ctx, claimRef.Namespace, claimRef.Name, req.GetVolumeId(), req.GetNodeId()); err != nil {
		klog.Warningf("Failed to post volume detached event: %v", err)
	}
}

// ControllerPublishVolume tracks volume attachment to a node and enforces RWO semantics.
// Returns publish_context with NVMe connection parameters for NodeStageVolume.
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	startTime := time.Now()
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()

	klog.V(2).Infof("ControllerPublishVolume called for volume %s to node %s", volumeID, nodeID)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID is required")
	}

	// Validate volume ID format (security: prevent injection)
	if err := utils.ValidateVolumeID(volumeID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume ID: %v", err)
	}

	// Determine access mode from VolumeCapability (singular, not slice)
	// ControllerPublishVolume uses GetVolumeCapability() which returns a single capability
	accessMode := "RWO"
	isRWX := false
	if cap := req.GetVolumeCapability(); cap != nil {
		if cap.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			accessMode = "RWX"
			isRWX = true
		}
	}

	// Acquire per-VMI lock if serialization is enabled
	// This prevents concurrent volume operations on the same VMI from racing
	if vmiGrouper := cs.driver.GetVMIGrouper(); vmiGrouper != nil {
		volCtx := req.GetVolumeContext()
		pvcNamespace := volCtx["csi.storage.k8s.io/pvc/namespace"]
		pvcName := volCtx["csi.storage.k8s.io/pvc/name"]
		if pvcNamespace != "" && pvcName != "" {
			vmiKey, unlock := vmiGrouper.LockVMI(ctx, pvcNamespace, pvcName)
			if vmiKey != "" {
				defer unlock()
				klog.V(3).Infof("VMI serialization active for volume %s (VMI: %s)", volumeID, vmiKey)
			}
		}
	}

	// Verify volume exists on RDS
	volume, err := cs.driver.rdsClient.GetVolume(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
	}

	// Get attachment manager
	am := cs.driver.GetAttachmentManager()
	if am == nil {
		// No attachment manager = skip tracking (single-node scenario or disabled)
		klog.V(3).Infof("Attachment manager not available, skipping tracking for volume %s", volumeID)
		return &csi.ControllerPublishVolumeResponse{
			PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
		}, nil
	}

	// Check existing attachment
	existing, exists := am.GetAttachment(volumeID)
	if exists {
		// Check if already attached to requesting node (idempotent)
		if am.IsAttachedToNode(volumeID, nodeID) {
			klog.V(2).Infof("Volume %s already attached to node %s (idempotent)", volumeID, nodeID)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
			}, nil
		}

		// Different node - behavior depends on access mode
		if isRWX {
			// RWX: Allow second attachment if within limit
			nodeCount := am.GetNodeCount(volumeID)
			if nodeCount >= 2 {
				// ROADMAP-5: 2-node migration limit reached
				klog.Warningf("RWX volume %s already attached to 2 nodes, rejecting 3rd attachment to %s",
					volumeID, nodeID)
				return nil, status.Errorf(codes.FailedPrecondition,
					"Volume %s already attached to 2 nodes (migration limit). Wait for migration to complete. Attached nodes: %v",
					volumeID, existing.GetNodeIDs())
			}

			// Parse migration timeout from VolumeContext
			volCtx := req.GetVolumeContext()
			migrationTimeoutParams := map[string]string{
				"migrationTimeoutSeconds": volCtx["migrationTimeoutSeconds"],
			}
			migrationTimeout := ParseMigrationTimeout(migrationTimeoutParams)

			// Allow second attachment for migration
			klog.V(2).Infof("Allowing second attachment of RWX volume %s to node %s (migration target)", volumeID, nodeID)
			if err := am.AddSecondaryAttachment(ctx, volumeID, nodeID, migrationTimeout); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to track secondary attachment: %v", err)
			}

			// Record RWX dual-attach (distinct from RWO conflict)
			if cs.driver.metrics != nil {
				cs.driver.metrics.RecordAttachmentOp("attach_secondary", nil, time.Since(startTime))
			}

			return &csi.ControllerPublishVolumeResponse{
				PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
			}, nil
		}

		// RWO: Check grace period first
		gracePeriod := cs.driver.GetAttachmentGracePeriod()
		if gracePeriod > 0 && am.IsWithinGracePeriod(volumeID, gracePeriod) {
			klog.V(2).Infof("Volume %s within grace period, allowing attachment handoff from %s to %s",
				volumeID, existing.NodeID, nodeID)

			// Record grace period usage metric
			if cs.driver.metrics != nil {
				cs.driver.metrics.RecordGracePeriodUsed()
			}

			// Clear the old attachment and detach timestamp before new attach
			if err := am.UntrackAttachment(ctx, volumeID); err != nil {
				klog.Warningf("Failed to clear old attachment for volume %s during grace period handoff: %v", volumeID, err)
			}
			am.ClearDetachTimestamp(volumeID)
			// Fall through to track new attachment
		} else {
			// CSI-06: Before rejecting, verify blocking node still exists
			nodeExists, err := cs.validateBlockingNodeExists(ctx, existing.NodeID)
			if err != nil {
				// API error - fail closed to prevent data corruption
				klog.Errorf("Failed to verify node %s existence: %v", existing.NodeID, err)
				return nil, status.Errorf(codes.Internal, "failed to verify node %s: %v", existing.NodeID, err)
			}

			if !nodeExists {
				// Node deleted - auto-clear stale attachment (self-healing)
				klog.Warningf("Volume %s attached to deleted node %s, clearing stale attachment", volumeID, existing.NodeID)
				if err := am.UntrackAttachment(ctx, volumeID); err != nil {
					klog.Warningf("Failed to clear stale attachment for volume %s: %v", volumeID, err)
					// Continue anyway - in-memory state may be stale
				}
				// Fall through to allow new attachment
			} else {
				// CSI-02: Node exists - genuine RWO conflict - hint about RWX
				klog.Warningf("RWO volume %s already attached to node %s, rejecting attachment to node %s",
					volumeID, existing.NodeID, nodeID)

				// Post event for operator visibility (best effort)
				cs.postAttachmentConflictEvent(ctx, req, existing.NodeID)

				// Record conflict metric
				if cs.driver.metrics != nil {
					cs.driver.metrics.RecordAttachmentConflict()
				}

				return nil, status.Errorf(codes.FailedPrecondition,
					"Volume %s already attached to node %s. For multi-node access, use RWX with block volumes.",
					volumeID, existing.NodeID)
			}
		}
	}

	// No existing attachment - track new primary attachment with access mode
	if err := am.TrackAttachmentWithMode(ctx, volumeID, nodeID, accessMode); err != nil {
		// Check if this is a conflict (race condition - another request won)
		if existing, exists := am.GetAttachment(volumeID); exists && !am.IsAttachedToNode(volumeID, nodeID) {
			if cs.driver.metrics != nil {
				cs.driver.metrics.RecordAttachmentConflict()
			}
			return nil, status.Errorf(codes.FailedPrecondition,
				"volume %s already attached to node %s, cannot attach to %s",
				volumeID, existing.NodeID, nodeID)
		}
		return nil, status.Errorf(codes.Internal, "failed to track attachment: %v", err)
	}

	// Record attachment success metric
	duration := time.Since(startTime)
	if cs.driver.metrics != nil {
		cs.driver.metrics.RecordAttachmentOp("attach", nil, duration)
	}

	// Post attachment event (best effort)
	cs.postVolumeAttachedEvent(ctx, req, duration)

	klog.V(2).Infof("Successfully published volume %s to node %s", volumeID, nodeID)

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
	}, nil
}

// ControllerUnpublishVolume removes volume attachment tracking for a node.
// This method is idempotent - returns success even if volume not currently attached.
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	startTime := time.Now()
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()

	klog.V(2).Infof("ControllerUnpublishVolume called for volume %s from node %s", volumeID, nodeID)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	// Note: nodeID can be empty for force-detach scenarios per CSI spec

	// Validate volume ID format (security: prevent injection)
	if err := utils.ValidateVolumeID(volumeID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume ID: %v", err)
	}

	// Get attachment manager
	am := cs.driver.GetAttachmentManager()
	if am == nil {
		// No attachment manager = nothing to untrack
		klog.V(3).Infof("Attachment manager not available, skipping untrack for volume %s", volumeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	// Remove this node's attachment (handles both RWO and RWX)
	fullyDetached, err := am.RemoveNodeAttachment(ctx, volumeID, nodeID)
	if err != nil {
		klog.Warningf("Error removing node attachment for volume %s: %v (returning success)", volumeID, err)
	}

	if fullyDetached {
		// Record detachment metric
		if cs.driver.metrics != nil {
			cs.driver.metrics.RecordAttachmentOp("detach", nil, time.Since(startTime))
		}
		cs.postVolumeDetachedEvent(ctx, req)
	} else {
		// Partial detach (RWX migration - one node still attached)
		klog.V(2).Infof("Volume %s partially detached from node %s, other node(s) still attached", volumeID, nodeID)
		if cs.driver.metrics != nil {
			cs.driver.metrics.RecordAttachmentOp("detach_partial", nil, time.Since(startTime))
		}
	}

	klog.V(2).Infof("Successfully unpublished volume %s from node %s", volumeID, nodeID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
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

// ControllerExpandVolume expands a volume on the backend storage
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	klog.V(2).Infof("ControllerExpandVolume called for volume: %s", volumeID)

	// Validate request
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	// Validate volume ID format
	if err := utils.ValidateVolumeID(volumeID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume ID: %v", err)
	}

	// Get required capacity
	requiredBytes := req.GetCapacityRange().GetRequiredBytes()
	if requiredBytes == 0 {
		return nil, status.Error(codes.InvalidArgument, "required capacity is required")
	}

	// Enforce size limits
	if requiredBytes < minVolumeSizeBytes {
		return nil, status.Errorf(codes.OutOfRange, "required bytes %d is less than minimum %d", requiredBytes, minVolumeSizeBytes)
	}

	limitBytes := req.GetCapacityRange().GetLimitBytes()
	if limitBytes > 0 && requiredBytes > limitBytes {
		return nil, status.Errorf(codes.OutOfRange, "required bytes %d exceeds limit bytes %d", requiredBytes, limitBytes)
	}

	if requiredBytes > maxVolumeSizeBytes {
		return nil, status.Errorf(codes.OutOfRange, "required bytes %d exceeds maximum %d", requiredBytes, maxVolumeSizeBytes)
	}

	// Check if volume exists
	existingVolume, err := cs.driver.rdsClient.GetVolume(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
	}

	// Check if expansion is needed
	if existingVolume.FileSizeBytes >= requiredBytes {
		klog.V(2).Infof("Volume %s already at or above requested size (%d >= %d), no expansion needed",
			volumeID, existingVolume.FileSizeBytes, requiredBytes)
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         existingVolume.FileSizeBytes,
			NodeExpansionRequired: false,
		}, nil
	}

	// Resize volume on RDS
	klog.V(2).Infof("Expanding volume %s from %d to %d bytes", volumeID, existingVolume.FileSizeBytes, requiredBytes)

	if err := cs.driver.rdsClient.ResizeVolume(volumeID, requiredBytes); err != nil {
		// Check if this is a capacity error
		if containsString(err.Error(), "not enough space") {
			return nil, status.Errorf(codes.ResourceExhausted, "insufficient storage on RDS for expansion: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to resize volume on RDS: %v", err)
	}

	klog.V(2).Infof("Successfully expanded volume %s on RDS to %d bytes", volumeID, requiredBytes)

	// Return response indicating node expansion is required to resize the filesystem
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         requiredBytes,
		NodeExpansionRequired: true,
	}, nil
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

		// RWX block-only validation (ROADMAP-4)
		// RWX with filesystem volumes risks data corruption - reject with actionable error
		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			if cap.GetMount() != nil {
				return fmt.Errorf("RWX access mode requires volumeMode: Block. " +
					"Filesystem volumes risk data corruption with multi-node access. " +
					"For KubeVirt VM live migration, use volumeMode: Block in your PVC")
			}
			// Log valid RWX block usage for debugging/auditing
			klog.V(2).Info("RWX block volume capability validated (KubeVirt live migration use case)")
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
