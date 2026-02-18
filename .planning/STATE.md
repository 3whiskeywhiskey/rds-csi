# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-17)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Quick tasks (snapshot tech debt cleanup)

## Current Position

Phase: 32 of 32 (Resilience Regression Tests) — COMPLETE
Plan: 1 of 1 in current phase
Status: Phase 32 complete — v0.11.0 milestone COMPLETE
Last activity: 2026-02-18 — Quick-007 complete (snapshot tech debt cleanup: creation-time= added, dead code removed)

Progress: v0.10.0 [██████████] 100% (19/19 plans) | v0.11.0 [██████████] 100% (6/6 plans)

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
| v0.11.0 Data Protection | 29-32 | 6/6 | ✅ Shipped 2026-02-18 |

*Updated: 2026-02-18*

| Phase 29-snapshot-implementation-fix P01 | 12 min | 2 tasks | 7 files |
| Phase 29-snapshot-implementation-fix P02 | 10 min | 2 tasks | 4 files |
| Phase 30-snapshot-validation P01 | 7 min | 2 tasks | 2 files |
| Phase 30-snapshot-validation P02 | 35 min | 2 tasks | 6 files |
| Phase 31-scheduled-snapshots P01 | 3 min | 3 tasks | 5 files |
| Phase 32-resilience-regression-tests P02 | 8 | 2 tasks | 2 files |
| Phase 32-resilience-regression-tests P01 | 6 | 3 tasks | 5 files |
| Quick-007-snapshot-tech-debt | 12 min | 2 tasks | 4 files |

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
- [Phase 32-resilience-regression-tests]: TC-09 uses iptables OUTPUT chain on worker node to block NVMe/TCP to RDS, verifying ctrl_loss_tmo=-1 infinite retry reconnection
- [Phase 32-resilience-regression-tests]: TC-10 includes DANGER warning: RDS restart affects ALL NVMe/TCP connections on ALL cluster nodes (maintenance window required)
- [Phase 32-resilience-regression-tests]: TC-11 documents controller pod restart as shortcut to trigger immediate reconciliation (vs 5-minute default interval)
- [Phase 32-resilience-regression-tests]: SetErrorMode resets operationNum=0 on mode change to clear counter state for the new mode
- [Phase 32-resilience-regression-tests]: RESIL-01/02 use SetErrorMode (not Stop/Start) because MockRDSServer.shutdown channel closes permanently on Stop()
- [Phase 32-resilience-regression-tests]: RESIL-03 is a unit test in reconciler_test.go because E2E framework uses K8sClient: nil (no AttachmentManager available)
- [Quick-007]: Dead code removal safe — GenerateSnapshotIDFromSource/ExtractSourceVolumeIDFromSnapshotID/ExtractTimestampFromSnapshotID had zero production callers after phase 29-30 format change
- [Quick-007]: ListSnapshots fallback removed — slot UUID is name-hash (not source UUID), so ExtractSourceVolumeIDFromSnapshotID returned wrong source IDs for new-format IDs
- [Quick-007]: creation-time= added to mock formatSnapshotDetail using strings.ToLower(t.Format("Jan/02/2006 15:04:05")) to match RouterOS lowercase month format (parseRouterOSTime title-cases internally)

### Pending Todos

None.

### Blockers/Concerns

- RDS restart testing affects site networking — requires confidence before running (document before executing)
- Pre-existing race condition in pkg/rds tests under -race flag (TestReconnection_WithBackoff, TestOnReconnectCallback) — unrelated to snapshot work
- Phase 31 (hardware validation) requires manual execution against real RDS at 10.42.68.1 — TC-08 is now documented and ready

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 002 | Fix deployment manifests to use DaemonSet | 2026-02-07 | — | [002-fix-deployment-manifests](./quick/002-fix-deployment-manifests/) |
| 003 | Generate README badges and status | 2026-02-08 | — | [003-generate-readme-badges](./quick/003-generate-readme-badges/) |
| 004 | Update README and documentation | 2026-02-08 | — | [004-update-readme-and-documentation-to-refle](./quick/004-update-readme-and-documentation-to-refle/) |
| 005 | Fix README remove non-existent Helm instructions | 2026-02-08 | — | [005-fix-readme-md-remove-non-existent-helm-i](./quick/005-fix-readme-md-remove-non-existent-helm-i/) |
| 006 | Fix blkid race condition on NVMe reconnect | 2026-02-12 | — | [006-fix-blkid-race-condition](./quick/006-fix-blkid-race-condition/) |
| 007 | Snapshot tech debt: creation-time, dead code, fallback removal | 2026-02-18 | 3dd73e6 | [7-clean-up-snapshot-tech-debt-add-creation](./quick/7-clean-up-snapshot-tech-debt-add-creation/) |

## Session Continuity

Last session: 2026-02-18
Stopped at: Completed Quick-007 (snapshot tech debt cleanup)
Resume file: None
Next action: v0.11.0 Data Protection milestone COMPLETE — all phases 29-32 done, quick tasks 001-007 done
