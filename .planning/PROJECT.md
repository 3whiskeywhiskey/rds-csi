# RDS CSI Driver

## What This Is

A Kubernetes CSI driver for MikroTik ROSE Data Server (RDS) that provides dynamic NVMe/TCP block storage. The driver manages volume lifecycle via SSH/RouterOS CLI (control plane) and connects to storage via NVMe over TCP (data plane).

## Core Value

**Volumes remain accessible after NVMe-oF reconnections.** When network hiccups or RDS restarts cause connection drops, the driver detects and handles controller renumbering so mounted volumes continue working without pod restarts.

## Current Milestone: v0.6.0 Block Volume Support

**Goal:** Implement CSI block volume support to enable KubeVirt VMs with RDS storage.

**Target features:**
- NodeStageVolume handles block volumes (skip filesystem formatting)
- NodePublishVolume creates block device files for VMs
- NodeUnpublishVolume/NodeUnstageVolume clean up block mounts
- Hardware validation: VM boots and live migrates on metal cluster

**Problem being solved:** v0.5.0 added RWX capability for live migration, but the node plugin only supports filesystem volumes. KubeVirt VMs fail to start because NodeStageVolume formats the device as ext4 instead of leaving it as a raw block device.

## Current State

**Version:** v0.2.0 (shipped 2026-01-31)
**LOC:** 22,424 Go
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

### Active

- [ ] NodeStageVolume handles block volumes without formatting
- [ ] NodePublishVolume creates block device files at target path
- [ ] NodeUnpublishVolume/NodeUnstageVolume clean up block mounts
- [ ] KubeVirt VMs boot successfully with RDS block volumes
- [ ] KubeVirt live migration works end-to-end on metal cluster

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
*Last updated: 2026-02-03 after starting v0.6.0 milestone (block volume support)*
