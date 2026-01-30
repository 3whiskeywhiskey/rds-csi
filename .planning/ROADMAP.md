# Roadmap: RDS CSI Driver - Production Stability

## Overview

This roadmap transforms the RDS CSI driver from functional-but-fragile to production-ready by addressing NVMe-oF reconnection reliability. Four phases build incrementally: establishing reliable device discovery, detecting and recovering from stale mounts, adding proper backoff and error handling, and enabling production observability. Each phase delivers verifiable improvements while maintaining backward compatibility with existing volumes.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Foundation - Device Path Resolution** - Reliable NQN-based device discovery
- [ ] **Phase 2: Stale Mount Detection and Recovery** - Automatic recovery from controller renumbering
- [ ] **Phase 3: Reconnection Resilience** - Production-grade error handling and backoff
- [ ] **Phase 4: Observability** - Monitoring and operational visibility

## Phase Details

### Phase 1: Foundation - Device Path Resolution
**Goal**: Driver reliably resolves NVMe device paths using NQN lookups instead of hardcoded paths
**Depends on**: Nothing (first phase)
**Requirements**: DEVP-01, DEVP-02, DEVP-03
**Success Criteria** (what must be TRUE):
  1. Driver can resolve device path from NQN via sysfs scan even after controller renumbering
  2. Driver detects orphaned subsystems (appear connected but have no device) before attempting use
  3. Driver stores NQN (not device path) in staging metadata and resolves on-demand
  4. All device lookups use cached resolver with TTL validation (no hardcoded /dev/nvmeXnY assumptions)
**Plans**: 3 plans in 2 waves

Plans:
- [ ] 01-01-PLAN.md - Sysfs scanning and DeviceResolver with TTL cache (Wave 1)
- [ ] 01-02-PLAN.md - Orphan detection and connector integration (Wave 2)
- [ ] 01-03-PLAN.md - Comprehensive unit tests (Wave 2, parallel)

### Phase 2: Stale Mount Detection and Recovery
**Goal**: Driver automatically detects and recovers from stale mounts caused by NVMe-oF reconnections
**Depends on**: Phase 1
**Requirements**: MOUNT-01, MOUNT-02, MOUNT-03
**Success Criteria** (what must be TRUE):
  1. Driver detects stale mounts by comparing mount device with current device path before operations
  2. Driver automatically remounts staging paths when staleness detected (transparent to pods)
  3. Driver force-unmounts stuck mounts that won't unmount normally using lazy unmount
  4. Driver posts Kubernetes events to PVC when mount failures or recovery actions occur
**Plans**: TBD

Plans:
- [ ] TBD during planning

### Phase 3: Reconnection Resilience
**Goal**: Driver handles connection failures gracefully with proper backoff and configurable parameters
**Depends on**: Phase 2
**Requirements**: CONN-01, CONN-02, CONN-03
**Success Criteria** (what must be TRUE):
  1. Driver sets ctrl_loss_tmo and reconnect_delay parameters on NVMe connect (configurable via StorageClass)
  2. Driver uses exponential backoff with jitter for retry operations (prevents thundering herd)
  3. User can configure connection timeouts and retry behavior via StorageClass parameters
  4. Driver cleans up orphaned NVMe connections on startup or after failed operations
**Plans**: TBD

Plans:
- [ ] TBD during planning

### Phase 4: Observability
**Goal**: Operators have visibility into driver health and connection state via metrics and events
**Depends on**: Phase 3
**Requirements**: OBS-01, OBS-02, OBS-03
**Success Criteria** (what must be TRUE):
  1. Driver posts Kubernetes events for mount failures, recovery actions, and connection issues (already validated in Phase 2, now comprehensive)
  2. Driver reports volume health condition via NodeGetVolumeStats response
  3. Driver exposes Prometheus metrics endpoint showing connection failures, mount operations, and orphan detection
  4. Operators can query metrics to understand driver behavior and diagnose issues proactively
**Plans**: TBD

Plans:
- [ ] TBD during planning

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation - Device Path Resolution | 0/3 | Planned | - |
| 2. Stale Mount Detection and Recovery | 0/TBD | Not started | - |
| 3. Reconnection Resilience | 0/TBD | Not started | - |
| 4. Observability | 0/TBD | Not started | - |

---
*Roadmap created: 2026-01-30*
*Last updated: 2026-01-30*
