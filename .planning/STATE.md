# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-01-31)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.3.0 Volume Fencing — prevent multi-node attachment conflicts

## Current Position

Phase: 9 of 9 (Implement and Test Fix)
Plan: 1 of 3 complete
Status: In progress
Last activity: 2026-01-31 — Completed 09-01-PLAN.md (document code path and implement fix)

Progress: [████████░░] 80% (prior milestones) + 09-01 of v0.5

## Milestone History

- **v1 Production Stability** — shipped 2026-01-31
  - Phases 1-4, 17 plans
  - NVMe-oF reconnection reliability

- **v0.3.0 Volume Fencing** — in progress (all phases complete, pending audit)
  - Phases 5-7, 12 plans
  - ControllerPublish/Unpublish implementation

- **v0.5.0 KubeVirt Hotplug Fix** — in progress
  - Phases 8-10 (collapsed 9+10 into single implementation phase)
  - Fix upstream KubeVirt concurrent hotplug bug

## Accumulated Context

### Decisions

| ID        | Decision                                   | Phase | Context                      |
| --------- | ------------------------------------------ | ----- | ---------------------------- |
| ROADMAP-1 | Use ControllerPublish/Unpublish for fencing | 05    | Standard CSI approach        |
| ROADMAP-2 | Store state in-memory + PV annotations      | 05    | Survives controller restarts |
| ROADMAP-3 | Start from Phase 5 (continues from v1)      | 05    | v1 shipped Phase 4           |
| ATTACH-01 | In-memory map with RWMutex for tracking     | 05-01 | Simple, fast, single controller |
| ATTACH-02 | Per-volume locking with VolumeLockManager   | 05-01 | Prevents deadlocks, allows concurrency |
| ATTACH-03 | Lock order: release manager before per-volume | 05-01 | Critical deadlock prevention |
| ATTACH-04 | Rollback on persistence failure             | 05-02 | Ensures in-memory/PV consistency |
| ATTACH-05 | PV annotations for state persistence        | 05-02 | Survives controller restarts |
| ATTACH-06 | Initialize before orphan reconciler         | 05-02 | State ready before operations |
| CSI-01    | Warning event type for attachment conflicts | 06-01 | Blocks pod scheduling         |
| CSI-02    | Actionable message format with both nodes   | 06-01 | Operator visibility           |
| CSI-03    | Idempotent same-node publish returns success | 06-02 | CSI spec compliance           |
| CSI-04    | FAILED_PRECONDITION (code 9) for RWO conflicts | 06-02 | Standard CSI error code       |
| CSI-05    | snake_case keys in publish_context          | 06-02 | Matches volumeContext conventions |
| CSI-06    | Validate blocking node exists, auto-clear if deleted | 06-02 | Self-healing for stale state |
| CSI-07    | Fail-closed on K8s API errors              | 06-02 | Safety over availability      |
| TEST-01   | Test volume IDs use valid UUID format      | 06-03 | Required by validation        |
| TEST-02   | MockClient implements full RDSClient       | 06-03 | Test isolation                |
| GRACE-01  | Per-volume grace period with detachTimestamps map | 07-01 | Preserves detach history for migration |
| METRICS-01 | Sub-second histogram buckets (0.01-5s)    | 07-01 | Attachment ops mostly in-memory |
| EVENTS-01 | Normal event type for routine lifecycle    | 07-01 | VolumeAttached/Detached not failures |
| RECONCILE-01 | Fail-open on K8s API errors during reconciliation | 07-02 | Don't clear valid attachments on transient errors |
| RECONCILE-02 | 5-minute reconciler interval default      | 07-02 | Balance cleanup latency vs API load |
| GRACE-02  | Grace period check before node validation  | 07-02 | Allows migration handoff before conflict |
| TEST-03   | Use fake.NewSimpleClientset for reconciler tests | 07-03 | Standard Kubernetes testing approach |
| BUG-01    | Fix double-stop panic by clearing channels | 07-03 | Subsequent Stop() calls are no-op |
| BUG-02    | Fix race condition with local channel capture | 07-03 | Eliminate concurrent read/write on channels |
| EVENTS-02 | EventPoster interface in attachment package | 07-04 | Avoid circular dependency with driver |
| EVENTS-03 | Best-effort event posting pattern | 07-04 | Never fail operations for observability |
| EVENTS-04 | PV lookup for PVC info in unpublish | 07-04 | volumeContext not available in unpublish |
| HOTPLUG-01 | Check ALL hotplug volumes for VolumeReady | 09-01 | Simpler than tracking "new" volumes |
| HOTPLUG-02 | Early return from cleanupAttachmentPods | 09-01 | Cleaner than per-pod skip logic |

### Pending Todos

None

### Blockers/Concerns

Production issue motivating this milestone:
- Volume ping-pong between nodes every ~7 minutes
- `CONFLICT: PVC is in use by VMI` errors
- No ControllerPublish/Unpublish = no fencing

### Roadmap Evolution

- v0.5 milestone added: KubeVirt hotplug fix (phases 8-11)
  - Motivation: GitHub issue #12, kubevirt/kubevirt#9708
  - Approach: Fork KubeVirt, fix virt-controller, contribute upstream

## Session Continuity

Last session: 2026-01-31
Stopped at: Completed 09-01-PLAN.md
Resume file: None

### Current Work State

**v0.5 KubeVirt Hotplug Fix Progress:**

**Phase 8 (Fork and CI/CD Setup):**
- ✓ Fork created: https://github.com/whiskey-works/kubevirt
- ✓ CI workflow added: `.github/workflows/build-images.yaml`
- ✓ PR #1 build passed
- ○ Merge PR #1, test deployment with custom images

**Phase 9 (Implement and Test Fix):**
- ✓ 09-01: Document code path and implement fix (wave 1) - COMPLETE
  - Code path documented in 09-01-CODEPATH.md
  - Fix committed to hotplug-fix-v1 branch (cc1b700)
  - allHotplugVolumesReady() checks VolumeReady phase before pod deletion
- ○ 09-02: Unit tests for fix (wave 2)
- ○ 09-03: Manual validation on metal cluster (wave 3, has checkpoint)

**Fix summary:**
- Added allHotplugVolumesReady() helper function
- Modified cleanupAttachmentPods() to check all volumes ready before deleting old pods
- Fix location: /tmp/kubevirt-fork/pkg/virt-controller/watch/vmi/volume-hotplug.go

**Next steps:**
1. Execute 09-02-PLAN.md (unit tests)
2. Execute 09-03-PLAN.md (manual validation)
3. Push hotplug-fix-v1 branch, create PR to trigger CI

---
*State initialized: 2026-01-30*
*Last updated: 2026-01-31 — 09-01 complete, fix implemented*
