---
phase: quick
plan: 006
subsystem: storage
tags: [blkid, nvme-tcp, filesystem, data-safety, race-condition]

# Dependency graph
requires:
  - phase: node-service
    provides: NodeStageVolume implementation with Format/IsFormatted calls
provides:
  - Safe blkid exit code handling preventing data loss on NVMe-oF reconnect
  - Retry logic for transient device errors after NVMe connect
  - Format refusal when device state cannot be determined
affects: [all volume mounting, NVMe-oF reconnect scenarios, PXE boot recovery]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Exit code distinction for blkid (2=no fs, 1=device error)"
    - "Retry-with-backoff pattern for transient device errors"
    - "Fail-safe principle: refuse to format when uncertain"

key-files:
  created: []
  modified:
    - pkg/mount/mount.go
    - pkg/mount/mount_test.go
    - pkg/driver/node.go
    - pkg/driver/node_test.go

key-decisions:
  - "blkid exit 1 treated as error (not 'not formatted') to prevent data loss"
  - "NodeStageVolume retries IsFormatted 5 times with 2s delay for transient errors"
  - "Format refuses to run mkfs when IsFormatted returns any error"
  - "Context cancellation respected during retry loop"

patterns-established:
  - "exec.ExitError type assertion for proper exit code parsing"
  - "Retry loop with context cancellation support"
  - "Fail-safe error handling: when in doubt, return error rather than proceed"

# Metrics
duration: 6min
completed: 2026-02-12
---

# Quick Task 006: Fix blkid Race Condition Causing Data Loss

**Prevented data loss on NVMe-oF reconnect by distinguishing blkid exit codes and adding retry logic for transient device errors**

## Performance

- **Duration:** 5m 53s
- **Started:** 2026-02-12T21:12:45Z
- **Completed:** 2026-02-12T21:18:38Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Fixed critical data loss bug where blkid exit 1 (device error) was treated as "not formatted"
- Implemented retry logic in NodeStageVolume for transient device errors after NVMe connect
- Added comprehensive test coverage for exit code handling and retry behavior
- All verification checks pass (fmt + vet + lint + test)

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix IsFormatted exit code handling and Format safety** - `559ee20` (fix)
   - IsFormatted now uses exec.ExitError to parse actual exit codes
   - Exit 2 (no filesystem) returns (false, nil) - safe to format
   - Exit 1 (device error) returns (false, error) - CRITICAL: prevents formatting
   - Format adds audit logging before mkfs
   - Added tests for exit 1, exit 3, and Format refusal on blkid error

2. **Task 2: Add retry logic in NodeStageVolume for transient device errors** - `083cca8` (fix)
   - NodeStageVolume retries IsFormatted up to 5 times with 2s delay
   - Returns error if device remains unreadable after all retries
   - Respects context cancellation during retry loop
   - mockMounter updated to support configurable IsFormatted behavior
   - Added TestNodeStageVolume_IsFormattedRetry to verify retry logic

## Files Created/Modified

- `pkg/mount/mount.go` - IsFormatted exit code parsing, Format audit logging
- `pkg/mount/mount_test.go` - Test cases for exit 1, exit 3, Format refusal
- `pkg/driver/node.go` - Retry loop for IsFormatted in NodeStageVolume
- `pkg/driver/node_test.go` - TestNodeStageVolume_IsFormattedRetry, mockMounter fields

## Decisions Made

1. **Exit code distinction via exec.ExitError:** String matching on error messages is fragile. Type assertion to exec.ExitError provides reliable exit code extraction.

2. **5 retries with 2s delay:** After NVMe-oF connect, device may need ~8-10 seconds to become ready for I/O. 5 retries × 2s = 10s max wait is sufficient for most reconnect scenarios.

3. **Fail-safe principle:** When device state cannot be determined after all retries, return error rather than proceeding to format. Data preservation takes priority over availability.

4. **Context cancellation support:** Retry loop checks context.Done() to prevent indefinite blocking when parent context is cancelled.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation proceeded smoothly with all tests passing on first attempt after code compilation fixes.

## Technical Context

### The Bug

On NVMe-oF reconnect after PXE boot, the device may not be immediately ready for I/O operations. When NodeStageVolume calls IsFormatted, blkid fails with exit status 1 (device error: "cannot read device"). The old implementation treated exit 1 identically to exit 2 (no filesystem found), returning (false, nil). This caused Format() to run mkfs.ext4 on the device, destroying all existing data.

### The Fix

**Part 1: Exit Code Distinction (Task 1)**
- Exit 0 with output → formatted, return (true, nil)
- Exit 2 → no filesystem, return (false, nil) - safe to format
- Exit 1 → device error, return (false, error) - NEVER format
- Other exits → return (false, error)

**Part 2: Retry Logic (Task 2)**
- After NVMe connect, retry IsFormatted up to 5 times with 2s delay
- On transient errors (exit 1), retry
- On definitive answers (exit 0, exit 2), proceed immediately
- After all retries fail, return error and refuse to format

### Safety Guarantees

1. **Format never called on errors:** Format() checks `if err != nil` before proceeding to mkfs. This check existed before, but now IsFormatted actually returns errors for device failures.

2. **No false positives:** Exit 2 is the ONLY exit code that means "no filesystem found". All other conditions return errors.

3. **Retry prevents false negatives:** Transient device errors after NVMe connect are retried, avoiding unnecessary format failures for new volumes.

4. **Audit trail:** Format() logs at V(2) before running mkfs, creating evidence of format decisions.

## Test Coverage

**mount_test.go:**
- TestIsFormatted: exit 0 (formatted), exit 2 (not formatted), exit 1 (error), exit 3 (error)
- TestFormat_BlkidDeviceError: Verifies Format refuses mkfs when blkid exits 1

**node_test.go:**
- TestNodeStageVolume_IsFormattedRetry: Verifies retry logic and formatCalled=false

All existing tests pass without modification.

## Next Phase Readiness

- Critical data safety issue resolved
- NVMe-oF reconnect scenarios now safe from accidental reformatting
- Ready for production deployment on NixOS diskless worker nodes
- Monitoring recommendation: Track "refusing to format" errors in logs as potential device issues

---
*Phase: quick*
*Completed: 2026-02-12*
