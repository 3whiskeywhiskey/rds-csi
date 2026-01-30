---
phase: 02-stale-mount-detection
plan: 03
subsystem: mount
tags: [nvme, mount, recovery, stale-detection, exponential-backoff]

# Dependency graph
requires:
  - phase: 02-01
    provides: DeviceResolver for NQN-to-device resolution, /proc/mountinfo parsing
provides:
  - StaleMountChecker for detecting stale mounts (mount not found, device disappeared, device mismatch)
  - MountRecoverer for automatic recovery with exponential backoff
  - StaleReason enum for distinguishing stale conditions
  - RecoveryResult tracking for recovery attempts
affects: [02-stale-mount-detection, driver-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Stale detection via device path comparison (mount device vs NQN-resolved device)"
    - "Exponential backoff retry (1s, 2s, 4s) with MaxAttempts limit"
    - "Context-aware recovery with cancellation support"
    - "Refuse recovery if mount is in use (data loss prevention)"

key-files:
  created:
    - pkg/mount/stale.go
    - pkg/mount/recovery.go
  modified: []

key-decisions:
  - "Three stale conditions: mount not found, device disappeared, device path mismatch"
  - "Exponential backoff between recovery attempts (1s, 2s, 4s)"
  - "Default 3 recovery attempts before giving up"
  - "Refuse recovery if mount is in use (prevents data loss)"
  - "GetStaleInfo helper for debugging with full device path details"

patterns-established:
  - "Symlink resolution for device path comparison (filepath.EvalSymlinks)"
  - "RecoveryResult struct for tracking recovery outcome, attempts, device paths"
  - "Context cancellation between retry attempts"

# Metrics
duration: 1min
completed: 2026-01-30
---

# Phase 2 Plan 3: Stale Mount Detection and Recovery Summary

**Stale mount detection via device path comparison and automatic recovery with exponential backoff retry**

## Performance

- **Duration:** 1 min
- **Started:** 2026-01-30T20:46:45Z
- **Completed:** 2026-01-30T20:48:11Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- StaleMountChecker detects three stale conditions: mount not found, device disappeared, device path mismatch
- MountRecoverer handles automatic unmount/remount with exponential backoff (1s, 2s, 4s)
- Recovery respects context cancellation and refuses to proceed if mount is in use
- StaleInfo provides detailed debugging information with old/new device paths

## Task Commits

Each task was committed atomically:

1. **Task 1: Create StaleMountChecker for stale mount detection** - `a796aa2` (feat)
2. **Task 2: Create MountRecoverer with retry logic** - `aa3d5cd` (feat)

## Files Created/Modified

- `pkg/mount/stale.go` - Detects stale mounts by comparing mount device with NQN-resolved device
- `pkg/mount/recovery.go` - Automatic mount recovery with exponential backoff and retry logic

## Decisions Made

- **Three stale conditions:** mount not found (mount disappeared from /proc/mountinfo), device disappeared (device path no longer exists), device path mismatch (mount device differs from NQN-resolved device)
- **Exponential backoff:** 1s, 2s, 4s between recovery attempts (default InitialBackoff 1s, BackoffMultiplier 2.0)
- **MaxAttempts:** Default 3 recovery attempts before giving up
- **In-use protection:** Refuse recovery if mount is in use by checking IsMountInUse before forcing unmount
- **Symlink resolution:** Use filepath.EvalSymlinks for accurate device path comparison (handles /dev/disk/by-id/* symlinks)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Next Phase Readiness

- Stale detection and recovery logic complete
- Ready for integration into CSI node operations (NodePublishVolume, NodeUnpublishVolume, NodeGetVolumeStats)
- Event posting from 02-02 enables recovery result reporting to users
- Recovery handles all three stale conditions automatically

**Blockers:** None

**Concerns:** None - recovery respects in-use mounts and context cancellation

---
*Phase: 02-stale-mount-detection*
*Completed: 2026-01-30*
