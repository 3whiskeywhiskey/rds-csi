# Requirements - v0.6.0 Block Volume Support

**Defined:** 2026-02-03
**Core Value:** Volumes remain accessible after NVMe-oF reconnections

## v1 Requirements

### Block Volume Operations

- [ ] **BLOCK-01**: NodeStageVolume skips filesystem formatting when `volumeMode: Block`
- [ ] **BLOCK-02**: NodeStageVolume stores device path metadata in staging directory for block volumes
- [ ] **BLOCK-03**: NodePublishVolume creates block device file at target path using mknod
- [ ] **BLOCK-04**: NodeUnpublishVolume unmounts and removes block device file
- [ ] **BLOCK-05**: NodeUnstageVolume handles both filesystem and block volumes correctly

### Quality and Compatibility

- [ ] **BLOCK-06**: Existing filesystem volume functionality works without regression
- [ ] **BLOCK-07**: Clear error messages for invalid volume mode combinations

### Hardware Validation

- [ ] **VAL-01**: KubeVirt VM boots successfully with RDS block volume on metal cluster
- [ ] **VAL-02**: KubeVirt live migration completes end-to-end (VM migrates between nodes)
- [ ] **VAL-03**: Migration metrics and events are emitted correctly during validation

## Future Requirements

(Deferred to later milestones)

- CSI volume snapshots for block volumes
- Volume cloning support
- Volume expansion for block volumes
- Cluster filesystem support (GFS2/OCFS2) for true RWX filesystem volumes

## Out of Scope

- **Filesystem-mode RWX volumes**: v0.5.0 explicitly rejects these to prevent data corruption; block-only RWX is the correct approach for KubeVirt
- **Volume encryption**: Separate concern, not related to block vs filesystem mode
- **NFS wrapper**: Defeats NVMe/TCP latency benefits; users should use NFS CSI if needed

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| BLOCK-01 | TBD | Pending |
| BLOCK-02 | TBD | Pending |
| BLOCK-03 | TBD | Pending |
| BLOCK-04 | TBD | Pending |
| BLOCK-05 | TBD | Pending |
| BLOCK-06 | TBD | Pending |
| BLOCK-07 | TBD | Pending |
| VAL-01 | TBD | Pending |
| VAL-02 | TBD | Pending |
| VAL-03 | TBD | Pending |

**Coverage:**
- v1 requirements: 10 total
- Mapped to phases: 0 (roadmap pending)
- Unmapped: 10 ⚠️

---
*Requirements defined: 2026-02-03*
*Last updated: 2026-02-03 after initial definition*
