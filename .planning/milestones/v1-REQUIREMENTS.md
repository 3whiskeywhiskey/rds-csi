# Requirements Archive: v1 Production Stability

**Archived:** 2026-01-31
**Status:** SHIPPED

This is the archived requirements specification for v1.
For current requirements, see `.planning/REQUIREMENTS.md` (created for next milestone).

---

# Requirements: RDS CSI Driver - Production Stability

**Defined:** 2026-01-30
**Core Value:** Volumes remain accessible after NVMe-oF reconnections

## v1 Requirements

Requirements for production stability release. Each maps to roadmap phases.

### Device Path Resolution

- [x] **DEVP-01**: Driver resolves device path by NQN lookup in sysfs, not hardcoded paths
  - *Outcome: Implemented via SysfsScanner.FindDeviceByNQN() scanning /sys/class/nvme-subsystem*
- [x] **DEVP-02**: Driver detects orphaned subsystems (appear connected but no device path)
  - *Outcome: Implemented via DeviceResolver.IsOrphanedSubsystem() with connection check callback*
- [x] **DEVP-03**: Driver caches NQN-to-path mappings with TTL and validation
  - *Outcome: DeviceResolver with 10s TTL cache, validates device existence on cache hit*

### Mount Validation

- [x] **MOUNT-01**: Driver detects stale mounts by comparing mount device vs current device path
  - *Outcome: StaleMountChecker with 3 detection conditions (not found, disappeared, mismatch)*
- [x] **MOUNT-02**: Driver automatically remounts staging path when stale mount detected
  - *Outcome: MountRecoverer with exponential backoff (1s, 2s, 4s), max 3 attempts*
- [x] **MOUNT-03**: Driver can force unmount stuck mounts that won't unmount normally
  - *Outcome: ForceUnmount with 10s wait then lazy unmount, in-use protection*

### Connection Resilience

- [x] **CONN-01**: Driver sets kernel reconnection parameters (ctrl_loss_tmo, reconnect_delay) on connect
  - *Outcome: ConnectionConfig passed to nvme connect via BuildConnectArgs(), defaults -1/5/0*
- [x] **CONN-02**: Driver uses exponential backoff with jitter for retry operations
  - *Outcome: RetryWithBackoff using k8s wait.ExponentialBackoffWithContext, 10% jitter*
- [x] **CONN-03**: User can configure timeouts via StorageClass parameters
  - *Outcome: ParseNVMEConnectionParams extracts ctrlLossTmo/reconnectDelay/keepAliveTmo*

### Observability

- [x] **OBS-01**: Driver posts Kubernetes events for mount failures and recovery actions
  - *Outcome: EventPoster with 7 event types, posts to PVC via client-go EventRecorder*
- [x] **OBS-02**: Driver reports volume health condition via NodeGetVolumeStats
  - *Outcome: VolumeCondition always returned, Abnormal=true if stale mount detected*
- [x] **OBS-03**: Driver exposes Prometheus metrics endpoint for monitoring
  - *Outcome: 10 metrics with rds_csi_ prefix, HTTP server on :9809*

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Advanced Recovery

- **RECOV-01**: Background health monitoring goroutine with periodic checks
- **RECOV-02**: Automatic cache invalidation on connection state changes
- **RECOV-03**: Proactive mount health checks (before pod requests)

### Integration

- **INTEG-01**: external-health-monitor sidecar integration
- **INTEG-02**: Volume attachment fencing for multi-node safety

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Volume snapshots | Separate milestone, not related to reliability |
| Controller HA | Separate milestone, requires leader election |
| Volume encryption | Separate milestone, different concern |
| NVMe multipath | Single RDS controller, multipath not applicable |
| Automatic pod restart | CSI spec says drivers report, orchestrators act |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| DEVP-01 | Phase 1 | Complete |
| DEVP-02 | Phase 1 | Complete |
| DEVP-03 | Phase 1 | Complete |
| MOUNT-01 | Phase 2 | Complete |
| MOUNT-02 | Phase 2 | Complete |
| MOUNT-03 | Phase 2 | Complete |
| CONN-01 | Phase 3 | Complete |
| CONN-02 | Phase 3 | Complete |
| CONN-03 | Phase 3 | Complete |
| OBS-01 | Phase 4 | Complete |
| OBS-02 | Phase 4 | Complete |
| OBS-03 | Phase 4 | Complete |

**Coverage:**
- v1 requirements: 12 total
- Mapped to phases: 12
- Shipped: 12
- Unmapped: 0

---

## Milestone Summary

**Shipped:** 12 of 12 v1 requirements
**Adjusted:** None — all requirements implemented as specified
**Dropped:** None

---
*Archived: 2026-01-31 as part of v1 milestone completion*
