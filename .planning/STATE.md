# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-01-30)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 4 - Observability (COMPLETE)

## Current Position

Phase: 4 of 4 (Observability)
Plan: 5 of 5 complete (04-01, 04-02, 04-03, 04-04, 04-05)
Status: Phase 4 complete - All phases complete
Last activity: 2026-01-31 - Completed 04-05-PLAN.md

Progress: [██████████] 100% (17/17 plans complete)

## Performance Metrics

**Velocity:**
- Total plans completed: 17
- Average duration: 2.6 min
- Total execution time: 0.73 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 3 | 9 min | 3 min |
| 02-stale-mount-detection | 5 | 14 min | 2.8 min |
| 03-reconnection-resilience | 4 | 11 min | 2.75 min |
| 04-observability | 5 | 16 min | 3.2 min |

**Recent Trend:**
- Last 5 plans: 04-01 (1 min), 04-04 (1 min), 04-02 (3 min), 04-03 (5 min), 04-05 (6 min)
- Trend: All phases complete - project milestone achieved

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Research best practices first (Limited testing ability, need high confidence)
- 10s default TTL for DeviceResolver cache (balances freshness vs overhead)
- Prefer nvmeXnY device format over nvmeXcYnZ (multipath compatibility)
- Dependency injection for testability (isConnectedFn allows orphan detection without circular dependency)
- Orphan = appears connected in nvme list-subsys but no device in sysfs
- Mock filesystem using t.TempDir() for sysfs simulation without root access
- Cannot test nvmeXcYnZ fallback path without real /dev devices (documented limitation)
- Parse /proc/mountinfo directly instead of external library (avoid dependency for simple parsing)
- Refuse force unmount if mount is in use (prevents data loss)
- 10s wait for normal unmount before escalating to lazy (per CONTEXT.md)
- Use consistent event reasons for filtering (EventReasonMountFailure, EventReasonRecoveryFailed, EventReasonStaleMountDetected)
- Warning events for failures, Normal for informational (distinguishes actionable vs context)
- Don't fail operations if PVC lookup fails (event posting is best-effort)
- EventSink adapter for context API mismatch (client-go v0.28 EventInterface requires context)
- Three stale conditions: mount not found, device disappeared, device path mismatch
- Exponential backoff between recovery attempts (1s, 2s, 4s)
- Default 3 recovery attempts before giving up
- Refuse recovery if mount is in use (prevents data loss)
- Symlink resolution for device path comparison (filepath.EvalSymlinks)
- NodePublishVolume checks and recovers stale mounts before bind mount
- NodeGetVolumeStats reports abnormal VolumeCondition on stale (no recovery)
- VolumeCondition always returned in NodeGetVolumeStats, even for healthy volumes
- Health check errors report Abnormal=false with "Health check inconclusive" message
- GetResolver() method on Connector interface for accessing DeviceResolver
- Skip integration tests on macOS using testing.Short() (no /proc/self/mountinfo)
- Use t.TempDir() for mock filesystems (auto-cleanup, no manual deletion)
- Mock interfaces for unit testing (Mounter, fake Kubernetes client)
- Accept fake client namespace quirks in event posting tests
- ctrl_loss_tmo=-1 default prevents filesystem read-only mount after timeout
- 5 second reconnect_delay default balances responsiveness and target load
- 10% jitter via wait.Backoff.Jitter prevents thundering herd on mass reconnection
- IsRetryableError string matching for broad transient error coverage
- ConnectWithContext delegates to ConnectWithConfig for backward compatibility
- ConnectWithRetry uses utils.RetryWithBackoff with DefaultBackoffConfig
- Both existing and new volume paths include connection parameters in VolumeContext
- Orphan cleanup failures logged as warnings, don't block startup (best-effort cleanup)
- Create fresh connector instance for orphan cleanup (driver creates node server internally)
- Use /sys/class/nvme-subsystem for NQN enumeration (more reliable than parsing nvme-cli output)
- Table-driven tests with expectedArgs/unexpectedArgs for BuildConnectArgs verification
- testBackoffConfig() returns 1ms delays for fast test execution
- MockConnector tracks DisconnectWithContext calls for test verification
- testableOrphanCleaner wrapper controls resolver behavior in tests
- Orphan events use structured logging instead of K8s events (no PVC available)
- Connection failure events include target address for debugging
- Connection recovery events include attempt count for metrics
- Use custom prometheus.Registry instead of DefaultRegistry to avoid restart panics
- All Prometheus metrics use rds_csi_ namespace prefix
- Labels are low-cardinality (operation, status) to avoid metric explosion
- Port 9809 for metrics server (common storage controller port)
- Named return values for defer-based metrics recording
- Setter injection (SetPromMetrics, SetMetrics) for optional metrics support
- SetMountDeviceFunc added to StaleMountChecker for cross-package test injection
- Device mismatch test adapted for macOS (tests inconclusive path)

### Pending Todos

None - all phases complete.

### Blockers/Concerns

None - project implementation complete.

## Session Continuity

Last session: 2026-01-31
Stopped at: Completed 04-05-PLAN.md - Observability unit tests (Prometheus + VolumeCondition)
Resume file: None

---
*State initialized: 2026-01-30*
*Last updated: 2026-01-31 - Phase 4 complete, all 17 plans executed successfully*
