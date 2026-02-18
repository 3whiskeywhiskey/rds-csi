---
phase: 30-snapshot-validation
plan: 01
subsystem: testing
tags: [mock, snapshot, copy-from, rds, ssh]

# Dependency graph
requires:
  - phase: 29-snapshot-implementation-fix
    provides: "SSH commands.go with /disk add copy-from= snapshot semantics"
provides:
  - "Mock RDS server handles /disk add copy-from= for snapshot creation and restore"
  - "Snapshot disk entries stored in s.snapshots (not s.volumes, not NVMe-exported)"
  - "handleDiskRemove checks both s.volumes and s.snapshots (no silent idempotency on snapshots)"
  - "formatSnapshotDetail omits nvme-tcp-export/port/nqn fields"
  - "6-subtest TestMockRDS_SnapshotCopyFrom validates full copy-from flow"
affects:
  - 30-snapshot-validation (plans 02+)
  - test/sanity
  - test/e2e

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "copy-from=[find slot=<name>] for snapshot disk creation in RouterOS CLI format"
    - "Separate snapshots map in mock (not volumes) for correct NVMe export semantics"
    - "Destination slot extraction uses last non-source slot= match to avoid copy-from=[find slot=...] collision"

key-files:
  created: []
  modified:
    - test/mock/rds_server.go
    - test/mock/rds_server_test.go

key-decisions:
  - "Mock snapshot state uses Slot (not Name) as key and struct field, aligning with copy-from disk semantics"
  - "handleDiskAddCopyFrom distinguishes snapshot vs restore by presence of nvme-tcp-export=yes"
  - "extractParam bug fixed: slot= inside copy-from=[find slot=...] was shadowing destination slot= — fixed with last-non-source match"
  - "formatSnapshotDetail is a separate function from formatDiskDetail to enforce no NVMe export fields on snapshots"

patterns-established:
  - "Test pattern: use utils.GenerateSnapshotID(csiName, sourceVolID) for valid snapshot IDs in tests (raw strings fail validation)"
  - "Test helper: setupSnapshotTestClient() creates mock server + rds.RDSClient with cleanup function"

# Metrics
duration: 7min
completed: 2026-02-18
---

# Phase 30 Plan 01: Snapshot Validation Summary

**Mock RDS server rewritten to use /disk add copy-from semantics for snapshots, replacing obsolete Btrfs subvolume handlers; 6 subtests validate end-to-end snapshot lifecycle**

## Performance

- **Duration:** 7 min
- **Started:** 2026-02-18T04:17:20Z
- **Completed:** 2026-02-18T04:24:40Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Removed all 4 Btrfs subvolume handlers (add/remove/print/print-detail) that no longer matched Phase 29 SSH commands
- Updated MockSnapshot struct and mock server to store copy-from snapshot entries in s.snapshots with Slot/FilePath/FileSizeBytes/SourceVolume/CreatedAt
- handleDiskRemove now correctly removes from s.snapshots (previously silently succeeded on unknown slots, which would leave snapshot state intact)
- New formatSnapshotDetail() emits no nvme-tcp-export/port/nqn fields (snapshots are not NVMe-exported)
- 6 subtests in TestMockRDS_SnapshotCopyFrom cover the full lifecycle against a real rds.RDSClient connection

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace Btrfs subvolume handlers with copy-from snapshot semantics** - `f2c0eca` (feat)
2. **Task 2: Add snapshot copy-from unit tests for mock RDS server** - `8e3a434` (test)

## Files Created/Modified

- `test/mock/rds_server.go` - Replaced Btrfs handlers with copy-from snapshot semantics; new handleDiskAddCopyFrom(); updated handleDiskRemove/handleDiskPrintDetail; new formatSnapshotDetail(); updated MockSnapshot struct
- `test/mock/rds_server_test.go` - Added TestMockRDS_SnapshotCopyFrom with 6 subtests; added setupSnapshotTestClient helper

## Decisions Made

- Used `utils.GenerateSnapshotID()` in tests instead of raw strings — raw strings like `snap-copytest1-at-1234567890` fail ValidateSnapshotID since they don't contain a full UUID
- `handleDiskAddCopyFrom` detects restore vs snapshot creation by presence of `nvme-tcp-export=yes` — clean and matches what RestoreSnapshot sends
- Fixed pre-existing `extractParam` limitation: `slot=` regex matched inside `copy-from=[find slot=...]` before destination slot — fixed with last-non-source-slot scan

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed extractParam picking wrong slot= in copy-from commands**
- **Found during:** Task 2 (running tests)
- **Issue:** `extractParam(command, "slot")` matched `slot=pvc-...` inside `copy-from=[find slot=pvc-...]` before the destination `slot=snap-...` at end of command, causing destination slot to be wrong
- **Fix:** In `handleDiskAddCopyFrom`, replaced `extractParam(command, "slot")` with a dedicated regex that finds all `slot=` matches and picks the last non-source one
- **Files modified:** `test/mock/rds_server.go`
- **Verification:** TestMockRDS_SnapshotCopyFrom all 6 subtests pass
- **Committed in:** `8e3a434` (included in task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - Bug)
**Impact on plan:** Essential fix for copy-from snapshot creation to work correctly. No scope creep.

## Issues Encountered

- None beyond the extractParam bug documented above.

## Next Phase Readiness

- Mock server now correctly handles all snapshot SSH command formats from Phase 29
- CSI sanity tests and e2e tests can use mock for snapshot operations with realistic behavior
- Ready for Plan 02: snapshot CSI controller integration testing

---
*Phase: 30-snapshot-validation*
*Completed: 2026-02-18*

## Self-Check: PASSED

- test/mock/rds_server.go: FOUND
- test/mock/rds_server_test.go: FOUND
- .planning/phases/30-snapshot-validation/30-01-SUMMARY.md: FOUND
- commit f2c0eca: FOUND
- commit 8e3a434: FOUND
