# Milestone v1: Production Stability

**Status:** SHIPPED 2026-01-31
**Phases:** 1-4
**Total Plans:** 17

## Overview

This roadmap transforms the RDS CSI driver from functional-but-fragile to production-ready by addressing NVMe-oF reconnection reliability. Four phases build incrementally: establishing reliable device discovery, detecting and recovering from stale mounts, adding proper backoff and error handling, and enabling production observability. Each phase delivers verifiable improvements while maintaining backward compatibility with existing volumes.

## Phases

### Phase 1: Foundation - Device Path Resolution
**Goal**: Driver reliably resolves NVMe device paths using NQN lookups instead of hardcoded paths
**Depends on**: Nothing (first phase)
**Requirements**: DEVP-01, DEVP-02, DEVP-03
**Plans**: 3 plans in 2 waves

Plans:
- [x] 01-01-PLAN.md - Sysfs scanning and DeviceResolver with TTL cache (Wave 1)
- [x] 01-02-PLAN.md - Orphan detection and connector integration (Wave 2)
- [x] 01-03-PLAN.md - Comprehensive unit tests (Wave 2, parallel)

**Details:**
- SysfsScanner for NVMe controller discovery via /sys/class/nvme-subsystem
- DeviceResolver with 10s TTL cache for NQN-to-device-path resolution
- Thread-safe cache with RWMutex for concurrent access
- Orphan detection (connected but no device)

### Phase 2: Stale Mount Detection and Recovery
**Goal**: Driver automatically detects and recovers from stale mounts caused by NVMe-oF reconnections
**Depends on**: Phase 1
**Requirements**: MOUNT-01, MOUNT-02, MOUNT-03
**Plans**: 5 plans in 4 waves

Plans:
- [x] 02-01-PLAN.md - Mount infrastructure: procmounts parsing, force unmount, in-use detection (Wave 1)
- [x] 02-02-PLAN.md - Kubernetes event posting for mount failures (Wave 1, parallel)
- [x] 02-03-PLAN.md - Stale detection and recovery logic with retry (Wave 2)
- [x] 02-04-PLAN.md - Integration into CSI node operations (Wave 3)
- [x] 02-05-PLAN.md - Comprehensive unit tests (Wave 4)

**Details:**
- /proc/mountinfo parsing for mount device lookup
- StaleMountChecker with 3 detection conditions
- MountRecoverer with exponential backoff (1s, 2s, 4s)
- ForceUnmount with lazy escalation and in-use protection
- Kubernetes EventPoster for PVC events

### Phase 3: Reconnection Resilience
**Goal**: Driver handles connection failures gracefully with proper backoff and configurable parameters
**Depends on**: Phase 2
**Requirements**: CONN-01, CONN-02, CONN-03
**Plans**: 4 plans in 4 waves

Plans:
- [x] 03-01-PLAN.md - Foundation: ConnectionConfig, param parsing, retry utilities (Wave 1)
- [x] 03-02-PLAN.md - Integrate connection params into nvme.go and controller.go (Wave 2)
- [x] 03-03-PLAN.md - Node integration and orphan cleanup on startup (Wave 3)
- [x] 03-04-PLAN.md - Comprehensive unit tests (Wave 4)

**Details:**
- ConnectionConfig with ctrl_loss_tmo, reconnect_delay, keep_alive_tmo
- StorageClass parameter parsing (ctrlLossTmo, reconnectDelay, keepAliveTmo)
- RetryWithBackoff using k8s wait.ExponentialBackoffWithContext with 10% jitter
- OrphanCleaner for startup cleanup of stale NVMe connections

### Phase 4: Observability
**Goal**: Operators have visibility into driver health and connection state via metrics and events
**Depends on**: Phase 3
**Requirements**: OBS-01, OBS-02, OBS-03
**Plans**: 5 plans in 3 waves

Plans:
- [x] 04-01-PLAN.md - Add GET_VOLUME_STATS and VOLUME_CONDITION capabilities, update NodeGetVolumeStats (Wave 1)
- [x] 04-02-PLAN.md - Create Prometheus metrics package with CSI-specific metrics (Wave 1, parallel)
- [x] 04-03-PLAN.md - Add HTTP metrics server and integrate metrics into driver (Wave 2)
- [x] 04-04-PLAN.md - Extend EventPoster with connection and orphan event types (Wave 1)
- [x] 04-05-PLAN.md - Comprehensive unit tests for observability features (Wave 3)

**Details:**
- VolumeCondition always returned in NodeGetVolumeStats
- 10 Prometheus metrics with rds_csi_ namespace prefix
- HTTP metrics server on port 9809
- Extended EventPoster with connection/orphan event types

---

## Milestone Summary

**Decimal Phases:** None (no urgent insertions needed)

**Key Decisions:**
- 10s default TTL for DeviceResolver cache (balances freshness vs overhead)
- Prefer nvmeXnY device format over nvmeXcYnZ (multipath compatibility)
- ctrl_loss_tmo=-1 default (prevents filesystem read-only mount after timeout)
- 10% jitter via wait.Backoff.Jitter (prevents thundering herd)
- Refuse force unmount if mount is in use (prevents data loss)
- Custom prometheus.Registry to avoid restart panics

**Issues Resolved:**
- Stale mounts after NVMe-oF reconnection (the original bug)
- Hardcoded device path assumptions
- No visibility into driver behavior

**Issues Deferred:**
- Background health monitoring (v2 RECOV-01)
- Proactive mount health checks (v2 RECOV-03)
- external-health-monitor sidecar integration (v2 INTEG-01)

**Technical Debt Incurred:**
- None — all observability methods fully wired

---

*For current project status, see .planning/ROADMAP.md (created for next milestone)*
*Archived: 2026-01-31 as part of v1 milestone completion*
