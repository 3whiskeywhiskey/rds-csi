# Project Milestones: RDS CSI Driver

## v1 Production Stability (Shipped: 2026-01-31)

**Delivered:** NVMe-oF reconnection reliability — volumes remain accessible after controller renumbering

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
- 1 day from start to ship (2026-01-30 → 2026-01-31)

**Git range:** `feat(01-01)` → `feat(04-05)`

**What's next:** Production testing on hardware, then consider v2 features (background health monitoring, external-health-monitor sidecar integration)

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
