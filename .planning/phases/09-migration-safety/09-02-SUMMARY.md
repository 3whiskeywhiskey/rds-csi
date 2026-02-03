---
phase: 09
plan: 02
subsystem: controller
tags: [migration, timeout, enforcement, safety, rwx, rwo]
requires:
  - 09-01  # Migration timeout tracking and parsing
  - 08-02  # AttachmentState with multi-node support
provides:
  - migration-timeout-enforcement
  - rwo-grace-period-documentation
  - timed-out-migration-rejection
affects:
  - 09-03  # Device-in-use checks will work with timeout enforcement
  - 09-04  # Metrics will measure timeout occurrences
tech-stack:
  added: []
  patterns:
    - safety-gate-before-dual-attach
    - distinct-rwo-rwx-conflict-handling
decisions:
  - id: 09-02-01
    choice: Check timeout before allowing secondary attachment
    rationale: Prevents indefinite dual-attach from stuck migrations
  - id: 09-02-02
    choice: RWO grace period documented as reattachment-only
    rationale: Clarifies it's for sequential handoff, not concurrent access
  - id: 09-02-03
    choice: Detailed error message with elapsed time and remediation
    rationale: Operators need actionable guidance when timeout exceeded
key-files:
  created: []
  modified:
    - pkg/driver/controller.go
metrics:
  tasks: 3
  commits: 1
  duration: 127s
  files_modified: 1
completed: 2026-02-03
---

# Phase 09 Plan 02: Migration Timeout Enforcement Summary

**One-liner:** Added migration timeout check in ControllerPublishVolume to reject timed-out migrations and documented RWO grace period distinction

## What Was Built

Implemented SAFETY-01 and SAFETY-02 requirements to enforce migration timeouts and clarify RWO vs RWX conflict handling:

1. **Migration Timeout Check Before Secondary Attachment** (Task 1)
   - Added `IsMigrationTimedOut()` check in RWX secondary attachment path
   - Timeout checked BEFORE allowing new secondary attachment
   - Prevents indefinite dual-attach if previous migration fails
   - Error message includes:
     - Elapsed time since migration started
     - Configured maximum timeout
     - Remediation steps (detach source node or adjust StorageClass)

2. **RWO Grace Period Clarification** (Task 2)
   - Added SAFETY-02 documentation comment in RWO conflict path
   - Clarifies grace period is for reattachment AFTER detach
   - Explicitly states it does NOT allow concurrent multi-node attachment
   - Distinguishes between:
     - **Grace period:** Sequential handoff (A detaches → B attaches)
     - **RWX migration:** Concurrent access (A and B both attached)

3. **VolumeContext Parameter Flow** (Task 3)
   - Verified `migrationTimeoutSeconds` in CreateVolume VolumeContext
   - Already implemented in Plan 09-01 (Auto-fix #3)
   - Flows from CreateVolume to ControllerPublishVolume via CSI spec

## How It Works

**Normal RWX Migration (Within Timeout):**
```go
// KubeVirt starts migration at T=0
ControllerPublishVolume(volume, targetNode, RWX)
  ├─ Existing attachment found (sourceNode)
  ├─ isRWX = true
  ├─ NodeCount = 1 (< 2 limit)
  ├─ IsMigrationTimedOut() = false  ← Added in this plan
  ├─ Parse migrationTimeout from VolumeContext
  └─ Allow secondary attachment
     └─ Sets MigrationStartedAt = time.Now()
```

**Timed-Out Migration (Stuck):**
```go
// Migration stuck, T=6min (timeout=5min)
ControllerPublishVolume(volume, anotherNode, RWX)
  ├─ Existing attachment found (2 nodes already)
  ├─ isRWX = true
  ├─ NodeCount = 2 (at limit)
  ├─ Skip to timeout check...

// OR attempting new migration after timeout
ControllerPublishVolume(volume, newTargetNode, RWX)
  ├─ Existing attachment found (sourceNode, oldTargetNode)
  ├─ isRWX = true
  ├─ NodeCount = 2 (< 3 limit, but...)
  ├─ IsMigrationTimedOut() = true  ← Catches stuck migration
  │    elapsed = 6m0s
  │    max = 5m0s
  └─ REJECT: "Volume pvc-xxx migration timeout exceeded (6m0s elapsed, 5m0s max).
              Previous migration may be stuck. Detach source node to reset,
              or adjust migrationTimeoutSeconds in StorageClass."
```

**RWO Grace Period (Sequential Reattachment):**
```go
// Pod moves from Node A to Node B
// T=0: ControllerUnpublishVolume(volume, nodeA)
//      └─ Records DetachedAt timestamp
// T=1s: ControllerPublishVolume(volume, nodeB, RWO)
//      ├─ Existing attachment found (nodeA, recently detached)
//      ├─ isRWX = false
//      ├─ IsWithinGracePeriod(volume, 5s) = true
//      ├─ SAFETY-02 comment clarifies this is SEQUENTIAL  ← Added in this plan
//      ├─ Clear old attachment
//      └─ Allow new attachment (NOT concurrent)
```

**RWO Concurrent Attempt (Fails Immediately):**
```go
// Attempting concurrent attachment without detach
ControllerPublishVolume(volume, nodeB, RWO)
  ├─ Existing attachment found (nodeA, currently attached)
  ├─ isRWX = false
  ├─ IsWithinGracePeriod(volume, 5s) = false (no recent detach)
  ├─ SAFETY-02: Grace period does NOT allow concurrent dual-attach
  └─ REJECT: "Volume pvc-xxx already attached to node nodeA.
              For multi-node access, use RWX with block volumes."
```

## Key Decisions Made

### Decision 09-02-01: Timeout Check Before Secondary Attachment
**Choice:** Call `IsMigrationTimedOut()` in RWX path before allowing secondary attachment

**Context:** Need to prevent indefinite dual-attach if migration gets stuck

**Alternatives Considered:**
- Check timeout only when hitting 2-node limit - misses stuck 1-node migrations
- Check timeout in periodic background job - delayed detection
- No timeout enforcement - indefinite dual-attach risk

**Why This Way:**
- Enforces timeout at point of decision (secondary attachment)
- Immediate feedback to operator when timeout exceeded
- Prevents additional attachments to stuck migrations

### Decision 09-02-02: Document RWO Grace Period Distinction
**Choice:** Add SAFETY-02 comment explaining grace period vs concurrent attachment

**Context:** Risk of confusion - grace period might be misunderstood as allowing RWX-like behavior

**Alternatives Considered:**
- Let code speak for itself - too subtle, easy to misunderstand
- Remove grace period entirely - breaks pod migration use case
- Rename grace period - would break existing configurations

**Why This Way:**
- Code comments prevent future maintainer confusion
- Explicit distinction between sequential (RWO grace) and concurrent (RWX migration)
- Documents intent without changing behavior

### Decision 09-02-03: Detailed Error Message with Remediation
**Choice:** Include elapsed time, max timeout, and two remediation options

**Context:** Operator needs to understand WHY request failed and WHAT to do

**Alternatives Considered:**
- Generic "timeout exceeded" - not actionable
- Only show elapsed time - doesn't explain how to fix
- Only suggest one fix - limits operator flexibility

**Why This Way:**
- Elapsed vs max time helps diagnose if timeout is too short
- "Detach source node" - immediate recovery path
- "Adjust migrationTimeoutSeconds" - configuration fix for persistent issue
- Follows CSI error message best practices

## Verification Performed

### Build Verification
```bash
go build ./...
# ✓ Compiled successfully
```

### Test Verification
```bash
go test -v ./pkg/driver -run TestControllerPublishVolume
# ✓ All 12 ControllerPublishVolume tests passed
# ✓ TestControllerPublishVolume_RWXDualAttach - 2-node limit works
# ✓ TestControllerPublishVolume_RWOConflict - grace period distinct from RWX
```

### Manual Verification
```bash
# Task 1: Migration timeout check
grep -n "IsMigrationTimedOut" pkg/driver/controller.go
# 542:			if existing.IsMigrationTimedOut() {

# Task 2: SAFETY-02 comment
grep -A5 "SAFETY-02" pkg/driver/controller.go
# 576:		// SAFETY-02: Grace period is ONLY for reattachment AFTER detach
# 577:		// It does NOT allow concurrent multi-node attachment like RWX
# ...

# Task 3: migrationTimeoutSeconds in VolumeContext
grep "migrationTimeoutSeconds" pkg/driver/controller.go
# 131:					"migrationTimeoutSeconds": fmt.Sprintf("%.0f", migrationTimeout.Seconds()),
# 221:				"migrationTimeoutSeconds": fmt.Sprintf("%.0f", migrationTimeout.Seconds()),
# 548:					"Previous migration may be stuck. Detach source node to reset, or adjust migrationTimeoutSeconds in StorageClass.",
# 555:				"migrationTimeoutSeconds": volCtx["migrationTimeoutSeconds"],
```

## Deviations from Plan

None - plan executed exactly as written.

Task 3 (migrationTimeoutSeconds in VolumeContext) was already completed in Plan 09-01 as Auto-fix #3. This plan verified the parameter flow is complete and documented it.

## Test Coverage

### Existing Tests Continue to Pass
- `TestControllerPublishVolume_Success` - primary attachment
- `TestControllerPublishVolume_Idempotent` - same-node idempotency
- `TestControllerPublishVolume_RWOConflict` - RWO dual-attach fails
- `TestControllerPublishVolume_RWXDualAttach` - RWX 2-node limit
- `TestControllerPublishVolume_RWXIdempotent` - RWX idempotency

### Future Test Coverage Needed (Phase 9 Plan 3/4)
- Test timeout enforcement with mock migration state
- Test error message content for timed-out migrations
- Test remediation paths (detach to reset, adjust timeout)

## Next Phase Readiness

**Phase 9 Plan 3 Prerequisites Met:**
- ✅ Migration timeout enforced in ControllerPublishVolume
- ✅ Error messages include actionable guidance
- ✅ RWO grace period documented as distinct from RWX migration

**Phase 9 Plan 4 Prerequisites Met:**
- ✅ Timeout enforcement ready for metrics instrumentation
- ✅ Error paths ready for observability hooks

**Integration Points for Plans 3-4:**
```go
// Plan 3 will add device-in-use checks that complement timeout enforcement
if existing.IsMigrationTimedOut() {
    // Timeout check (Plan 2) prevents indefinite dual-attach
    return timeout_error
}
// Device check (Plan 3) detects active usage
if deviceInUse(device) {
    return busy_error
}

// Plan 4 will add metrics for timeout occurrences
if existing.IsMigrationTimedOut() {
    metrics.RecordMigrationTimeout(volumeID, elapsed)
    return timeout_error
}
```

**Known Limitations:**
- No metrics for timeout occurrences yet (Phase 10 will add)
- Timeout enforcement relies on AttachmentManager state (in-memory)
  - Lost on controller restart (acceptable - migration retries)

**Blockers:** None

**Concerns:**
- Default 5 minute timeout not validated on real KubeVirt migrations
  - Mitigation: User-configurable via StorageClass parameter
  - Research suggests 5min is conservative (typical: 30s-2min)

## Files Changed

**Modified:**
- `pkg/driver/controller.go` - Added timeout check and documentation

## Commits

| Commit  | Message                                                      | Files |
| ------- | ------------------------------------------------------------ | ----- |
| d627bf7 | feat(09-02): add migration timeout enforcement in ControllerPublishVolume | 1     |

**Total:** 1 commit, 1 file modified

## Duration

**Started:** 2026-02-03T14:56:29Z
**Completed:** 2026-02-03T14:58:36Z
**Duration:** 127 seconds (2m 7s)

---

*Plan executed successfully. Ready for Phase 9 Plan 3 (Device-in-Use Detection).*
