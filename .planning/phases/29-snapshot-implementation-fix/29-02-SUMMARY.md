---
phase: 29-snapshot-implementation-fix
plan: 02
subsystem: storage
tags: [rds, snapshot, csi, controller, idempotency, copy-from, snapshotid]

# Dependency graph
requires:
  - phase: 29-snapshot-implementation-fix
    plan: 01
    provides: Rewritten snapshot SSH commands, updated SnapshotInfo/CreateSnapshotOptions types, GenerateSnapshotIDFromSource, ExtractSourceVolumeIDFromSnapshotID
provides:
  - CSI CreateSnapshot with deterministic IDs (snap-<source-uuid>-at-<hash>) satisfying idempotency
  - CSI ListSnapshots with defense-in-depth source volume fallback via ExtractSourceVolumeIDFromSnapshotID
  - GenerateSnapshotID(csiName, sourceVolumeID) deterministic function in pkg/utils/snapshotid.go
  - Updated snapshotIDPattern accepting both timestamp and hex hash suffixes
  - TestCreateSnapshotIdempotency unit test verifying same (name,source) returns same ID
affects:
  - e2e snapshot idempotency test (TC-08.4) now semantically correct

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "GenerateSnapshotID(csiName, sourceVolumeID): snap-<source-uuid>-at-<10-hex-hash> using UUID v5 of CSI name"
    - "Deterministic snapshot IDs satisfy CSI idempotency without storing CSI name in RDS"
    - "ListSnapshots fallback: derive SourceVolume from snapshot name when field is empty"
    - "snapshotIDPattern accepts [a-f0-9]+ suffix (both decimal timestamps and hex hashes)"

key-files:
  created: []
  modified:
    - pkg/utils/snapshotid.go
    - pkg/driver/controller.go
    - pkg/driver/controller_test.go
    - pkg/rds/commands_test.go

key-decisions:
  - "GenerateSnapshotID uses UUID v5 hash of CSI snapshot name (not timestamp) for deterministic CSI idempotency"
  - "snapshotIDPattern relaxed from \\d+ to [a-f0-9]+ to accept both decimal timestamps and hex hash suffixes"
  - "CreateSnapshot reads base path via paramVolumePath constant (not 'basePath' string literal) for consistency"
  - "ListSnapshots adds fallback: ExtractSourceVolumeIDFromSnapshotID when SourceVolume is empty (defense-in-depth)"
  - "GenerateSnapshotIDFromSource retained for timestamp-based ID generation; non-idempotent by design"

patterns-established:
  - "Snapshot ID generation: deterministic = GenerateSnapshotID, unique-per-call = GenerateSnapshotIDFromSource"

# Metrics
duration: 10min
completed: 2026-02-18
---

# Phase 29 Plan 02: CSI Controller Snapshot RPCs Summary

**Deterministic snapshot IDs (snap-<source-uuid>-at-<hash>) satisfying CSI idempotency; ListSnapshots source fallback via name parsing; all 10 packages passing**

## Performance

- **Duration:** 10 min
- **Started:** 2026-02-18T02:41:04Z
- **Completed:** 2026-02-18T02:50:28Z
- **Tasks:** 2 (combined into 1 commit due to tight coupling between snapshotid.go and controller.go)
- **Files modified:** 4

## Accomplishments

- Added `GenerateSnapshotID(csiName, sourceVolumeID)` — deterministic snapshot ID generation using UUID v5 of the CSI snapshot name as suffix, satisfying CSI idempotency (same call → same ID) while embedding source lineage
- Fixed `CreateSnapshot` to use deterministic IDs (was timestamp-based from Plan 01 deviation; Plan 02 corrects this to proper idempotency semantics) and consistent `paramVolumePath` parameter extraction
- Added defense-in-depth source volume fallback in `ListSnapshots` using `ExtractSourceVolumeIDFromSnapshotID` when `SnapshotInfo.SourceVolume` is empty

## Task Commits

Combined into one commit (type changes and controller RPC updates are tightly coupled):

1. **Tasks 1+2: Update snapshot ID generation and controller RPCs** - `0151877` (feat)

**Plan metadata:** (created after this section)

## Files Created/Modified

- `pkg/utils/snapshotid.go` - Added `GenerateSnapshotID(csiName, sourceVolumeID)` deterministic function; updated `snapshotIDPattern` from `\d+` to `[a-f0-9]+`; reorganized function docs; retained `GenerateSnapshotIDFromSource` for non-idempotent timestamp IDs
- `pkg/driver/controller.go` - Switched `CreateSnapshot` from `GenerateSnapshotIDFromSource` to `GenerateSnapshotID`; fixed base path to use `paramVolumePath` constant; added `ExtractSourceVolumeIDFromSnapshotID` fallback in `ListSnapshots`; removed stale "Btrfs" comments
- `pkg/driver/controller_test.go` - Added `TestCreateSnapshotIdempotency` verifying same (name,source) = same ID; updated stale test comment and test case name; fixed "Btrfs snapshot" comment to "CoW snapshot"
- `pkg/rds/commands_test.go` - Added deterministic hash format test case to `TestValidateSnapshotID`

## Decisions Made

- **Deterministic IDs over timestamps:** Plan 01 used `GenerateSnapshotIDFromSource` (timestamp-based, non-idempotent). Plan 02 switches to `GenerateSnapshotID(csiName, sourceVolumeID)` using UUID v5 of CSI name as suffix. This satisfies CSI idempotency requirements (external-snapshotter and CSI sanity tests both require same name → same ID).
- **Pattern relaxed to `[a-f0-9]+`:** Updated `snapshotIDPattern` from `\d+` to `[a-f0-9]+` to accept both legacy Unix timestamp suffixes (all decimal digits are valid hex) and new deterministic hex hash suffixes. Backward compatible.
- **Defense-in-depth in ListSnapshots:** Even though `pkg/rds/commands.go` already derives `SourceVolume` from snapshot name via `ExtractSourceVolumeIDFromSnapshotID`, the controller now also does it as a fallback when `SourceVolume` is empty. Belt and suspenders.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan 01 introduced non-idempotent timestamp IDs that break CSI semantics**
- **Found during:** Task 1 analysis
- **Issue:** Plan 01 used `GenerateSnapshotIDFromSource` (timestamp-based) as a blocking fix to compilation errors. While it compiled, it violates CSI CreateSnapshot idempotency semantics. The e2e TC-08.4 test expects same (name,source) → same snapshot ID on repeat calls.
- **Fix:** Added `GenerateSnapshotID(csiName, sourceVolumeID)` with deterministic suffix (UUID v5 of CSI name). Updated `CreateSnapshot` to use this instead of the timestamp-based function. Updated test naming to reflect deterministic semantics.
- **Files modified:** `pkg/utils/snapshotid.go`, `pkg/driver/controller.go`, `pkg/driver/controller_test.go`
- **Verification:** `TestCreateSnapshotIdempotency` passes; all snapshot tests pass
- **Committed in:** 0151877

---

**Total deviations:** 1 auto-fixed (Rule 1 - semantic bug in Plan 01's timestamp-based approach)
**Impact on plan:** Fix was necessary for CSI correctness. No scope creep.

## Issues Encountered

- Pre-existing race condition in `pkg/rds` tests under `-race` flag (unrelated to this plan's changes — confirmed by running same tests at previous commit). Tests pass without `-race` and in isolation.

## Next Phase Readiness

- All CSI controller snapshot RPCs updated and semantically correct
- Phase 29 Plans 01+02 complete: SSH backend + CSI controller layer both rewritten
- Remaining Phase 29 work: Plans 03+ (if any) or move to Phase 30
- The e2e mock server (`test/mock/rds_server.go`) still uses old Btrfs subvolume SSH commands — this needs to be updated for e2e tests to pass, but is separate work

## Self-Check: PASSED

- pkg/utils/snapshotid.go: FOUND (GenerateSnapshotID function at line 34)
- pkg/driver/controller.go: FOUND (uses GenerateSnapshotID, paramVolumePath, ExtractSourceVolumeIDFromSnapshotID)
- pkg/driver/controller_test.go: FOUND (TestCreateSnapshotIdempotency at line 2420)
- pkg/rds/commands_test.go: FOUND (deterministic hash format test case)
- Commit 0151877: FOUND
- btrfs in controller.go: 0 references (CLEAN)
- FSLabel in controller.go: 0 references (CLEAN)
- getBtrfsFSLabel in codebase: 0 source code references (CLEAN)
- All 10 packages passing tests: CONFIRMED (go test ./pkg/...)

---
*Phase: 29-snapshot-implementation-fix*
*Completed: 2026-02-18*
