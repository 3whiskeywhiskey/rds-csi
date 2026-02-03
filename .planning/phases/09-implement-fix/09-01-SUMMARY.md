---
phase: 09-implement-fix
plan: 01
subsystem: kubevirt
tags: [go, kubevirt, hotplug, virt-controller, volume-attachment]

# Dependency graph
requires:
  - phase: 08-fork-setup
    provides: KubeVirt fork with CI workflow
provides:
  - Documented code path explaining the hotplug race condition
  - Fix in cleanupAttachmentPods to wait for VolumeReady before pod deletion
affects: [09-02, 09-03, upstream-kubevirt]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "allHotplugVolumesReady() pattern for checking volume readiness"
    - "Early return in cleanup functions when prerequisites not met"

key-files:
  created:
    - .planning/phases/09-implement-fix/09-01-CODEPATH.md
  modified:
    - pkg/virt-controller/watch/vmi/volume-hotplug.go

key-decisions:
  - "Check ALL hotplug volumes (not just new ones) for VolumeReady phase"
  - "Early return from cleanupAttachmentPods rather than per-pod skip"
  - "Only block deletion when oldPods > 0 AND numReadyVolumes > 0"

patterns-established:
  - "Volume readiness gate: Check vmi.Status.VolumeStatus.Phase == VolumeReady"

# Metrics
duration: 2min
completed: 2026-01-31
---

# Phase 09 Plan 01: Implement Fix Summary

**KubeVirt hotplug fix: cleanupAttachmentPods now waits for all VolumeReady before deleting old attachment pod**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-31T18:47:13Z
- **Completed:** 2026-01-31T18:49:50Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Documented the complete code path from volume add request to premature pod deletion
- Identified exact location of race condition in cleanupAttachmentPods
- Implemented fix with allHotplugVolumesReady() helper function
- Committed fix to hotplug-fix-v1 branch in KubeVirt fork

## Task Commits

Each task was committed atomically:

1. **Task 1: Document the hotplug code path** - `3cb16fd` (docs)
2. **Task 2: Implement the fix in cleanupAttachmentPods** - `cc1b700` (fix)

## Files Created/Modified

**rds-csi repo:**
- `.planning/phases/09-implement-fix/09-01-CODEPATH.md` - Detailed analysis of race condition

**kubevirt-fork (/tmp/kubevirt-fork):**
- `pkg/virt-controller/watch/vmi/volume-hotplug.go` - Fix implementation (+40 lines)

## Code Changes Summary

### allHotplugVolumesReady() function (new)
```go
func allHotplugVolumesReady(vmi *v1.VirtualMachineInstance) bool {
    // Build map of hotplug volume names from spec
    // Check each has VolumeReady in status
    // Return true only if all ready
}
```

### cleanupAttachmentPods() modification
```go
// Before ANY deletion:
if len(oldPods) > 0 && numReadyVolumes > 0 && !allHotplugVolumesReady(vmi) {
    log.Log.Object(vmi).V(3).Infof("Not cleaning up old attachment pods yet: waiting for all hotplug volumes to reach VolumeReady phase")
    return nil
}
```

## Decisions Made
- **Check ALL hotplug volumes:** Rather than tracking which volumes are "new", check all hotplug volumes are ready. Simpler and catches all cases.
- **Early return pattern:** Return from function entirely rather than skipping individual pods. Cleaner control flow.
- **Guard conditions:** Only apply check when oldPods > 0 AND numReadyVolumes > 0. This ensures:
  - No-op when no old pods to clean
  - No blocking when removing all volumes (numReadyVolumes == 0)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- **Go build fails on macOS:** Expected - KubeVirt has Linux-only syscalls. Used `gofmt -l` and `goimports -l` to verify syntax correctness instead.

## Next Phase Readiness

- Fix is committed to `hotplug-fix-v1` branch in /tmp/kubevirt-fork
- Ready for unit tests (09-02-PLAN.md)
- Ready for manual validation (09-03-PLAN.md)
- Fix references upstream issues: kubevirt/kubevirt#6564, #9708, #16520

---
*Phase: 09-implement-fix*
*Completed: 2026-01-31*
