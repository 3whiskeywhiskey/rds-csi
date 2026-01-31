# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-01-31)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.3.0 Volume Fencing — prevent multi-node attachment conflicts

## Current Position

Phase: 10 of 10 (Upstream Contribution)
Plan: 2 of 2 prepared (awaiting manual submission)
Status: Prepared — PR ready, not submitted
Last activity: 2026-01-31 — Prepared 10-02 (PR description and branch ready)

Progress: [██████████] 100% (implementation) — upstream PR pending user submission

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
| TEST-04    | Tests added to existing vmi_test.go | 09-02 | Already has test infrastructure |
| TEST-05    | Direct cleanupAttachmentPods() invocation | 09-02 | Better unit test isolation |
| CI-01      | Use GitHub Actions CI for KubeVirt builds | 09-03 | macOS incompatible with Bazel/Linux syscalls |
| GHCR-01    | Made GHCR packages public for cluster access | 09-03 | Simpler than imagePullSecret |
| TEST-06    | Test on nested K3s worker for realistic workload | 09-03 | Production-like validation scenario |
| PREP-01    | Cherry-pick commits to exclude CI workflow files | 10-01 | Upstream doesn't need fork-specific CI |
| PREP-02    | Use --signoff flag during cherry-pick for DCO | 10-01 | Simpler than interactive rebase |
| PREP-03    | Base upstream-pr on upstream/main | 10-01 | Ensures PR applies cleanly to upstream |
| PR-01      | Defer upstream PR submission to user | 10-02 | User controls timing of upstream engagement |

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
Stopped at: Completed 10-01-PLAN.md (prepared commits for upstream PR)
Resume file: None

### Current Work State

**v0.5 KubeVirt Hotplug Fix Progress:**

**Phase 8 (Fork and CI/CD Setup):**
- ✓ Fork created: https://github.com/whiskey-works/kubevirt
- ✓ CI workflow added: `.github/workflows/build-images.yaml`
- ✓ PR #1 build passed
- ○ Merge PR #1, test deployment with custom images

**Phase 9 (Implement and Test Fix):** ✅ COMPLETE
- ✓ 09-01: Document code path and implement fix (wave 1)
  - Code path documented in 09-01-CODEPATH.md
  - Fix committed to hotplug-fix-v1 branch (cc1b700)
  - allHotplugVolumesReady() checks VolumeReady phase before pod deletion
- ✓ 09-02: Unit tests for fix (wave 2)
  - 5 unit tests added to vmi_test.go (6546421)
  - Tests cover bug reproduction, normal operation, regression scenarios
  - CI validated tests after push
- ✓ 09-03: Manual validation on metal cluster (wave 3)
  - Custom images deployed via GitHub Actions CI (workflow 21549308226)
  - Images: ghcr.io/whiskey-works/kubevirt/*:hotplug-fix-v1-708d58b902
  - GHCR packages made public for cluster access
  - ✅ Multi-volume hotplug validated: VM stayed Running, no I/O errors
  - ✅ Volume removal validated: clean detachment
  - ✅ Single-volume regression check passed
  - Validation documented in 09-03-VALIDATION.md

**Fix summary:**
- Added allHotplugVolumesReady() helper function
- Modified cleanupAttachmentPods() to check all volumes ready before deleting old pods
- Fix location: /tmp/kubevirt-fork/pkg/virt-controller/watch/vmi/volume-hotplug.go
- Test location: /tmp/kubevirt-fork/pkg/virt-controller/watch/vmi/vmi_test.go
- Validated on: metal cluster with nested K3s worker (homelab-node-1)

**Phase 10 (Upstream Contribution):** ✅ PREPARED (awaiting manual submission)
- ✓ 10-01: Prepare commits for upstream PR (wave 1)
  - Created upstream-pr branch from upstream/main
  - Cherry-picked fix commits with DCO sign-off
  - Commits: dde549140a (fix), d9622dd922 (tests)
  - Branch pushed to whiskey-works/kubevirt
- ⏸ 10-02: Submit PR to kubevirt/kubevirt (wave 2) — PREPARED, NOT SUBMITTED
  - PR description ready: `.planning/phases/10-upstream-contribution/10-02-PR-DESCRIPTION.md`
  - Branch ready: `whiskey-works/kubevirt:upstream-pr`
  - To submit: https://github.com/kubevirt/kubevirt/compare/main...whiskey-works:kubevirt:upstream-pr
  - References: #6564, #9708, #16520

## Developer Notes

**SSH key for whiskey-works repos:**
```bash
export GIT_SSH_COMMAND="ssh -i ~/.ssh/whiskey_ed25519 -F /dev/null"
```
Use this when pushing to `whiskey-works/*` repos to avoid SSH key mismatch with default key.

---
*State initialized: 2026-01-30*
*Last updated: 2026-01-31 — Phase 10 prepared, upstream PR awaiting manual submission*
