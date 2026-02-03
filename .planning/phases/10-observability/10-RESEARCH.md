# Phase 10: Observability - Research

**Researched:** 2026-02-03
**Domain:** Prometheus metrics for KubeVirt VM live migration, Kubernetes events for migration lifecycle, user documentation for safe RWX usage patterns
**Confidence:** HIGH

## Summary

Phase 10 adds migration-specific observability to the RWX capability implemented in Phases 8-9. The research confirms that the existing observability infrastructure (`pkg/observability/prometheus.go` with prometheus/client_golang, `pkg/driver/events.go` with EventRecorder) provides the foundation needed. The AttachmentState from Phase 9 already tracks `MigrationStartedAt`, `MigrationTimeout`, and dual-node state, which are the exact signals needed for migration metrics. This phase adds: (1) three new Prometheus metrics (`migrations_total` counter, `migration_duration_seconds` histogram, `active_migrations` gauge) to expose migration outcomes and performance, (2) three Kubernetes events (MigrationStarted, MigrationCompleted, MigrationFailed) posted to PVCs for operator visibility, and (3) comprehensive user documentation explaining that RWX block volumes are safe only for KubeVirt live migration, not general RWX workloads.

**Key findings:**
1. **Prometheus histogram is standard for duration tracking** - Official Prometheus docs recommend histograms over counters for duration because they support percentile calculation and cross-instance aggregation
2. **Gauge increment/decrement pattern tracks active operations** - Standard pattern: `gauge.Inc()` when migration starts, `defer gauge.Dec()` when migration ends (success or failure)
3. **KubeVirt exposes rich migration metrics** - KubeVirt itself tracks 10+ migration metrics including phase transitions, data processed, memory transfer rate, providing reference patterns for CSI driver metrics
4. **Counter labels should be bounded** - Use `result` label with fixed values (success/failed/timeout) to avoid unbounded cardinality
5. **Events should be actionable** - Include timing, node IDs, and clear guidance (e.g., "Migration completed: source node1 → target node2 in 45s")
6. **Documentation must warn about data corruption** - Explicitly state RWX block is safe only for KubeVirt QEMU coordination, filesystem volumes are rejected, general RWX workloads will corrupt data

**Primary recommendation:** Add migration metrics using existing Metrics struct in `pkg/observability/prometheus.go`, extend EventPoster in `pkg/driver/events.go` with three migration event methods, record metrics at key transition points (AddSecondaryAttachment, RemoveNodeAttachment, timeout detection), and create `docs/kubevirt-migration.md` with clear safety warnings and examples.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/prometheus/client_golang` | v1.23.2 (already in deps) | Prometheus metrics (Counter, Gauge, Histogram) | Official Prometheus Go client, already used in pkg/observability |
| `k8s.io/client-go/tools/record` | v0.28.0 (already in deps) | Kubernetes EventRecorder | Already used in pkg/driver/events.go |
| `time` (Go stdlib) | Go 1.24 | Duration calculation for migration timing | Standard Go time handling |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `k8s.io/klog/v2` | v2.100.1 (already in deps) | Structured logging | Log migration state transitions at V(2) level |
| `fmt` (Go stdlib) | Go 1.24 | Format event messages with migration context | Include node IDs, durations, timeouts in event strings |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Histogram for durations | Summary | Histogram allows aggregation across instances; Summary gives exact quantiles but can't aggregate (Prometheus best practice: use Histogram) |
| Separate metrics for each result | Single counter with label | Single counter with bounded label (success/failed/timeout) prevents metric explosion |
| Events to PV | Events to PVC | PVC events are more visible to users (kubectl describe pvc), PV events require finding the PV first |

**Installation:**
```bash
# No new dependencies needed - all packages already in go.mod
# prometheus/client_golang v1.23.2 already present
# k8s.io/client-go v0.28.0 already present
# time, fmt are Go stdlib
```

## Architecture Patterns

### Recommended Project Structure
```
pkg/
├── observability/
│   └── prometheus.go          # MODIFY: Add migration metrics (3 new metrics)
├── driver/
│   ├── events.go              # MODIFY: Add migration event methods (3 methods)
│   └── controller.go          # MODIFY: Record metrics and post events at key points
├── attachment/
│   └── manager.go             # MODIFY: Record metrics in AddSecondaryAttachment, RemoveNodeAttachment
docs/
└── kubevirt-migration.md      # NEW: User documentation for safe RWX usage
```

### Pattern 1: Histogram for Migration Duration Tracking
**What:** Use Prometheus Histogram to track migration duration with percentile calculation support
**When to use:** Measuring operation duration in distributed systems where aggregation is needed
**Example:**
```go
// Source: https://prometheus.io/docs/practices/histograms/
// Source: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

// In Metrics struct
type Metrics struct {
	// ... existing fields

	// Migration metrics
	migrationsTotal        *prometheus.CounterVec   // Labels: result (success/failed/timeout)
	migrationDuration      prometheus.Histogram
	activeMigrations       prometheus.Gauge
}

// In NewMetrics()
migrationsTotal: prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "migration",
		Name:      "migrations_total",
		Help:      "Total number of KubeVirt live migrations by result",
	},
	[]string{"result"}, // success, failed, timeout
)

migrationDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
	Namespace: namespace,
	Subsystem: "migration",
	Name:      "migration_duration_seconds",
	Help:      "Duration of KubeVirt live migrations from dual-attach to source detach",
	// KubeVirt migrations typically take 30s-120s for small VMs, up to 10min for large memory
	// Buckets cover fast migrations (30s) to slow migrations (10min+)
	Buckets:   []float64{15, 30, 60, 90, 120, 180, 300, 600},
})

activeMigrations: prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Subsystem: "migration",
	Name:      "active_migrations",
	Help:      "Number of currently active migrations (volumes in dual-attach state)",
})

// Register in NewMetrics()
reg.MustRegister(
	m.migrationsTotal,
	m.migrationDuration,
	m.activeMigrations,
)

// Recording methods
func (m *Metrics) RecordMigrationStarted() {
	m.activeMigrations.Inc()
}

func (m *Metrics) RecordMigrationResult(result string, duration time.Duration) {
	m.migrationsTotal.WithLabelValues(result).Inc()
	m.migrationDuration.Observe(duration.Seconds())
	m.activeMigrations.Dec()
}
```

### Pattern 2: Gauge Increment/Decrement for Active Migrations
**What:** Use Gauge.Inc() when secondary attaches, Gauge.Dec() when migration completes or fails
**When to use:** Tracking current in-flight operations (connections, migrations, active requests)
**Example:**
```go
// Source: https://prometheus.io/docs/guides/go-application/
// Source: https://oneuptime.com/blog/post/2026-01-07-go-prometheus-custom-metrics/view
package attachment

import (
	"time"
	"k8s.io/klog/v2"
)

// In AddSecondaryAttachment (when migration starts)
func (am *AttachmentManager) AddSecondaryAttachment(ctx context.Context, volumeID, nodeID string, migrationTimeout time.Duration) error {
	// ... lock and validation ...

	// Track migration start time
	now := time.Now()
	existing.MigrationStartedAt = &now
	existing.MigrationTimeout = migrationTimeout

	// Record metric: migration started
	if am.metrics != nil {
		am.metrics.RecordMigrationStarted()
	}

	klog.V(2).Infof("Migration started: volume=%s, source=%s, target=%s, timeout=%v",
		volumeID, existing.Nodes[0].NodeID, nodeID, migrationTimeout)

	return nil
}

// In RemoveNodeAttachment (when migration completes)
func (am *AttachmentManager) RemoveNodeAttachment(ctx context.Context, volumeID, nodeID string) error {
	// ... find and remove node ...

	// If removing primary node (migration source), migration completed
	if found && len(newNodes) == 1 && existing.MigrationStartedAt != nil {
		// Calculate migration duration
		duration := time.Since(*existing.MigrationStartedAt)

		// Record metric: migration succeeded
		if am.metrics != nil {
			am.metrics.RecordMigrationResult("success", duration)
		}

		// Clear migration state
		existing.MigrationStartedAt = nil
		existing.MigrationTimeout = 0

		klog.V(2).Infof("Migration completed: volume=%s, duration=%v", volumeID, duration)
	}

	return nil
}
```

### Pattern 3: Counter with Bounded Labels for Migration Results
**What:** Use single counter with `result` label (success/failed/timeout) instead of separate counters
**When to use:** Tracking operation outcomes with fixed set of possible results
**Example:**
```go
// Source: https://prometheus.io/docs/practices/histograms/
// Source: Prometheus best practice - bounded label cardinality
package driver

// In ControllerPublishVolume (when detecting timeout)
if existing.IsMigrationTimedOut() {
	klog.Warningf("RWX volume %s migration timed out (%v elapsed, %v max)",
		volumeID, time.Since(*existing.MigrationStartedAt), existing.MigrationTimeout)

	// Record metric: migration failed (timeout)
	duration := time.Since(*existing.MigrationStartedAt)
	cs.driver.metrics.RecordMigrationResult("timeout", duration)

	// Post event: MigrationFailed
	cs.eventPoster.PostMigrationFailed(ctx, pvcNamespace, pvcName, volumeID,
		existing.Nodes[0].NodeID, nodeID, "timeout", duration)

	return nil, status.Errorf(codes.FailedPrecondition,
		"Volume %s migration timeout exceeded (%v elapsed, %v max). "+
		"Previous migration may be stuck. Detach source node to reset.",
		volumeID, duration, existing.MigrationTimeout)
}

// Query examples:
// rate(rds_csi_migration_migrations_total{result="success"}[5m]) - successful migrations/sec
// rate(rds_csi_migration_migrations_total{result="timeout"}[5m])  - timed out migrations/sec
// histogram_quantile(0.95, rds_csi_migration_migration_duration_seconds_bucket) - 95th percentile duration
```

### Pattern 4: Kubernetes Events for Migration Lifecycle
**What:** Post Normal events on start/complete, Warning event on failure, include node context
**When to use:** Operator needs visibility into migration progress and failures
**Example:**
```go
// Source: https://pkg.go.dev/k8s.io/client-go/tools/record
// Pattern from existing pkg/driver/events.go
package driver

import (
	"context"
	"fmt"
	"time"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	EventReasonMigrationStarted   = "MigrationStarted"
	EventReasonMigrationCompleted = "MigrationCompleted"
	EventReasonMigrationFailed    = "MigrationFailed"
)

// PostMigrationStarted posts a Normal event when secondary node attaches (migration begins)
func (ep *EventPoster) PostMigrationStarted(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode string, timeout time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get PVC %s/%s for migration started event: %v", pvcNamespace, pvcName, err)
		return nil // Don't fail operation
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration started - source: %s, target: %s, timeout: %s",
		volumeID, sourceNode, targetNode, timeout)
	ep.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonMigrationStarted, eventMessage)

	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationStarted)
	}

	klog.V(2).Infof("Posted migration started event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}

// PostMigrationCompleted posts a Normal event when source node detaches (migration succeeds)
func (ep *EventPoster) PostMigrationCompleted(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode string, duration time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get PVC %s/%s for migration completed event: %v", pvcNamespace, pvcName, err)
		return nil
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration completed - source: %s → target: %s (duration: %s)",
		volumeID, sourceNode, targetNode, duration.Round(time.Second))
	ep.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonMigrationCompleted, eventMessage)

	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationCompleted)
	}

	klog.V(2).Infof("Posted migration completed event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}

// PostMigrationFailed posts a Warning event when migration times out or fails
func (ep *EventPoster) PostMigrationFailed(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode, reason string, duration time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get PVC %s/%s for migration failed event: %v", pvcNamespace, pvcName, err)
		return nil
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration failed - source: %s, attempted target: %s, reason: %s, elapsed: %s",
		volumeID, sourceNode, targetNode, reason, duration.Round(time.Second))
	ep.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonMigrationFailed, eventMessage)

	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationFailed)
	}

	klog.V(2).Infof("Posted migration failed event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}
```

### Pattern 5: User Documentation with Explicit Safety Warnings
**What:** Document safe usage patterns with clear warnings about data corruption risks
**When to use:** Features that work correctly in narrow use cases but corrupt data if misused
**Example:**
```markdown
# Source: KubeVirt documentation patterns and CSI driver safety warnings
# docs/kubevirt-migration.md

# KubeVirt Live Migration with RDS CSI Driver

## Overview

The RDS CSI driver supports KubeVirt VM live migration through temporary ReadWriteMany (RWX) block volume access during migration. This document explains safe usage patterns and critical limitations.

## Safe Usage: KubeVirt Live Migration ONLY

✅ **SAFE**: KubeVirt VM live migration with block volumes
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: vm-disk
spec:
  accessModes:
    - ReadWriteMany    # RWX allowed for KubeVirt
  volumeMode: Block    # REQUIRED - filesystem rejected
  storageClassName: rds-nvme
  resources:
    requests:
      storage: 20Gi
```

❌ **UNSAFE - DATA CORRUPTION RISK**: General RWX workloads
```yaml
# DO NOT DO THIS - Multiple pods writing simultaneously will corrupt data
apiVersion: v1
kind: Pod
metadata:
  name: app-1
spec:
  containers:
  - name: app
    volumeDevices:
    - name: shared-disk
      devicePath: /dev/xvda
  volumes:
  - name: shared-disk
    persistentVolumeClaim:
      claimName: vm-disk  # Another pod also using this - DATA CORRUPTION!
```

## Why RWX is Safe for KubeVirt (But Not General Use)

### QEMU Coordination During Migration
KubeVirt uses QEMU's live migration protocol which:
1. Source VM pauses I/O before final handoff
2. Target VM receives memory state including in-flight I/O
3. Only one QEMU process issues writes at any time
4. Migration completes in seconds (< 5 minutes default timeout)

### NO Coordination for General Workloads
The CSI driver does NOT provide:
- Distributed locking between nodes
- I/O fencing to prevent split-brain
- Cluster filesystem (GFS2, OCFS2) support
- Write ordering guarantees across nodes

**Result**: Two pods on different nodes writing to the same block device will corrupt data.

## StorageClass Configuration

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rds-kubevirt
provisioner: rds.csi.mikrotik.com
parameters:
  # Migration timeout: Max duration for dual-node attachment
  # Default: 300 seconds (5 minutes)
  # Range: 30-3600 seconds
  # Increase for VMs with large memory or slow network
  migrationTimeoutSeconds: "300"
volumeBindingMode: WaitForFirstConsumer
```

## Monitoring Migration

### Prometheus Metrics

```promql
# Active migrations (dual-attach state)
rds_csi_migration_active_migrations

# Migration success rate
rate(rds_csi_migration_migrations_total{result="success"}[5m])

# Migration timeouts
rate(rds_csi_migration_migrations_total{result="timeout"}[5m])

# 95th percentile migration duration
histogram_quantile(0.95, rate(rds_csi_migration_migration_duration_seconds_bucket[5m]))
```

### Kubernetes Events

```bash
# Watch migration events on PVC
kubectl describe pvc vm-disk | grep -A 5 Migration

# Example events:
# Normal   MigrationStarted    10m   rds-csi-controller  [pvc-xyz]: KubeVirt live migration started - source: node1, target: node2, timeout: 5m0s
# Normal   MigrationCompleted  10m   rds-csi-controller  [pvc-xyz]: KubeVirt live migration completed - source: node1 → target: node2 (duration: 45s)
# Warning  MigrationFailed     5m    rds-csi-controller  [pvc-xyz]: KubeVirt live migration failed - source: node1, attempted target: node2, reason: timeout, elapsed: 5m0s
```

## Troubleshooting

### Migration Timeout
**Error**: `Volume %s migration timeout exceeded`

**Cause**: Migration took longer than `migrationTimeoutSeconds` (default 5 minutes)

**Solution**:
1. Check VM memory size - large VMs take longer to migrate
2. Increase timeout in StorageClass: `migrationTimeoutSeconds: "600"`
3. Verify network bandwidth between nodes (NVMe/TCP performance)
4. Check for memory-intensive workloads during migration

### Data Corruption After Using RWX
**Symptom**: VM fails to boot, filesystem errors, data loss

**Cause**: Used RWX volume with multiple pods writing simultaneously (not KubeVirt migration)

**Prevention**: ONLY use RWX access mode for KubeVirt VMs, never for general workloads

**Recovery**: Restore from backup - corruption is not recoverable

## Future Enhancements

Currently deferred (not implemented):
- Cluster filesystem support (GFS2, OCFS2) for true RWX workloads
- RDS-level namespace reservations for split-brain protection
- KubeVirt API integration for richer migration awareness

For general shared storage needs, use NFS or other cluster-aware filesystems instead of RWX block volumes.
```

### Anti-Patterns to Avoid
- **High-cardinality labels** - Don't use volumeID or nodeID as metric labels; log them instead
- **Separate counters per result** - Use single counter with `result` label (success/failed/timeout)
- **Event spam** - Don't post events on every internal state change; only post at lifecycle boundaries (start/complete/fail)
- **Missing timeout handling** - Always record metric and post event when migration times out (treat as failed migration)
- **Misleading documentation** - Don't say "RWX supported" without huge warnings about data corruption risk for non-KubeVirt use
- **Recording duration on start** - Only observe histogram when migration completes (success or timeout); use gauge for active count

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Percentile calculation | Calculate quantiles in driver | Prometheus histogram_quantile() | Histogram supports aggregation across instances, driver can't see all data |
| Event deduplication | Manual event caching | EventRecorder built-in aggregation | client-go automatically aggregates similar events |
| Duration bucket selection | Linear buckets | Non-linear buckets for migration | Migrations vary widely (30s-600s); logarithmic spacing captures distribution better |
| Separate success/failure metrics | Two counters | Single counter with label | Reduces cardinality, enables single PromQL query for all results |
| Migration phase tracking | Multiple gauges per phase | Single active_migrations gauge | Simple increment on start, decrement on complete/timeout covers all cases |

**Key insight:** Prometheus is designed for aggregation across distributed systems. Use histogram for duration (not counter), use bounded labels (not per-volume), and let Prometheus do the math (histogram_quantile, rate) rather than pre-computing in the driver.

## Common Pitfalls

### Pitfall 1: Not Recording Metric on Migration Timeout
**What goes wrong:** `migrations_total` only counts success/failure, timeouts are invisible
**Why it happens:** Timeout detection happens in ControllerPublishVolume (attach path), not RemoveNodeAttachment (detach path) where success is recorded
**How to avoid:** Call `RecordMigrationResult("timeout", duration)` when `IsMigrationTimedOut()` returns true
**Warning signs:** Low migration count in metrics but many "migration timeout exceeded" errors in logs
**Source:** Phase 9 implementation - timeout detected in controller.go ControllerPublishVolume

### Pitfall 2: Gauge Not Decremented on Failure
**What goes wrong:** `active_migrations` gauge increases forever, never decreases on timeout/failure
**Why it happens:** Only decrement path is in successful RemoveNodeAttachment, timeout path doesn't decrement
**How to avoid:** Always decrement gauge in RecordMigrationResult regardless of result (success/failed/timeout)
**Warning signs:** active_migrations metric grows over time, never decreases even when no migrations running
**Source:** Gauge increment/decrement pattern - must decrement on all exit paths

### Pitfall 3: High-Cardinality Volume/Node Labels
**What goes wrong:** Prometheus memory explosion, scrape failures, slow queries
**Why it happens:** Adding volumeID or nodeID as labels creates unbounded cardinality (one time series per volume)
**How to avoid:** Only use bounded labels (result: success/failed/timeout); log volumeID/nodeID in event messages and klog
**Warning signs:** Prometheus scrape timeout errors, high Prometheus memory usage, slow PromQL queries
**Source:** [Prometheus best practices](https://prometheus.io/docs/practices/instrumentation/#do-not-overuse-labels) - bounded label cardinality

### Pitfall 4: Recording Duration Before Migration Starts
**What goes wrong:** Histogram observes zero or negative durations
**Why it happens:** Recording duration in AddSecondaryAttachment before MigrationStartedAt is set
**How to avoid:** Only observe duration in RemoveNodeAttachment (success) or timeout detection (failure)
**Warning signs:** Histogram buckets with zero-second durations, impossible migration times
**Source:** Histogram best practices - observe duration after operation completes

### Pitfall 5: Posting Events Without PVC Context
**What goes wrong:** Event posting fails because PVC can't be found (only have volumeID)
**Why it happens:** AttachmentManager doesn't track PVC namespace/name, only volumeID
**How to avoid:** Get PVC from VolumeContext in ControllerPublishVolume, pass to event methods
**Warning signs:** "Failed to get PVC for migration event" warnings in logs, no events visible in kubectl describe pvc
**Source:** Existing pattern in pkg/driver/events.go - all event methods accept pvcNamespace, pvcName parameters

### Pitfall 6: Misleading RWX Documentation
**What goes wrong:** Users deploy general RWX workloads (e.g., shared Redis data dir), data corruption ensues
**Why it happens:** Documentation says "RWX supported" without explaining it's only safe for KubeVirt
**How to avoid:** Lead with "SAFE: KubeVirt only" and "UNSAFE: general workloads", use ✅ and ❌ symbols, explain QEMU coordination
**Warning signs:** User issues reporting data corruption, complaints about "broken RWX support"
**Source:** CSI driver security patterns - features with narrow safe use cases need prominent warnings

## Code Examples

Verified patterns from official sources:

### Example 1: Adding Migration Metrics to Metrics Struct
```go
// Source: https://prometheus.io/docs/guides/go-application/
// Location: pkg/observability/prometheus.go
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type Metrics struct {
	registry *prometheus.Registry

	// ... existing fields ...

	// Migration metrics (Phase 10)
	migrationsTotal   *prometheus.CounterVec
	migrationDuration prometheus.Histogram
	activeMigrations  prometheus.Gauge
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		// ... existing metrics ...

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
			Name:      "migration_duration_seconds",
			Help:      "Duration of KubeVirt live migrations from dual-attach to source detach",
			// Buckets: 15s, 30s, 1min, 1.5min, 2min, 3min, 5min, 10min
			// Covers typical migration times (30s-120s) to large VMs (5-10min)
			Buckets:   []float64{15, 30, 60, 90, 120, 180, 300, 600},
		}),

		activeMigrations: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "migration",
			Name:      "active_migrations",
			Help:      "Number of currently active migrations (volumes in dual-attach state)",
		}),
	}

	// Register all metrics
	reg.MustRegister(
		// ... existing metrics ...
		m.migrationsTotal,
		m.migrationDuration,
		m.activeMigrations,
	)

	return m
}

// RecordMigrationStarted increments the active migrations gauge
func (m *Metrics) RecordMigrationStarted() {
	m.activeMigrations.Inc()
}

// RecordMigrationResult records migration outcome and duration, decrements active gauge
func (m *Metrics) RecordMigrationResult(result string, duration time.Duration) {
	m.migrationsTotal.WithLabelValues(result).Inc()
	m.migrationDuration.Observe(duration.Seconds())
	m.activeMigrations.Dec()
}
```

### Example 2: Recording Metrics in AddSecondaryAttachment (Migration Start)
```go
// Source: Phase 9 implementation pattern
// Location: pkg/attachment/manager.go
package attachment

import (
	"context"
	"time"
	"k8s.io/klog/v2"
)

func (am *AttachmentManager) AddSecondaryAttachment(ctx context.Context, volumeID, nodeID string, migrationTimeout time.Duration) error {
	am.volumeLocks.Lock(volumeID)
	defer am.volumeLocks.Unlock(volumeID)

	am.mu.RLock()
	existing, exists := am.attachments[volumeID]
	am.mu.RUnlock()

	if !exists {
		return fmt.Errorf("volume %s not attached", volumeID)
	}

	// Validation: must be RWX, must have exactly 1 node, must be block
	if existing.AccessMode != "RWX" {
		return fmt.Errorf("cannot add secondary attachment to RWO volume %s", volumeID)
	}
	if len(existing.Nodes) != 1 {
		return fmt.Errorf("volume %s already has %d nodes", volumeID, len(existing.Nodes))
	}

	// Add secondary node
	existing.Nodes = append(existing.Nodes, NodeAttachment{
		NodeID:     nodeID,
		AttachedAt: time.Now(),
	})

	// Track migration start time for timeout enforcement
	now := time.Now()
	existing.MigrationStartedAt = &now
	existing.MigrationTimeout = migrationTimeout

	// OBSERVABILITY: Record metric - migration started
	if am.metrics != nil {
		am.metrics.RecordMigrationStarted()
	}

	klog.V(2).Infof("Migration started: volume=%s, source=%s, target=%s, timeout=%v",
		volumeID, existing.Nodes[0].NodeID, nodeID, migrationTimeout)

	return nil
}
```

### Example 3: Recording Metrics in RemoveNodeAttachment (Migration Complete)
```go
// Source: Phase 9 implementation pattern
// Location: pkg/attachment/manager.go
package attachment

func (am *AttachmentManager) RemoveNodeAttachment(ctx context.Context, volumeID, nodeID string) error {
	am.volumeLocks.Lock(volumeID)
	defer am.volumeLocks.Unlock(volumeID)

	am.mu.RLock()
	existing, exists := am.attachments[volumeID]
	am.mu.RUnlock()

	if !exists {
		klog.V(2).Infof("Volume %s not attached (idempotent)", volumeID)
		return nil
	}

	// Find and remove the node
	newNodes := []NodeAttachment{}
	found := false
	removedPrimary := false
	for i, na := range existing.Nodes {
		if na.NodeID == nodeID {
			found = true
			if i == 0 {
				removedPrimary = true // Removing first node (migration source)
			}
			continue // Skip this node
		}
		newNodes = append(newNodes, na)
	}

	if !found {
		klog.V(2).Infof("Node %s not attached to volume %s (idempotent)", nodeID, volumeID)
		return nil
	}

	// Update attachment state
	existing.Nodes = newNodes

	// If removing primary node (migration source) and was migrating, record completion
	if removedPrimary && len(newNodes) == 1 && existing.MigrationStartedAt != nil {
		// Calculate migration duration
		duration := time.Since(*existing.MigrationStartedAt)

		// OBSERVABILITY: Record metric - migration succeeded
		if am.metrics != nil {
			am.metrics.RecordMigrationResult("success", duration)
		}

		// Clear migration state
		existing.MigrationStartedAt = nil
		existing.MigrationTimeout = 0

		klog.V(2).Infof("Migration completed: volume=%s, duration=%v", volumeID, duration)
	}

	// If down to 0 nodes, remove from map
	if len(newNodes) == 0 {
		am.mu.Lock()
		delete(am.attachments, volumeID)
		am.mu.Unlock()
	}

	return nil
}
```

### Example 4: Posting Migration Events
```go
// Source: https://pkg.go.dev/k8s.io/client-go/tools/record
// Location: pkg/driver/events.go
package driver

import (
	"context"
	"fmt"
	"time"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	EventReasonMigrationStarted   = "MigrationStarted"
	EventReasonMigrationCompleted = "MigrationCompleted"
	EventReasonMigrationFailed    = "MigrationFailed"
)

// PostMigrationStarted posts a Normal event when secondary node attaches (migration begins)
func (ep *EventPoster) PostMigrationStarted(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode string, timeout time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		// Don't fail operation just because event couldn't be posted
		klog.Warningf("Failed to get PVC %s/%s for migration started event: %v", pvcNamespace, pvcName, err)
		return nil
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration started - source: %s, target: %s, timeout: %s",
		volumeID, sourceNode, targetNode, timeout)
	ep.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonMigrationStarted, eventMessage)

	// Record that we posted an event
	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationStarted)
	}

	klog.V(2).Infof("Posted migration started event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}

// PostMigrationCompleted posts a Normal event when source node detaches (migration succeeds)
func (ep *EventPoster) PostMigrationCompleted(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode string, duration time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get PVC %s/%s for migration completed event: %v", pvcNamespace, pvcName, err)
		return nil
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration completed - source: %s → target: %s (duration: %s)",
		volumeID, sourceNode, targetNode, duration.Round(time.Second))
	ep.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonMigrationCompleted, eventMessage)

	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationCompleted)
	}

	klog.V(2).Infof("Posted migration completed event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}

// PostMigrationFailed posts a Warning event when migration times out or fails
func (ep *EventPoster) PostMigrationFailed(ctx context.Context, pvcNamespace, pvcName, volumeID, sourceNode, targetNode, reason string, duration time.Duration) error {
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get PVC %s/%s for migration failed event: %v", pvcNamespace, pvcName, err)
		return nil
	}

	eventMessage := fmt.Sprintf("[%s]: KubeVirt live migration failed - source: %s, attempted target: %s, reason: %s, elapsed: %s",
		volumeID, sourceNode, targetNode, reason, duration.Round(time.Second))
	ep.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonMigrationFailed, eventMessage)

	if ep.metrics != nil {
		ep.metrics.RecordEventPosted(EventReasonMigrationFailed)
	}

	klog.V(2).Infof("Posted migration failed event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)
	return nil
}
```

### Example 5: Timeout Handling in ControllerPublishVolume
```go
// Source: Phase 9 implementation pattern
// Location: pkg/driver/controller.go
package driver

import (
	"context"
	"time"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	accessMode := getAccessMode(req.GetVolumeCapability())

	// ... validation ...

	// Check existing attachment
	existing, exists := cs.driver.attachmentManager.GetAttachment(volumeID)
	if exists && accessMode == "RWX" {
		// For RWX volumes, allow up to 2 nodes (KubeVirt migration)
		if len(existing.Nodes) >= 2 {
			return nil, status.Errorf(codes.FailedPrecondition,
				"Volume %s already attached to 2 nodes (migration limit). Attached nodes: %v",
				volumeID, existing.GetNodeIDs())
		}

		// Check if existing migration has timed out
		if existing.IsMigrationTimedOut() {
			duration := time.Since(*existing.MigrationStartedAt)

			klog.Warningf("RWX volume %s migration timed out (%v elapsed, %v max), rejecting new secondary attachment",
				volumeID, duration, existing.MigrationTimeout)

			// OBSERVABILITY: Record metric - migration timed out
			cs.driver.metrics.RecordMigrationResult("timeout", duration)

			// OBSERVABILITY: Post event - migration failed
			// Get PVC from VolumeContext
			pvcNamespace := req.GetVolumeContext()["csi.storage.k8s.io/pvc/namespace"]
			pvcName := req.GetVolumeContext()["csi.storage.k8s.io/pvc/name"]
			cs.eventPoster.PostMigrationFailed(ctx, pvcNamespace, pvcName, volumeID,
				existing.Nodes[0].NodeID, nodeID, "timeout", duration)

			return nil, status.Errorf(codes.FailedPrecondition,
				"Volume %s migration timeout exceeded (%v elapsed, %v max). "+
				"Previous migration may be stuck. Detach source node to reset, or adjust migrationTimeoutSeconds in StorageClass.",
				volumeID, duration, existing.MigrationTimeout)
		}

		// Allow secondary attachment (within migration window)
		migrationTimeout := ParseMigrationTimeout(req.GetVolumeContext())
		if err := cs.driver.attachmentManager.AddSecondaryAttachment(ctx, volumeID, nodeID, migrationTimeout); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to add secondary attachment: %v", err)
		}

		// OBSERVABILITY: Post event - migration started
		pvcNamespace := req.GetVolumeContext()["csi.storage.k8s.io/pvc/namespace"]
		pvcName := req.GetVolumeContext()["csi.storage.k8s.io/pvc/name"]
		cs.eventPoster.PostMigrationStarted(ctx, pvcNamespace, pvcName, volumeID,
			existing.Nodes[0].NodeID, nodeID, migrationTimeout)

		return &csi.ControllerPublishVolumeResponse{}, nil
	}

	// ... normal attachment logic ...
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Counter for duration tracking | Histogram for duration | Prometheus best practices (2020+) | Enables percentile queries, aggregation across instances |
| Separate metrics per result | Single metric with label | Prometheus label best practices (2018+) | Reduces metric count, enables single PromQL query |
| Manual gauge tracking | Inc/Dec pattern with defer | Go best practices (2015+) | Guarantees decrement even on panic/error |
| Generic "RWX supported" docs | "Safe for KubeVirt only" warnings | CSI security patterns (2023+) | Prevents data corruption from misuse |
| Verbose event messages | Structured format with node IDs | Kubernetes event patterns (2020+) | Easier parsing, actionable information |

**Deprecated/outdated:**
- Counter metrics for operation duration - Use Histogram instead (enables percentile calculation)
- Separate success/failure counters - Use single counter with `result` label (bounded cardinality)
- Millisecond-based buckets for migration - Use second-based buckets (standard Prometheus unit)
- Events without context (node IDs, duration) - Include actionable details for operators

## Open Questions

Things that couldn't be fully resolved:

1. **Histogram bucket range for migration duration**
   - What we know: KubeVirt migrations typically take 30s-120s for small VMs, up to 10min for large memory
   - What's unclear: Exact distribution in production with NVMe/TCP vs other transports
   - Recommendation: Start with `[]float64{15, 30, 60, 90, 120, 180, 300, 600}` (covers 15s to 10min), adjust based on actual p95/p99 from metrics

2. **Should MigrationFailed event include next steps?**
   - What we know: Timeout indicates stuck migration, operator needs to take action
   - What's unclear: Whether to include remediation steps in event message (e.g., "Delete source pod to reset")
   - Recommendation: Include brief guidance in event message ("Detach source node to reset"), full troubleshooting in docs/kubevirt-migration.md

3. **Expose migration state via /metrics labels?**
   - What we know: `active_migrations` gauge shows count, but not which volumes or nodes
   - What's unclear: Whether to add `volume_migration_state` gauge with volume/source/target labels (high cardinality)
   - Recommendation: Don't add per-volume labels to metrics (unbounded cardinality), rely on events for per-volume visibility

4. **Should documentation include Grafana dashboard?**
   - What we know: Operators will want to visualize migration metrics in Grafana
   - What's unclear: Whether to include pre-built dashboard JSON in docs/ or just PromQL examples
   - Recommendation: Start with PromQL examples in docs, create dashboard if users request it (avoids maintenance burden)

## Sources

### Primary (HIGH confidence)
- [Prometheus histogram best practices](https://prometheus.io/docs/practices/histograms/) - Histogram vs summary, bucket selection, aggregation
- [Prometheus Go application instrumentation](https://prometheus.io/docs/guides/go-application/) - Official setup, metric types, best practices
- [prometheus/client_golang package docs](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus) - Counter, Gauge, Histogram API
- [k8s.io/client-go/tools/record](https://pkg.go.dev/k8s.io/client-go/tools/record) - EventRecorder, event posting patterns
- [KubeVirt monitoring metrics](https://github.com/kubevirt/monitoring/blob/main/docs/metrics.md) - KubeVirt's own migration metrics (reference implementation)

### Secondary (MEDIUM confidence)
- [How to Create Custom Metrics in Go with Prometheus (2026)](https://oneuptime.com/blog/post/2026-01-07-go-prometheus-custom-metrics/view) - Recent patterns, gauge usage
- [Kubernetes observability trends 2026](https://www.usdsi.org/data-science-insights/kubernetes-observability-and-monitoring-trends-in-2026) - OpenTelemetry, cost-aware monitoring, AI-driven insights
- [Prometheus Monitoring with Golang](https://medium.com/devbulls/prometheus-monitoring-with-golang-c0ec035a6e37) - Counter vs Gauge vs Histogram comparison
- [CSI driver metrics libraries](https://pkg.go.dev/github.com/kubernetes-csi/csi-lib-utils/metrics) - Standard CSI metrics patterns

### Tertiary (LOW confidence)
- [Kubernetes monitoring tools 2026](https://openobserve.ai/blog/top-10-k8s-monitoring-tools/) - Ecosystem overview (not directly applicable)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - prometheus/client_golang and client-go/tools/record already in use, well-documented
- Architecture: HIGH - Patterns verified from Prometheus docs, existing pkg/observability and pkg/driver/events.go
- Pitfalls: HIGH - Sourced from Prometheus best practices, KubeVirt migration patterns, Go gauge usage patterns
- Code examples: HIGH - Based on prometheus/client_golang official docs, KubeVirt metrics reference, existing codebase patterns

**Research date:** 2026-02-03
**Valid until:** 90 days (mature ecosystem - Prometheus, client-go are stable; KubeVirt patterns well-established)

**Key constraints from Phase 10 requirements:**
- Metric prefix: `rds_csi_migration_`
- Three metrics required: `migrations_total`, `migration_duration_seconds`, `active_migrations`
- Three events required: MigrationStarted (Normal), MigrationCompleted (Normal), MigrationFailed (Warning)
- Documentation location: `docs/kubevirt-migration.md`
- Must warn against general RWX usage (data corruption risk)

**Existing codebase assets:**
- `pkg/observability/prometheus.go` - Metrics struct with custom registry, already has attachment metrics pattern
- `pkg/driver/events.go` - EventPoster with PVC event posting pattern, graceful failure handling
- `pkg/attachment/types.go` - AttachmentState with `MigrationStartedAt`, `MigrationTimeout`, `IsMigrating()`, `IsMigrationTimedOut()`
- `pkg/attachment/manager.go` - AddSecondaryAttachment, RemoveNodeAttachment - migration start/complete detection points
- `pkg/driver/controller.go` - ControllerPublishVolume - migration timeout detection point
