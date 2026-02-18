---
phase: quick-007
plan: 01
subsystem: testing
tags: [snapshot, csi, mock, tech-debt, cleanup]

# Dependency graph
requires:
  - phase: phase-30-snapshot-validation
    provides: "GenerateSnapshotID name-hash format, source-volume= field in mock, ExtractSourceVolumeIDFromSnapshotID deprecated"
provides:
  - "formatSnapshotDetail emits creation-time= in RouterOS format"
  - "parseSnapshotInfo reads creation-time= only (no timestamp fallback)"
  - "ListSnapshots uses s.SourceVolume directly (no ExtractSourceVolumeIDFromSnapshotID fallback)"
  - "snapshotid.go has zero dead code (GenerateSnapshotIDFromSource/ExtractSourceVolumeIDFromSnapshotID/ExtractTimestampFromSnapshotID removed)"
affects: [snapshot-operations, mock-server, controller-list-snapshots]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Mock server emits all required fields (including creation-time=) matching production RouterOS output format"
    - "parseSnapshotInfo relies solely on creation-time= field for timestamps — no slot-name-based fallbacks"
    - "ListSnapshots filters on SourceVolume field directly — source must come from disk output, not derived from ID"

key-files:
  created: []
  modified:
    - test/mock/rds_server.go
    - pkg/rds/commands.go
    - pkg/driver/controller.go
    - pkg/utils/snapshotid.go

key-decisions:
  - "Dead code removal: GenerateSnapshotIDFromSource, ExtractSourceVolumeIDFromSnapshotID, ExtractTimestampFromSnapshotID deleted from snapshotid.go — all callers were removed in phase 29-30 when snapshot ID format changed from source-uuid-based to name-hash-based"
  - "No ListSnapshots fallback: snapshot IDs embed name-hash (not source UUID), so ExtractSourceVolumeIDFromSnapshotID would always return wrong source — removing the fallback forces correct behavior (source-volume= field from disk output)"
  - "creation-time= field added to mock using strings.ToLower(CreatedAt.Format('Jan/02/2006 15:04:05')) to produce RouterOS lowercase month abbreviation format (parseRouterOSTime title-cases internally)"

patterns-established:
  - "Mock output must include all fields that production parseSnapshotInfo reads — gaps cause silent zero-value timestamps"

# Metrics
duration: 12min
completed: 2026-02-18
---

# Quick Task 007: Snapshot Tech Debt Cleanup Summary

**Mock emits creation-time= field in RouterOS format, three dead snapshot ID functions removed, and ListSnapshots fallback that returned wrong source IDs deleted**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-02-18T19:40:00Z
- **Completed:** 2026-02-18T19:52:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added `creation-time=` field to `formatSnapshotDetail` in mock server using RouterOS lowercase month format — this fixed TC-08.6 (snapshot creation timestamp test) which was failing pre-change
- Removed `ExtractTimestampFromSnapshotID` fallback from `parseSnapshotInfo` — slot name suffix is a name-derived hash (not a Unix timestamp), so the fallback always failed for new-format IDs
- Removed `ExtractSourceVolumeIDFromSnapshotID` fallback from `ListSnapshots` — the slot UUID is derived from the CSI snapshot name (not the source volume UUID), so the fallback returned wrong source IDs
- Deleted three dead functions from `snapshotid.go`: `GenerateSnapshotIDFromSource`, `ExtractSourceVolumeIDFromSnapshotID`, `ExtractTimestampFromSnapshotID`
- Cleaned unused imports (`strconv`, `time`) from `snapshotid.go`

## Task Commits

Each task was committed atomically:

1. **Task 1: Add creation-time= to mock and remove timestamp fallback from parseSnapshotInfo** - `e210eef` (fix)
2. **Task 2: Remove ListSnapshots fallback and delete dead code from snapshotid.go** - `3dd73e6` (refactor)

**Plan metadata:** (this summary commit)

## Files Created/Modified
- `test/mock/rds_server.go` - `formatSnapshotDetail` emits `creation-time=feb/18/2026 14:30:00` format
- `pkg/rds/commands.go` - `parseSnapshotInfo` timestamp extraction simplified to single `parseRouterOSTime` call
- `pkg/driver/controller.go` - `ListSnapshots` source volume filter uses `s.SourceVolume` directly
- `pkg/utils/snapshotid.go` - Three dead functions and two unused imports removed (90 lines deleted)

## Decisions Made
- Dead code removal was safe: all three deleted functions had zero callers in production code
- The ListSnapshots fallback was actively harmful (returning name-hash UUIDs as source volume IDs)
- creation-time= in mock uses `strings.ToLower(t.Format("Jan/02/2006 15:04:05"))` because `parseRouterOSTime` already title-cases the first letter during parse

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

Pre-existing failures in `test/e2e/` (state recovery and orphan detection tests, 2 failures) were present before and after this change. My change actually fixed a 3rd pre-existing failure: TC-08.6 (snapshot creation timestamp) was failing because the mock lacked `creation-time=`.

The `TestSSHClientConnect` test in `pkg/rds` hangs indefinitely under `make test` — this is the documented pre-existing race condition in STATE.md. All other tests pass.

## Next Phase Readiness
- Snapshot tech debt fully cleared — mock output matches production field set
- snapshotid.go is lean with only active, maintained functions
- No known blockers for future snapshot work

## Self-Check

Files exist:
- `test/mock/rds_server.go` — contains `creation-time=` field
- `pkg/rds/commands.go` — timestamp fallback removed
- `pkg/driver/controller.go` — ListSnapshots fallback removed
- `pkg/utils/snapshotid.go` — dead functions deleted

Commits exist:
- `e210eef` — fix(quick-007): add creation-time= to mock and remove timestamp fallback
- `3dd73e6` — refactor(quick-007): remove ListSnapshots fallback and dead code from snapshotid.go

Zero code references to deleted functions in `pkg/`: confirmed via grep (only comment reference remains).

## Self-Check: PASSED

---
*Phase: quick-007*
*Completed: 2026-02-18*
