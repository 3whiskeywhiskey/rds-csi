---
phase: 07-robustness-observability
plan: 01
subsystem: attachment-observability
completed: 2026-01-31
duration: 3 minutes

tags:
  - grace-period
  - prometheus-metrics
  - kubernetes-events
  - live-migration
  - observability

requires:
  - 05-01-attachment-tracking
  - 05-02-attachment-persistence
  - 06-01-controller-publish-implementation
  - 06-02-controller-unpublish-implementation

provides:
  - grace-period-tracking
  - attachment-metrics
  - attachment-events

affects:
  - 07-02-attachment-reconciler
  - future-monitoring-dashboards

tech-stack:
  added: []
  patterns:
    - per-volume-grace-period
    - prometheus-histogram-buckets
    - kubernetes-event-posting

key-files:
  created: []
  modified:
    - pkg/attachment/types.go
    - pkg/attachment/manager.go
    - pkg/observability/prometheus.go
    - pkg/driver/events.go

decisions:
  - id: GRACE-01
    choice: Per-volume grace period tracking with detachTimestamps map
    context: Separate map from attachments to preserve detach history
    alternatives: Single global grace period timer
  - id: METRICS-01
    choice: Sub-second histogram buckets (0.01 to 5 seconds)
    context: Attachment operations are mostly in-memory with PV annotation I/O
    alternatives: Full-second buckets (1-60s)
  - id: EVENTS-01
    choice: Normal event type for VolumeAttached/Detached
    context: Routine operations, not failures
    alternatives: Warning events
---

# Phase 7 Plan 01: Grace Period Tracking and Attachment Metrics Summary

**One-liner:** Per-volume grace period tracking for live migration handoff with comprehensive Prometheus metrics and Kubernetes event posting

## What Was Built

This plan adds production-grade observability and grace period support to the attachment tracking system. The key additions are:

1. **Grace Period Tracking**: DetachedAt field in AttachmentState and detachTimestamps map in AttachmentManager to track when each volume was detached, enabling per-volume grace period checks for KubeVirt live migration handoff.

2. **Prometheus Metrics**: Comprehensive attachment-specific metrics including attach/detach counters, conflict counter, reconciliation counter, operation duration histogram, grace period usage counter, and stale cleared counter.

3. **Event Posting Methods**: EventPoster methods for VolumeAttached, VolumeDetached, and StaleAttachmentCleared events to provide operational visibility into attachment lifecycle.

## Files Changed

### pkg/attachment/types.go
- Added DetachedAt *time.Time field to AttachmentState for tracking detachment timestamps

### pkg/attachment/manager.go
- Added detachTimestamps map[string]time.Time field to track last detach time per volume
- Initialize detachTimestamps in NewAttachmentManager
- Record detach timestamp in UntrackAttachment before deleting from attachments map
- Added IsWithinGracePeriod(volumeID, gracePeriod) method to check if volume within grace period
- Added GetDetachTimestamp(volumeID) method for observability
- Added ClearDetachTimestamp(volumeID) method for cleanup after reattachment

### pkg/observability/prometheus.go
- Added 7 attachment-specific metric fields: attachmentAttachTotal, attachmentDetachTotal, attachmentConflictsTotal, attachmentReconcileTotal, attachmentOpDuration, attachmentGracePeriodUsed, attachmentStaleCleared
- Initialize metrics with rds_csi_attachment_* namespace and subsystem
- Register all attachment metrics in NewMetrics
- Added RecordAttachmentOp(operation, err, duration) for attach/detach with timing
- Added RecordAttachmentConflict() for RWO violations
- Added RecordGracePeriodUsed() for grace period usage tracking
- Added RecordStaleAttachmentCleared() for reconciler cleanup
- Added RecordReconcileAction(action) for reconciliation tracking

### pkg/driver/events.go
- Added event reason constants: EventReasonVolumeAttached, EventReasonVolumeDetached, EventReasonStaleAttachmentCleared
- Added time import for duration formatting
- Added PostVolumeAttached(ctx, pvcNamespace, pvcName, volumeID, nodeID, duration) with Normal event type
- Added PostVolumeDetached(ctx, pvcNamespace, pvcName, volumeID, nodeID) with Normal event type
- Added PostStaleAttachmentCleared(ctx, pvcNamespace, pvcName, volumeID, staleNodeID) with Normal event type

## Decisions Made

### GRACE-01: Per-volume grace period tracking with separate map

**Decision:** Use separate detachTimestamps map alongside attachments map to preserve detach history even after volume is removed from attachments.

**Reasoning:**
- Allows concurrent migrations (multiple volumes can be in grace period simultaneously)
- Preserves detach timestamp even after UntrackAttachment removes from attachments map
- Simple time.Since() check against configurable grace period duration
- Clean separation of concerns (attachment state vs grace period tracking)

**Alternatives considered:**
- Global grace period timer: Rejected because it prevents concurrent migrations
- DetachedAt field only: Rejected because field is lost when attachment deleted from map
- Single timestamp for all volumes: Rejected because it doesn't support concurrent migrations

### METRICS-01: Sub-second histogram buckets for attachment operations

**Decision:** Use histogram buckets from 0.01 to 5 seconds: `[]float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}`

**Reasoning:**
- Attachment operations are mostly in-memory (map updates + mutex)
- PV annotation I/O adds latency but should be sub-second in typical clusters
- Sub-second granularity helps identify performance issues
- Follows Prometheus best practice of logarithmic spacing

**Alternatives considered:**
- Full-second buckets (1-60s): Rejected as too coarse for fast in-memory operations
- Millisecond-only buckets: Rejected as missing upper range for slow API calls

### EVENTS-01: Normal event type for routine attachment lifecycle

**Decision:** Use EventTypeNormal for VolumeAttached, VolumeDetached, and StaleAttachmentCleared.

**Reasoning:**
- These are routine operations, not failures or warnings
- Provides visibility without alarming operators
- Matches Kubernetes convention (Normal = informational, Warning = actionable issue)
- AttachmentConflict remains Warning (actionable issue)

**Alternatives considered:**
- Warning events: Rejected because routine operations shouldn't trigger alerts
- No events: Rejected because operators need visibility into attachment lifecycle

## Deviations from Plan

None - plan executed exactly as written.

## Testing

All existing tests pass:
- pkg/attachment tests: 18/18 passed
- pkg/observability tests: No test file yet (metrics are initialized but not called in tests)
- pkg/driver tests: 30/30 passed

New functionality not yet tested (will be tested when integrated):
- IsWithinGracePeriod logic (will be tested in 07-02 reconciler tests)
- Metrics recording (will be tested when ControllerPublish/Unpublish call them)
- Event posting (will be tested when ControllerPublish/Unpublish call them)

## Integration Points

### Upstream Dependencies
- 05-01: AttachmentManager provides state tracking foundation
- 05-02: PV annotation persistence provides durability
- 06-01/06-02: ControllerPublish/Unpublish will call metrics and events

### Downstream Consumers
- 07-02: Reconciler will use IsWithinGracePeriod and RecordStaleAttachmentCleared
- Future: ControllerPublish will call RecordAttachmentOp and RecordGracePeriodUsed
- Future: Prometheus dashboards will consume attachment_* metrics

## Next Phase Readiness

**Ready for 07-02 (Attachment Reconciler):** Yes

The reconciler can now:
- Check IsWithinGracePeriod before clearing stale attachments
- Record RecordStaleAttachmentCleared when cleaning up
- Post PostStaleAttachmentCleared events for operator visibility
- Track RecordReconcileAction for both clear_stale and sync_annotation actions

**Blockers:** None

**Concerns:** None - grace period tracking and metrics are self-contained additions

## Technical Debt

None introduced. Clean additions to existing packages.

## Future Enhancements

1. **Histogram bucket tuning**: May need adjustment after production data shows actual latency distribution
2. **Grace period configuration**: Currently hardcoded, will need flag/env var in driver initialization
3. **Metric labels**: Could add node_id label if cardinality is acceptable (bounded by cluster size)
4. **Event aggregation**: May want to add event deduplication if attachment churn is high

## Performance Impact

Negligible:
- Grace period check: O(1) map lookup + time comparison
- Metrics recording: O(1) counter/histogram update
- Event posting: Async, doesn't block CSI operations
- Memory: +1 time.Time per volume in detachTimestamps map (~24 bytes/volume)

## Commits

```
a6b2914 feat(07-01): add attachment lifecycle event posting methods
8dfa2a5 feat(07-01): add attachment-specific Prometheus metrics
158cba6 feat(07-01): add grace period tracking to AttachmentState and AttachmentManager
```

## Success Criteria Met

- [x] AttachmentState has DetachedAt field for tracking detachment time
- [x] AttachmentManager has detachTimestamps map and IsWithinGracePeriod method
- [x] Prometheus metrics exist for attachment operations (attach_total, detach_total, conflicts_total, operation_duration_seconds, grace_period_used_total, stale_cleared_total, reconcile_total)
- [x] EventPoster has methods for posting VolumeAttached, VolumeDetached, and StaleAttachmentCleared events
- [x] All packages build and pass vet checks
- [x] Existing tests still pass

---

*Plan executed: 2026-01-31*
*Duration: 3 minutes*
*Commits: 3*
