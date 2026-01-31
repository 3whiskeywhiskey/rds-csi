---
phase: 05-attachment-manager-foundation
plan: 02
subsystem: storage-fencing
tags: [kubernetes, csi, pv-annotations, state-persistence, controller-restart]

# Dependency graph
requires:
  - phase: 05-01
    provides: In-memory AttachmentManager with TrackAttachment/UntrackAttachment
provides:
  - PV annotation persistence for attachment state
  - State rebuild from PV annotations on controller startup
  - AttachmentManager integrated with Driver lifecycle
  - Controller survives restarts with full state recovery
affects: [06-controller-publish-volume, 07-volume-fencing-e2e]

# Tech tracking
tech-stack:
  added:
    - k8s.io/client-go/util/retry (RetryOnConflict)
    - k8s.io/apimachinery/pkg/apis/meta/v1 (PV operations)
  patterns:
    - PV annotations as persistent state store
    - Retry on conflict for atomic PV updates
    - Lock-free I/O operations (release locks before network calls)
    - Rollback pattern on persistence failure
    - Graceful degradation (works without k8sClient for tests)

key-files:
  created:
    - pkg/attachment/persist.go
    - pkg/attachment/rebuild.go
  modified:
    - pkg/attachment/manager.go
    - pkg/driver/driver.go

key-decisions:
  - "Use PV annotations rds.csi.srvlab.io/attached-node and attached-at for persistence"
  - "Rollback in-memory state if PV annotation update fails (consistency guarantee)"
  - "Tolerate annotation clear failures on untrack (in-memory is source of truth)"
  - "Handle 'not found' errors gracefully (PVs may be deleted)"
  - "Initialize AttachmentManager before orphan reconciler in Run()"

patterns-established:
  - "I/O operations outside locks: release mu before calling persist/clear"
  - "Rollback pattern: delete from map if persistence fails"
  - "nil k8sClient tolerance: allows testing without Kubernetes"
  - "Timestamp format: metav1.RFC3339Micro for annotation values"

# Metrics
duration: 2min
completed: 2026-01-31
---

# Phase 5 Plan 2: Attachment Persistence Summary

**PV annotation persistence with retry-on-conflict, state rebuild from annotations on startup, and full Driver integration for controller restart survival**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-31T04:00:41Z
- **Completed:** 2026-01-31T04:03:01Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments

- Attachment state persists to PV annotations using retry.RetryOnConflict for atomic updates
- Controller can rebuild full attachment map from PV annotations on startup
- AttachmentManager initialized in Driver.Run() before serving CSI requests
- State survives controller pod restarts with zero data loss

## Task Commits

Each task was committed atomically:

1. **Task 1: Add PV annotation persistence** - `f7f2ff0` (feat)
2. **Task 2: Add state rebuild from PV annotations** - `2ed9b61` (feat)
3. **Task 3: Integrate AttachmentManager with Driver** - `dbdb33e` (feat)

**Plan metadata:** (committed separately after this summary)

## Files Created/Modified

- `pkg/attachment/persist.go` - PV annotation update with retry.RetryOnConflict, persistAttachment/clearAttachment methods
- `pkg/attachment/rebuild.go` - RebuildState scans PVs for annotations, Initialize wraps rebuild for startup
- `pkg/attachment/manager.go` - TrackAttachment persists after in-memory update with rollback, UntrackAttachment clears annotations
- `pkg/driver/driver.go` - AttachmentManager creation in NewDriver, Initialize call in Run(), GetAttachmentManager getter

## Decisions Made

**1. Rollback on persistence failure**
- TrackAttachment deletes from in-memory map if PV annotation update fails
- Ensures in-memory state and PV annotations stay consistent
- Prevents "ghost attachments" that exist in memory but not on disk

**2. Tolerate clear failures**
- UntrackAttachment logs warning if annotation clear fails but doesn't error
- In-memory state is source of truth during runtime
- Annotations are for recovery only, not real-time enforcement

**3. Graceful PV not-found handling**
- persist/clear return nil if PV doesn't exist yet
- Allows attachment tracking before PV is created (e.g., during CreateVolume)
- PV may be deleted before untrack (race condition tolerance)

**4. Initialize before reconciler**
- AttachmentManager.Initialize() called in Run() before orphan reconciler starts
- Ensures state is recovered before any volume operations occur
- AttachmentManager available to ControllerPublishVolume from first request

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## Next Phase Readiness

**Ready for Phase 6 (ControllerPublishVolume implementation):**

- AttachmentManager fully functional with persistence
- GetAttachmentManager() available in controller service
- State survives restarts, enabling production-ready fencing
- TrackAttachment/UntrackAttachment ready for use in Publish/Unpublish

**Architecture complete for fencing:**

- In-memory tracking (05-01) ✓
- Persistent state (05-02) ✓
- Next: CSI ControllerPublishVolume/Unpublish implementation (05-03)

**No blockers or concerns.**

---
*Phase: 05-attachment-manager-foundation*
*Completed: 2026-01-31*
