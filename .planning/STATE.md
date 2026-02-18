# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-17)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 30 - Snapshot Validation (v0.11.0)

## Current Position

Phase: 30 of 32 (Snapshot Validation)
Plan: 1 of 2 in current phase
Status: Plan 01 complete
Last activity: 2026-02-18 — Phase 30 Plan 01 complete (mock RDS server copy-from snapshot semantics)

Progress: v0.10.0 [██████████] 100% (19/19 plans) | v0.11.0 [███░░░░░░░] 50% (3/6 plans)

## Performance Metrics

**Velocity:**
- Total plans completed: 115 (v0.1.0-v0.10.0)
- v0.10.0 plans completed: 19/19 (100%)
- Average duration: ~5 min per plan (v0.10.0)
- Total execution time: ~1.5 hours (v0.10.0)

**By Milestone:**

| Milestone | Phases | Plans | Status |
|-----------|--------|-------|--------|
| v0.1.0-v0.8.0 | 1-21 | 79/79 | ✅ Shipped 2026-02-04 |
| v0.9.0 Production Readiness | 22-25.2 | 17/17 | ✅ Shipped 2026-02-06 |
| v0.10.0 Feature Enhancements | 26-28 | 19/19 | ✅ Shipped 2026-02-06 |
| v0.11.0 Data Protection | 29-32 | 3/6 | 🚧 In progress |

*Updated: 2026-02-18*

| Phase 29-snapshot-implementation-fix P01 | 12 min | 2 tasks | 7 files |
| Phase 29-snapshot-implementation-fix P02 | 10 min | 2 tasks | 4 files |
| Phase 30-snapshot-validation P01 | 7 min | 2 tasks | 2 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- v0.11.0: `/disk add copy-from` replaces Btrfs subvolume snapshots (file-backed disks aren't subvolumes)
- v0.11.0: Btrfs filesystem label is `storage-pool-fs` (not `storage-pool`) in RouterOS
- v0.11.0: Snapshot disks must NOT be NVMe-exported (immutable, write-protected by omission)
- Quick-006 (2026-02-12): blkid exit 1 treated as error to prevent data loss on NVMe-oF reconnect
- v0.10.0 snapshot code in pkg/rds/commands.go uses wrong Btrfs subvolume approach — full rewrite needed
- [Phase 29-snapshot-implementation-fix P01]: Snapshot IDs use snap-<source-uuid>-at-<unix-timestamp> format embedding source lineage directly in slot name
- [Phase 29-snapshot-implementation-fix P01]: Snapshot disks created without NVMe export flags (not network-exported, immutable backing files)
- [Phase 29-snapshot-implementation-fix P01]: copy-from uses [find slot=<name>] to reference source by slot (more reliable than file path)
- [Phase 29-snapshot-implementation-fix P02]: GenerateSnapshotID(csiName, sourceVolumeID) uses UUID v5 hash of CSI name for deterministic suffix, satisfying CSI idempotency
- [Phase 29-snapshot-implementation-fix P02]: snapshotIDPattern accepts [a-f0-9]+ suffix (both decimal timestamps and hex hashes, backward compatible)
- [Phase 29-snapshot-implementation-fix P02]: ListSnapshots derives SourceVolume from snapshot name as fallback when field is empty
- [Phase 30-snapshot-validation]: Mock snapshot state uses Slot (not Name) as key and struct field, aligning with copy-from disk semantics
- [Phase 30-snapshot-validation]: formatSnapshotDetail is a separate function from formatDiskDetail to enforce no NVMe export fields on snapshots

### Pending Todos

None.

### Blockers/Concerns

- Phase 29 Plans 01+02 complete: SSH backend + CSI controller layer both rewritten
- RDS restart testing affects site networking — requires confidence before running (document before executing)
- Pre-existing race condition in pkg/rds tests under -race flag (TestReconnection_WithBackoff, TestOnReconnectCallback) — unrelated to snapshot work

## Session Continuity

Last session: 2026-02-18
Stopped at: Completed 30-01-PLAN.md — mock RDS server copy-from snapshot semantics
Resume file: None
Next action: Phase 30 Plan 02 — snapshot CSI integration testing / sanity tests.
