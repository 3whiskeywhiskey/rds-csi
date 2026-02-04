// Package attachment provides thread-safe tracking of volume-to-node attachments
// for the RDS CSI driver.
package attachment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/observability"
)

// EventPoster is an interface for posting Kubernetes events for attachment lifecycle.
// This interface allows the reconciler to post events without creating a circular
// dependency with the driver package.
type EventPoster interface {
	// PostStaleAttachmentCleared posts an event when a stale attachment is cleared
	PostStaleAttachmentCleared(ctx context.Context, pvcNamespace, pvcName, volumeID, staleNodeID string) error
}

// AttachmentReconciler periodically checks for stale attachments and cleans them up.
// Stale attachments occur when a node is deleted without proper cleanup of attached volumes.
type AttachmentReconciler struct {
	manager     *AttachmentManager
	k8sClient   kubernetes.Interface
	interval    time.Duration
	gracePeriod time.Duration
	metrics     *observability.Metrics
	eventPoster EventPoster // Optional, may be nil

	// Control channels
	stopCh chan struct{}
	doneCh chan struct{}
	mu     sync.Mutex
}

// ReconcilerConfig holds configuration for the AttachmentReconciler.
type ReconcilerConfig struct {
	Manager     *AttachmentManager
	K8sClient   kubernetes.Interface
	Interval    time.Duration // Default: 5 minutes
	GracePeriod time.Duration // Default: 30 seconds
	Metrics     *observability.Metrics
	EventPoster EventPoster // Optional, may be nil - for posting lifecycle events
}

// NewAttachmentReconciler creates a new AttachmentReconciler.
func NewAttachmentReconciler(config ReconcilerConfig) (*AttachmentReconciler, error) {
	if config.Manager == nil {
		return nil, fmt.Errorf("manager is required")
	}
	if config.K8sClient == nil {
		return nil, fmt.Errorf("k8sClient is required")
	}
	if config.Interval <= 0 {
		config.Interval = 5 * time.Minute
	}
	if config.GracePeriod <= 0 {
		config.GracePeriod = 30 * time.Second
	}

	return &AttachmentReconciler{
		manager:     config.Manager,
		k8sClient:   config.K8sClient,
		interval:    config.Interval,
		gracePeriod: config.GracePeriod,
		metrics:     config.Metrics,
		eventPoster: config.EventPoster,
	}, nil
}

// Start begins the background reconciliation loop.
// Returns immediately; reconciliation runs in a separate goroutine.
// Call Stop() to gracefully shut down.
func (r *AttachmentReconciler) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.stopCh != nil {
		r.mu.Unlock()
		return fmt.Errorf("reconciler already running")
	}
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	klog.Infof("Starting attachment reconciler (interval=%v, grace_period=%v)", r.interval, r.gracePeriod)

	go r.run(ctx)

	return nil
}

// Stop gracefully stops the reconciliation loop.
// Blocks until the reconciler has fully stopped.
func (r *AttachmentReconciler) Stop() {
	r.mu.Lock()
	if r.stopCh == nil {
		r.mu.Unlock()
		return
	}
	close(r.stopCh)
	doneCh := r.doneCh
	// Clear channels so subsequent Stop() calls are no-op
	r.stopCh = nil
	r.doneCh = nil
	r.mu.Unlock()

	// Wait for run() to exit
	<-doneCh

	klog.Info("Attachment reconciler stopped")
}

// run is the main reconciliation loop.
func (r *AttachmentReconciler) run(ctx context.Context) {
	// Capture channels as local variables to avoid race with Stop()
	r.mu.Lock()
	stopCh := r.stopCh
	doneCh := r.doneCh
	r.mu.Unlock()

	defer close(doneCh)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Run initial reconciliation immediately
	r.reconcile(ctx)

	for {
		select {
		case <-ticker.C:
			r.reconcile(ctx)
		case <-stopCh:
			klog.V(2).Info("Attachment reconciler shutting down")
			return
		case <-ctx.Done():
			klog.V(2).Info("Attachment reconciler context cancelled")
			return
		}
	}
}

// reconcile performs a single reconciliation pass.
func (r *AttachmentReconciler) reconcile(ctx context.Context) {
	startTime := time.Now()
	klog.V(4).Info("Starting attachment reconciliation")

	// Get all current attachments
	attachments := r.manager.ListAttachments()

	clearedCount := 0
	for volumeID, state := range attachments {
		// Check if context is cancelled
		if ctx.Err() != nil {
			klog.V(2).Info("Reconciliation interrupted by context cancellation")
			return
		}

		// Check if node still exists
		nodeExists, err := r.nodeExists(ctx, state.NodeID)
		if err != nil {
			// API error - fail open (don't clear on transient errors)
			klog.Warningf("Failed to check node %s for volume %s: %v (skipping)", state.NodeID, volumeID, err)
			continue
		}

		if nodeExists {
			// Node exists, attachment is valid
			continue
		}

		// Node deleted - check if within grace period
		detachTime := r.manager.GetDetachTimestamp(volumeID)
		if !detachTime.IsZero() && time.Since(detachTime) < r.gracePeriod {
			klog.V(4).Infof("Node %s deleted but within grace period for volume %s", state.NodeID, volumeID)
			continue
		}

		// Clear stale attachment
		staleNodeID := state.NodeID // Capture before clearing
		klog.Infof("Clearing stale attachment: volume=%s node=%s (node deleted)", volumeID, staleNodeID)
		if err := r.manager.UntrackAttachment(ctx, volumeID); err != nil {
			klog.Errorf("Failed to clear stale attachment for volume %s: %v", volumeID, err)
			continue
		}

		clearedCount++

		// Record metrics
		if r.metrics != nil {
			r.metrics.RecordStaleAttachmentCleared()
			r.metrics.RecordReconcileAction("clear_stale")
		}

		// Post event (best effort - don't fail reconciliation if event posting fails)
		r.postStaleAttachmentClearedEvent(ctx, volumeID, staleNodeID)
	}

	duration := time.Since(startTime)

	// Record reconcile duration
	if r.metrics != nil {
		r.metrics.RecordAttachmentOp("reconcile", nil, duration)
	}

	if clearedCount > 0 {
		klog.Infof("Attachment reconciliation complete: cleared %d stale attachments (duration=%v)", clearedCount, duration)
	} else {
		klog.V(4).Infof("Attachment reconciliation complete: no stale attachments (duration=%v)", duration)
	}
}

// nodeExists checks if a Kubernetes node exists.
func (r *AttachmentReconciler) nodeExists(ctx context.Context, nodeID string) (bool, error) {
	_, err := r.k8sClient.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetGracePeriod returns the configured grace period duration.
func (r *AttachmentReconciler) GetGracePeriod() time.Duration {
	return r.gracePeriod
}

// postStaleAttachmentClearedEvent posts an event when a stale attachment is cleared.
// This is a best-effort operation - failures are logged but don't affect reconciliation.
func (r *AttachmentReconciler) postStaleAttachmentClearedEvent(ctx context.Context, volumeID, staleNodeID string) {
	if r.eventPoster == nil {
		return
	}

	// Look up the PV to get the bound PVC information
	// Volume ID is typically the PV name (e.g., pvc-<uuid>)
	pv, err := r.k8sClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, metav1.GetOptions{})
	if err != nil {
		klog.V(4).Infof("Cannot get PV %s for stale attachment event: %v", volumeID, err)
		return
	}

	claimRef := pv.Spec.ClaimRef
	if claimRef == nil {
		klog.V(4).Infof("PV %s has no claimRef for stale attachment event", volumeID)
		return
	}

	if err := r.eventPoster.PostStaleAttachmentCleared(ctx, claimRef.Namespace, claimRef.Name, volumeID, staleNodeID); err != nil {
		klog.Warningf("Failed to post stale attachment cleared event for volume %s: %v", volumeID, err)
	}
}
