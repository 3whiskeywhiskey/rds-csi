# Roadmap: RDS CSI Driver

## Milestones

- ✅ **v0.1.0 Foundation** - Phases 1-3 (shipped 2024-03-15)
- ✅ **v0.2.0 Controller Service** - Phases 4-5 (shipped 2024-04-01)
- ✅ **v0.3.0 Volume Fencing** - Phases 6-8 (shipped 2026-02-03)
- ✅ **v0.4.0 Production Hardening** - Phases 9-11 (shipped 2024-06-30)
- ✅ **v0.5.0 NVMe-oF Reconnection** - Phases 12-14 (shipped 2025-01-15)
- ✅ **v0.6.0 Block Volumes & KubeVirt** - Phases 15-16 (shipped 2025-11-15)
- ✅ **v0.7.0 Migration Tracking** - Phases 17-19 (shipped 2026-01-20)
- ✅ **v0.8.0 Code Quality & Logging** - Phases 20-21 (shipped 2026-02-04)
- ✅ **v0.9.0 Production Readiness & Test Maturity** - Phases 22-25.2 (shipped 2026-02-06)
- ✅ **v0.10.0 Feature Enhancements** - Phases 26-28 (shipped 2026-02-06) - [Archive](.planning/milestones/v0.10.0-ROADMAP.md)
- ✅ **v0.11.0 Data Protection** - Phases 29-32 (shipped 2026-02-18)

## Phases

<details>
<summary>✅ v0.1.0-v0.10.0 (Phases 1-28) - SHIPPED 2026-02-06</summary>

See milestone archives in `.planning/milestones/` for complete phase details:
- `.planning/milestones/v0.1.0-ROADMAP.md`
- `.planning/milestones/v0.2.0-ROADMAP.md`
- `.planning/milestones/v0.3.0-ROADMAP.md`
- `.planning/milestones/v0.4.0-ROADMAP.md`
- `.planning/milestones/v0.5.0-ROADMAP.md`
- `.planning/milestones/v0.6.0-ROADMAP.md`
- `.planning/milestones/v0.7.0-ROADMAP.md`
- `.planning/milestones/v0.8.0-ROADMAP.md`
- `.planning/milestones/v0.9.0-ROADMAP.md`
- `.planning/milestones/v0.10.0-ROADMAP.md`

</details>

### ✅ v0.11.0 Data Protection (Shipped 2026-02-18)

**Milestone Goal:** Fix broken snapshot implementation and ensure data safety with automated backups

- [x] **Phase 29: Snapshot Implementation Fix** - Rewrite SSH snapshot commands to use `/disk add copy-from` CoW; update CSI controller RPCs ✅ 2026-02-18
- [x] **Phase 30: Snapshot Validation** - Update mock RDS server to copy-from semantics; pass CSI sanity tests; validate against real hardware ✅ 2026-02-17
- [x] **Phase 31: Scheduled Snapshots** - CronJob with configurable schedule and retention cleanup; Helm chart template ✅ 2026-02-18
- [x] **Phase 32: Resilience Regression Tests** - Validate NVMe reconnect, RDS restart, and node failure scenarios don't regress ✅ 2026-02-18

## Phase Details

### Phase 29: Snapshot Implementation Fix

**Goal:** Users can create, list, and delete volume snapshots that survive source volume deletion, using `/disk add copy-from` CoW on Btrfs
**Depends on:** Phase 28 (v0.10.0 complete)
**Requirements:** SNAP-01, SNAP-02, SNAP-03, SNAP-04, SNAP-05, SNAP-06
**Success Criteria** (what must be TRUE):
  1. `kubectl create -f volumesnapshot.yaml` creates a CoW copy of the volume on RDS with no NVMe export, without error
  2. Deleting the source PVC after snapshot creation does not delete the snapshot (independent CoW copy)
  3. `kubectl get volumesnapshot` returns snapshot metadata including source volume, creation time, and size
  4. Creating a PVC from a snapshot source provisions a new writable volume restored from the snapshot CoW copy
  5. Deleting a VolumeSnapshot removes both the disk entry and the underlying file from RDS storage
**Plans:** 2 plans in 2 waves

Plans:
- [x] 29-01-PLAN.md — Rewrite SSH snapshot commands (types, ID generation, /disk add copy-from) in pkg/rds/
- [x] 29-02-PLAN.md — Update CSI controller snapshot RPCs to use new SSH backend

### Phase 30: Snapshot Validation

**Goal:** Snapshot operations are verified correct by automated tests and real hardware, with no mock-reality divergence for the copy-from approach
**Depends on:** Phase 29
**Requirements:** TEST-01, TEST-02, TEST-03
**Success Criteria** (what must be TRUE):
  1. CSI sanity test suite passes all snapshot test cases with zero failures (no regressions from snapshot fix)
  2. Mock RDS server responds to copy-from commands with semantics matching real RouterOS behavior (independent copies, no NVMe export)
  3. Hardware validation test case TC-08 (snapshot create/restore/delete) passes against real RDS hardware end-to-end
**Plans:** 2 plans in 2 waves

Plans:
- [x] 30-01-PLAN.md — Update mock RDS server: replace Btrfs subvolume handlers with copy-from semantics; add snapshot unit tests
- [x] 30-02-PLAN.md — Run CSI sanity snapshot tests and fix failures; update TC-08 hardware validation for copy-from approach

### Phase 31: Scheduled Snapshots

**Goal:** Users can configure automated periodic snapshots with retention-based cleanup deployed as part of the Helm chart
**Depends on:** Phase 30
**Requirements:** SCHED-01, SCHED-02, SCHED-03
**Success Criteria** (what must be TRUE):
  1. A CronJob configured via Helm creates a VolumeSnapshot for a target PVC on the configured schedule (e.g., daily at 02:00)
  2. The cleanup script deletes VolumeSnapshots older than the configured retention age, keeping the N most recent
  3. `helm install` with scheduled snapshot values enabled deploys the CronJob; `helm uninstall` removes it cleanly
**Plans:** 1 plan

Plans:
- [x] 31-01-PLAN.md — Add scheduledSnapshots values/schema, CronJob template with snapshot creation and retention cleanup, NOTES.txt update

### Phase 32: Resilience Regression Tests

**Goal:** Documented test cases confirm that NVMe reconnect, RDS restart, and node failure scenarios leave volumes intact and accessible
**Depends on:** Phase 29 (snapshot fix complete; resilience work is independent but shares milestone)
**Requirements:** RESIL-01, RESIL-02, RESIL-03
**Success Criteria** (what must be TRUE):
  1. After a simulated network interruption causes NVMe connection drop, pods with mounted volumes recover and continue I/O without manual intervention
  2. After an RDS restart, volumes remain mounted and data written before the restart is readable after reconnection
  3. After a node failure, stale VolumeAttachment objects are detected and removed automatically, allowing volumes to be reattached on another node
**Plans:** 2 plans in 1 wave

Plans:
- [x] 32-01-PLAN.md — Automated E2E resilience regression tests using mock error injection (RESIL-01, RESIL-02, RESIL-03)
- [x] 32-02-PLAN.md — Hardware validation test cases TC-09, TC-10, TC-11 for manual resilience testing; update TESTING.md

## Progress

**Execution Order:** 29 → 30 → 31 → 32

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 29. Snapshot Implementation Fix | v0.11.0 | 2/2 | ✅ Complete | 2026-02-18 |
| 30. Snapshot Validation | v0.11.0 | 2/2 | ✅ Complete | 2026-02-17 |
| 31. Scheduled Snapshots | v0.11.0 | 1/1 | ✅ Complete | 2026-02-18 |
| 32. Resilience Regression Tests | v0.11.0 | 2/2 | ✅ Complete | 2026-02-18 |

---
*Last updated: 2026-02-18 after Phase 32 execution*
