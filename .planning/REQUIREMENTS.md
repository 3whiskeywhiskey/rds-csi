# Requirements - v0.5.0 KubeVirt Live Migration

## v1 Requirements

### RWX Capability

- [ ] **RWX-01**: User can create PVC with `accessModes: [ReadWriteMany]` and `volumeMode: Block` that is accepted by the driver
- [ ] **RWX-02**: User receives an error when creating PVC with `accessModes: [ReadWriteMany]` and `volumeMode: Filesystem` (explicitly rejected to prevent corruption)
- [ ] **RWX-03**: Volume can be attached to exactly 2 nodes simultaneously during migration (not unlimited multi-attach)

### Migration Safety

- [ ] **SAFETY-01**: Migration timeout (5 min default, configurable) allows dual-attachment window without triggering RWO conflict
- [ ] **SAFETY-02**: Non-migration dual-attach attempts fail immediately with FAILED_PRECONDITION (not delayed by grace period)
- [ ] **SAFETY-03**: AttachmentState tracks secondary attachment with migration timestamp for cleanup
- [ ] **SAFETY-04**: NodeUnstageVolume verifies no open file descriptors before issuing NVMe disconnect

### Observability

- [ ] **OBS-01**: Prometheus metrics expose migrations_total, migration_duration_seconds, active_migrations gauge
- [ ] **OBS-02**: Kubernetes events posted to PVC: MigrationStarted, MigrationCompleted, MigrationFailed
- [ ] **OBS-03**: User documentation explains RWX is safe only for KubeVirt live migration (not general RWX workloads)

## Future Requirements

(Deferred to later milestones)

- Cluster filesystem support (GFS2/OCFS2) for true RWX filesystem volumes
- RDS-level namespace reservations/fencing for split-brain protection
- KubeVirt API client integration for richer migration awareness

## Out of Scope

- **NFS wrapper**: Defeats NVMe/TCP latency benefits; user should use NFS CSI if they need NFS
- **Unlimited multi-attach**: 2-node limit is sufficient for migration; more would enable unsafe usage
- **Automatic pod restart**: CSI spec says drivers report issues; orchestrators (kubelet/scheduler) act on them

## Traceability

| Requirement | Phase | Verified |
|-------------|-------|----------|
| RWX-01 | Phase 8 | [ ] |
| RWX-02 | Phase 8 | [ ] |
| RWX-03 | Phase 8 | [ ] |
| SAFETY-01 | Phase 9 | [ ] |
| SAFETY-02 | Phase 9 | [ ] |
| SAFETY-03 | Phase 9 | [ ] |
| SAFETY-04 | Phase 9 | [ ] |
| OBS-01 | Phase 10 | [ ] |
| OBS-02 | Phase 10 | [ ] |
| OBS-03 | Phase 10 | [ ] |

---
*Requirements defined: 2026-02-03*
*Traceability updated: 2026-02-03*
