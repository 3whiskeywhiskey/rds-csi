---
phase: 03-reconnection-resilience
plan: 02
subsystem: nvme-connector
tags: [nvme, connection-config, retry, backoff, volume-context]
dependency-graph:
  requires: [03-01]
  provides: [ConnectWithConfig, ConnectWithRetry, VolumeContext-params]
  affects: [03-03, 03-04]
tech-stack:
  added: []
  patterns: [delegation, exponential-backoff]
key-files:
  created: []
  modified:
    - pkg/nvme/nvme.go
    - pkg/driver/controller.go
decisions:
  - ConnectWithContext delegates to ConnectWithConfig for backward compatibility
  - ConnectWithRetry uses utils.RetryWithBackoff with DefaultBackoffConfig
  - Both existing and new volume paths include connection parameters in VolumeContext
metrics:
  duration: 2 min
  completed: 2026-01-30
---

# Phase 3 Plan 2: NVMe Connector Integration Summary

**One-liner:** ConnectWithConfig and ConnectWithRetry methods integrate connection parameters with exponential backoff retry into NVMe connector, flowing through VolumeContext from CreateVolume.

## What Was Built

### NVMe Connector Interface Extensions

Added two new methods to `Connector` interface in `pkg/nvme/nvme.go`:

1. **ConnectWithConfig** - Accepts `ConnectionConfig` and builds nvme connect command with resilience parameters:
   - Uses `BuildConnectArgs(target, config)` from config.go
   - Includes `-l` (ctrl_loss_tmo), `-c` (reconnect_delay), `-k` (keep_alive_tmo) flags
   - Full operation tracking, metrics, orphan detection preserved

2. **ConnectWithRetry** - Wraps ConnectWithConfig with exponential backoff:
   - Uses `utils.RetryWithBackoff` with `DefaultBackoffConfig()`
   - 5 attempts, 1s initial delay, 2x factor, 10% jitter
   - Logs retry attempts for debugging
   - Returns wrapped error on exhaustion

### Backward Compatibility

`ConnectWithContext` now delegates to `ConnectWithConfig` with `DefaultConnectionConfig()`:
```go
func (c *connector) ConnectWithContext(ctx context.Context, target Target) (string, error) {
    return c.ConnectWithConfig(ctx, target, DefaultConnectionConfig())
}
```

### Controller VolumeContext Enhancement

`CreateVolume` in `pkg/driver/controller.go` now:

1. Parses NVMe connection parameters from StorageClass via `ParseNVMEConnectionParams(params)`
2. Returns `InvalidArgument` error for invalid parameter values
3. Includes connection parameters in VolumeContext for both:
   - Existing volume responses (idempotency path)
   - New volume responses (creation path)

VolumeContext now includes:
- `ctrlLossTmo` - Controller loss timeout (default: -1 unlimited)
- `reconnectDelay` - Reconnect delay seconds (default: 5)
- `keepAliveTmo` - Keep-alive timeout (default: 0 kernel default)

## Key Links

| From | To | Via |
|------|-----|-----|
| nvme.go | config.go | `BuildConnectArgs(target, config)` |
| nvme.go | retry.go | `utils.RetryWithBackoff(ctx, backoff, fn)` |
| controller.go | params.go | `ParseNVMEConnectionParams(params)` |

## Deviations from Plan

None - plan executed exactly as written.

## Commits

| Hash | Type | Description |
|------|------|-------------|
| a44855a | feat | Integrate connection parameters into NVMe connector and controller |

## Verification Results

- `go build ./...` - Pass
- `go test ./pkg/nvme/...` - Pass (all 30+ tests)
- `go test ./pkg/driver/...` - Pass (all 25+ tests)
- `go vet ./pkg/nvme/... ./pkg/driver/...` - Pass
- Connector interface includes ConnectWithConfig and ConnectWithRetry
- VolumeContext includes ctrlLossTmo, reconnectDelay, keepAliveTmo
- ConnectWithRetry uses RetryWithBackoff with DefaultBackoffConfig

## Next Phase Readiness

**Ready for 03-03:** NodeStageVolume integration
- ConnectWithConfig available for node plugin to use
- VolumeContext carries connection parameters from controller
- ConnectWithRetry available for resilient connection attempts
- All building blocks in place for node-side integration
