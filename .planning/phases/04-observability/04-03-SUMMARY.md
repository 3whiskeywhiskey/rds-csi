---
phase: "04"
plan: "03"
subsystem: observability
tags: [prometheus, metrics, http-server, instrumentation]
dependency-graph:
  requires: ["04-02"]
  provides: ["metrics-server", "operation-instrumentation"]
  affects: ["04-05"]
tech-stack:
  added: ["net/http"]
  patterns: ["metrics-server", "defer-instrumentation", "setter-injection"]
key-files:
  created: []
  modified:
    - cmd/rds-csi-plugin/main.go
    - pkg/driver/driver.go
    - pkg/driver/node.go
    - pkg/nvme/nvme.go
    - pkg/mount/recovery.go
    - pkg/nvme/orphan_test.go
decisions:
  - key: metrics-address-default
    choice: ":9809"
    reason: "Common port for storage controllers per RESEARCH.md"
  - key: named-return-for-defer
    choice: "Use named return values for metrics defer"
    reason: "Required to capture err in defer for RecordVolumeOp/RecordNVMeConnect"
  - key: setter-injection
    choice: "SetPromMetrics() and SetMetrics() setter methods"
    reason: "Avoid breaking existing constructors, optional metrics support"
metrics:
  duration: "5 min"
  completed: "2026-01-31"
---

# Phase 04 Plan 03: HTTP Metrics Server and CSI Instrumentation Summary

HTTP metrics server exposing Prometheus metrics with full CSI operation instrumentation.

## What Was Built

### 1. Metrics Flag and HTTP Server (main.go)
- Added `--metrics-address` flag with default `:9809`
- HTTP server starts in goroutine after driver creation
- Serves `/metrics` endpoint using observability.Metrics.Handler()
- Logs metrics status at startup

### 2. Driver Integration (driver.go)
- Added `Metrics *observability.Metrics` to DriverConfig struct
- Added `metrics *observability.Metrics` to Driver struct
- Added `GetMetrics()` method for accessing metrics instance
- Stored metrics from config in NewDriver()

### 3. Node Operation Instrumentation (node.go)
- NodeStageVolume records timing and success/failure via defer
- NodeUnstageVolume records timing and success/failure via defer
- NodeGetVolumeStats records stale mount detection
- Connector and Recoverer receive metrics via setter injection

### 4. NVMe Connection Instrumentation (nvme.go)
- Added `promMetrics *observability.Metrics` to connector struct
- Added `SetPromMetrics()` method to Connector interface
- ConnectWithConfig records NVMe connect timing and result
- DisconnectWithContext records disconnect on success

### 5. Recovery Instrumentation (recovery.go)
- Added `metrics *observability.Metrics` to MountRecoverer
- Added `SetMetrics()` setter method
- Recover() records success/failure of stale mount recovery

## Key Implementation Details

### Defer Pattern for Metrics
Used named return values to enable defer capturing the error:
```go
func (ns *NodeServer) NodeStageVolume(...) (resp *csi.NodeStageVolumeResponse, err error) {
    metricsStart := time.Now()
    defer func() {
        if ns.driver.metrics != nil {
            ns.driver.metrics.RecordVolumeOp("stage", err, time.Since(metricsStart))
        }
    }()
    // ...
}
```

### Setter Injection for Optional Metrics
Avoids breaking existing constructors:
```go
// In NewNodeServer
if driver.metrics != nil {
    connector.SetPromMetrics(driver.metrics)
    recoverer.SetMetrics(driver.metrics)
}
```

### Nil-Check Guards
All metrics recording is guarded:
```go
if ns.driver.metrics != nil {
    ns.driver.metrics.RecordStaleMountDetected()
}
```

## Files Modified

| File | Changes |
|------|---------|
| cmd/rds-csi-plugin/main.go | Added metrics flag, server startup, config integration |
| pkg/driver/driver.go | Added Metrics field to config/struct, GetMetrics() method |
| pkg/driver/node.go | Instrumented stage/unstage, stale detection, metrics injection |
| pkg/nvme/nvme.go | Added promMetrics field, SetPromMetrics(), recording in connect/disconnect |
| pkg/mount/recovery.go | Added metrics field, SetMetrics(), recording in Recover() |
| pkg/nvme/orphan_test.go | Added SetPromMetrics() to MockConnector for interface compliance |

## Commits

| Hash | Message |
|------|---------|
| d30bd2c | feat(04-03): add metrics flag and HTTP server, integrate into driver |
| e876b01 | feat(04-03): instrument CSI operations with Prometheus metrics |

## Deviations from Plan

None - plan executed exactly as written.

## Verification

```bash
# Build passes
go build ./...

# Driver tests pass
go test ./pkg/driver/... -v

# Metrics flag exists
grep -n "metrics-address" cmd/rds-csi-plugin/main.go
# 52:	metricsAddr = flag.String("metrics-address", ":9809", ...)

# HTTP server configured
grep -n "ListenAndServe" cmd/rds-csi-plugin/main.go
# 179:	if err := http.ListenAndServe(*metricsAddr, mux); ...

# Instrumentation in place
grep -rn "RecordVolumeOp\|RecordNVMeConnect" pkg/driver/ pkg/nvme/
# pkg/driver/node.go:102: RecordVolumeOp("stage", ...)
# pkg/driver/node.go:254: RecordVolumeOp("unstage", ...)
# pkg/nvme/nvme.go:522: RecordNVMeConnect(err, duration)
```

## Next Phase Readiness

Plan 04-05 (metrics package tests) can now verify:
- RecordVolumeOp increments counters and records histograms
- RecordNVMeConnect tracks connection attempts
- RecordStaleMountDetected and RecordStaleRecovery work correctly
- Handler() returns proper Prometheus exposition format
