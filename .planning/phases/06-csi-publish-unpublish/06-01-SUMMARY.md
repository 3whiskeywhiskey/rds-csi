---
phase: 06-csi-publish-unpublish
plan: 01
subsystem: api
tags: [csi, grpc, kubernetes, events, controller]

# Dependency graph
requires:
  - phase: 05-attachment-manager-foundation
    provides: AttachmentManager for tracking volume attachments
provides:
  - PUBLISH_UNPUBLISH_VOLUME capability in ControllerGetCapabilities
  - PostAttachmentConflict event method for attachment conflict visibility
affects: [06-02-controller-publish, 06-03-controller-unpublish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "EventPoster pattern extended for controller events"

key-files:
  created: []
  modified:
    - pkg/driver/driver.go
    - pkg/driver/events.go

key-decisions:
  - "Warning event type for attachment conflicts (blocks pod scheduling)"
  - "Actionable message format with both nodes and guidance"

patterns-established:
  - "Attachment conflict events follow EventPoster pattern from node events"

# Metrics
duration: 5min
completed: 2026-01-31
---

# Phase 6 Plan 1: Capability and Event Foundation Summary

**PUBLISH_UNPUBLISH_VOLUME capability added to enable external-attacher, PostAttachmentConflict event method for operator visibility**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-31T00:00:00Z
- **Completed:** 2026-01-31T00:05:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Added PUBLISH_UNPUBLISH_VOLUME to controller service capabilities
- Created EventReasonAttachmentConflict constant
- Implemented PostAttachmentConflict method with actionable operator messaging
- External-attacher will now call ControllerPublishVolume/ControllerUnpublishVolume

## Task Commits

Each task was committed atomically:

1. **Task 1: Add PUBLISH_UNPUBLISH_VOLUME capability** - `1695291` (feat)
2. **Task 2: Add PostAttachmentConflict event method** - `c473489` (feat)

## Files Created/Modified
- `pkg/driver/driver.go` - Added RPC_PUBLISH_UNPUBLISH_VOLUME capability
- `pkg/driver/events.go` - Added EventReasonAttachmentConflict constant and PostAttachmentConflict method

## Decisions Made
- Used Warning event type (not Normal) for attachment conflicts since they block pod scheduling
- Message format includes both nodes (requested and attached) plus actionable guidance for operators
- Returns nil on PVC get error to avoid failing main operation due to event posting

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- PUBLISH_UNPUBLISH_VOLUME capability declared - external-attacher will call ControllerPublish/Unpublish
- PostAttachmentConflict event method ready for use in ControllerPublishVolume (Plan 06-02)
- All builds pass, tests pass

---
*Phase: 06-csi-publish-unpublish*
*Completed: 2026-01-31*
