package attachment

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	// driverName is the CSI driver name used to filter PVs
	driverName = "rds.csi.srvlab.io"
)

// RebuildState reconstructs the in-memory attachment state from PersistentVolume annotations.
// This is called on controller startup to recover state after a restart.
// Returns nil if k8sClient is nil (allows operation without k8s in tests).
func (am *AttachmentManager) RebuildState(ctx context.Context) error {
	if am.k8sClient == nil {
		klog.V(2).Info("Skipping state rebuild (no k8s client)")
		return nil
	}

	klog.Info("Rebuilding attachment state from PersistentVolumes")

	// List all PVs in the cluster
	pvList, err := am.k8sClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Acquire write lock to rebuild state
	am.mu.Lock()
	defer am.mu.Unlock()

	// Clear existing state
	am.attachments = make(map[string]*AttachmentState)

	// Process each PV
	rebuiltCount := 0
	for _, pv := range pvList.Items {
		// Only process PVs belonging to our driver
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != driverName {
			continue
		}

		// Extract volume ID from CSI volume handle
		volumeID := pv.Spec.CSI.VolumeHandle
		if volumeID == "" {
			klog.Warningf("PV %s has empty volume handle, skipping", pv.Name)
			continue
		}

		// Check for attachment annotation
		if pv.Annotations == nil {
			continue
		}

		nodeID, hasNode := pv.Annotations[AnnotationAttachedNode]
		if !hasNode || nodeID == "" {
			continue
		}

		// Parse timestamp if present, otherwise use current time
		attachedAt := time.Now()
		if attachedAtStr, ok := pv.Annotations[AnnotationAttachedAt]; ok {
			if parsed, err := time.Parse(metav1.RFC3339Micro, attachedAtStr); err == nil {
				attachedAt = parsed
			} else {
				klog.Warningf("Failed to parse attachment timestamp for volume %s: %v", volumeID, err)
			}
		}

		// Create and store attachment state
		state := &AttachmentState{
			VolumeID:   volumeID,
			NodeID:     nodeID,
			AttachedAt: attachedAt,
		}
		am.attachments[volumeID] = state
		rebuiltCount++

		klog.V(2).Infof("Rebuilt attachment: volume=%s, node=%s", volumeID, nodeID)
	}

	klog.Infof("State rebuild complete: %d attachments recovered", rebuiltCount)
	return nil
}

// Initialize initializes the AttachmentManager by rebuilding state from PersistentVolumes.
// This should be called once during driver startup.
func (am *AttachmentManager) Initialize(ctx context.Context) error {
	klog.Info("Initializing AttachmentManager")

	if err := am.RebuildState(ctx); err != nil {
		return err
	}

	klog.Info("AttachmentManager initialized successfully")
	return nil
}
