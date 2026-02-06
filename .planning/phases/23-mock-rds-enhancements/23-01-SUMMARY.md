---
phase: 23-mock-rds-enhancements
plan: 01
subsystem: testing
tags: [mock-server, ssh, timing-simulation, error-injection, environment-config]

# Dependency graph
requires:
  - phase: 22-csi-sanity
    provides: CSI sanity test suite with in-process driver testing
provides:
  - Environment-based configuration system for mock RDS behavior
  - Timing simulation with SSH latency (150-250ms jitter) and operation delays
  - Error injection for disk_full, ssh_timeout, and command_fail scenarios
  - Backward-compatible mock (fast by default, realistic timing opt-in)
affects: [24-node-idempotency, 25-e2e-testing, mock-testing]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Environment variable-based configuration for test behavior"
    - "Layered error injection at operation boundaries (SSH, command, execution)"
    - "Timing simulation with jitter for realistic latency testing"

key-files:
  created:
    - test/mock/config.go
    - test/mock/timing.go
    - test/mock/error_injection.go
  modified:
    - test/mock/rds_server.go

key-decisions:
  - "Fast tests by default (no timing delays) with MOCK_RDS_REALISTIC_TIMING=true opt-in"
  - "SSH latency 200ms with ±50ms jitter (150-250ms range) to catch timeout issues"
  - "Error injection at operation start (before processing) to test driver error handling"
  - "Thread-safe operation counter with mutex for concurrent error injection"

patterns-established:
  - "Environment variable configuration pattern: MOCK_RDS_* prefix for all settings"
  - "Error injection modes: none, disk_full, ssh_timeout, command_fail"
  - "Timing simulator with per-operation type delays (SSH, disk add, disk remove)"

# Metrics
duration: 3min
completed: 2026-02-04
---

# Phase 23 Plan 01: Mock RDS Enhancements Summary

**Environment-configurable mock RDS with timing simulation (150-250ms SSH latency) and error injection for disk_full, ssh_timeout, and command_fail scenarios**

## Performance

- **Duration:** 3 minutes
- **Started:** 2026-02-04T22:41:17Z
- **Completed:** 2026-02-04T22:44:16Z
- **Tasks:** 3
- **Files modified:** 4 (3 created, 1 modified)

## Accomplishments
- Environment-based configuration system enables test-specific mock behavior without code changes
- Timing simulation with configurable SSH latency (200ms ± 50ms jitter) and disk operation delays
- Error injection framework supports disk_full, ssh_timeout, and command_fail modes
- Backward compatible: fast tests by default, realistic timing only when explicitly enabled
- Thread-safe operation counter with mutex protection for concurrent testing

## Task Commits

Each task was committed atomically:

1. **Task 1: Create configuration and timing simulation infrastructure** - `29584b4` (feat)
2. **Task 2: Create error injection infrastructure** - `989f260` (feat)
3. **Task 3: Integrate configuration, timing, and error injection into rds_server.go** - `bb701f5` (feat)

## Files Created/Modified

### Created
- `test/mock/config.go` - MockRDSConfig struct with environment variable loading (timing, error injection, observability settings)
- `test/mock/timing.go` - TimingSimulator with SSH latency simulation (jitter-based) and disk operation delays
- `test/mock/error_injection.go` - ErrorInjector with layered error injection (SSH connect, disk add/remove)

### Modified
- `test/mock/rds_server.go` - Integration of config, timing, and error injection into server lifecycle

## Decisions Made

1. **Fast tests by default**: Timing simulation disabled by default to avoid slowing down CI. Enable with `MOCK_RDS_REALISTIC_TIMING=true` only when testing timeout behavior.

2. **SSH latency with jitter**: Base 200ms latency with ±50ms jitter (150-250ms range) to expose timeout bugs consistently. Jitter prevents test flakiness from fixed timing assumptions.

3. **Error injection at operation start**: Check error injection before normal processing (not after) to test driver error handling without side effects. Enables testing retry logic and error propagation.

4. **Thread-safe operation counter**: Mutex-protected counter allows concurrent error injection testing. Tests can verify behavior under concurrent CreateVolume/DeleteVolume operations.

5. **History depth configuration**: Configurable history tracking (default 100 entries) with ability to disable for performance-critical tests.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation proceeded smoothly with clear requirements from CONTEXT.md and RESEARCH.md.

## Environment Variables

### Timing Control
- `MOCK_RDS_REALISTIC_TIMING` - Enable timing simulation (default: false)
- `MOCK_RDS_SSH_LATENCY_MS` - SSH latency in ms (default: 200)
- `MOCK_RDS_SSH_LATENCY_JITTER_MS` - Latency jitter in ms (default: 50)
- `MOCK_RDS_DISK_ADD_DELAY_MS` - Disk add delay in ms (default: 500)
- `MOCK_RDS_DISK_REMOVE_DELAY_MS` - Disk remove delay in ms (default: 300)

### Error Injection
- `MOCK_RDS_ERROR_MODE` - Error mode: none, disk_full, ssh_timeout, command_fail (default: none)
- `MOCK_RDS_ERROR_AFTER_N` - Fail after N operations (default: 0 = immediate)

### Observability
- `MOCK_RDS_ENABLE_HISTORY` - Enable command history (default: true)
- `MOCK_RDS_HISTORY_DEPTH` - Max history entries (default: 100)
- `MOCK_RDS_ROUTEROS_VERSION` - RouterOS version to simulate (default: "7.16")

## Verification

- **Compilation**: All files compile without errors
- **Backward compatibility**: Existing sanity tests pass without environment variables (18 passed controller tests)
- **Error injection**: `MOCK_RDS_ERROR_MODE=disk_full` causes CreateVolume to fail with "not enough space" error and driver retries correctly
- **Timing simulation**: Configurable delays work as expected (verified via implementation, timing tests deferred to Phase 24)

## Next Phase Readiness

**Ready for Phase 23-02 (RouterOS Output Format Validation)**

The mock RDS now supports realistic timing and error injection, enabling:
- Timeout testing (Phase 24 node idempotency with NVMe connection timeouts)
- Error handling validation (disk full, SSH failures, command failures)
- RouterOS version compatibility testing (Phase 23-02 will add version-specific output formatting)

**No blockers.** Configuration system is extensible for additional mock enhancements.

---
*Phase: 23-mock-rds-enhancements*
*Completed: 2026-02-04*
