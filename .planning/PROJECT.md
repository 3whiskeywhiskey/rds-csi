# RDS CSI Driver

## What This Is

A Kubernetes CSI driver for MikroTik ROSE Data Server (RDS) that provides dynamic NVMe/TCP block storage. The driver manages volume lifecycle via SSH/RouterOS CLI (control plane) and connects to storage via NVMe over TCP (data plane).

## Core Value

**Volumes remain accessible after NVMe-oF reconnections.** When network hiccups or RDS restarts cause connection drops, the driver detects and handles controller renumbering so mounted volumes continue working without pod restarts.

## Current Milestone: v0.8.0 Code Quality and Logging Cleanup

**Goal:** Systematic codebase audit and refactoring to improve maintainability, reduce log noise, and address technical debt

**Target outcomes:**
- Logging cleanup: Reduce production log noise by rationalizing debug/info levels across all packages
- Error handling: Consistent error patterns with proper context propagation
- Code duplication: Extract common patterns into reusable utilities
- Code organization: Refactor overgrown packages for better separation of concerns
- Test coverage: Fill gaps in critical paths (especially error scenarios)

## Latest Milestone: v0.7.0 State Management Refactoring and Observability (Shipped: 2026-02-04)

**Delivered:** VolumeAttachment-based state rebuild and complete migration metrics observability

**Shipped features:**
- VolumeAttachment-based state rebuild (Phase 15 - complete)
- Migration metrics emission (Phase 16 - complete)
- PV annotations informational-only
- Stale attachment state eliminated by design

## Current State

**Version:** v0.7.0 (shipped 2026-02-04)
**LOC:** 31,882 Go
**Tech Stack:** Go 1.24, CSI Spec v1.5.0+, NVMe/TCP, SSH/RouterOS CLI

### What's Working

- Volume creation/deletion via SSH with NVMe/TCP export
- Node staging with NVMe connect, format, mount
- **NQN-based device path resolution** (no hardcoded paths)
- **Automatic stale mount detection and recovery**
- **Kernel reconnection parameters** (ctrl_loss_tmo, reconnect_delay)
- **Prometheus metrics** on :9809
- **Kubernetes events** for mount failures
- **VolumeCondition** health reporting

### Known Limitations

- Single RDS controller (no multipath)
- No volume snapshots yet
- No controller HA (single replica)

## Requirements

### Validated

- ✓ Volume creation via SSH (`/disk add`) with NVMe/TCP export — v1
- ✓ Volume deletion via SSH (`/disk remove`) — v1
- ✓ Node staging: NVMe connect, format filesystem, mount to staging path — v1
- ✓ Node publishing: bind mount to pod path — v1
- ✓ Node unstaging: unmount, NVMe disconnect — v1
- ✓ SSH connection pooling with retry logic — v1
- ✓ Orphan volume reconciliation (detect/cleanup unused volumes) — v1
- ✓ Input validation preventing command injection — v1
- ✓ Secure mount options enforced (nosuid, nodev, noexec) — v1
- ✓ Security event logging for audit trails — v1
- ✓ Detect NVMe-oF controller renumbering after reconnection — v1
- ✓ Update or remount when device paths change (nvme0 → nvme3) — v1
- ✓ Cleanup stale mounts referencing non-existent device paths — v1
- ✓ Health checks via VolumeCondition in NodeGetVolumeStats — v1
- ✓ Validate device paths are functional before returning them — v1
- ✓ Handle orphaned NVMe subsystems (appear connected but no device path) — v1
- ✓ Prometheus metrics endpoint — v1
- ✓ NodeStageVolume handles block volumes without formatting — v0.6.0
- ✓ NodePublishVolume creates block device files using mknod — v0.6.0
- ✓ NodeUnpublishVolume/NodeUnstageVolume clean up block mounts — v0.6.0
- ✓ KubeVirt VMs boot successfully with RDS block volumes — v0.6.0
- ✓ KubeVirt live migration works end-to-end on metal cluster — v0.6.0
- ✓ Mount storm prevention (NQN filtering, duplicate detection, circuit breaker) — v0.6.0
- ✓ System volume protection via configurable NQN prefix filtering — v0.6.0
- ✓ Controller rebuilds state from VolumeAttachment objects (not PV annotations) — v0.7.0
- ✓ Migration state detection from multiple VolumeAttachment objects — v0.7.0
- ✓ PV annotations are informational-only (never read during rebuild) — v0.7.0
- ✓ Migration metrics (count, duration, active) observable via Prometheus — v0.7.0

### Active

- [ ] Production logs contain only actionable information at info level
- [ ] Error handling follows consistent patterns throughout codebase
- [ ] Test coverage >85% on critical paths
- [ ] Common patterns extracted into reusable utilities
- [ ] Code smells from analysis resolved or explicitly deferred

### Out of Scope

- Volume snapshots — separate milestone
- Controller HA — separate milestone, requires leader election
- Volume encryption — separate milestone, different concern
- NVMe multipath — single RDS controller, not applicable
- Automatic pod restart — CSI spec says drivers report, orchestrators act

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 10s TTL for DeviceResolver cache | Balances freshness vs scanning overhead | ✓ Good |
| Prefer nvmeXnY over nvmeXcYnZ | Multipath compatibility | ✓ Good |
| ctrl_loss_tmo=-1 default | Prevents filesystem read-only mount | ✓ Good |
| 10% jitter in backoff | Prevents thundering herd | ✓ Good |
| Refuse force unmount if in use | Prevents data loss | ✓ Good |
| Custom prometheus.Registry | Avoids restart panics | ✓ Good |
| Orphan cleanup on startup | Best-effort, non-blocking | ✓ Good |

## Constraints

- **Testing**: RDS restart affects site networking; need confidence before testing
- **Compatibility**: Must work with existing volumes and mounts (no breaking changes)
- **Dependencies**: Uses nvme-cli binary; solutions must work within that constraint

---
*Last updated: 2026-02-04 after starting v0.8.0 milestone (code quality and logging cleanup)*
