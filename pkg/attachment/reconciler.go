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

// AttachmentReconciler periodically checks for stale attachments and cleans them up.
// Stale attachments occur when a node is deleted without proper cleanup of attached volumes.
type AttachmentReconciler struct {
	manager     *AttachmentManager
	k8sClient   kubernetes.Interface
	interval    time.Duration
	gracePeriod time.Duration
	metrics     *observability.Metrics

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
	r.mu.Unlock()

	// Wait for run() to exit
	<-doneCh

	klog.Info("Attachment reconciler stopped")
}

// run is the main reconciliation loop.
func (r *AttachmentReconciler) run(ctx context.Context) {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Run initial reconciliation immediately
	r.reconcile(ctx)

	for {
		select {
		case <-ticker.C:
			r.reconcile(ctx)
		case <-r.stopCh:
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
	klog.V(3).Info("Starting attachment reconciliation")

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
			klog.V(3).Infof("Node %s deleted but within grace period for volume %s", state.NodeID, volumeID)
			continue
		}

		// Clear stale attachment
		klog.Infof("Clearing stale attachment: volume=%s node=%s (node deleted)", volumeID, state.NodeID)
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
	}

	duration := time.Since(startTime)

	// Record reconcile duration
	if r.metrics != nil {
		r.metrics.RecordAttachmentOp("reconcile", nil, duration)
	}

	if clearedCount > 0 {
		klog.Infof("Attachment reconciliation complete: cleared %d stale attachments (duration=%v)", clearedCount, duration)
	} else {
		klog.V(3).Infof("Attachment reconciliation complete: no stale attachments (duration=%v)", duration)
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
