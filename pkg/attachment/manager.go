// Package attachment provides thread-safe tracking of volume-to-node attachments
// for the RDS CSI driver. It uses in-memory state with RWMutex for concurrent access
// and per-volume locks to serialize operations on individual volumes.
package attachment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// AttachmentManager tracks which volumes are attached to which nodes
// and provides thread-safe operations for managing attachment state.
type AttachmentManager struct {
	// mu protects the attachments map for concurrent access
	mu sync.RWMutex

	// attachments maps volumeID to attachment state
	attachments map[string]*AttachmentState

	// volumeLocks provides per-volume operation locking
	volumeLocks *VolumeLockManager

	// k8sClient is used for future PV annotation updates (can be nil initially)
	k8sClient kubernetes.Interface
}

// NewAttachmentManager creates a new AttachmentManager
func NewAttachmentManager(k8sClient kubernetes.Interface) *AttachmentManager {
	return &AttachmentManager{
		attachments: make(map[string]*AttachmentState),
		volumeLocks: NewVolumeLockManager(),
		k8sClient:   k8sClient,
	}
}

// TrackAttachment records that a volume is attached to a node.
// This method is idempotent - if the volume is already attached to the same node,
// it returns nil. If the volume is attached to a different node, it returns an error.
func (am *AttachmentManager) TrackAttachment(ctx context.Context, volumeID, nodeID string) error {
	// Acquire per-volume lock to serialize operations on this volume
	am.volumeLocks.Lock(volumeID)
	defer am.volumeLocks.Unlock(volumeID)

	// Check existing attachment under read lock
	am.mu.RLock()
	existing, exists := am.attachments[volumeID]
	am.mu.RUnlock()

	if exists {
		// Idempotent: already attached to same node
		if existing.NodeID == nodeID {
			klog.V(2).Infof("Volume %s already attached to node %s (idempotent)", volumeID, nodeID)
			return nil
		}

		// Error: attached to different node
		return fmt.Errorf("volume %s already attached to node %s", volumeID, existing.NodeID)
	}

	// Create new attachment state
	state := &AttachmentState{
		VolumeID:   volumeID,
		NodeID:     nodeID,
		AttachedAt: time.Now(),
	}

	// Store under write lock
	am.mu.Lock()
	am.attachments[volumeID] = state
	am.mu.Unlock()

	klog.V(2).Infof("Tracked attachment: volume=%s, node=%s", volumeID, nodeID)
	return nil
}

// UntrackAttachment removes the attachment record for a volume.
// This method is idempotent - if the volume is not tracked, it returns nil.
func (am *AttachmentManager) UntrackAttachment(ctx context.Context, volumeID string) error {
	// Acquire per-volume lock to serialize operations on this volume
	am.volumeLocks.Lock(volumeID)
	defer am.volumeLocks.Unlock(volumeID)

	// Check if exists before deleting
	am.mu.RLock()
	_, exists := am.attachments[volumeID]
	am.mu.RUnlock()

	if !exists {
		klog.V(2).Infof("Volume %s not tracked, nothing to untrack (idempotent)", volumeID)
		return nil
	}

	// Delete under write lock
	am.mu.Lock()
	delete(am.attachments, volumeID)
	am.mu.Unlock()

	klog.V(2).Infof("Untracked attachment: volume=%s", volumeID)
	return nil
}

// GetAttachment retrieves the attachment state for a volume.
// Returns the state and a boolean indicating if the volume is tracked.
// This is a read-only operation and does not block other readers.
func (am *AttachmentManager) GetAttachment(volumeID string) (*AttachmentState, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	state, exists := am.attachments[volumeID]
	return state, exists
}

// ListAttachments returns a copy of all current attachments.
// The returned map is a copy to prevent external mutation of internal state.
func (am *AttachmentManager) ListAttachments() map[string]*AttachmentState {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Create a copy to prevent external mutation
	copy := make(map[string]*AttachmentState, len(am.attachments))
	for volumeID, state := range am.attachments {
		copy[volumeID] = state
	}

	return copy
}
