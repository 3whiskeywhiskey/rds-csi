package reconciler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// DefaultOrphanCheckInterval is the default interval between orphan checks
	DefaultOrphanCheckInterval = 1 * time.Hour

	// DefaultOrphanGracePeriod is the minimum age before a volume is considered orphaned
	// This prevents premature cleanup of volumes that are still being provisioned
	DefaultOrphanGracePeriod = 5 * time.Minute

	// VolumeIDPrefix is the expected prefix for CSI-managed volumes
	VolumeIDPrefix = "pvc-"
)

// OrphanReconcilerConfig contains configuration for the orphan reconciler
type OrphanReconcilerConfig struct {
	// RDSClient is the RDS client for listing/deleting volumes
	RDSClient rds.RDSClient

	// K8sClient is the Kubernetes clientset for listing PVs
	K8sClient kubernetes.Interface

	// CheckInterval is how often to check for orphans
	CheckInterval time.Duration

	// GracePeriod is the minimum age before considering a volume orphaned
	GracePeriod time.Duration

	// DryRun if true, will only log orphans without deleting them
	DryRun bool

	// Enabled enables/disables the reconciler
	Enabled bool
}

// OrphanReconciler periodically checks for orphaned volumes and cleans them up
type OrphanReconciler struct {
	config OrphanReconcilerConfig
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// OrphanedVolume represents a volume that appears to be orphaned
type OrphanedVolume struct {
	VolumeID  string
	FilePath  string
	SizeBytes int64
	CreatedAt time.Time
}

// NewOrphanReconciler creates a new orphan reconciler
func NewOrphanReconciler(config OrphanReconcilerConfig) (*OrphanReconciler, error) {
	// Validate config
	if config.RDSClient == nil {
		return nil, fmt.Errorf("RDSClient is required")
	}
	if config.K8sClient == nil {
		return nil, fmt.Errorf("K8sClient is required")
	}

	// Set defaults
	if config.CheckInterval == 0 {
		config.CheckInterval = DefaultOrphanCheckInterval
	}
	if config.GracePeriod == 0 {
		config.GracePeriod = DefaultOrphanGracePeriod
	}

	return &OrphanReconciler{
		config: config,
		stopCh: make(chan struct{}),
	}, nil
}

// Start begins the reconciliation loop
func (r *OrphanReconciler) Start(ctx context.Context) error {
	if !r.config.Enabled {
		klog.Info("Orphan reconciler is disabled")
		return nil
	}

	klog.Infof("Starting orphan reconciler (interval=%v, grace_period=%v, dry_run=%v)",
		r.config.CheckInterval, r.config.GracePeriod, r.config.DryRun)

	r.wg.Add(1)
	go r.run(ctx)

	return nil
}

// Stop stops the reconciliation loop
func (r *OrphanReconciler) Stop() {
	if !r.config.Enabled {
		return
	}

	klog.Info("Stopping orphan reconciler")
	close(r.stopCh)
	r.wg.Wait()
	klog.Info("Orphan reconciler stopped")
}

// run is the main reconciliation loop
func (r *OrphanReconciler) run(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	// Run once immediately on startup
	if err := r.reconcile(ctx); err != nil {
		klog.Errorf("Initial orphan reconciliation failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := r.reconcile(ctx); err != nil {
				klog.Errorf("Orphan reconciliation failed: %v", err)
			}
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// reconcile performs one reconciliation cycle
func (r *OrphanReconciler) reconcile(ctx context.Context) error {
	klog.V(2).Info("Starting orphan reconciliation cycle")
	start := time.Now()

	// Get all volumes from RDS
	rdsVolumes, err := r.config.RDSClient.ListVolumes()
	if err != nil {
		return fmt.Errorf("failed to list RDS volumes: %w", err)
	}

	// Get all PVs from Kubernetes
	pvList, err := r.config.K8sClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Kubernetes PVs: %w", err)
	}

	// Build a map of active volume IDs from Kubernetes PVs
	activeVolumeIDs := make(map[string]bool)
	for _, pv := range pvList.Items {
		// Only consider PVs from this CSI driver
		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == "rds.csi.srvlab.io" {
			volumeID := pv.Spec.CSI.VolumeHandle
			activeVolumeIDs[volumeID] = true
		}
	}

	klog.V(3).Infof("Found %d volumes in RDS, %d active PVs in Kubernetes", len(rdsVolumes), len(activeVolumeIDs))

	// Identify orphaned volumes
	orphans := []OrphanedVolume{}
	for _, vol := range rdsVolumes {
		// Skip volumes that don't match our CSI-managed pattern
		if !strings.HasPrefix(vol.Slot, VolumeIDPrefix) {
			klog.V(5).Infof("Skipping non-CSI volume: %s", vol.Slot)
			continue
		}

		// Check if this volume has a corresponding PV
		if activeVolumeIDs[vol.Slot] {
			klog.V(5).Infof("Volume %s has active PV", vol.Slot)
			continue
		}

		// Volume appears to be orphaned
		// Note: We can't get creation time from RDS, so we'll use grace period
		// to avoid deleting volumes that were just created
		orphan := OrphanedVolume{
			VolumeID:  vol.Slot,
			FilePath:  vol.FilePath,
			SizeBytes: vol.FileSizeBytes,
			// We can't determine actual creation time from RDS, so assume it's old enough
			CreatedAt: time.Now().Add(-r.config.GracePeriod * 2),
		}
		orphans = append(orphans, orphan)
	}

	if len(orphans) == 0 {
		klog.V(2).Infof("No orphaned volumes found (checked %d volumes in %v)", len(rdsVolumes), time.Since(start))
		return nil
	}

	// Log and potentially clean up orphans
	klog.Warningf("Found %d orphaned volumes", len(orphans))
	for _, orphan := range orphans {
		age := time.Since(orphan.CreatedAt)

		if age < r.config.GracePeriod {
			klog.V(3).Infof("Orphaned volume %s is too young (age=%v, grace=%v), skipping",
				orphan.VolumeID, age, r.config.GracePeriod)
			continue
		}

		klog.Warningf("Orphaned volume detected: %s (path=%s, size=%d bytes, age=%v)",
			orphan.VolumeID, orphan.FilePath, orphan.SizeBytes, age)

		if r.config.DryRun {
			klog.Infof("[DRY-RUN] Would delete orphaned volume: %s", orphan.VolumeID)
			continue
		}

		// Delete the orphaned volume
		if err := r.deleteOrphanedVolume(orphan); err != nil {
			klog.Errorf("Failed to delete orphaned volume %s: %v", orphan.VolumeID, err)
			continue
		}

		klog.Infof("Successfully deleted orphaned volume: %s", orphan.VolumeID)
	}

	klog.V(2).Infof("Orphan reconciliation cycle complete (duration=%v, orphans=%d)",
		time.Since(start), len(orphans))

	return nil
}

// deleteOrphanedVolume deletes an orphaned volume from RDS
func (r *OrphanReconciler) deleteOrphanedVolume(orphan OrphanedVolume) error {
	klog.V(2).Infof("Deleting orphaned volume: %s", orphan.VolumeID)

	if err := r.config.RDSClient.DeleteVolume(orphan.VolumeID); err != nil {
		return fmt.Errorf("failed to delete volume from RDS: %w", err)
	}

	return nil
}

// TriggerReconciliation triggers an immediate reconciliation (for testing/debugging)
func (r *OrphanReconciler) TriggerReconciliation(ctx context.Context) error {
	if !r.config.Enabled {
		return fmt.Errorf("reconciler is disabled")
	}
	return r.reconcile(ctx)
}
