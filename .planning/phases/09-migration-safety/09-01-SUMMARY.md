---
phase: 09
plan: 01
subsystem: attachment
tags: [migration, timeout, parameters, rwx]
requires:
  - 08-02  # AttachmentState with AccessMode and multi-node support
provides:
  - migration-timeout-tracking
  - migration-timeout-parsing
  - migration-state-detection
affects:
  - 09-02  # Will use IsMigrating() and IsMigrationTimedOut()
  - 09-03  # Will reference migration timeout in enforcement logic
tech-stack:
  added: []
  patterns:
    - timeout-tracking-in-state
    - parameter-validation-with-clamping
decisions:
  - id: 09-01-01
    choice: Migration timeout stored in AttachmentState
    rationale: Allows per-volume timeout based on StorageClass parameters
  - id: 09-01-02
    choice: Default 5 minute timeout with 30s-1h range
    rationale: Balances realistic migration time vs preventing indefinite dual-attach
  - id: 09-01-03
    choice: Pass timeout via VolumeContext
    rationale: Parameters flow from CreateVolume to ControllerPublishVolume
key-files:
  created: []
  modified:
    - pkg/attachment/types.go
    - pkg/attachment/manager.go
    - pkg/driver/params.go
    - pkg/driver/controller.go
    - pkg/attachment/manager_test.go
metrics:
  tasks: 3
  commits: 4
  duration: 263s
  files_modified: 5
completed: 2026-02-03
---

# Phase 09 Plan 01: Migration Timeout Tracking Summary

**One-liner:** Added migration start time tracking and configurable timeout parsing (30s-1h, default 5min) to AttachmentState

## What Was Built

Extended the attachment tracking system to support migration-specific timeouts:

1. **AttachmentState Migration Fields** (Task 1)
   - Added `MigrationStartedAt *time.Time` to track dual-attach start
   - Added `MigrationTimeout time.Duration` for configurable timeout
   - Added `IsMigrating()` helper to check migration state
   - Added `IsMigrationTimedOut()` helper to check timeout exceeded

2. **ParseMigrationTimeout Parameter Parser** (Task 2)
   - Parses `migrationTimeoutSeconds` from StorageClass parameters
   - Default: 5 minutes (300 seconds)
   - Valid range: 30 seconds (min) to 1 hour (max)
   - Validation with warnings for invalid values
   - Automatic clamping to safe bounds

3. **Migration Tracking in AddSecondaryAttachment** (Task 3)
   - Updated signature to accept `migrationTimeout` parameter
   - Records `MigrationStartedAt` timestamp when secondary attaches
   - Stores `MigrationTimeout` for timeout enforcement
   - `RemoveNodeAttachment` clears migration state when down to 1 node
   - Added `ClearMigrationState()` method for manual cleanup

4. **Integration with CreateVolume and ControllerPublishVolume**
   - CreateVolume parses timeout and adds to VolumeContext
   - ControllerPublishVolume extracts timeout from VolumeContext
   - Timeout flows through entire volume lifecycle

## How It Works

**Volume Creation:**
```yaml
StorageClass:
  parameters:
    migrationTimeoutSeconds: "600"  # 10 minutes
```

CreateVolume parses this and adds to VolumeContext for later use.

**Secondary Attachment (Migration Start):**
```go
// When KubeVirt starts live migration
am.AddSecondaryAttachment(ctx, volumeID, targetNode, 10*time.Minute)
// Sets:
//   - MigrationStartedAt = time.Now()
//   - MigrationTimeout = 10 minutes
```

**Timeout Check (Phase 9 Plan 2 will use):**
```go
if state.IsMigrating() && state.IsMigrationTimedOut() {
    // Migration exceeded timeout - reject new operations
}
```

**Migration Completion:**
```go
// When source node detaches
am.RemoveNodeAttachment(ctx, volumeID, sourceNode)
// Automatically clears MigrationStartedAt and MigrationTimeout
```

## Key Decisions Made

### Decision 09-01-01: Migration Timeout in AttachmentState
**Choice:** Store migration timeout per-volume in AttachmentState

**Context:** Need to track timeout for each volume independently based on StorageClass

**Alternatives Considered:**
- Global timeout setting - too inflexible for different workload needs
- Calculate timeout in check logic - loses StorageClass configuration intent

**Why This Way:** Per-volume timeout allows different StorageClasses to have different migration timeouts (e.g., large VM volumes get longer timeout)

### Decision 09-01-02: 5 Minute Default with 30s-1h Range
**Choice:** Default 5 minutes, clamp to 30 seconds minimum, 1 hour maximum

**Context:** Need realistic bounds to prevent both too-short (causing false timeouts) and too-long (indefinite dual-attach)

**Alternatives Considered:**
- No maximum - could allow indefinite dual-attach on misconfiguration
- Shorter default (2-3 min) - might be too tight for large VM migrations
- Longer default (10 min) - would delay timeout detection unnecessarily

**Why This Way:**
- 5 minutes handles typical KubeVirt VM migration (30s-2min) with safety margin
- 30s minimum prevents unrealistic timeouts
- 1 hour maximum prevents indefinite dual-attach scenarios
- Based on research findings: typical migrations complete in 30s-2min

### Decision 09-01-03: Pass Timeout via VolumeContext
**Choice:** Add migrationTimeoutSeconds to VolumeContext in CreateVolume

**Context:** ControllerPublishVolume doesn't have direct access to StorageClass parameters

**Alternatives Considered:**
- Query StorageClass in ControllerPublishVolume - expensive K8s API call per attach
- Hardcode timeout - loses StorageClass configurability

**Why This Way:** VolumeContext is designed for passing volume-specific parameters from controller to node operations

## Verification Performed

### Build Verification
```bash
go build ./...
# ✓ Compiled successfully
```

### Test Verification
```bash
make test
# ✓ All attachment tests passed
# ✓ All driver tests passed
# ✓ Updated test calls to include timeout parameter
```

### Manual Verification
```bash
# Verified new fields exist
grep "MigrationStartedAt" pkg/attachment/types.go
grep "ParseMigrationTimeout" pkg/driver/params.go

# Verified helper methods
grep "IsMigrating\|IsMigrationTimedOut" pkg/attachment/types.go

# Verified integration
grep "migrationTimeoutSeconds" pkg/driver/controller.go
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated AddSecondaryAttachment calls in tests**
- **Found during:** Test execution after Task 3
- **Issue:** Tests failing because AddSecondaryAttachment signature changed
- **Fix:** Updated all test calls to include `5*time.Minute` timeout parameter
- **Files modified:** pkg/attachment/manager_test.go
- **Commit:** 69b619f

**2. [Rule 2 - Missing Critical] Connected timeout parsing to ControllerPublishVolume**
- **Found during:** Build after Task 3
- **Issue:** AddSecondaryAttachment call in controller.go missing timeout parameter
- **Fix:** Added VolumeContext parsing and timeout extraction in ControllerPublishVolume
- **Files modified:** pkg/driver/controller.go
- **Commit:** 69b619f

**3. [Rule 2 - Missing Critical] Added timeout to VolumeContext in CreateVolume**
- **Found during:** Implementation planning
- **Issue:** Migration timeout needed to flow from CreateVolume to ControllerPublishVolume
- **Fix:** Parse timeout in CreateVolume and add to VolumeContext
- **Files modified:** pkg/driver/controller.go
- **Commit:** 69b619f

## Test Coverage

### New Test Updates
- Updated `TestAttachmentManager_AddSecondaryAttachment` test cases to pass timeout
- All dual-attach tests now include migration timeout parameter
- Tests verify idempotent behavior with timeout tracking

### Existing Tests
- All existing attachment tests pass unchanged
- Controller tests pass with new VolumeContext field
- No regression in any package

## Next Phase Readiness

**Phase 9 Plan 2 Prerequisites Met:**
- ✅ AttachmentState has `MigrationStartedAt` field
- ✅ AttachmentState has `MigrationTimeout` field
- ✅ `IsMigrating()` helper available for state detection
- ✅ `IsMigrationTimedOut()` helper available for timeout check
- ✅ ParseMigrationTimeout provides validated timeout values

**Integration Points for Plan 2:**
```go
// Plan 2 will use these to distinguish migration from conflict:
if state.IsMigrating() {
    if state.IsMigrationTimedOut() {
        // Migration exceeded timeout - treat as conflict
    } else {
        // Valid migration in progress - allow
    }
}
```

**Known Limitations:**
- Timeout tracking is in-memory only (lost on controller restart)
  - Acceptable: Migration will timeout and retry after restart
- No metrics for migration duration yet (Phase 10 will add)

**Blockers:** None

**Concerns:**
- Default 5 minute timeout not validated on real hardware yet
  - Mitigation: User-configurable via StorageClass parameter
  - Research suggests 5min is conservative (typical: 30s-2min)

## Files Changed

**Modified:**
- `pkg/attachment/types.go` - Added migration fields and helpers
- `pkg/attachment/manager.go` - Track migration in AddSecondaryAttachment
- `pkg/driver/params.go` - Added ParseMigrationTimeout
- `pkg/driver/controller.go` - Integrated timeout parsing and passing
- `pkg/attachment/manager_test.go` - Updated test calls

## Commits

| Commit  | Message                                                 | Files |
| ------- | ------------------------------------------------------- | ----- |
| 464e988 | feat(09-01): add migration tracking fields to AttachmentState | 1     |
| ba4c4a5 | feat(09-01): add ParseMigrationTimeout parameter parser | 1     |
| 6d0a122 | feat(09-01): track migration timing in AddSecondaryAttachment | 1     |
| 69b619f | fix(09-01): pass migration timeout to AddSecondaryAttachment | 2     |

**Total:** 4 commits, 5 files modified

## Duration

**Started:** 2026-02-03T14:49:14Z
**Completed:** 2026-02-03T14:53:37Z
**Duration:** 263 seconds (4m 23s)

---

*Plan executed successfully. Ready for Phase 9 Plan 2 (Migration Timeout Enforcement).*
