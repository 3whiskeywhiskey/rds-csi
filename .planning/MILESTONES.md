# Project Milestones: RDS CSI Driver

## v0.8.0 Code Quality and Logging Cleanup (Shipped: 2026-02-04)

**Delivered:** Systematic codebase cleanup with improved maintainability, reduced log noise, and comprehensive test coverage

**Phases completed:** 17-21 (20 plans total)

**Key accomplishments:**
- Logging cleanup: 78% reduction in security logger code, 83% reduction in DeleteVolume logging, V(3) eliminated codebase-wide
- Error handling standardization: 96.1% %w compliance, 10 sentinel errors integrated, patterns documented in CONVENTIONS.md
- Test coverage expansion: 65.0% total coverage (exceeds 55% target), coverage enforcement tooling configured
- Code quality improvements: Severity mapping table reduces cyclomatic complexity 80%, golangci-lint enforcement enabled
- Test infrastructure stabilization: Fixed failing block volume tests, 148 tests pass consistently with race detector
- Maintainability improvements: Documented verbosity conventions, error handling patterns, and complexity baselines

**Stats:**
- 88 files modified (+14,120 insertions, -584 deletions)
- 33,687 lines of Go (current codebase)
- 5 phases, 20 plans
- 91 days timeline (2025-11-05 to 2026-02-04, work concentrated on 2026-02-04 with 130 commits)

**Git range:** `29dffe1` (feat(17-01)) → `571d230` (docs(21-04))

**See:** `.planning/milestones/v0.8.0-ROADMAP.md`

**What's next:** Next milestone cycle - potential areas include coverage gap closure, deferred package refactoring (QUAL-03), or new feature development

---

## v0.7.0 State Management Refactoring and Observability (Shipped: 2026-02-04)

**Delivered:** VolumeAttachment-based state rebuild and complete migration metrics observability

**Phases completed:** 15-16 (5 plans total)

**Key accomplishments:**
- VolumeAttachment objects are now authoritative - Controller rebuilds attachment state from VolumeAttachment objects instead of PV annotations, eliminating stale state bugs
- Migration state survives restarts - Dual VolumeAttachment detection automatically restores migration state with correct timestamps after controller restart
- PV annotations informational-only - Annotations are write-only for debugging, never read during state rebuild, preventing annotation/reality desync
- Migration metrics observability complete - AttachmentManager.SetMetrics() wired, enabling full Prometheus metrics for migration count, duration, and active migrations
- Comprehensive test coverage - 14 tests prove VolumeAttachment authority, 84.5% coverage for rebuild subsystem

**Stats:**
- 22 files modified
- +2,978 insertions, -198 deletions
- 31,882 lines of Go (current codebase)
- 2 phases, 5 plans
- 1 day timeline (2026-02-04)

**Git range:** `3885862` (feat(15-01)) → `33b8be5` (docs(16))

**See:** `.planning/milestones/v0.7.0-ROADMAP.md`

**What's next:** All Phase 16 commits completed - migration metrics fully wired and observable

---

## v0.6.0 Block Volume Support (Shipped: 2026-02-04)

**Delivered:** CSI block volume support for KubeVirt VMs with validated live migration on metal cluster

**Phases completed:** 11-14 (9 plans total)

**Key accomplishments:**
- Block volume lifecycle: NodeStageVolume/NodePublishVolume handle block volumes without formatting, using mknod for device nodes
- KubeVirt integration: VM boot and live migration validated on metal cluster (r740xd → c4140, ~15s)
- Mount storm prevention: Fixed critical devtmpfs propagation bug (mknod vs bind mount approach)
- System volume protection: NQN prefix filtering prevents orphan cleaner from disconnecting system volumes (nixos-*)
- Error resilience framework: Circuit breaker, filesystem health checks, procmounts timeout (10s), duplicate mount detection (100 threshold)
- Stale state fix: Clear PV annotations on detachment to prevent false positive attachments across controller restarts

**Stats:**
- 64 files modified
- +10,453 insertions, -289 deletions
- 31,807 lines of Go (current codebase)
- 4 phases, 9 plans
- 1 day timeline (2026-02-03 to 2026-02-04)

**Git range:** `74fc6bf` → `cf354a0`

**Critical fixes:**
- commit 0ea6bee: mknod for block volumes (prevents devtmpfs mount storm)
- commit 62197ce: Clear PV annotations on detachment (fixes stale attachment state)

**See:** `.planning/milestones/v0.6.0-ROADMAP.md`

**What's next:** v0.7.0 State Management Refactoring and Observability - VolumeAttachment-based state rebuild (Phase 15 complete), migration metrics emission (Phase 16 planned)

---

## v0.5.0 KubeVirt Live Migration (In Progress)

**Goal:** Enable KubeVirt VM live migration with RDS CSI volumes

**Phases:** 8-10

**Key features:**
- ReadWriteMany (RWX) access mode for block volumes only
- 2-node simultaneous attachment during migration window
- Migration-specific timeout separate from RWO conflict detection
- Prometheus metrics for migration tracking
- Kubernetes events for migration lifecycle

**Requirements:** 10 total (RWX-01-03, SAFETY-01-04, OBS-01-03)

**See:** .planning/milestones/v0.5-ROADMAP.md

---

## v0.3.0 Volume Fencing (Shipped: 2026-02-03)

**Delivered:** ControllerPublish/Unpublish implementation - prevents volume ping-pong between nodes

**Phases completed:** 5-7 (12 plans total)

**Key accomplishments:**
- AttachmentManager with in-memory tracking and PV annotation persistence
- ControllerPublishVolume enforces ReadWriteOnce semantics
- ControllerUnpublishVolume with cleanup and idempotency
- Background reconciler detects stale attachments from deleted nodes
- Grace period (30s) for KubeVirt live migration handoff
- Prometheus attachment metrics (attach_total, detach_total, conflicts_total)
- Kubernetes events for attachment conflicts

**See:** .planning/milestones/v0.3-ROADMAP.md

---

## v1 Production Stability (Shipped: 2026-01-31)

**Delivered:** NVMe-oF reconnection reliability - volumes remain accessible after controller renumbering

**Phases completed:** 1-4 (17 plans total)

**Key accomplishments:**
- NQN-based device path resolution via sysfs scanning (no hardcoded /dev/nvmeXnY paths)
- Automatic stale mount detection and recovery with exponential backoff
- Kernel reconnection parameters (ctrl_loss_tmo, reconnect_delay) configurable via StorageClass
- Kubernetes event posting for mount failures and recovery actions
- Prometheus metrics endpoint (:9809) with 10 CSI-specific metrics
- VolumeCondition health reporting in NodeGetVolumeStats

**Stats:**
- 30 files created/modified
- 7,434 lines of Go added (22,424 total)
- 4 phases, 17 plans, ~50 tasks
- 1 day from start to ship (2026-01-30 to 2026-01-31)

**Git range:** `feat(01-01)` to `feat(04-05)`

**See:** .planning/milestones/v1-ROADMAP.md

---

## v0.3.0 Volume Fencing (Shipped: 2026-01-31)

**Delivered:** ControllerPublish/Unpublish implementation — prevents multi-node attachment conflicts

**Phases completed:** 5-7 (12 plans total)

**Key accomplishments:**
- AttachmentManager with persistent state via PV annotations
- ControllerPublishVolume enforces RWO semantics with FAILED_PRECONDITION on conflicts
- Background reconciler cleans stale attachments from deleted nodes
- Grace period (30s) prevents false conflicts during KubeVirt live migrations
- Prometheus metrics for attachment operations
- VMI serialization option to mitigate upstream kubevirt concurrency issues

**Git range:** v0.3.0 → v0.4.0

---

## v0.5.0 KubeVirt Hotplug Fix (In Progress)

**Goal:** Fix upstream KubeVirt bug where concurrent volume hotplug causes VM pauses

**Status:** Phase 9 complete, Phase 10 (upstream contribution) pending

**Phases:** 8-10

**Phase 9 Accomplishments (Implement and Test Fix):**
- Fork created: github.com/whiskey-works/kubevirt with GitHub Actions CI
- Fix implemented: `allHotplugVolumesReady()` check in cleanupAttachmentPods
- Unit tests: 5 tests covering bug reproduction and regression scenarios
- Manual validation: Multi-volume hotplug on metal cluster - PASSED
  - VM stays Running during concurrent hotplug (no pause)
  - Volume removal works correctly
  - No I/O errors observed
- Images: ghcr.io/whiskey-works/kubevirt/*:hotplug-fix-v1-708d58b902

**Related issues:**
- kubevirt/kubevirt#6564, #9708, #16520
- rds-csi#12

**Next:** Create PR to kubevirt/kubevirt with fix + tests

**See:** `.planning/milestones/v0.5-ROADMAP.md`

---
