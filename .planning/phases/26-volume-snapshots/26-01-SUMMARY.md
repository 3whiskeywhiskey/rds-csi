---
phase: 26-volume-snapshots
plan: 01
subsystem: storage
tags: [snapshots, btrfs, csi, rds, types, interfaces]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: Basic RDS types and client interface
provides:
  - SnapshotInfo type with all metadata fields
  - Snapshot ID utilities (generation, validation, name-to-ID conversion)
  - RDSClient interface extended with 5 snapshot methods
  - MockClient with full snapshot CRUD operations
  - Stub implementations in sshClient for Plan 26-02
affects: [26-02-snapshot-commands, 26-03-controller-service, snapshot-testing, restore-workflow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Snapshot ID format (snap-<uuid>) mirrors volume ID format (pvc-<uuid>)
    - Idempotent snapshot operations (create, delete)
    - Reuse existing validation patterns (safeSlotPattern, volumeNamespace)

key-files:
  created:
    - pkg/utils/snapshotid.go
  modified:
    - pkg/rds/types.go
    - pkg/rds/client.go
    - pkg/rds/mock.go
    - pkg/rds/commands.go
    - pkg/rds/pool_test.go
    - pkg/reconciler/orphan_reconciler_test.go

key-decisions:
  - Reuse volumeNamespace UUID for SnapshotNameToID (no collision risk)
  - MockClient.CreateSnapshot is idempotent (same name + same source = return existing)
  - MockClient.DeleteSnapshot is idempotent (not found = return nil)
  - sshClient stubs return "not yet implemented" (Plan 26-02 implements)

patterns-established:
  - Snapshot types mirror volume types structure (SnapshotInfo ↔ VolumeInfo)
  - Snapshot ID validation mirrors volume ID validation (strict UUID + fallback)
  - MockClient snapshot methods follow existing patterns (mutex, checkError, return copies)

# Metrics
duration: 4min 19s
completed: 2026-02-06
---

# Phase 26 Plan 01: Snapshot Data Model Foundation Summary

**SnapshotInfo type, snap-<uuid> ID utilities, RDSClient interface extended with 5 snapshot methods, and MockClient with full snapshot CRUD for testing**

## Performance

- **Duration:** 4 minutes 19 seconds
- **Started:** 2026-02-06T05:29:26Z
- **Completed:** 2026-02-06T05:33:45Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- SnapshotInfo type captures name, source volume, size, creation time, read-only flag, and filesystem label
- Snapshot ID utilities (GenerateSnapshotID, ValidateSnapshotID, SnapshotNameToID) mirror volumeid.go patterns
- RDSClient interface extended with CreateSnapshot, DeleteSnapshot, GetSnapshot, ListSnapshots, RestoreSnapshot methods
- MockClient implements all snapshot operations with idempotency (same name + same source = return existing)
- sshClient has stub implementations for Plan 26-02 to implement

## Task Commits

Each task was committed atomically:

1. **Task 1: Snapshot types and ID utilities** - `e00b04e` (feat)
2. **Task 2: RDSClient interface extension and MockClient snapshot support** - `fbe6c05` (feat)

**Fixes during execution:**
- `b1ea34d` (fix) - Added snapshot stubs to orphan_reconciler_test mockRDSClient

**Plan metadata:** (will be committed separately)

## Files Created/Modified
- `pkg/rds/types.go` - Added SnapshotInfo, CreateSnapshotOptions, SnapshotNotFoundError types
- `pkg/utils/snapshotid.go` - New file with GenerateSnapshotID, ValidateSnapshotID, SnapshotNameToID functions
- `pkg/rds/client.go` - Extended RDSClient interface with 5 snapshot methods
- `pkg/rds/mock.go` - Added snapshots map and full snapshot CRUD implementation
- `pkg/rds/commands.go` - Added stub implementations for sshClient (returns "not yet implemented")
- `pkg/rds/pool_test.go` - Added snapshot stubs to mockRDSClient
- `pkg/reconciler/orphan_reconciler_test.go` - Added snapshot stubs to mockRDSClient

## Decisions Made
1. **Reuse volumeNamespace for SnapshotNameToID**: Volume names (from PV names) and snapshot names (from VolumeSnapshot names) are inherently different strings, so UUID namespace collision is impossible
2. **CreateSnapshot idempotency**: Same name + same source = return existing snapshot (allows CSI controller retries)
3. **DeleteSnapshot idempotency**: Not found = return nil (allows cleanup retries)
4. **Strict validation pattern**: Snapshot IDs follow snap-<uuid> format with same security patterns as volume IDs (command injection prevention)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added snapshot stubs to test mockRDSClient implementations**
- **Found during:** Task 2 (Compiling after RDSClient interface extension)
- **Issue:** pool_test.go and orphan_reconciler_test.go have their own mockRDSClient implementations that didn't implement new snapshot methods
- **Fix:** Added 5 snapshot stub methods to both test mocks (return nil)
- **Files modified:** pkg/rds/pool_test.go, pkg/reconciler/orphan_reconciler_test.go
- **Verification:** `go test ./pkg/rds/... -count=1` passes, `go vet ./pkg/...` passes
- **Committed in:** b1ea34d (separate fix commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Fix necessary for compilation. Test mocks need interface compliance. No scope creep.

## Issues Encountered
None - plan executed smoothly.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Snapshot types and interfaces defined, ready for Plan 26-02 (SSH commands)
- MockClient fully functional for snapshot testing in Plan 26-03 (Controller service)
- No blockers for continuing with snapshot implementation
- SnapshotInfo has all 6 required fields: Name, SourceVolume, FileSizeBytes, CreatedAt, ReadOnly, FSLabel
- ValidateSnapshotID rejects command injection characters (security foundation for RouterOS commands)

---
*Phase: 26-volume-snapshots*
*Completed: 2026-02-06*
