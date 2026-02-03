---
phase: 14-error-resilience-mount-storm-prevention
plan: 02
subsystem: mount
tags: [mount-storm, timeout, procmounts, moby, mountinfo, error-resilience]

# Dependency graph
requires:
  - phase: 14-01
    provides: NQN prefix filtering for orphan cleaner
provides:
  - Safe procmounts parsing with 10s timeout protection
  - Mount storm detection with 100 mount threshold
  - Production-ready mount parsing via moby/sys/mountinfo
affects: [14-03-stale-mount-detection, driver-node-service]

# Tech tracking
tech-stack:
  added: [github.com/moby/sys/mountinfo v0.7.2]
  patterns: [context-based timeout for system operations, threshold-based mount storm detection]

key-files:
  created: []
  modified: [pkg/mount/procmounts.go, pkg/mount/procmounts_test.go]

key-decisions:
  - "Use moby/sys/mountinfo for production-ready mount parsing (Docker/containerd standard)"
  - "10 second timeout for procmounts parsing prevents hangs"
  - "100 mount threshold for duplicate detection catches mount storms"
  - "Deprecate GetMounts in favor of GetMountsWithTimeout"

patterns-established:
  - "Context-based timeout pattern for filesystem operations"
  - "Actionable error messages with remediation steps"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 14 Plan 02: Safe Procmounts Parsing Summary

**Timeout-protected mount parsing with storm detection prevents driver hangs during filesystem corruption**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-03T18:49:28Z
- **Completed:** 2026-02-03T18:52:04Z
- **Tasks:** 3
- **Files modified:** 4 (go.mod, go.sum, pkg/mount/procmounts.go, pkg/mount/procmounts_test.go)

## Accomplishments
- Added moby/sys/mountinfo dependency for production-ready mount parsing
- Implemented GetMountsWithTimeout with 10s timeout protection
- Implemented DetectDuplicateMounts with 100 mount threshold for storm detection
- Added 8 comprehensive unit tests covering success, timeout, threshold, and edge cases
- Deprecated GetMounts in favor of timeout-protected version

## Task Commits

Each task was committed atomically:

1. **Task 1: Add moby/sys/mountinfo dependency** - `784389a` (chore)
2. **Task 2: Implement GetMountsWithTimeout and DetectDuplicateMounts** - `6eee582` (feat)
3. **Task 3: Add tests for timeout and duplicate detection** - `fb73691` (test)

## Files Created/Modified
- `go.mod` - Added github.com/moby/sys/mountinfo v0.7.2 dependency
- `go.sum` - Updated checksums for new dependency
- `pkg/mount/procmounts.go` - Added GetMountsWithTimeout, DetectDuplicateMounts, ConvertMobyMount; deprecated GetMounts
- `pkg/mount/procmounts_test.go` - Added 8 comprehensive tests for new functionality

## Decisions Made

1. **Use moby/sys/mountinfo library**: Production-ready mount parsing used by Docker and containerd. Handles edge cases (spaces in paths, special characters, format variations) that manual parsing may miss.

2. **10 second timeout threshold**: Balances detection speed vs. normal system variance. Large systems with thousands of mounts should parse in <1s. 10s catches hangs without false positives.

3. **100 mount threshold for storm detection**: During Phase 13, corrupted filesystem caused 1000+ duplicate mounts. Threshold of 100 provides early warning while avoiding false positives from legitimate multi-mount scenarios.

4. **Deprecate GetMounts**: Mark old function as deprecated but keep for backward compatibility. New code should use GetMountsWithTimeout.

5. **Actionable error messages**: Both timeout and storm detection errors include remediation steps (check /proc/mounts manually, use findmnt/umount for cleanup).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

**moby/sys/mountinfo.Info.Options field type mismatch**: Initial implementation assumed Options was []string (as in test plan), but actual type is string. Fixed by removing strings.Join call in ConvertMobyMount. Caught immediately by compiler.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

**Ready for Plan 14-03 (Stale mount detection integration):**
- GetMountsWithTimeout provides safe mount parsing for stale detection
- DetectDuplicateMounts can be integrated into mount health checks
- ConvertMobyMount enables compatibility between moby and legacy MountInfo types

**No blockers.**

**Integration points for driver:**
- NodeStageVolume/NodeUnstageVolume should use GetMountsWithTimeout
- Mount health checks should call DetectDuplicateMounts before operations
- Error handling should surface timeout/storm errors to kubelet

---
*Phase: 14-error-resilience-mount-storm-prevention*
*Completed: 2026-02-03*
