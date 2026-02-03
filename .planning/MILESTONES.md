# Project Milestones: RDS CSI Driver

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
