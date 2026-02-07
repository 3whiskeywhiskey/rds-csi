---
phase: 26-volume-snapshots
plan: 02
subsystem: storage
tags: [snapshots, btrfs, ssh, routeros, rds]

# Dependency graph
requires:
  - phase: 26-01
    provides: SnapshotInfo types, snapshot ID utilities, RDSClient interface
provides:
  - sshClient snapshot SSH command implementations
  - RouterOS /disk/btrfs/subvolume command wrappers
  - Snapshot parsing functions for RouterOS output
  - Full unit test coverage for snapshot operations
affects: [26-03-controller-service, snapshot-testing, restore-workflow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - RouterOS Btrfs subvolume commands for snapshot lifecycle
    - Idempotent snapshot operations (create returns existing, delete returns nil)
    - Auto-cleanup on partial snapshot creation failures
    - Writable snapshot-of-snapshot for restore workflow

key-files:
  created: []
  modified:
    - pkg/rds/commands.go
    - pkg/rds/commands_test.go

key-decisions:
  - CreateSnapshot uses read-only=yes for immutable snapshots
  - DeleteSnapshot is idempotent (not found = return nil per CSI spec)
  - RestoreSnapshot creates writable clone (no read-only flag) + disk entry
  - parseSnapshotInfo handles missing fields gracefully (controller tracks metadata)
  - ListSnapshots filters snap-* prefix at parse level (defense in depth)

patterns-established:
  - Snapshot parsers follow existing volume parser patterns (normalizeRouterOSOutput, regex extraction)
  - Snapshot operations use same retry logic as volume operations (runCommandWithRetry)
  - Router OS command validation via utils.ValidateSnapshotID before execution

# Metrics
duration: 4min 54s
completed: 2026-02-06
---

# Phase 26 Plan 02: Snapshot SSH Commands Summary

**RouterOS /disk/btrfs/subvolume SSH commands for snapshot lifecycle with idempotent operations and auto-cleanup**

## Performance

- **Duration:** 4 minutes 54 seconds
- **Started:** 2026-02-06T05:31:27Z
- **Completed:** 2026-02-06T05:36:21Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- All 5 sshClient snapshot methods implemented with RouterOS commands
- CreateSnapshot: `/disk/btrfs/subvolume/add read-only=yes` creates immutable snapshots
- DeleteSnapshot: idempotent deletion (not found = success per CSI spec)
- GetSnapshot: queries subvolume metadata via RouterOS
- ListSnapshots: returns only snap-* prefixed subvolumes
- RestoreSnapshot: writable clone + disk entry for NVMe/TCP export
- Parser functions handle RouterOS output with graceful field handling
- Auto-cleanup on partial failures (best-effort removal)
- Full unit test coverage (parsing, MockClient CRUD, validation)

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement sshClient snapshot SSH commands** - `8601a5f` (feat)
2. **Task 2: Unit tests for snapshot command parsing and mock integration** - `d45812c` (test)

## Files Created/Modified
- `pkg/rds/commands.go` - Implemented 5 snapshot methods and 2 parser functions (parseSnapshotInfo, parseSnapshotList)
- `pkg/rds/commands_test.go` - Added 4 test functions (TestParseSnapshotInfo, TestParseSnapshotList, TestMockClientSnapshotOperations, TestValidateSnapshotID)

## Decisions Made
1. **CreateSnapshot auto-cleanup**: On creation failure, attempt to remove partial snapshot via best-effort cleanup command (log warning if cleanup fails)
2. **Router OS field handling**: parseSnapshotInfo leaves SourceVolume and CreatedAt empty (RouterOS may not return these in subvolume output; controller layer will track them in VolumeSnapshotContent annotations)
3. **RestoreSnapshot two-step flow**: Step 1 creates writable Btrfs clone, Step 2 creates /disk entry for NVMe/TCP export (open question documented: exact relationship needs hardware validation per RESEARCH.md)
4. **ListSnapshots filtering**: Filter snap-* prefix in both RouterOS query (`where name~"snap-"`) and parsing (defense in depth)
5. **Empty list return**: parseSnapshotList returns empty slice (not nil) when no snapshots found (Go best practice)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None - plan executed smoothly with RouterOS command patterns following existing volume operation implementations.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SSH command layer complete, ready for Plan 26-03 (Controller service implementation)
- RestoreSnapshot flow documented with open question about Btrfs subvolume-to-disk relationship (flagged for hardware validation)
- MockClient fully tested for snapshot CRUD operations (ready for controller service unit tests)
- All parsers handle RouterOS output format with TODO comments for hardware validation adjustments
- Command injection prevention verified via ValidateSnapshotID (security foundation for controller RPCs)

---
*Phase: 26-volume-snapshots*
*Completed: 2026-02-06*
