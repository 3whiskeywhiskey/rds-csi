# Requirements: RDS CSI Driver

**Defined:** 2026-02-17
**Core Value:** Volumes remain accessible after NVMe-oF reconnections

## v0.11.0 Requirements

Requirements for v0.11.0 Data Protection milestone. Each maps to roadmap phases.

### Snapshot Implementation

- [ ] **SNAP-01**: CreateSnapshot creates a CoW copy of a volume via `/disk add copy-from`
- [ ] **SNAP-02**: DeleteSnapshot removes the copied disk entry and underlying file
- [ ] **SNAP-03**: RestoreSnapshot creates a new volume from a snapshot via copy-from
- [ ] **SNAP-04**: ListSnapshots returns snapshot metadata (source volume, creation time, size)
- [ ] **SNAP-05**: Snapshot survives deletion of source volume (independent CoW copy)
- [ ] **SNAP-06**: Snapshot disk is not NVMe-exported (immutable, no writes possible)

### Snapshot Testing

- [ ] **TEST-01**: Snapshot create/restore/delete validated against real RDS hardware
- [ ] **TEST-02**: Mock RDS server updated to use copy-from semantics instead of Btrfs subvolume
- [ ] **TEST-03**: CSI sanity tests pass for snapshot operations

### Scheduled Snapshots

- [ ] **SCHED-01**: CronJob manifest creates VolumeSnapshot on configurable schedule
- [ ] **SCHED-02**: Retention cleanup deletes snapshots older than configurable age
- [ ] **SCHED-03**: CronJob template included in Helm chart

### Resilience Regression

- [ ] **RESIL-01**: NVMe reconnect after network interruption preserves mounted volumes
- [ ] **RESIL-02**: RDS restart doesn't cause data loss on connected volumes
- [ ] **RESIL-03**: Node failure cleanup removes stale VolumeAttachments

## Future Requirements

### Snapshot Enhancements

- **SNAP-F01**: Snapshot-based volume cloning (CreateVolume from volume, not just from snapshot)
- **SNAP-F02**: Cross-RDS snapshot replication for disaster recovery

## Out of Scope

| Feature | Reason |
|---------|--------|
| Btrfs subvolume snapshots | File-backed disks aren't subvolumes; copy-from is the correct approach |
| Application-consistent snapshots | Requires app-level quiesce hooks (Velero territory, not CSI driver) |
| Snapshot scheduling operator | CronJob is simpler and sufficient; operator adds unnecessary complexity |
| Read-only snapshot mounting | Snapshots are for backup/restore, not for mounting as read-only volumes |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| SNAP-01 | Phase 29 | Pending |
| SNAP-02 | Phase 29 | Pending |
| SNAP-03 | Phase 29 | Pending |
| SNAP-04 | Phase 29 | Pending |
| SNAP-05 | Phase 29 | Pending |
| SNAP-06 | Phase 29 | Pending |
| TEST-01 | Phase 30 | Pending |
| TEST-02 | Phase 30 | Pending |
| TEST-03 | Phase 30 | Pending |
| SCHED-01 | Phase 31 | Pending |
| SCHED-02 | Phase 31 | Pending |
| SCHED-03 | Phase 31 | Pending |
| RESIL-01 | Phase 32 | Pending |
| RESIL-02 | Phase 32 | Pending |
| RESIL-03 | Phase 32 | Pending |

**Coverage:**
- v0.11.0 requirements: 15 total
- Mapped to phases: 15
- Unmapped: 0

---
*Requirements defined: 2026-02-17*
*Last updated: 2026-02-17 after roadmap creation (phases 29-32)*
