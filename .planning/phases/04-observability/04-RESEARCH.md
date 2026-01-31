# Phase 4: Observability - Research

**Researched:** 2026-01-30
**Domain:** Prometheus metrics for CSI drivers, Kubernetes events, NodeGetVolumeStats VolumeCondition, observability patterns
**Confidence:** HIGH

## Summary

This phase implements comprehensive observability for the RDS CSI driver through three pillars: Kubernetes events for operational visibility, NodeGetVolumeStats VolumeCondition for volume health reporting, and Prometheus metrics for monitoring and alerting. The research confirms that the codebase already has significant infrastructure in place (EventPoster in pkg/driver/events.go, SecurityMetrics in pkg/security/metrics.go, and partial NodeGetVolumeStats implementation), reducing this phase primarily to: (1) adding VolumeCondition to NodeGetVolumeStats response, (2) bridging existing internal metrics to Prometheus exposition format, and (3) enhancing event coverage for comprehensive mount failure/recovery scenarios.

Key findings:
1. **EventPoster already exists** - pkg/driver/events.go has EventPoster with PostMountFailure, PostRecoveryFailed, PostStaleMountDetected methods
2. **NodeGetVolumeStats partially implemented** - Already returns VolumeCondition with abnormal=true for stale mounts, but not declared as capability
3. **SecurityMetrics tracks all operations** - pkg/security/metrics.go already tracks counters for SSH, volume ops, NVMe, mount, failures - just needs Prometheus exposition
4. **prometheus/client_golang is the standard** - Version v1.23.2, simple API for Counter/Gauge/Histogram with labels
5. **CSI VolumeCondition is straightforward** - Two required fields: `abnormal` (bool) and `message` (string)
6. **Metrics HTTP endpoint is common pattern** - CSI drivers expose metrics on separate HTTP port (e.g., 8095, 9809), not gRPC

**Primary recommendation:** Add Prometheus exposition to existing SecurityMetrics, declare NODE_GET_VOLUME_STATS and VOLUME_CONDITION capabilities, ensure NodeGetVolumeStats always returns VolumeCondition, and add new event types for comprehensive coverage.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/prometheus/client_golang` | v1.23.2 | Prometheus metrics exposition | Official Prometheus Go client, standard for all Go services |
| `github.com/prometheus/client_golang/prometheus/promhttp` | (part of above) | HTTP handler for /metrics endpoint | Official HTTP handler, works with net/http |
| CSI spec VolumeCondition | CSI v1.5.0+ | Volume health reporting | Part of CSI spec, used by Kubelet for volume health |
| `k8s.io/client-go/tools/record` | v0.28.0 (already in deps) | Kubernetes events | Already used by EventPoster, standard K8s events API |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net/http` | Go 1.24 stdlib | HTTP server for metrics | Serve /metrics endpoint alongside CSI gRPC server |
| `github.com/kubernetes-csi/csi-lib-utils/metrics` | v0.17.x | CSI-specific metrics utilities | Optional - provides CSIMetricsManager, may add complexity vs rolling own |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| prometheus/client_golang directly | csi-lib-utils/metrics CSIMetricsManager | CSIMetricsManager adds abstraction and sidecar integration, but prometheus/client_golang is simpler for plugin-only metrics |
| Separate metrics struct | Extend existing SecurityMetrics | SecurityMetrics already tracks everything needed; adding Prometheus exposition is cleaner |
| OpenTelemetry | Prometheus client | OpenTelemetry is more complex; Prometheus is dominant in K8s ecosystem |

**Installation:**
```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

## Architecture Patterns

### Recommended Project Structure
```
pkg/
├── driver/
│   ├── events.go          # EXISTING: EventPoster - may need enhancement
│   ├── node.go            # MODIFY: Add VOLUME_CONDITION capability, enhance NodeGetVolumeStats
│   └── driver.go          # MODIFY: Add NodeServiceCapability for VOLUME_CONDITION
├── security/
│   ├── metrics.go         # EXISTING: SecurityMetrics - keep as internal tracking
│   └── events.go          # EXISTING: SecurityEvent types
├── observability/         # NEW: Prometheus integration
│   ├── prometheus.go      # Prometheus registry, metric definitions, HTTP handler
│   └── prometheus_test.go # Tests for metrics exposition
cmd/rds-csi-plugin/
└── main.go                # MODIFY: Start HTTP server for /metrics endpoint
```

### Pattern 1: Prometheus Metrics Registry with Labels
**What:** Create a custom Prometheus registry with CSI-specific metrics using CounterVec/GaugeVec/HistogramVec.
**When to use:** Driver initialization, expose via HTTP.
**Example:**
```go
// Source: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus
package observability

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusMetrics struct {
    registry *prometheus.Registry

    // Volume operation counters
    volumeOpsTotal    *prometheus.CounterVec
    volumeOpsDuration *prometheus.HistogramVec

    // Connection metrics
    nvmeConnections     prometheus.Gauge
    nvmeConnectFailures *prometheus.CounterVec

    // Mount metrics
    mountOpsTotal    *prometheus.CounterVec
    staleMountsTotal prometheus.Counter

    // Health gauges
    volumeHealthStatus *prometheus.GaugeVec
}

func NewPrometheusMetrics(namespace string) *PrometheusMetrics {
    registry := prometheus.NewRegistry()

    m := &PrometheusMetrics{
        registry: registry,

        volumeOpsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "volume_operations_total",
                Help:      "Total number of volume operations",
            },
            []string{"operation", "status"}, // stage, unstage, publish, unpublish + success/failure
        ),

        volumeOpsDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Name:      "volume_operation_duration_seconds",
                Help:      "Duration of volume operations in seconds",
                Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
            },
            []string{"operation"},
        ),

        nvmeConnections: prometheus.NewGauge(prometheus.GaugeOpts{
            Namespace: namespace,
            Name:      "nvme_connections_active",
            Help:      "Number of active NVMe/TCP connections",
        }),

        nvmeConnectFailures: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "nvme_connect_failures_total",
                Help:      "Total number of NVMe connection failures",
            },
            []string{"reason"},
        ),

        mountOpsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "mount_operations_total",
                Help:      "Total number of mount operations",
            },
            []string{"operation", "status"}, // mount, unmount + success/failure
        ),

        staleMountsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "stale_mounts_detected_total",
            Help:      "Total number of stale mounts detected",
        }),

        volumeHealthStatus: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Name:      "volume_health_status",
                Help:      "Volume health status (1=healthy, 0=unhealthy)",
            },
            []string{"volume_id"},
        ),
    }

    // Register all metrics
    registry.MustRegister(
        m.volumeOpsTotal,
        m.volumeOpsDuration,
        m.nvmeConnections,
        m.nvmeConnectFailures,
        m.mountOpsTotal,
        m.staleMountsTotal,
        m.volumeHealthStatus,
    )

    return m
}

// Handler returns an http.Handler for the /metrics endpoint
func (m *PrometheusMetrics) Handler() http.Handler {
    return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
```

### Pattern 2: NodeGetVolumeStats with VolumeCondition
**What:** Return VolumeCondition in NodeGetVolumeStatsResponse, with health check based on mount staleness.
**When to use:** Every NodeGetVolumeStats call.
**Example:**
```go
// Source: CSI spec csi.proto VolumeCondition definition
// Note: NodeGetVolumeStats in node.go already has partial implementation
// Need to ensure VolumeCondition is ALWAYS returned (not just on stale)

func (ns *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
    // ... existing validation ...

    // Check mount health
    var volumeCondition *csi.VolumeCondition

    nqn, err := volumeIDToNQN(volumeID)
    if err == nil && ns.staleChecker != nil {
        stale, reason, checkErr := ns.staleChecker.IsMountStale(volumePath, nqn)
        if checkErr != nil {
            // Cannot determine health - report as potentially unhealthy
            volumeCondition = &csi.VolumeCondition{
                Abnormal: false, // Assume healthy if check fails
                Message:  fmt.Sprintf("Health check inconclusive: %v", checkErr),
            }
        } else if stale {
            volumeCondition = &csi.VolumeCondition{
                Abnormal: true,
                Message:  fmt.Sprintf("Stale mount detected: %s", reason),
            }
        } else {
            // Healthy
            volumeCondition = &csi.VolumeCondition{
                Abnormal: false,
                Message:  "Volume is healthy",
            }
        }
    } else {
        // No staleness check available, assume healthy
        volumeCondition = &csi.VolumeCondition{
            Abnormal: false,
            Message:  "Volume is healthy",
        }
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
        VolumeCondition: volumeCondition, // ALWAYS return VolumeCondition
    }, nil
}
```

### Pattern 3: Node Service Capabilities Declaration
**What:** Declare GET_VOLUME_STATS and VOLUME_CONDITION capabilities.
**When to use:** Driver initialization in driver.go.
**Example:**
```go
// Source: CSI spec NodeServiceCapability_RPC_Type
// File: pkg/driver/driver.go

func initializeNodeCapabilities() []*csi.NodeServiceCapability {
    return []*csi.NodeServiceCapability{
        {
            Type: &csi.NodeServiceCapability_Rpc{
                Rpc: &csi.NodeServiceCapability_RPC{
                    Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
                },
            },
        },
        {
            Type: &csi.NodeServiceCapability_Rpc{
                Rpc: &csi.NodeServiceCapability_RPC{
                    Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
                },
            },
        },
        {
            Type: &csi.NodeServiceCapability_Rpc{
                Rpc: &csi.NodeServiceCapability_RPC{
                    Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
                },
            },
        },
        // VOLUME_CONDITION enables VolumeCondition in NodeGetVolumeStats response
        {
            Type: &csi.NodeServiceCapability_Rpc{
                Rpc: &csi.NodeServiceCapability_RPC{
                    Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
                },
            },
        },
    }
}
```

### Pattern 4: HTTP Metrics Server Alongside gRPC
**What:** Start separate HTTP server for Prometheus /metrics endpoint.
**When to use:** Driver startup in main.go.
**Example:**
```go
// Source: Standard pattern from CSI drivers (EBS, Secrets Store)
// File: cmd/rds-csi-plugin/main.go

var (
    metricsAddr = flag.String("metrics-address", ":9809", "Address for Prometheus metrics endpoint")
)

func main() {
    // ... existing initialization ...

    // Create Prometheus metrics
    promMetrics := observability.NewPrometheusMetrics("rds_csi")

    // Start metrics HTTP server
    if *metricsAddr != "" {
        go func() {
            mux := http.NewServeMux()
            mux.Handle("/metrics", promMetrics.Handler())

            klog.Infof("Starting metrics server on %s", *metricsAddr)
            if err := http.ListenAndServe(*metricsAddr, mux); err != nil && err != http.ErrServerClosed {
                klog.Errorf("Metrics server failed: %v", err)
            }
        }()
    }

    // ... rest of driver startup ...
}
```

### Pattern 5: Enhanced Event Types for Comprehensive Coverage
**What:** Extend EventPoster with additional event types for connection failures, reconnection attempts.
**When to use:** During NVMe connection lifecycle.
**Example:**
```go
// Source: Kubernetes Events API best practices
// File: pkg/driver/events.go

const (
    // Existing events
    EventReasonMountFailure       = "MountFailure"
    EventReasonRecoveryFailed     = "RecoveryFailed"
    EventReasonStaleMountDetected = "StaleMountDetected"

    // New events for comprehensive coverage
    EventReasonConnectionFailure  = "ConnectionFailure"
    EventReasonConnectionRecovery = "ConnectionRecovery"
    EventReasonOrphanDetected     = "OrphanDetected"
    EventReasonOrphanCleaned      = "OrphanCleaned"
    EventReasonVolumeUnhealthy    = "VolumeUnhealthy"
)

// PostConnectionFailure posts an event when NVMe connection fails
func (ep *EventPoster) PostConnectionFailure(ctx context.Context, pvcNamespace, pvcName, volumeID, nodeName, targetAddress string, err error) error {
    pvc, getErr := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
    if getErr != nil {
        klog.Warningf("Failed to get PVC %s/%s for event: %v", pvcNamespace, pvcName, getErr)
        return nil
    }

    eventMessage := fmt.Sprintf("[%s] on [%s]: Failed to connect to NVMe target %s: %v", volumeID, nodeName, targetAddress, err)
    ep.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonConnectionFailure, eventMessage)

    return nil
}
```

### Anti-Patterns to Avoid
- **High cardinality labels:** Don't use volumeID as a label in histograms/counters - use only for gauges that need per-volume tracking
- **Blocking on event posting:** Event posting should never block CSI operations - fail silently
- **Mixing CSI gRPC and HTTP metrics on same port:** Keep metrics HTTP server on separate port
- **Not returning VolumeCondition:** Always return VolumeCondition (even if healthy) when VOLUME_CONDITION capability is declared
- **Duplicate metrics systems:** Don't maintain both SecurityMetrics and Prometheus metrics separately - bridge them

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Prometheus exposition format | Custom text formatting | promhttp.Handler() | Handles all formatting, content negotiation, compression |
| Metric labels validation | Manual string checks | prometheus.CounterVec with label names | Validates labels at registration time |
| Thread-safe counters | sync.Mutex wrapping | prometheus.Counter | All Prometheus metrics are thread-safe |
| Histogram bucket calculations | Manual percentile math | prometheus.Histogram with DefBuckets | Standard buckets optimized for latency |
| K8s event deduplication | Manual event tracking | record.EventRecorder | Built-in aggregation and rate limiting |
| Metric registration | Global map tracking | prometheus.Registry | Handles duplicates, gathering, errors |

**Key insight:** Prometheus client_golang handles all the complexity of thread-safe metrics, exposition format, and HTTP serving. Focus on defining WHAT to measure, not HOW to expose it.

## Common Pitfalls

### Pitfall 1: High Cardinality Labels
**What goes wrong:** Using volumeID or podName as labels in counters/histograms causes metric explosion.
**Why it happens:** Each unique label combination creates a new time series.
**How to avoid:**
1. Use volumeID labels ONLY for gauges that track current state per volume
2. For counters/histograms, use categorical labels (operation type, error type, etc.)
3. Limit labels to < 10 unique values per dimension
**Warning signs:** Prometheus scrape times increasing, high memory usage in driver.

### Pitfall 2: Blocking CSI Operations on Event Posting
**What goes wrong:** CSI operation times out because event posting is slow or hangs.
**Why it happens:** Synchronous event posting when K8s API is slow or unreachable.
**How to avoid:**
1. EventPoster already handles this - returns nil on failure
2. Never add error returns that would block the main operation
3. Consider async event posting for high-volume scenarios
**Warning signs:** Mount timeouts correlating with K8s API latency.

### Pitfall 3: Missing VolumeCondition in Healthy State
**What goes wrong:** Kubelet logs errors about missing VolumeCondition when capability is declared.
**Why it happens:** Only returning VolumeCondition when volume is unhealthy.
**How to avoid:**
1. ALWAYS return VolumeCondition when VOLUME_CONDITION capability is declared
2. Use abnormal=false, message="Volume is healthy" for healthy volumes
3. Test NodeGetVolumeStats with healthy volumes
**Warning signs:** "volume condition not reported" warnings in Kubelet logs.

### Pitfall 4: Duplicate Metric Registration
**What goes wrong:** Panic on driver restart with "duplicate collector registration attempted".
**Why it happens:** Using prometheus.MustRegister() with DefaultRegisterer multiple times.
**How to avoid:**
1. Use a custom prometheus.Registry instead of DefaultRegisterer
2. Use prometheus.NewRegistry() per driver instance
3. Or use promauto.With(registry) for automatic registration
**Warning signs:** Panics during driver restart or hot reload.

### Pitfall 5: Not Exposing Metrics in Kubernetes
**What goes wrong:** Prometheus can't scrape metrics, observability is incomplete.
**Why it happens:** Missing Service, ServiceMonitor, or pod annotations.
**How to avoid:**
1. Add prometheus.io/scrape, prometheus.io/port annotations to pod spec
2. Create Service object exposing metrics port
3. Create ServiceMonitor if using Prometheus Operator
4. Document metrics port in deployment manifests
**Warning signs:** Missing metrics in Prometheus despite driver running.

### Pitfall 6: Inconsistent Metric Naming
**What goes wrong:** Metrics don't follow Prometheus naming conventions, hard to query.
**Why it happens:** Ad-hoc naming without following conventions.
**How to avoid:**
1. Use namespace prefix (rds_csi_)
2. Counters end in _total
3. Histograms end in _seconds, _bytes, etc.
4. Use snake_case for names
5. Follow https://prometheus.io/docs/practices/naming/
**Warning signs:** Inconsistent metric names, hard-to-write PromQL queries.

## Code Examples

Verified patterns from official sources:

### Complete Prometheus Metrics Setup
```go
// Source: https://prometheus.io/docs/guides/go-application/
// Source: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus
package observability

import (
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
    namespace = "rds_csi"
)

// Metrics holds all Prometheus metrics for the driver
type Metrics struct {
    registry *prometheus.Registry

    // Counters
    volumeOpsTotal     *prometheus.CounterVec
    nvmeConnectsTotal  *prometheus.CounterVec
    mountOpsTotal      *prometheus.CounterVec
    eventsPostedTotal  *prometheus.CounterVec
    staleMountsTotal   prometheus.Counter
    orphansCleanedTotal prometheus.Counter

    // Histograms
    volumeOpDuration *prometheus.HistogramVec
    nvmeConnectDuration prometheus.Histogram

    // Gauges
    activeConnections prometheus.Gauge
}

// NewMetrics creates and registers all metrics
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

        nvmeConnectsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "nvme_connects_total",
                Help:      "Total number of NVMe connection attempts by status",
            },
            []string{"status"},
        ),

        mountOpsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "mount_operations_total",
                Help:      "Total number of mount/unmount operations by type and status",
            },
            []string{"operation", "status"},
        ),

        eventsPostedTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "events_posted_total",
                Help:      "Total number of Kubernetes events posted by reason",
            },
            []string{"reason"},
        ),

        staleMountsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "stale_mounts_detected_total",
            Help:      "Total number of stale mounts detected",
        }),

        orphansCleanedTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "orphans_cleaned_total",
            Help:      "Total number of orphaned connections cleaned up",
        }),

        volumeOpDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Name:      "volume_operation_duration_seconds",
                Help:      "Duration of volume operations in seconds",
                Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
            },
            []string{"operation"},
        ),

        nvmeConnectDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Namespace: namespace,
            Name:      "nvme_connect_duration_seconds",
            Help:      "Duration of NVMe connection establishment in seconds",
            Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
        }),

        activeConnections: prometheus.NewGauge(prometheus.GaugeOpts{
            Namespace: namespace,
            Name:      "nvme_connections_active",
            Help:      "Number of currently active NVMe connections",
        }),
    }

    // Register all metrics
    reg.MustRegister(
        m.volumeOpsTotal,
        m.nvmeConnectsTotal,
        m.mountOpsTotal,
        m.eventsPostedTotal,
        m.staleMountsTotal,
        m.orphansCleanedTotal,
        m.volumeOpDuration,
        m.nvmeConnectDuration,
        m.activeConnections,
    )

    return m
}

// Handler returns HTTP handler for /metrics endpoint
func (m *Metrics) Handler() http.Handler {
    return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
        EnableOpenMetrics: true,
    })
}

// RecordVolumeOp records a volume operation with timing
func (m *Metrics) RecordVolumeOp(operation string, err error, duration time.Duration) {
    status := "success"
    if err != nil {
        status = "failure"
    }
    m.volumeOpsTotal.WithLabelValues(operation, status).Inc()
    m.volumeOpDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordNVMeConnect records an NVMe connection attempt
func (m *Metrics) RecordNVMeConnect(err error, duration time.Duration) {
    status := "success"
    if err != nil {
        status = "failure"
    }
    m.nvmeConnectsTotal.WithLabelValues(status).Inc()
    if err == nil {
        m.nvmeConnectDuration.Observe(duration.Seconds())
        m.activeConnections.Inc()
    }
}

// RecordNVMeDisconnect records an NVMe disconnection
func (m *Metrics) RecordNVMeDisconnect() {
    m.activeConnections.Dec()
}

// RecordStaleMountDetected records stale mount detection
func (m *Metrics) RecordStaleMountDetected() {
    m.staleMountsTotal.Inc()
}

// RecordOrphanCleaned records orphan cleanup
func (m *Metrics) RecordOrphanCleaned() {
    m.orphansCleanedTotal.Inc()
}

// RecordEventPosted records an event posted to Kubernetes
func (m *Metrics) RecordEventPosted(reason string) {
    m.eventsPostedTotal.WithLabelValues(reason).Inc()
}
```

### CSI Spec VolumeCondition Response
```go
// Source: https://github.com/container-storage-interface/spec/blob/master/csi.proto
// VolumeCondition fields:
//   abnormal (bool): REQUIRED - false=healthy, true=unhealthy
//   message (string): REQUIRED - Human-readable description

// Always return VolumeCondition when capability is declared
response := &csi.NodeGetVolumeStatsResponse{
    Usage: [...],
    VolumeCondition: &csi.VolumeCondition{
        Abnormal: false,  // or true if unhealthy
        Message:  "Volume is healthy",  // or describe the issue
    },
}
```

### Node Service Capability Declaration
```go
// Source: CSI spec NodeServiceCapability_RPC_Type enum
// https://github.com/container-storage-interface/spec/blob/master/csi.proto

// In driver.go initializeCapabilities():
nscaps := []*csi.NodeServiceCapability{
    {
        Type: &csi.NodeServiceCapability_Rpc{
            Rpc: &csi.NodeServiceCapability_RPC{
                Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
            },
        },
    },
    {
        Type: &csi.NodeServiceCapability_Rpc{
            Rpc: &csi.NodeServiceCapability_RPC{
                Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
            },
        },
    },
    {
        Type: &csi.NodeServiceCapability_Rpc{
            Rpc: &csi.NodeServiceCapability_RPC{
                Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
            },
        },
    },
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| No volume health reporting | VolumeCondition in NodeGetVolumeStats | CSI 1.3 (2020) | Kubelet can monitor volume health |
| Custom metrics formats | Prometheus OpenMetrics | 2019+ | Standardized scraping, Grafana integration |
| Separate metrics sidecars | Driver-native metrics endpoint | 2021+ | Simpler deployment, less resource overhead |
| Manual event posting | record.EventRecorder | Kubernetes 1.0+ | Built-in rate limiting, aggregation |

**Deprecated/outdated:**
- **Older CSI spec without VOLUME_CONDITION:** Ensure CSI spec v1.3+ is used (v1.10.0 in go.mod)
- **OpenCensus for metrics:** Migrate to Prometheus or OpenTelemetry
- **Events without structured data:** Use event annotations for machine-readable context

## Open Questions

Things that couldn't be fully resolved:

1. **Bridge existing SecurityMetrics to Prometheus or replace?**
   - What we know: SecurityMetrics tracks all operations internally, Prometheus needs exposition
   - What's unclear: Should we maintain both systems or replace SecurityMetrics?
   - Recommendation: Keep SecurityMetrics for internal tracking, add Prometheus metrics that read from it OR add Prometheus recording alongside existing tracking. Avoid duplicate counting.

2. **Metrics cardinality for volume_id in gauges**
   - What we know: Using volume_id in gauge labels enables per-volume health tracking
   - What's unclear: With many volumes, does this cause issues?
   - Recommendation: Use volume_id label for volumeHealthStatus gauge only, not for counters/histograms. Document maximum recommended volume count.

3. **VOLUME_CONDITION capability alpha status**
   - What we know: VOLUME_CONDITION is marked as alpha in CSI spec
   - What's unclear: Will behavior change in future CSI versions?
   - Recommendation: Implement as specified, monitor CSI spec updates. Alpha features are stable enough for production CSI drivers.

4. **Metrics port selection**
   - What we know: Different CSI drivers use different ports (8095, 9808, 9809)
   - What's unclear: Is there a standard port for CSI driver metrics?
   - Recommendation: Use 9809 (common for storage controllers), make configurable via flag.

## Sources

### Primary (HIGH confidence)
- [CSI Spec csi.proto](https://github.com/container-storage-interface/spec/blob/master/csi.proto) - VolumeCondition message definition, NodeServiceCapability enums
- [prometheus/client_golang](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus) - v1.23.2, Counter/Gauge/Histogram APIs
- [Prometheus Go Application Guide](https://prometheus.io/docs/guides/go-application/) - Standard instrumentation patterns
- [Kubernetes CSI Volume Health Monitoring](https://kubernetes-csi.github.io/docs/volume-health-monitor.html) - NodeGetVolumeStats integration with Kubelet

### Secondary (MEDIUM confidence)
- [csi-lib-utils/metrics](https://pkg.go.dev/github.com/kubernetes-csi/csi-lib-utils/metrics) - CSIMetricsManager patterns (optional use)
- [Secrets Store CSI Driver Metrics](https://secrets-store-csi-driver.sigs.k8s.io/topics/metrics) - Example metric names and labels
- [AWS EBS CSI Driver Metrics](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/docs/metrics.md) - Production CSI driver metrics patterns

### Tertiary (LOW confidence)
- WebSearch results on CSI driver metrics best practices - General patterns, not driver-specific

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - prometheus/client_golang is the de facto standard, CSI spec VolumeCondition is well-defined
- Architecture: HIGH - Patterns based on existing codebase (EventPoster, SecurityMetrics) and official CSI/Prometheus documentation
- Pitfalls: MEDIUM - Based on Prometheus best practices and CSI driver patterns, some need production validation

**Research date:** 2026-01-30
**Valid until:** 2026-03-01 (30 days - Prometheus client and CSI spec are stable)
