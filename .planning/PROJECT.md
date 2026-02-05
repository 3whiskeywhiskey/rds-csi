# RDS CSI Driver

## What This Is

A Kubernetes CSI driver for MikroTik ROSE Data Server (RDS) that provides dynamic NVMe/TCP block storage. The driver manages volume lifecycle via SSH/RouterOS CLI (control plane) and connects to storage via NVMe over TCP (data plane).

## Core Value

**Volumes remain accessible after NVMe-oF reconnections.** When network hiccups or RDS restarts cause connection drops, the driver detects and handles controller renumbering so mounted volumes continue working without pod restarts.

## Current Milestone: v0.9.0 Production Readiness & Test Maturity

**Goal:** Validate CSI spec compliance and build production-ready testing infrastructure

**Target features:**
- CSI sanity test suite integration and compliance validation
- Comprehensive RDS mock for automated CI testing
- E2E test framework with both automated and manual test scenarios
- Critical coverage gaps closed (error paths, edge cases)
- Opportunistic package refactoring when code substantially modified

## Latest Milestone: v0.8.0 Code Quality and Logging Cleanup (Shipped: 2026-02-04)

**Delivered:** Systematic codebase cleanup with improved maintainability, reduced log noise, and comprehensive test coverage

**Shipped features:**
- Logging cleanup: 78% reduction in security logger code, V(3) eliminated codebase-wide
- Error handling standardization: 96.1% %w compliance, 10 sentinel errors integrated
- Test coverage expansion: 65.0% total coverage (up from 48%)
- Code quality improvements: Severity mapping table, complexity metrics configured
- Test infrastructure stabilization: 148 tests pass consistently
- Maintainability improvements: Documented conventions in CONVENTIONS.md

## Current State

**Version:** v0.8.0 (shipped 2026-02-04)
**LOC:** 33,687 Go
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
- ✓ Production logs contain only actionable information at info level — v0.8.0 (V(3) eliminated, V(2)=outcomes only)
- ✓ Error handling follows consistent patterns throughout codebase — v0.8.0 (96.1% %w compliance, sentinel errors, CONVENTIONS.md)
- ✓ Test coverage >60% on critical paths — v0.8.0 (65.0% total, enforcement configured)
- ✓ Common patterns extracted into reusable utilities — v0.8.0 (table-driven helpers, Wrap*Error functions)
- ✓ Code smells from analysis resolved or explicitly deferred — v0.8.0 (4 resolved, 1 deferred with rationale)

### Active

- [ ] CSI sanity tests pass with zero failures
- [ ] Mock RDS server supports all driver commands
- [ ] E2E test suite runs in CI without real hardware
- [ ] Critical error paths have test coverage
- [ ] Manual test scenarios documented for hardware validation

### Out of Scope

- Volume snapshots — separate milestone
- Controller HA — separate milestone, requires leader election
- Volume encryption — separate milestone, different concern
- NVMe multipath — single RDS controller, not applicable
- Automatic pod restart — CSI spec says drivers report, orchestrators act
- Security hardening (SSH host key verification, injection fuzzing) — separate security milestone
- Stress/load testing — hardware constraints, would require dedicated test infrastructure
- Package refactoring unless code substantially modified — defer to v1.0+ if not triggered

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
| Table-driven severity mapping | Reduces complexity, single source of truth | ✓ Good |
| Coverage threshold 60% | Realistic for hardware-dependent code | ✓ Good |
| Sentinel errors over string matching | Type-safe, errors.Is() compatible | ✓ Good |
| V(2) for outcomes, V(4) for diagnostics | Clear operator vs debug separation | ✓ Good |
| Defer QUAL-03 package refactoring | Risk > benefit at current scale | ✓ Good |

## Constraints

- **Testing**: RDS restart affects site networking; need confidence before testing
- **Compatibility**: Must work with existing volumes and mounts (no breaking changes)
- **Dependencies**: Uses nvme-cli binary; solutions must work within that constraint

---
*Last updated: 2026-02-04 after starting v0.9.0 milestone*
