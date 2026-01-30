# Requirements: RDS CSI Driver - Production Stability

**Defined:** 2026-01-30
**Core Value:** Volumes remain accessible after NVMe-oF reconnections

## v1 Requirements

Requirements for production stability release. Each maps to roadmap phases.

### Device Path Resolution

- [ ] **DEVP-01**: Driver resolves device path by NQN lookup in sysfs, not hardcoded paths
- [ ] **DEVP-02**: Driver detects orphaned subsystems (appear connected but no device path)
- [ ] **DEVP-03**: Driver caches NQN-to-path mappings with TTL and validation

### Mount Validation

- [ ] **MOUNT-01**: Driver detects stale mounts by comparing mount device vs current device path
- [ ] **MOUNT-02**: Driver automatically remounts staging path when stale mount detected
- [ ] **MOUNT-03**: Driver can force unmount stuck mounts that won't unmount normally

### Connection Resilience

- [ ] **CONN-01**: Driver sets kernel reconnection parameters (ctrl_loss_tmo, reconnect_delay) on connect
- [ ] **CONN-02**: Driver uses exponential backoff with jitter for retry operations
- [ ] **CONN-03**: User can configure timeouts via StorageClass parameters

### Observability

- [ ] **OBS-01**: Driver posts Kubernetes events for mount failures and recovery actions
- [ ] **OBS-02**: Driver reports volume health condition via NodeGetVolumeStats
- [ ] **OBS-03**: Driver exposes Prometheus metrics endpoint for monitoring

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

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| DEVP-01 | TBD | Pending |
| DEVP-02 | TBD | Pending |
| DEVP-03 | TBD | Pending |
| MOUNT-01 | TBD | Pending |
| MOUNT-02 | TBD | Pending |
| MOUNT-03 | TBD | Pending |
| CONN-01 | TBD | Pending |
| CONN-02 | TBD | Pending |
| CONN-03 | TBD | Pending |
| OBS-01 | TBD | Pending |
| OBS-02 | TBD | Pending |
| OBS-03 | TBD | Pending |

**Coverage:**
- v1 requirements: 12 total
- Mapped to phases: 0
- Unmapped: 12 (pending roadmap creation)

---
*Requirements defined: 2026-01-30*
*Last updated: 2026-01-30 after initial definition*
