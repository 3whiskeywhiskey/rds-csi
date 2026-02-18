# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-17)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 31 - Hardware Validation (v0.11.0)

## Current Position

Phase: 31 of 32 (Hardware Validation)
Plan: 1 of TBD in current phase
Status: Phase 31 Plan 01 complete
Last activity: 2026-02-18 — Phase 31 Plan 01 complete (scheduled snapshots Helm CronJob template)

Progress: v0.10.0 [██████████] 100% (19/19 plans) | v0.11.0 [██████░░░░] 83% (5/6 plans)

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
| v0.11.0 Data Protection | 29-32 | 4/6 | 🚧 In progress |

*Updated: 2026-02-18*

| Phase 29-snapshot-implementation-fix P01 | 12 min | 2 tasks | 7 files |
| Phase 29-snapshot-implementation-fix P02 | 10 min | 2 tasks | 4 files |
| Phase 30-snapshot-validation P01 | 7 min | 2 tasks | 2 files |
| Phase 30-snapshot-validation P02 | 35 min | 2 tasks | 6 files |
| Phase 31-scheduled-snapshots P01 | 3 min | 3 tasks | 5 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- v0.11.0: `/disk add copy-from` replaces Btrfs subvolume snapshots (file-backed disks aren't subvolumes)
- v0.11.0: Btrfs filesystem label is `storage-pool-fs` (not `storage-pool`) in RouterOS
- v0.11.0: Snapshot disks must NOT be NVMe-exported (immutable, write-protected by omission)
- Quick-006 (2026-02-12): blkid exit 1 treated as error to prevent data loss on NVMe-oF reconnect
- [Phase 29-snapshot-implementation-fix P01]: Snapshot IDs use snap-<source-uuid>-at-<unix-timestamp> format embedding source lineage directly in slot name
- [Phase 29-snapshot-implementation-fix P01]: Snapshot disks created without NVMe export flags (not network-exported, immutable backing files)
- [Phase 29-snapshot-implementation-fix P01]: copy-from uses [find slot=<name>] to reference source by slot (more reliable than file path)
- [Phase 29-snapshot-implementation-fix P02]: GenerateSnapshotID(csiName, sourceVolumeID) uses UUID v5 hash of CSI name for deterministic suffix, satisfying CSI idempotency
- [Phase 29-snapshot-implementation-fix P02]: snapshotIDPattern accepts [a-f0-9]+ suffix (both decimal timestamps and hex hashes, backward compatible)
- [Phase 30-snapshot-validation P01]: Mock snapshot state uses Slot (not Name) as key and struct field, aligning with copy-from disk semantics
- [Phase 30-snapshot-validation P01]: formatSnapshotDetail is a separate function from formatDiskDetail to enforce no NVMe export fields on snapshots
- [Phase 30-snapshot-validation P02]: GenerateSnapshotID bases ID only on CSI snapshot name (UUID v5), NOT source volume — CSI spec requires same name always yields same ID regardless of source
- [Phase 30-snapshot-validation P02]: source-volume field added to mock formatSnapshotDetail output; parseSnapshotInfo reads it directly (fallback extraction via ExtractSourceVolumeIDFromSnapshotID removed — slot UUID is now name-hash not source UUID)
- [Phase 30-snapshot-validation P02]: TC-08 SSH verification changed to /disk print detail where slot~"snap-" (copy-from file-backed disk, not Btrfs subvolume)
- [Phase 31-scheduled-snapshots P01]: dig helper for nil-safe retention value access in Helm template (avoids nil pointer on --set without retention sub-object)
- [Phase 31-scheduled-snapshots P01]: Temp file at /tmp/snapshots.txt for retention while loop to avoid pipe subshell DELETED counter bug
- [Phase 31-scheduled-snapshots P01]: Namespaced Role (not ClusterRole) for snapshot-scheduler since VolumeSnapshots are namespace-scoped

### Pending Todos

None.

### Blockers/Concerns

- RDS restart testing affects site networking — requires confidence before running (document before executing)
- Pre-existing race condition in pkg/rds tests under -race flag (TestReconnection_WithBackoff, TestOnReconnectCallback) — unrelated to snapshot work
- Phase 31 (hardware validation) requires manual execution against real RDS at 10.42.68.1 — TC-08 is now documented and ready

## Session Continuity

Last session: 2026-02-18
Stopped at: Completed 31-01-PLAN.md — Helm scheduled snapshots CronJob template with retention cleanup
Resume file: None
Next action: Phase 31 remaining plans — hardware validation execution against real RDS hardware.
