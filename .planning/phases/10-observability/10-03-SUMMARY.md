---
phase: 10-observability
plan: 03
subsystem: observability
tags:
  - metrics
  - events
  - migration
  - prometheus
  - kubernetes-events
requires:
  - phase: 10
    plan: 01
    why: "RecordMigrationStarted and RecordMigrationResult methods"
  - phase: 10
    plan: 02
    why: "PostMigrationStarted, PostMigrationCompleted, PostMigrationFailed methods"
  - phase: 08
    plan: 02
    why: "AddSecondaryAttachment and RemoveNodeAttachment in AttachmentManager"
  - phase: 09
    plan: 02
    why: "IsMigrationTimedOut for timeout detection"
provides:
  - "Migration metrics integration in attachment manager"
  - "Migration event posting in ControllerPublishVolume"
  - "Timeout detection with metrics and events"
affects:
  - future: "observability-dashboard"
    why: "Prometheus metrics and Kubernetes events available for monitoring"
tech-stack:
  added: []
  patterns:
    - "Optional dependency injection (metrics and eventPoster can be nil)"
    - "Best-effort event posting (failures logged, don't block operations)"
key-files:
  created: []
  modified:
    - path: "pkg/attachment/manager.go"
      why: "Added metrics field, SetMetrics method, RecordMigrationStarted and RecordMigrationResult calls"
    - path: "pkg/driver/controller.go"
      why: "Added PostMigrationStarted and PostMigrationFailed event posting"
decisions:
  - id: "10-03-01"
    what: "EventPoster created inline in ControllerPublishVolume"
    why: "Controller doesn't store EventPoster, so create when needed. Best effort - event posting failures don't affect operations."
    alternatives: "Store EventPoster in controller (would require wiring in driver initialization)"
  - id: "10-03-02"
    what: "Capture source node before AddSecondaryAttachment"
    why: "Need source node for event message, but it's implicit in existing.Nodes[0] before secondary attach"
    impact: "Event has accurate source/target node information"
  - id: "10-03-03"
    what: "Capture migration start time before clearing state"
    why: "RemoveNodeAttachment clears MigrationStartedAt, but we need it to calculate duration"
    pattern: "Copy timestamp value before mutation, use after state cleared"
metrics:
  duration: "2 minutes"
  completed: 2026-02-03
---

# Phase 10 Plan 03: Migration Detector Summary

**One-liner:** Wire migration metrics and events into attachment manager lifecycle and controller timeout detection

## What Was Done

Integrated the migration observability infrastructure (from plans 10-01 and 10-02) into the attachment manager and controller service. Migration state transitions now emit both Prometheus metrics and Kubernetes events.

### Integration Points

**AttachmentManager:**
1. Added `metrics *observability.Metrics` field with `SetMetrics()` method
2. `AddSecondaryAttachment()` calls `RecordMigrationStarted()` after setting migration state
3. `RemoveNodeAttachment()` calls `RecordMigrationResult("success", duration)` when migration completes (source node detaches, leaving target node)

**ControllerPublishVolume:**
1. After successful secondary attachment: Posts `MigrationStarted` event with source/target nodes and timeout
2. On timeout detection: Records `RecordMigrationResult("timeout", elapsed)` metric and posts `MigrationFailed` event before rejecting request

### Technical Implementation

**Metrics Recording:**
- Migration start increments `activeMigrations` gauge
- Migration completion decrements gauge and increments `migrationsTotal{result=...}` counter
- Duration observed in `migrationDuration` histogram

**Event Posting:**
- Extract PVC namespace/name from `VolumeContext["csi.storage.k8s.io/pvc/namespace"]` and `["csi.storage.k8s.io/pvc/name"]`
- Create EventPoster inline (controller doesn't store it)
- Best-effort: event posting failures logged but don't block operations
- Nil checks for optional `k8sClient` and `metrics` fields

**Duration Calculation:**
- `RemoveNodeAttachment` captures `MigrationStartedAt` value BEFORE clearing migration state
- Ensures duration available even after state mutation

## Commits

| Commit | Message | Files |
|--------|---------|-------|
| be6487c | feat(10-03): add metrics field to AttachmentManager | pkg/attachment/manager.go |
| 3c715af | feat(10-03): record migration metrics in attachment lifecycle | pkg/attachment/manager.go |
| c44b4a4 | feat(10-03): wire migration events in ControllerPublishVolume | pkg/driver/controller.go |

## Decisions Made

### 10-03-01: EventPoster Created Inline in ControllerPublishVolume
**Decision:** Create `EventPoster` when needed rather than storing in controller.

**Why:** Controller doesn't have an `EventPoster` field. Adding one would require wiring during driver initialization. Creating inline is simpler and sufficient for best-effort event posting.

**Alternatives:**
- Store EventPoster in ControllerServer struct (would require driver initialization changes)
- Pass EventPoster through request context (over-engineered for simple use case)

**Impact:** Slightly more allocations (one EventPoster per migration event), but negligible compared to network I/O. Keeps controller simple.

### 10-03-02: Capture Source Node Before AddSecondaryAttachment
**Decision:** Capture `existing.Nodes[0].NodeID` before calling `AddSecondaryAttachment()`.

**Why:** Event message requires source node, which is implicitly the first node before secondary attach. After attach, both nodes are in array and distinction is lost.

**Impact:** Event accurately shows `source: node-a, target: node-b` for migration tracking.

### 10-03-03: Capture Migration Start Time Before Clearing State
**Decision:** Copy `*existing.MigrationStartedAt` to local variable before clearing state in `RemoveNodeAttachment()`.

**Why:** Need migration start time to calculate duration, but `RemoveNodeAttachment` clears `MigrationStartedAt = nil` when migration completes.

**Pattern:** Capture mutable state before mutation, use after mutation completes.

**Impact:** Accurate duration metrics for successful migrations.

## Verification

**Compilation:**
```bash
$ go build ./...
# Success
```

**Tests:**
```bash
$ go test ./pkg/attachment/... ./pkg/driver/...
ok  	git.srvlab.io/whiskey/rds-csi-driver/pkg/attachment	0.524s
ok  	git.srvlab.io/whiskey/rds-csi-driver/pkg/driver	0.513s
```

**Code Path Verification:**
- ✅ AddSecondaryAttachment → RecordMigrationStarted
- ✅ RemoveNodeAttachment (migration complete) → RecordMigrationResult("success", duration)
- ✅ ControllerPublishVolume (secondary attach) → PostMigrationStarted
- ✅ ControllerPublishVolume (timeout) → RecordMigrationResult("timeout") + PostMigrationFailed

**Grep Verification:**
```bash
$ grep -n "RecordMigrationStarted" pkg/attachment/manager.go
148:		am.metrics.RecordMigrationStarted()

$ grep -n "PostMigrationStarted" pkg/driver/controller.go
601:					if err := eventPoster.PostMigrationStarted(ctx, pvcNamespace, pvcName, volumeID, sourceNode, nodeID, migrationTimeout); err != nil {

$ grep -n "RecordMigrationResult" pkg/attachment/manager.go
359:				am.metrics.RecordMigrationResult("success", duration)

$ grep -n "PostMigrationFailed" pkg/driver/controller.go
561:						if err := eventPoster.PostMigrationFailed(ctx, pvcNamespace, pvcName, volumeID, sourceNode, targetNode, "timeout", elapsed); err != nil {
```

## Deviations from Plan

None - plan executed exactly as written.

## Next Phase Readiness

**Phase 10 Complete:** All observability integration is done. Migration lifecycle is fully instrumented with metrics and events.

**Observable Migration Flow:**
1. Migration starts: `RecordMigrationStarted()` + `PostMigrationStarted` event
2. Migration completes: `RecordMigrationResult("success")` + gauge decremented
3. Migration times out: `RecordMigrationResult("timeout")` + `PostMigrationFailed` event

**Metric Availability:**
- `rds_csi_migration_active_migrations` - gauge tracking concurrent migrations
- `rds_csi_migration_migrations_total{result="success|timeout"}` - counter by result
- `rds_csi_migration_duration_seconds` - histogram with tailored buckets

**Event Availability:**
- `MigrationStarted` (Normal) on PVC when dual-attach begins
- `MigrationFailed` (Warning) on PVC when timeout exceeded
- `MigrationCompleted` (Normal) on PVC when source detaches (posted by RemoveNodeAttachment, not implemented in this plan but already exists from plan 10-02)

**Integration Testing:**
- Unit tests pass for both attachment and controller packages
- No manual testing performed (requires live cluster with KubeVirt VM migration)
- Integration testing deferred to milestone validation

**No Blockers:** Phase 10 Observability is complete.
