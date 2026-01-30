---
phase: 02-stale-mount-detection
plan: 01
subsystem: mount
tags: [proc-mounts, lazy-unmount, force-unmount, process-detection]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: Basic mount utilities (Mounter interface)
provides:
  - /proc/mountinfo parsing for device path resolution
  - Force unmount with in-use detection
  - Process fd scanning to detect mount usage
affects: [02-02-detection, 02-03-recovery]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - /proc filesystem parsing for system state introspection
    - Escalating unmount strategy (normal → wait → lazy)
    - Process fd scanning for in-use detection

key-files:
  created:
    - pkg/mount/procmounts.go
  modified:
    - pkg/mount/mount.go

key-decisions:
  - "Parse /proc/self/mountinfo directly instead of using external library (avoid dependency for simple parsing)"
  - "Refuse force unmount if mount is in use (prevents data loss)"
  - "10 second wait for normal unmount before escalating to lazy (per CONTEXT.md)"
  - "Scan /proc/*/fd for open file handles to detect mount usage"

patterns-established:
  - "Handle escaped characters in mountinfo paths (\040 for space)"
  - "Canonical path resolution with filepath.EvalSymlinks for comparing device paths"
  - "Skip permission denied errors when scanning /proc (expected for other users' processes)"

# Metrics
duration: 2min
completed: 2026-01-30
---

# Phase 2 Plan 1: Mount Infrastructure Summary

**/proc/mountinfo parsing, force unmount with lazy escalation, and process-based in-use detection for stale mount recovery**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-30T21:13:08Z
- **Completed:** 2026-01-30T21:15:33Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Implemented /proc/self/mountinfo parsing to get mount source device from mount path
- Added ForceUnmount with timeout and lazy unmount escalation
- Implemented IsMountInUse by scanning /proc/*/fd for open file handles
- Refuses to force unmount in-use mounts to prevent data loss

## Task Commits

Each task was committed atomically:

1. **Task 1: Create /proc/mountinfo parsing utilities** - `6c8291c` (feat)
2. **Task 2: Add ForceUnmount and IsMountInUse to Mounter** - `fa9870a` (feat)

## Files Created/Modified
- `pkg/mount/procmounts.go` - Parse /proc/self/mountinfo, get mount device/info by path, handle escaped characters
- `pkg/mount/mount.go` - Extended Mounter interface with ForceUnmount and IsMountInUse methods

## Decisions Made

**Parse /proc/mountinfo directly:**
- Rationale: Simple parsing logic doesn't justify external dependency
- Used moby/sys/mountinfo pattern from research as reference
- Handles escaped characters (\040 for spaces, etc.)

**Refuse force unmount when in use:**
- Scans /proc/*/fd to detect processes with open handles
- Returns error if mount is in use
- Prevents data loss at cost of recovery failure

**10 second timeout before lazy unmount:**
- Per CONTEXT.md decision (NormalUnmountWait: 10 seconds)
- Polls IsLikelyMountPoint every 500ms
- Escalates to umount -l only after timeout

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation was straightforward.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

**Ready for:**
- Plan 02-02: StaleMountChecker implementation can use GetMountDevice to detect device path mismatches
- Plan 02-03: MountRecoverer can use ForceUnmount for recovery with in-use protection

**Provides foundation for:**
- Detecting stale mounts by comparing mount device to NQN-resolved device
- Safe recovery via force unmount when mount is not in use
- Refusing recovery when mount has active file handles

**No blockers or concerns.**

---
*Phase: 02-stale-mount-detection*
*Completed: 2026-01-30*
