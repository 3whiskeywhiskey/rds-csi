# Feature Landscape: KubeVirt Live Migration Support

**Domain:** CSI Driver for KubeVirt Virtual Machine Live Migration
**Researched:** 2026-02-03
**Updated:** 2026-02-03
**Confidence:** MEDIUM (verified with official KubeVirt docs and multiple CSI implementations)

## Executive Summary

KubeVirt determines VM live migration capability based on a **single critical factor: PVC access mode must be ReadWriteMany (RWX)**. A VM shows "LiveMigratable: false" when using ReadWriteOnce (RWO) volumes because the source and destination nodes must both access the volume simultaneously during migration.

The RDS CSI driver currently only advertises `SINGLE_NODE_WRITER` (RWO) and `SINGLE_NODE_READER_ONLY` (ROX). To enable live migration, the driver must add `MULTI_NODE_MULTI_WRITER` support, which requires solving the multi-attach problem for block storage.

**Key Research Findings:**

1. **KubeVirt LiveMigratable Condition**: The VMI status includes a `LiveMigratable` condition calculated at VM startup. When false, the message explicitly states: "cannot migrate VMI: PVC is not shared, live migration requires that all PVCs must be shared (using ReadWriteMany access mode)"

2. **Migration Workflow Timing**: During live migration, two virt-launcher pods exist simultaneously (source and target). The migration process uses:
   - Default progress timeout: 150 seconds
   - Default completion timeout per GiB: 800 seconds
   - Keepalive mechanism: 5s interval, 5 retries (30s total before abort)

3. **CSI Driver Approaches for RWX Block Storage**:
   - **Cluster Filesystem** (GFS2/OCFS2): Proven approach, requires DLM
   - **Native Multi-Attach**: Storage-level support for concurrent access
   - **NFS Wrapper**: Simple but adds latency (anti-pattern for NVMe/TCP)

## Table Stakes

Features that KubeVirt users expect for live migration. Missing these = live migration doesn't work.

| Feature | Why Expected | Complexity | Existing Support |
|---------|--------------|------------|------------------|
| **ReadWriteMany Access Mode** | KubeVirt checks PVC access mode at VMI startup; RWX required for LiveMigratable: true | **HIGH** | Driver only advertises SINGLE_NODE_WRITER |
| **Simultaneous Volume Attachment** | Source and destination nodes both need volume mounted during migration window (can be 30s to minutes) | **HIGH** | Grace period (30s) exists but enforces RWO |
| **VolumeMode: Block Support** | KubeVirt prefers block mode for VMs; better performance, less overhead | **COMPLETE** | Already implemented |
| **Publish Context with NVMe Parameters** | Node plugin needs nvme_address, nvme_port, nvme_nqn to connect | **COMPLETE** | Already implemented |
| **Idempotent ControllerUnpublishVolume** | Must succeed even if volume not attached; prevents migration cleanup failures | **COMPLETE** | Already implemented |
| **Grace Period for Handoff** | Attachment handoff must allow brief overlap during migration | **MEDIUM** | 30s grace period exists; may need tuning |

## Differentiators

Features that would make RDS CSI driver stand out for KubeVirt workloads.

| Feature | Value Proposition | Complexity | Dependency |
|---------|-------------------|------------|------------|
| **Migration-Aware Multi-Attach** | Allow 2-node attachment only during active migration; safer than full RWX | **MEDIUM** | Detect migration via VMI annotations |
| **Automatic Cluster FS Setup** | Transparent GFS2/OCFS2 formatting for RWX volumes | **HIGH** | DLM deployment required |
| **Migration Performance Metrics** | Track handoff timing, memory transfer rate, attachment overlap duration | **LOW** | Extends existing Prometheus metrics |
| **Live Migration Pre-Flight Validation** | ValidateVolumeCapabilities returns clear error if PVC won't support migration | **LOW** | Check access mode in validation |
| **NVMe Namespace Multi-Host** | Native NVMe protocol multi-host access without cluster filesystem | **VERY HIGH** | Requires RDS RouterOS support verification |
| **VMI Serialization Enhancement** | Already have per-VMI locking; extend for migration coordination | **LOW** | Existing VMIGrouper infrastructure |

## Anti-Features

Features to explicitly NOT build. Common mistakes in this domain.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **NFS-Per-PVC Wrapper** | Adds 7-10ms latency; defeats NVMe/TCP advantage (1-3ms) | Use cluster filesystem or native multi-attach |
| **Unrestricted RWX for All Volumes** | Security risk; allows unintended concurrent writes | RWX only for volumes explicitly requesting it via StorageClass |
| **RWX with ext4/xfs** | Silent data corruption when two nodes write simultaneously | Require GFS2/OCFS2 for RWX volumes; reject ext4/xfs |
| **Full N-Node Multi-Attach** | KubeVirt migration only needs 2 nodes; N-node is complex and risky | Limit to 2 simultaneous attachments maximum |
| **Blocking Unpublish on NVMe Disconnect** | Old node may be unreachable during failover; causes hangs | Unpublish returns success; disconnect is best-effort |
| **Extending Grace Period to Minutes** | Increases window for dual-write corruption risk | Keep short; let KubeVirt handle migration timing |

## KubeVirt Live Migration Workflow Details

### Migration Phases (for CSI understanding)

```
Source Node                          Destination Node
-----------                          ----------------
1. VMI Running
   Volume attached (RWX)

2. Migration starts
   (VirtualMachineInstanceMigration  Target virt-launcher pod created
    object created)                  ControllerPublishVolume called
                                     (RWX allows multi-attach)

3. Pre-copy phase                    Volume attached (RWX)
   Memory/CPU migration              Both nodes have NVMe connection
   Storage I/O coordinated           Target reads; source still writes
   by QEMU/KVM (not Kubernetes)

4. Cutover
   Source paused briefly             Target VM activated
   Final memory sync                 Target gains write access

5. Completion
   Source virt-launcher terminates   VMI Running on destination
   ControllerUnpublishVolume called  Volume attached (sole owner)
   Volume detached from source
```

### Timing Considerations

| Phase | Typical Duration | CSI Impact |
|-------|-----------------|------------|
| Target pod scheduling | 1-30 seconds | None (before attach) |
| ControllerPublishVolume | <1 second | Must allow if RWX |
| NodeStageVolume on target | 1-5 seconds | NVMe connect, mount |
| Memory pre-copy | 30s-30min (varies by VM size) | Both nodes attached |
| Cutover | 100ms-5s | Brief dual-access window |
| Source cleanup | 1-10 seconds | UnpublishVolume |

### Key Insight: QEMU Coordinates I/O

During live migration, QEMU/KVM handles I/O coordination between source and destination:
- Source VM continues writing normally
- Destination reads from same volume to pre-copy data
- At cutover, QEMU ensures no simultaneous writes
- **Kubernetes does not provide concurrency guarantees for RWX block**

This means the CSI driver only needs to:
1. Allow multi-attach for RWX volumes (ControllerPublishVolume)
2. Trust KubeVirt/QEMU to coordinate I/O safely
3. Clean up old attachment after migration (ControllerUnpublishVolume)

## Feature Dependencies

```
ReadWriteMany Access Mode (CSI driver capability)
    |
    +---> Driver advertises MULTI_NODE_MULTI_WRITER
    |
    +---> ControllerPublishVolume allows 2 attachments for RWX
            |
            +---> Option A: Cluster Filesystem (GFS2/OCFS2)
            |       |
            |       +---> DLM required in cluster
            |       +---> Auto-format on first mount (differentiator)
            |
            +---> Option B: Trust QEMU for I/O coordination (simpler)
            |       |
            |       +---> Limit to 2-node attachment max
            |       +---> Document: "Only for KubeVirt migration"
            |       +---> Risk: User mounts RWX manually = corruption
            |
            +---> Option C: NVMe Multi-Host Namespace (future)
                    |
                    +---> Requires RDS RouterOS verification
                    +---> Still needs FS coordination layer

Existing Features (leverage)
    |
    +---> Grace period (30s) for attachment handoff
    +---> VMI serialization (per-VMI locking)
    +---> Attachment conflict metrics
    +---> Kubernetes events for conflicts
```

## Implementation Approaches (Ranked)

### Option A: Trust QEMU + 2-Node Limit (Recommended for MVP)

**What:** Allow RWX by permitting 2 simultaneous attachments; trust KubeVirt/QEMU to coordinate I/O.

**Pros:**
- Simplest implementation (modify ControllerPublishVolume check)
- No cluster filesystem complexity
- Matches KubeVirt's actual behavior (QEMU coordinates I/O)
- Works with existing ext4/xfs volumes

**Cons:**
- Risk if user mounts RWX volume outside KubeVirt context
- Corruption possible if both mounts write simultaneously (non-KubeVirt)
- Requires documentation warning

**Implementation:**
1. Add `MULTI_NODE_MULTI_WRITER` to vcaps (driver.go)
2. Modify ControllerPublishVolume: allow up to 2 attachments for RWX
3. Add StorageClass parameter `allowMigration: true` (opt-in)
4. Document: "RWX volumes are safe only for KubeVirt live migration"

**Code Impact (estimated):**
- driver.go: Add MULTI_NODE_MULTI_WRITER capability (~5 lines)
- controller.go: Modify attachment check for RWX (~20 lines)
- params.go: Add allowMigration parameter (~10 lines)

### Option B: Cluster Filesystem (GFS2)

**What:** Format RWX volumes with GFS2, require DLM for safe concurrent access.

**Pros:**
- True multi-node write safety
- No corruption risk even outside KubeVirt
- Proven in OpenShift Virtualization

**Cons:**
- Requires DLM deployment (cluster-wide dependency)
- Complex NodeStageVolume logic (detect RWX, format GFS2)
- Performance overhead vs ext4
- Operational complexity

**Implementation:**
1. Add GFS2 mkfs support to NodeStageVolume
2. Deploy DLM as Helm dependency or prerequisite
3. Detect RWX at mount time, select GFS2 vs ext4
4. Add StorageClass parameter `fsType: gfs2`

**Code Impact (estimated):**
- node.go: GFS2 formatting, DLM check (~100 lines)
- mount.go: GFS2 mount options (~30 lines)
- Helm chart: DLM deployment (~200 lines)

### Option C: NFS Wrapper (Not Recommended)

**What:** Deploy NFS server per RWX PVC, export underlying RWO volume.

**Cons:** Adds 7-10ms latency; defeats NVMe/TCP purpose.

**Status:** Do not implement for NVMe/TCP workloads.

### Option D: NVMe Multi-Host (Future Research)

**What:** Use NVMe namespace sharing for native multi-host access.

**Status:** Requires investigation of MikroTik RouterOS NVMe/TCP capabilities.

## MVP Recommendation

For enabling KubeVirt live migration with minimal complexity:

**Phase 1: MVP (Option A - Trust QEMU)**
1. Add `MULTI_NODE_MULTI_WRITER` capability
2. Allow 2-node attachment for RWX volumes
3. Document KubeVirt-only use case
4. Extend existing metrics for migration tracking

**Phase 2: Safety (Option B - Cluster FS)**
1. Add GFS2 support for users who need true RWX safety
2. Document DLM requirement
3. Make GFS2 optional via StorageClass

**Rationale:** Option A gets live migration working quickly. Option B adds safety for broader RWX use cases.

## Open Questions

| Question | Impact | Investigation |
|----------|--------|---------------|
| Does MikroTik RDS support NVMe multi-host namespaces? | HIGH | Test nvme connect from 2 nodes simultaneously |
| Is 30s grace period sufficient for large VM migration? | MEDIUM | Test with 8GB+ memory VMs |
| Can we detect KubeVirt migration in progress from CSI? | MEDIUM | Check VMI annotations/labels from ControllerPublishVolume |
| What happens if RWX volume is mounted by non-KubeVirt pod? | HIGH | Corruption; need documentation warning |
| Does existing VMI serialization help or hinder migration? | LOW | Verify locking doesn't block target attachment |

## Sources and Confidence Levels

### HIGH Confidence (Official Documentation)

- [KubeVirt Live Migration Requirements](https://kubevirt.io/user-guide/compute/live_migration/) - "Virtual machines using a PersistentVolumeClaim (PVC) must have a shared ReadWriteMany (RWX) access mode to be live migrated"
- [KubeVirt Live Migration in 2020](https://kubevirt.io/2020/Live-migration.html) - Migration method details, QEMU coordination
- [CSI Spec AccessMode](https://github.com/container-storage-interface/spec/blob/master/spec.md) - MULTI_NODE_MULTI_WRITER definition
- Current RDS CSI driver code (pkg/driver/driver.go:253-260) - Only SINGLE_NODE_WRITER/SINGLE_NODE_READER_ONLY

### MEDIUM Confidence (Multiple Corroborating Sources)

- [Portworx RWX Raw Block for Live Migration](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration) - 2-node limit approach
- [democratic-csi RWX iSCSI Issue](https://github.com/democratic-csi/democratic-csi/issues/285) - QEMU coordinates I/O, Kubernetes doesn't
- [OpenShift Virtualization Storage](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization) - RWX requirement
- [HPE CSI RWX Raw Block](https://access.redhat.com/solutions/7128978) - Multi-attach for KubeVirt

### LOW Confidence (Needs Verification)

- NVMe/TCP multi-host namespace support in MikroTik RouterOS - No documentation found
- Performance impact of GFS2 vs ext4 on NVMe/TCP - No benchmarks
- DLM overhead in Kubernetes - Varies by implementation

## References

**KubeVirt Documentation:**
- [Live Migration - KubeVirt user guide](https://kubevirt.io/user-guide/compute/live_migration/)
- [Live Migration in KubeVirt (2020)](https://kubevirt.io/2020/Live-migration.html)
- [Volume Migration - KubeVirt user guide](https://kubevirt.io/user-guide/storage/volume_migration/)
- [Migration Policies - KubeVirt user guide](https://kubevirt.io/user-guide/cluster_admin/migration_policies/)

**CSI Driver Implementations:**
- [democratic-csi RWX Support](https://github.com/democratic-csi/democratic-csi/issues/285)
- [Portworx RWX Raw Block for Live Migration](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration)
- [KubeVirt CSI Driver](https://github.com/kubevirt/csi-driver)

**Access Modes:**
- [CSI Spec - Access Modes](https://github.com/container-storage-interface/spec/blob/master/spec.md)
- [Kubernetes CSI Raw Block Volume](https://kubernetes-csi.github.io/docs/raw-block.html)

**NVMe Multi-Host:**
- [NVMe Over Fabrics Part Two](https://nvmexpress.org/nvme-over-fabrics-part-two/)
- [Ceph NVMe-oF Multi-Attach Issue](https://github.com/ceph/ceph-nvmeof/issues/476)
