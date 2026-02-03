---
phase: 09-migration-safety
plan: 03
subsystem: storage
tags: [nvme, safety, lsof, device-busy, data-corruption-prevention]

# Dependency graph
requires:
  - phase: 08-core-rwx-capability
    provides: NodeUnstageVolume implementation
  - phase: 01-foundation-device-path-resolution
    provides: GetDevicePath method for NQN to device resolution
provides:
  - Device-in-use verification before NVMe disconnect
  - Prevents premature disconnect during forced pod termination
  - Returns FAILED_PRECONDITION error if device busy with process list
affects:
  - 09-04-migration-safety # Recovery procedures may need to handle device-busy errors

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Use lsof with timeout for device-busy detection"
    - "Graceful timeout handling (proceed if check times out)"
    - "Error-first contract for GetDevicePath (returns error not empty string when disconnected)"

key-files:
  created:
    - pkg/nvme/device.go
  modified:
    - pkg/driver/node.go
    - pkg/nvme/nvme.go

key-decisions:
  - "5-second timeout for lsof check (balance between responsiveness and false positives)"
  - "Proceed with disconnect on timeout (device likely unresponsive anyway)"
  - "Skip check if GetDevicePath returns error (device already disconnected)"
  - "Block unstage with FAILED_PRECONDITION if device busy (prevent data corruption)"

patterns-established:
  - "Device-in-use check pattern: lsof with timeout context, parse output, return structured result"
  - "Safety check placement: after unmount, before NVMe disconnect"
  - "GetDevicePath error handling: check error first to distinguish not-connected from lookup-failed"

# Metrics
duration: 2min
completed: 2026-02-03
---

# Phase 9 Plan 3: Device-In-Use Verification Summary

**Prevent data corruption during unstage by verifying no open file descriptors before NVMe disconnect using lsof with 5s timeout**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-03T14:57:34Z
- **Completed:** 2026-02-03T14:59:34Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Device-in-use detection using lsof with timeout prevents unsafe NVMe disconnects
- NodeUnstageVolume blocks unstage if processes hold device open (returns FAILED_PRECONDITION)
- Graceful handling of timeout and device-not-connected scenarios
- Documented GetDevicePath contract for error vs empty string semantics

## Task Commits

Each task was committed atomically:

1. **Task 1: Create device utility file with CheckDeviceInUse function** - `11758b4` (feat)
2. **Task 2: Add device-in-use check to NodeUnstageVolume** - `66ca0f7` (feat)
3. **Task 3: Document GetDevicePath contract for disconnected devices** - `392326f` (docs)

## Files Created/Modified

- `pkg/nvme/device.go` - CheckDeviceInUse function using lsof with 5s timeout
- `pkg/driver/node.go` - Device-in-use check in NodeUnstageVolume before disconnect
- `pkg/nvme/nvme.go` - Documented GetDevicePath error contract

## Decisions Made

**Decision 1: 5-second timeout for device check**
- Balance between waiting for slow lsof and detecting unresponsive devices
- If device doesn't respond in 5s, likely dead anyway - proceed with disconnect

**Decision 2: Skip check if device not connected**
- GetDevicePath returns error when device not found (idempotent unstage scenario)
- No point checking busy status for disconnected device
- Proceed with disconnect attempt (will be no-op)

**Decision 3: Block on busy device with FAILED_PRECONDITION**
- Return error code that indicates precondition not met (pod not terminated)
- Include process list in error message for debugging
- User can see what's holding device open

**Decision 4: Proceed on check failure or timeout**
- lsof missing, permission denied, or timeout: log warning but proceed
- Blocking on check failure would prevent cleanup in recovery scenarios
- Data safety: unmount already done, disconnect is cleanup

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation straightforward, all tests passed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Device-in-use verification complete and ready for:
- Plan 09-04: Graceful migration timeout recovery procedures
- Testing with forced pod termination scenarios
- Integration with KubeVirt live migration flow

**Safety layer complete:** NodeUnstageVolume now prevents premature NVMe disconnect that could corrupt data during forced pod termination.

---
*Phase: 09-migration-safety*
*Completed: 2026-02-03*
