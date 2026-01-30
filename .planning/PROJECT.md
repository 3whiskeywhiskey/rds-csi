# RDS CSI Driver - Production Stability

## What This Is

A Kubernetes CSI driver for MikroTik ROSE Data Server (RDS) that provides dynamic NVMe/TCP block storage. The driver manages volume lifecycle via SSH/RouterOS CLI (control plane) and connects to storage via NVMe over TCP (data plane). Currently functional but has reliability issues when NVMe-oF connections drop and reconnect.

## Core Value

**Volumes remain accessible after NVMe-oF reconnections.** When network hiccups or RDS restarts cause connection drops, the driver must detect and handle controller renumbering so mounted volumes continue working without pod restarts.

## Requirements

### Validated

- ✓ Volume creation via SSH (`/disk add`) with NVMe/TCP export — existing
- ✓ Volume deletion via SSH (`/disk remove`) — existing
- ✓ Node staging: NVMe connect, format filesystem, mount to staging path — existing
- ✓ Node publishing: bind mount to pod path — existing
- ✓ Node unstaging: unmount, NVMe disconnect — existing
- ✓ SSH connection pooling with retry logic — existing
- ✓ Orphan volume reconciliation (detect/cleanup unused volumes) — existing
- ✓ Input validation preventing command injection — existing
- ✓ Secure mount options enforced (nosuid, nodev, noexec) — existing
- ✓ Security event logging for audit trails — existing

### Active

- [ ] Detect NVMe-oF controller renumbering after reconnection
- [ ] Update or remount when device paths change (nvme0 → nvme3)
- [ ] Cleanup stale mounts referencing non-existent device paths
- [ ] Health checks to proactively detect connection issues
- [ ] Validate device paths are functional before returning them
- [ ] Handle orphaned NVMe subsystems (appear connected but no device path)

### Out of Scope

- Volume snapshots — deferred to future milestone
- Controller high availability — deferred to future milestone
- Volume encryption — deferred to future milestone
- Prometheus metrics endpoint — deferred to future milestone

## Context

**The Bug:**
When NVMe-oF connections drop and reconnect, Linux assigns new controller numbers (nvme3 instead of nvme0). The driver's mount points still reference old device paths (/dev/nvme0n1) which no longer exist. Any I/O to these mounts fails with "no available path" error.

**Evidence from production:**
- dpu-c4140: Active controllers nvme3, nvme4 but mounts reference nvme0n1, nvme2n1 (stale)
- dpu-r640: Active controllers nvme0, nvme1 with matching mounts (working)
- Correlation: more uptime + more reconnection cycles = more stale state

**Codebase state:**
- NVMe device path resolution already flagged as fragile (multiple fallback paths, precedence unclear)
- Orphan subsystem detection partially implemented but incomplete
- No tests for reconnection scenarios

**Testing constraints:**
- RDS reboot disrupts site networking (disruptive to test)
- Need high confidence before hardware testing
- Look for ways to test reconnection without full RDS restart

## Constraints

- **Testing**: RDS restart affects site networking; need confidence before testing
- **Compatibility**: Must work with existing volumes and mounts (no breaking changes)
- **Dependencies**: Uses nvme-cli binary; solutions must work within that constraint

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Research best practices first | Limited testing ability, need high confidence | — Pending |

---
*Last updated: 2026-01-30 after initialization*
