# Project Milestones: RDS CSI Driver

## v0.9.0 Production Readiness & Test Maturity (Shipped: 2026-02-06)

**Delivered:** Production-ready testing infrastructure with CSI spec compliance validation and resilience features

**Phases completed:** 22-25.2 (17 plans total)

**Key accomplishments:**
- CSI sanity test integration with comprehensive compliance validation in CI
- Mock RDS server with realistic SSH latency simulation and error injection capabilities
- Automated E2E test suite with Ginkgo v2 framework running in CI
- Test coverage increased to 68.6% (exceeds 65% target) with critical error path validation
- Attachment reconciliation system for automatic stale VolumeAttachment cleanup after infrastructure failures
- RDS connection manager with exponential backoff and auto-reconnection (never gives up)
- Resolved 134 linter issues blocking CI verification (golangci-lint v2 upgrade)
- Production incident response: Phase 25.1 inserted after 5-hour outage from stale attachments
- Code quality gates: Phase 25.2 inserted to unblock CI/CD verification pipeline

**Stats:**
- 122 files modified (+24,055 insertions, -910 deletions)
- 34,000+ lines of Go (current codebase)
- 6 phases (including 2 inserted decimal phases), 17 plans
- 92 days timeline (2025-11-05 to 2026-02-06)

**Git range:** `ce86034` (feat(22-01)) → `HEAD`

**See:** `.planning/milestones/v0.9.0-ROADMAP.md`

**What's next:** v0.10.0 Feature Enhancements - Volume snapshots, documentation, and Helm chart

---

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

## v0.5.0 NVMe-oF Reconnection (Shipped: 2025-01-15)

**Delivered:** Error resilience framework ensuring volumes remain accessible after network hiccups and RDS restarts

**Phases completed:** 12-14

**Key accomplishments:**
- NQN-based device path resolution via sysfs scanning (no hardcoded /dev/nvmeXnY paths)
- Automatic stale mount detection and recovery with exponential backoff
- Kernel reconnection parameters (ctrl_loss_tmo, reconnect_delay) configurable via StorageClass
- Circuit breaker pattern for error resilience
- Filesystem health checks before operations
- Procmounts timeout (10s) to prevent hanging
- Duplicate mount detection (100 threshold)

**See:** `.planning/milestones/v0.5.0-ROADMAP.md`

---

## v0.4.0 Production Hardening (Shipped: 2024-06-30)

**Delivered:** NVMe-oF reconnection reliability - volumes remain accessible after controller renumbering

**Phases completed:** 9-11

**Key accomplishments:**
- NQN-based device path resolution
- Automatic stale mount detection and recovery
- Kernel reconnection parameters configurable
- Kubernetes event posting for mount failures
- Prometheus metrics endpoint (:9809)
- VolumeCondition health reporting in NodeGetVolumeStats

**See:** `.planning/milestones/v0.4.0-ROADMAP.md`

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

**See:** `.planning/milestones/v0.3.0-ROADMAP.md`

---

## v0.2.0 Controller Service (Shipped: 2024-04-01)

**Delivered:** Complete volume lifecycle management via SSH/RouterOS CLI

**Phases completed:** 4-5

**Key accomplishments:**
- CreateVolume with file-backed NVMe/TCP export on RDS
- DeleteVolume with cleanup and idempotency
- ValidateVolumeCapabilities for access mode validation
- GetCapacity for RDS free space queries
- ListVolumes for volume enumeration

**See:** `.planning/milestones/v0.2.0-ROADMAP.md`

---

## v0.1.0 Foundation (Shipped: 2024-03-15)

**Delivered:** Project scaffolding, SSH client, CSI Identity service, build system

**Phases completed:** 1-3

**Key accomplishments:**
- SSH client wrapper for RouterOS CLI with retry logic
- CSI Identity service (GetPluginInfo, Probe, GetPluginCapabilities)
- Volume ID utilities with NQN generation and validation
- Multi-architecture build support (darwin/linux, amd64/arm64)
- Unit test foundation (17 test cases)

**See:** `.planning/milestones/v0.1.0-ROADMAP.md`

---
