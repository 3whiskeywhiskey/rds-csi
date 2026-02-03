# Technology Stack for KubeVirt Live Migration Support

**Project:** RDS CSI Driver
**Research Focus:** Stack additions for KubeVirt VM live migration
**Researched:** 2026-02-03
**Overall Confidence:** HIGH

## Executive Summary

**Key Finding:** No new dependencies or libraries are required. KubeVirt live migration support requires only **capability declaration changes** in the existing CSI driver code.

**Current State:**
- Driver supports `SINGLE_NODE_WRITER` and `SINGLE_NODE_READER_ONLY` (RWO/ROX)
- ControllerPublish/Unpublish implemented with RWO enforcement
- Attachment tracking and grace period mechanism already in place
- VMI grouper for per-VMI operation serialization already exists

**Required Change:**
- Add `MULTI_NODE_MULTI_WRITER` access mode to volume capabilities
- KubeVirt checks `VMI.status.conditions.LiveMigratable` based on PVC access mode
- With ReadWriteMany (RWX) support advertised, KubeVirt will enable live migration

**No new stack components needed.** This is a configuration/capability change, not a technology addition.

---

## Current Stack (No Changes)

The existing stack is sufficient for live migration support:

| Component | Version | Purpose | Status |
|-----------|---------|---------|--------|
| Go | 1.24 | Driver implementation | No change |
| github.com/container-storage-interface/spec | v1.10.0 | CSI spec types (VolumeCapability_AccessMode) | No change |
| NVMe/TCP | Protocol | Data plane (multi-node capable) | Already supports concurrent access |
| SSH/RouterOS CLI | Protocol | Control plane (volume management) | No change |
| k8s.io/client-go | v0.28.0 | Kubernetes API client | No change |
| Attachment Manager | pkg/attachment | RWO enforcement, grace period | Already handles handoff |
| VMI Grouper | pkg/driver | Per-VMI operation serialization | Already exists |

### Existing KubeVirt-Related Infrastructure

The driver already has significant infrastructure for KubeVirt support:

| Component | Location | Purpose |
|-----------|----------|---------|
| AttachmentManager | `pkg/attachment/manager.go` | Tracks volume-to-node attachments |
| AttachmentReconciler | `pkg/attachment/reconciler.go` | Reconciles stale attachments |
| VMIGrouper | `pkg/driver/vmi_grouper.go` | Per-VMI operation serialization |
| Grace period support | `manager.go:IsWithinGracePeriod()` | Allows attachment handoff |
| EventPoster | `pkg/driver/events.go` | Kubernetes events for lifecycle |

---

## What IS Needed: Capability Declaration

### Code Changes Required

**File:** `pkg/driver/driver.go`

**Current (lines 252-261):**
```go
func (d *Driver) addVolumeCapabilities() {
	d.vcaps = []*csi.VolumeCapability_AccessMode{
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		},
	}
}
```

**Required Addition:**
```go
func (d *Driver) addVolumeCapabilities() {
	d.vcaps = []*csi.VolumeCapability_AccessMode{
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		},
		{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
}
```

**That's it for capability declaration.** However, additional behavioral changes are needed in the attachment manager.

---

## CSI Access Mode Reference

From the CSI spec (v1.10.0), the complete access mode enum:

| Mode | Value | Definition |
|------|-------|------------|
| UNKNOWN | 0 | Not specified |
| SINGLE_NODE_WRITER | 1 | Can only be published once as read/write on a single node |
| SINGLE_NODE_READER_ONLY | 2 | Can only be published once as readonly on a single node |
| MULTI_NODE_READER_ONLY | 3 | Can be published as readonly at multiple nodes simultaneously |
| MULTI_NODE_SINGLE_WRITER | 4 | Multiple nodes, only one can write |
| **MULTI_NODE_MULTI_WRITER** | **5** | **Can be published as read/write at multiple nodes simultaneously** |
| SINGLE_NODE_SINGLE_WRITER | 6 | (Alpha) Single workload on single node |
| SINGLE_NODE_MULTI_WRITER | 7 | (Alpha) Multiple workloads on single node |

**For KubeVirt Live Migration:** `MULTI_NODE_MULTI_WRITER` (5) is required.

**Source:** [CSI Proto Definition](https://github.com/container-storage-interface/spec/blob/master/csi.proto)

---

## Why This Works: NVMe/TCP Multi-Node Characteristics

### NVMe/TCP Native Multi-Attach

NVMe/TCP protocol inherently supports multiple initiators connecting to the same target:

1. **Protocol-native multipath support** - Multiple nodes can connect to the same NQN simultaneously
2. **No filesystem state on nodes** - Block devices have no local state that prevents multi-attach
3. **RDS export model** - RouterOS exports volumes as NVMe/TCP targets; clients connect independently
4. **Built-in NVMe multipath** - RHEL 9+/modern Linux kernels enable NVMe multipath by default

**Key insight from Portworx:** "VMs only need volume visibility (not simultaneous access) during migration." The source node writes until cutover, then the destination node takes over. True concurrent writes don't happen.

**Source:** [Portworx - Kubernetes Native Virtualization Meets Enterprise Storage](https://portworx.com/blog/kubernetes-native-virtualization-meets-enterprise-storage/)

### Block Mode vs Filesystem Mode

**Critical Distinction:**
- **Block volumes** (volumeMode: Block): Support `MULTI_NODE_MULTI_WRITER` without complications
- **Filesystem volumes** (volumeMode: Filesystem): Require distributed filesystem or careful coordination

**RDS CSI Driver:** Supports both modes. For live migration:
- Block mode: Native multi-attach, no issues
- Filesystem mode: Requires cluster-aware filesystem (ext4/xfs NOT cluster-aware)

**Implication:** Live migration works best with `volumeMode: Block` or requires filesystem-level coordination for mount mode.

**Source:** [Kubernetes CSI - Raw Block Volume](https://kubernetes-csi.github.io/docs/raw-block.html) - "Block volumes are much more likely to support multi-node flavors of VolumeCapability_AccessMode_Mode than mount volumes"

---

## Attachment Manager Behavioral Changes

### Current Behavior

The existing attachment manager enforces strict RWO:

```go
// pkg/attachment/manager.go - TrackAttachment
if exists {
    if existing.NodeID == nodeID {
        return nil  // Idempotent
    }
    return fmt.Errorf("volume %s already attached to node %s", volumeID, existing.NodeID)
}
```

### Required Behavioral Change for RWX

For `MULTI_NODE_MULTI_WRITER` volumes, dual attachment must be allowed:

```go
// Conceptual change - check access mode before rejecting
if exists {
    if existing.NodeID == nodeID {
        return nil  // Idempotent
    }
    // NEW: Allow dual attachment for RWX volumes
    if accessMode == MULTI_NODE_MULTI_WRITER {
        // Track secondary attachment for migration
        return am.trackSecondaryAttachment(ctx, volumeID, nodeID)
    }
    return fmt.Errorf("volume %s already attached to node %s", volumeID, existing.NodeID)
}
```

### Migration Window Handling

During live migration, the driver sees this sequence:

1. **ControllerPublishVolume(volume, targetNode)** - Target node requests attachment
2. **ControllerUnpublishVolume(volume, sourceNode)** - Source node releases (after VM cutover)

**The window between steps 1 and 2 is the dual-attachment period.**

**Existing grace period (30s)** can be repurposed:
- Currently: Allows new attachment if recent detach
- New use: Configurable migration timeout for dual-attachment window

### Recommended: Extend AttachmentState

```go
// pkg/attachment/types.go
type AttachmentState struct {
    VolumeID       string
    NodeID         string      // Primary attachment
    AttachedAt     time.Time
    DetachedAt     *time.Time

    // NEW: Migration support
    SecondaryNodeID    string     // Secondary attachment during migration
    MigrationStartedAt *time.Time // When dual-attach began
    AccessMode         int32      // CSI access mode for this volume
}
```

**No new dependencies required** - this extends existing structures.

---

## KubeVirt Live Migration Flow with Multi-Attach

### How KubeVirt Uses ReadWriteMany (RWX)

When a VM is started, KubeVirt calculates the `VMI.status.conditions.LiveMigratable` condition:

```
Check PVC Access Mode
├─ ReadWriteMany (RWX) → LiveMigratable: True (memory-only migration)
├─ ReadWriteOnce (RWO) → LiveMigratable: False, reason: "DisksNotLiveMigratable"
└─ Error: "cannot migrate VMI: PVC X is not shared"
```

**Source:** [KubeVirt - Live Migration](https://kubevirt.io/user-guide/compute/live_migration/)

### Migration Method Classification

Based on storage capabilities:
- **LiveMigration**: Only memory copied (requires RWX volumes on shared storage)
- **BlockMigration**: Memory + disk blocks copied (for non-shared storage)

**With RWX support:** RDS CSI driver enables `LiveMigration` method (memory-only transfer), which is faster and doesn't require disk copying.

**Source:** [KubeVirt - Live Migration](https://kubevirt.io/2020/Live-migration.html)

### Migration Workflow (Detailed)

```
1. User initiates VM migration
   │
2. KubeVirt checks VMI.status.conditions.LiveMigratable
   │ └─ If False: Migration rejected
   │
3. KubeVirt creates virt-launcher pod on target node
   │
4. CSI external-attacher calls ControllerPublishVolume(targetNode)
   │ └─ RDS CSI: Allow dual attachment (MULTI_NODE_MULTI_WRITER)
   │
5. NodeStageVolume/NodePublishVolume on target node
   │ └─ NVMe/TCP connect (second initiator to same target)
   │
6. VM memory migration proceeds
   │
7. Cutover: VM paused on source, resumed on target
   │
8. Source virt-launcher pod terminates
   │
9. CSI external-attacher calls ControllerUnpublishVolume(sourceNode)
   │ └─ RDS CSI: Remove primary attachment, promote secondary
   │
10. NodeUnpublishVolume/NodeUnstageVolume on source node
    └─ NVMe/TCP disconnect (first initiator disconnects)
```

---

## StorageClass Configuration

### Current StorageClass (RWO only)

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rds-nvme
provisioner: rds.csi.srvlab.io
parameters:
  rdsAddress: "10.42.241.3"
  nvmeAddress: "10.42.68.1"
  nvmePort: "4420"
  volumePath: "/storage-pool/metal-csi"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
```

### Post-Change: No StorageClass Modification Required

**After adding MULTI_NODE_MULTI_WRITER capability:**
- Same StorageClass definition
- PVCs can request `accessModes: [ReadWriteMany]`
- Driver validates and accepts RWX requests
- KubeVirt sees RWX PVC → enables live migration

### User-Facing PVC Change

```yaml
# PVC for VM with live migration
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: vm-disk
spec:
  accessModes:
    - ReadWriteMany  # <-- Enables live migration
  volumeMode: Block   # <-- Recommended for NVMe/TCP
  resources:
    requests:
      storage: 10Gi
  storageClassName: rds-nvme
```

---

## Optional Enhancements (Not Required for Basic Support)

### Optional: Migration-Specific Metrics

Using existing prometheus/client_golang (no new dependency):

```go
// pkg/observability/prometheus.go - Add metrics
liveMigrationStarted   *prometheus.CounterVec   // Labels: volume_id
dualAttachmentDuration *prometheus.HistogramVec // Duration in dual-attach state
migrationTimeout       *prometheus.CounterVec   // Migration window exceeded
```

### Optional: KubeVirt API Client

For enhanced migration awareness:

| Library | Version | Purpose | When to Add |
|---------|---------|---------|-------------|
| kubevirt.io/client-go | v1.3.0 | Watch VMI migration events | If CSI signals insufficient |

**Why optional:** The driver can function without direct KubeVirt API access:
1. Using existing VMIGrouper's pod-based VMI detection
2. Relying on CSI ControllerPublish/Unpublish call patterns
3. Using grace period for handoff timing

---

## What NOT to Add (Anti-Patterns)

### DO NOT: Add NFS Layer

**Anti-Pattern:** Some solutions (OpenEBS Mayastor) add NFS Server Provisioner as RWX workaround.

**Why Avoid:**
- NFS adds latency (network overhead + filesystem overhead)
- NFS unsuitable for VM boot volumes (performance, reliability)
- RDS already provides shared block storage natively
- Portworx explicitly avoids this: "Rather than layering NFS over block storage..."

**Source:** [Portworx Blog](https://portworx.com/blog/kubernetes-native-virtualization-meets-enterprise-storage/)

### DO NOT: Implement Distributed Filesystem

**Anti-Pattern:** Add GlusterFS, CephFS, or similar for RWX filesystem mode.

**Why Avoid:**
- NVMe/TCP block devices already support multi-attach
- Distributed filesystems add complexity for minimal benefit
- KubeVirt VMs use block devices directly (virtio-blk, not filesystem)

**Recommendation:** Use `volumeMode: Block` for VMs, avoid filesystem mode.

### DO NOT: Add External Lock Manager

**Anti-Pattern:** Add etcd, Redis, or SCSI-3 persistent reservations for coordination.

**Why Avoid:**
- Attachment manager already provides per-volume locking
- NVMe/TCP doesn't have native reservation support like iSCSI
- In-memory locks with PV annotation persistence is sufficient
- VMs don't actually write concurrently - sequential during migration

### DO NOT: Disable RWO Enforcement Globally

**Anti-Pattern:** Remove RWO checking to enable RWX.

**Why Avoid:**
- RWO enforcement is per-PVC based on requested access mode
- Existing logic validates capabilities per request
- Multi-mode support is native to CSI spec (driver can support both RWO and RWX)

---

## Version Compatibility Matrix

| Component | Minimum Version | Tested With | Notes |
|-----------|-----------------|-------------|-------|
| Kubernetes | 1.26+ | 1.28 | CSI v1.5+ required |
| KubeVirt | 0.56+ | - | Live migration enabled by default |
| Linux kernel | 5.0+ | 6.x | NVMe/TCP module |
| nvme-cli | 2.0+ | 2.11 | For nvme connect/disconnect |
| RouterOS | 7.1+ | 7.x | RDS NVMe-oF export |

---

## Confidence Assessment

| Area | Confidence | Rationale |
|------|------------|-----------|
| No new dependencies needed | **HIGH** | CSI spec already includes MULTI_NODE_MULTI_WRITER enum, no external libraries required |
| NVMe/TCP multi-attach support | **HIGH** | Protocol specification and industry documentation confirm multi-initiator support |
| KubeVirt access mode check | **HIGH** | Official KubeVirt documentation and GitHub issues confirm RWX requirement |
| Capability declaration approach | **HIGH** | CSI specification and multiple driver examples show same pattern |
| Block mode recommendation | **HIGH** | CSI docs, Portworx, and NVMe characteristics confirm block mode superiority |
| Attachment manager changes | **MEDIUM** | Logic is straightforward but needs testing with real migrations |
| Migration timeout values | **MEDIUM** | Optimal timeout values need E2E testing |

---

## Implementation Phases

### Phase 1: Core RWX Support (Minimal Changes)
- Add MULTI_NODE_MULTI_WRITER to volume capabilities
- Modify ControllerPublishVolume to allow dual attachment for RWX volumes
- Add access mode awareness to attachment tracking
- **Estimated effort:** 2-3 days

### Phase 2: Enhanced Tracking (Recommended)
- Extend AttachmentState for migration info
- Add migration timeout handling
- Improve reconciler for migration cleanup
- **Estimated effort:** 2-3 days

### Phase 3: Metrics and Observability (Optional)
- Add migration-specific Prometheus metrics
- Dashboard updates
- Alerting rules for stuck migrations
- **Estimated effort:** 1-2 days

---

## Sources

**Primary (HIGH confidence):**
- [Live Migration - KubeVirt user guide](https://kubevirt.io/user-guide/compute/live_migration/)
- [Raw Block Volume - Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/raw-block.html)
- [CSI Specification - container-storage-interface/spec](https://github.com/container-storage-interface/spec/blob/master/csi.proto)
- [Portworx - Kubernetes Native Virtualization Meets Enterprise Storage](https://portworx.com/blog/kubernetes-native-virtualization-meets-enterprise-storage/)

**Supporting (MEDIUM confidence):**
- [KubeVirt - Volume Migration](https://kubevirt.io/user-guide/storage/volume_migration/)
- [Portworx RWX Block Volumes](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration)
- [Allow block live migration of PVCs that do not support ReadWriteMany - Issue #10642](https://github.com/kubevirt/kubevirt/issues/10642)
- [OpenEBS - KubeVirt VM Live Migration with Replicated PV](https://openebs.io/docs/Solutioning/read-write-many/kubevirt)
- [Red Hat - Storage considerations for OpenShift Virtualization](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization)
