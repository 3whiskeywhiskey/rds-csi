// Package observability provides Prometheus metrics for the RDS CSI driver.
package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// namespace is the Prometheus metric namespace prefix for all RDS CSI metrics.
	namespace = "rds_csi"
)

// Metrics holds all Prometheus metrics for the RDS CSI driver.
type Metrics struct {
	registry *prometheus.Registry

	// Volume operation metrics
	volumeOpsTotal    *prometheus.CounterVec
	volumeOpsDuration *prometheus.HistogramVec

	// NVMe connection metrics
	nvmeConnectsTotal     *prometheus.CounterVec
	nvmeConnectDuration   prometheus.Histogram
	nvmeConnectionsActive prometheus.Gauge

	// Mount operation metrics
	mountOpsTotal *prometheus.CounterVec

	// Stale mount metrics
	staleMountsDetectedTotal prometheus.Counter
	staleRecoveriesTotal     *prometheus.CounterVec

	// Orphan cleanup metrics
	orphansCleanedTotal prometheus.Counter

	// Kubernetes events metrics
	eventsPostedTotal *prometheus.CounterVec

	// Attachment operation metrics
	attachmentAttachTotal     *prometheus.CounterVec
	attachmentDetachTotal     *prometheus.CounterVec
	attachmentConflictsTotal  prometheus.Counter
	attachmentReconcileTotal  *prometheus.CounterVec
	attachmentOpDuration      *prometheus.HistogramVec
	attachmentGracePeriodUsed prometheus.Counter
	attachmentStaleCleared    prometheus.Counter

	// Migration operation metrics
	migrationsTotal    *prometheus.CounterVec
	migrationDuration  prometheus.Histogram
	activeMigrations   prometheus.Gauge
}

// NewMetrics creates a new Metrics instance with all metrics registered.
// Uses a custom registry to avoid panics on driver restart (not DefaultRegistry).
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		volumeOpsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "volume_operations_total",
				Help:      "Total number of volume operations by type and status",
			},
			[]string{"operation", "status"},
		),

		volumeOpsDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "volume_operation_duration_seconds",
				Help:      "Duration of volume operations in seconds",
				Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"operation"},
		),

		nvmeConnectsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "nvme_connects_total",
				Help:      "Total number of NVMe connection attempts by status",
			},
			[]string{"status"},
		),

		nvmeConnectDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "nvme_connect_duration_seconds",
			Help:      "Duration of NVMe connection establishment in seconds",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		}),

		nvmeConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "nvme_connections_active",
			Help:      "Number of currently active NVMe/TCP connections",
		}),

		mountOpsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "mount_operations_total",
				Help:      "Total number of mount/unmount operations by type and status",
			},
			[]string{"operation", "status"},
		),

		staleMountsDetectedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stale_mounts_detected_total",
			Help:      "Total number of stale mounts detected",
		}),

		staleRecoveriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "stale_recoveries_total",
				Help:      "Total number of stale mount recovery attempts by status",
			},
			[]string{"status"},
		),

		orphansCleanedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orphans_cleaned_total",
			Help:      "Total number of orphaned NVMe connections cleaned up",
		}),

		eventsPostedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "events_posted_total",
				Help:      "Total number of Kubernetes events posted by reason",
			},
			[]string{"reason"},
		),

		attachmentAttachTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "attachment",
				Name:      "attach_total",
				Help:      "Total attachment operations by status",
			},
			[]string{"status"}, // success, failure
		),

		attachmentDetachTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "attachment",
				Name:      "detach_total",
				Help:      "Total detachment operations by status",
			},
			[]string{"status"},
		),

		attachmentConflictsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "attachment",
			Name:      "conflicts_total",
			Help:      "Total attachment conflicts (RWO violations)",
		}),

		attachmentReconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "attachment",
				Name:      "reconcile_total",
				Help:      "Total reconciliation actions by type",
			},
			[]string{"action"}, // clear_stale, sync_annotation
		),

		attachmentOpDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "attachment",
				Name:      "operation_duration_seconds",
				Help:      "Duration of attachment operations",
				Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"operation"}, // attach, detach, reconcile
		),

		attachmentGracePeriodUsed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "attachment",
			Name:      "grace_period_used_total",
			Help:      "Total times grace period prevented a conflict",
		}),

		attachmentStaleCleared: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "attachment",
			Name:      "stale_cleared_total",
			Help:      "Total stale attachments cleared by reconciler",
		}),

		migrationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "migration",
				Name:      "migrations_total",
				Help:      "Total number of KubeVirt live migrations by result",
			},
			[]string{"result"}, // success, failed, timeout
		),

		migrationDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "migration",
			Name:      "duration_seconds",
			Help:      "Duration of KubeVirt live migrations in seconds",
			Buckets:   []float64{15, 30, 60, 90, 120, 180, 300, 600},
		}),

		activeMigrations: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "migration",
			Name:      "active_migrations",
			Help:      "Number of currently in-progress KubeVirt live migrations",
		}),
	}

	// Register all metrics with the custom registry
	reg.MustRegister(
		m.volumeOpsTotal,
		m.volumeOpsDuration,
		m.nvmeConnectsTotal,
		m.nvmeConnectDuration,
		m.nvmeConnectionsActive,
		m.mountOpsTotal,
		m.staleMountsDetectedTotal,
		m.staleRecoveriesTotal,
		m.orphansCleanedTotal,
		m.eventsPostedTotal,
		m.attachmentAttachTotal,
		m.attachmentDetachTotal,
		m.attachmentConflictsTotal,
		m.attachmentReconcileTotal,
		m.attachmentOpDuration,
		m.attachmentGracePeriodUsed,
		m.attachmentStaleCleared,
		m.migrationsTotal,
		m.migrationDuration,
		m.activeMigrations,
	)

	return m
}

// Handler returns an http.Handler for the /metrics endpoint.
// Use promhttp.HandlerFor with the custom registry for proper isolation.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// RecordVolumeOp records a volume operation with timing.
// operation should be one of: create, delete, stage, unstage, publish, unpublish.
func (m *Metrics) RecordVolumeOp(operation string, err error, duration time.Duration) {
	status := "success"
	if err != nil {
		status = "failure"
	}
	m.volumeOpsTotal.WithLabelValues(operation, status).Inc()
	m.volumeOpsDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordNVMeConnect records an NVMe connection attempt.
// On success (err == nil), also records the duration and increments active connections.
func (m *Metrics) RecordNVMeConnect(err error, duration time.Duration) {
	status := "success"
	if err != nil {
		status = "failure"
	}
	m.nvmeConnectsTotal.WithLabelValues(status).Inc()
	if err == nil {
		m.nvmeConnectDuration.Observe(duration.Seconds())
		m.nvmeConnectionsActive.Inc()
	}
}

// RecordNVMeDisconnect records an NVMe disconnection.
// Decrements the active connections gauge.
func (m *Metrics) RecordNVMeDisconnect() {
	m.nvmeConnectionsActive.Dec()
}

// RecordMountOp records a mount or unmount operation.
// operation should be one of: mount, unmount.
func (m *Metrics) RecordMountOp(operation string, err error) {
	status := "success"
	if err != nil {
		status = "failure"
	}
	m.mountOpsTotal.WithLabelValues(operation, status).Inc()
}

// RecordStaleMountDetected records that a stale mount was detected.
func (m *Metrics) RecordStaleMountDetected() {
	m.staleMountsDetectedTotal.Inc()
}

// RecordStaleRecovery records a stale mount recovery attempt.
func (m *Metrics) RecordStaleRecovery(err error) {
	status := "success"
	if err != nil {
		status = "failure"
	}
	m.staleRecoveriesTotal.WithLabelValues(status).Inc()
}

// RecordOrphanCleaned records that an orphaned NVMe connection was cleaned up.
func (m *Metrics) RecordOrphanCleaned() {
	m.orphansCleanedTotal.Inc()
}

// RecordEventPosted records that a Kubernetes event was posted.
// reason should match the event reason constants (e.g., MountFailure, RecoveryFailed).
func (m *Metrics) RecordEventPosted(reason string) {
	m.eventsPostedTotal.WithLabelValues(reason).Inc()
}

// RecordAttachmentOp records an attachment or detachment operation with duration.
// operation should be "attach" or "detach".
func (m *Metrics) RecordAttachmentOp(operation string, err error, duration time.Duration) {
	status := "success"
	if err != nil {
		status = "failure"
	}

	switch operation {
	case "attach":
		m.attachmentAttachTotal.WithLabelValues(status).Inc()
	case "detach":
		m.attachmentDetachTotal.WithLabelValues(status).Inc()
	}

	m.attachmentOpDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordAttachmentConflict records an RWO attachment conflict.
func (m *Metrics) RecordAttachmentConflict() {
	m.attachmentConflictsTotal.Inc()
}

// RecordGracePeriodUsed records when grace period prevented a conflict.
func (m *Metrics) RecordGracePeriodUsed() {
	m.attachmentGracePeriodUsed.Inc()
}

// RecordStaleAttachmentCleared records when reconciler cleared a stale attachment.
func (m *Metrics) RecordStaleAttachmentCleared() {
	m.attachmentStaleCleared.Inc()
}

// RecordReconcileAction records a reconciliation action.
// action should be "clear_stale" or "sync_annotation".
func (m *Metrics) RecordReconcileAction(action string) {
	m.attachmentReconcileTotal.WithLabelValues(action).Inc()
}

// RecordMigrationStarted records the start of a KubeVirt live migration.
// Increments the active migrations gauge.
func (m *Metrics) RecordMigrationStarted() {
	m.activeMigrations.Inc()
}

// RecordMigrationResult records the completion of a KubeVirt live migration.
// result must be one of: "success", "failed", "timeout".
// Increments the migrations counter, observes duration, and decrements active gauge.
func (m *Metrics) RecordMigrationResult(result string, duration time.Duration) {
	m.migrationsTotal.WithLabelValues(result).Inc()
	m.migrationDuration.Observe(duration.Seconds())
	m.activeMigrations.Dec()
}
