# Architecture Research: KubeVirt Live Migration Integration

**Domain:** CSI Controller Service - KubeVirt VM Migration Support
**Researched:** 2026-01-30
**Updated:** 2026-02-03 (KubeVirt Live Migration Architecture)
**Confidence:** HIGH

## Executive Summary

The RDS CSI driver architecture for KubeVirt integration is **complete**. All necessary components exist and are functional. This document clarifies what is supported, what is not, and how the existing architecture handles VM migration scenarios.

**Key Clarification:** KubeVirt "live migration" (zero-downtime memory+disk migration) requires RWX (ReadWriteMany) storage. The RDS CSI driver only supports RWO (ReadWriteOnce) due to NVMe/TCP protocol limitations. However, the driver fully supports **VM restart handoff** - fast failover where a VM restarts on a new node within seconds using the grace period mechanism.

## What Is vs Is Not Supported

| Feature | Supported | Reason |
|---------|-----------|--------|
| **True Live Migration** | No | Requires RWX - both nodes need simultaneous volume access during memory sync |
| **VM Restart Handoff** | Yes | Grace period (30s) allows fast reattachment on new node after detach |
| **Fast Failover** | Yes | Node failure triggers ControllerUnpublish, new node attaches within grace period |
| **Concurrent Operations** | Yes | VMIGrouper serializes per-VMI operations to prevent race conditions |
| **Stale Attachment Cleanup** | Yes | AttachmentReconciler detects deleted nodes, clears attachments |
| **RWO Enforcement** | Yes | AttachmentManager blocks multi-node attach attempts |

## Current Architecture Components

### Integration Points (Existing)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ CSI Controller Plugin                                                     │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │ ControllerServer (pkg/driver/controller.go)                          │ │
│  │                                                                       │ │
│  │  ControllerPublishVolume()                                           │ │
│  │    │                                                                  │ │
│  │    ├── VMIGrouper.LockVMI() ──────────────────────────────────────┐ │ │
│  │    │   Serialize operations per-VMI to prevent races              │ │ │
│  │    │                                                               │ │ │
│  │    ├── AttachmentManager.GetAttachment() ─────────────────────────┤ │ │
│  │    │   Check if volume already attached                           │ │ │
│  │    │                                                               │ │ │
│  │    ├── AttachmentManager.IsWithinGracePeriod() ───────────────────┤ │ │
│  │    │   Allow handoff if recently detached (30s default)           │ │ │
│  │    │                                                               │ │ │
│  │    ├── validateBlockingNodeExists() ──────────────────────────────┤ │ │
│  │    │   Self-heal if blocking node was deleted                     │ │ │
│  │    │                                                               │ │ │
│  │    └── AttachmentManager.TrackAttachment() ───────────────────────┘ │ │
│  │        Record new attachment, persist to PV annotation              │ │
│  │                                                                       │ │
│  │  ControllerUnpublishVolume()                                         │ │
│  │    │                                                                  │ │
│  │    └── AttachmentManager.UntrackAttachment() ─────────────────────┐ │ │
│  │        Record detach timestamp (enables grace period)             │ │ │
│  │                                                                   │ │ │
│  └───────────────────────────────────────────────────────────────────┘ │ │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │ Background Reconciler (pkg/attachment/reconciler.go)                 │ │
│  │                                                                       │ │
│  │  AttachmentReconciler.reconcile()                                    │ │
│  │    ├── List all tracked attachments                                  │ │
│  │    ├── Check node existence via K8s API                             │ │
│  │    ├── If node deleted + outside grace period → clear attachment    │ │
│  │    └── Post events and metrics                                       │ │
│  │                                                                       │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │ Supporting Components                                                 │ │
│  │                                                                       │ │
│  │  VMIGrouper (pkg/driver/vmi_grouper.go)                              │ │
│  │    - Resolves PVC → VMI ownership via pod labels/ownerRefs          │ │
│  │    - Per-VMI mutex prevents concurrent volume ops on same VM        │ │
│  │    - Cache with TTL (60s default) reduces API calls                 │ │
│  │                                                                       │ │
│  │  AttachmentManager (pkg/attachment/manager.go)                       │ │
│  │    - In-memory state: map[volumeID]*AttachmentState                 │ │
│  │    - Per-volume locking via VolumeLockManager                       │ │
│  │    - Detach timestamps for grace period calculation                 │ │
│  │    - PV annotation persistence for restart recovery                 │ │
│  │                                                                       │ │
│  │  Prometheus Metrics (pkg/observability/prometheus.go)               │ │
│  │    - rds_csi_attachment_attach_total                                │ │
│  │    - rds_csi_attachment_detach_total                                │ │
│  │    - rds_csi_attachment_conflicts_total                             │ │
│  │    - rds_csi_attachment_grace_period_used_total                     │ │
│  │    - rds_csi_attachment_stale_cleared_total                         │ │
│  │                                                                       │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

### Data Flow: VM Restart Handoff (Supported Scenario)

```
Time: T0
Node A:       VM Running with RWO volume attached

Time: T0+1s   Node A fails / Pod terminated
              Kubelet (or external-attacher):
                → NodeUnpublishVolume (unmount pod path)
                → NodeUnstageVolume (nvme disconnect)
              external-attacher deletes VolumeAttachment
                → ControllerUnpublishVolume(volumeID, nodeA)

              AttachmentManager:
                → Delete: attachments[volumeID]
                → Record: detachTimestamps[volumeID] = T0+1s

[Grace Period Window: 30s default]

Time: T0+4s   Kubernetes reschedules VM to Node B
              external-attacher creates new VolumeAttachment
                → ControllerPublishVolume(volumeID, nodeB)

              ControllerServer:
                → VMIGrouper.LockVMI() - serialize VM operations
                → GetAttachment(volumeID) - not found (already detached)
                → IsWithinGracePeriod(volumeID, 30s)
                  └── detachTime = T0+1s, elapsed = 3s < 30s → TRUE
                → Grace period allows handoff
                → TrackAttachment(volumeID, nodeB)

              Kubelet on Node B:
                → NodeStageVolume (nvme connect, format if needed, mount)
                → NodePublishVolume (bind mount to pod path)

Time: T0+5s   VM running on Node B

Total downtime: ~4-5 seconds (VM restart, not live migration)
```

### Data Flow: True Live Migration (NOT Supported)

```
Time: T0
Node A:       VM Running (memory + disk I/O active)
                  ↓
              Pre-copy phase: memory pages copied to Node B
                  ↓
Time: T0+5s   VM still running, iterative memory sync
Node B:       VM container starting, receiving memory

              *** BOTH NODES NEED VOLUME ACCESS SIMULTANEOUSLY ***
              *** NVMe/TCP TO SAME TARGET FROM TWO INITIATORS ***
              *** NOT SUPPORTED BY RDS/NVMe PROTOCOL ***
                  ↓
Time: T0+8s   Cutover: source VM paused, final sync
                  ↓
Time: T0+9s   Destination VM resumes
Node A:       VM stopped, volume detached

REQUIREMENT: RWX (ReadWriteMany) - NOT AVAILABLE WITH THIS DRIVER
RESULT: Live migration will fail or be rejected by KubeVirt
```

## Component Details

### VMIGrouper (pkg/driver/vmi_grouper.go) - EXISTING

**Purpose:** Serialize volume operations for the same VMI to prevent race conditions in upstream kubevirt-csi-driver.

**How it works:**
1. On ControllerPublishVolume, extract PVC namespace/name from volume context
2. Query pods in namespace, find pod mounting this PVC
3. Check pod ownerReferences for `VirtualMachineInstance` or KubeVirt labels
4. Acquire per-VMI mutex before proceeding
5. Release mutex on function return (defer unlock)

**Configuration:**
```go
VMIGrouperConfig{
    K8sClient: k8sClient,
    CacheTTL:  60 * time.Second,  // Cache PVC→VMI mapping
    Enabled:   true,
}
```

### AttachmentManager (pkg/attachment/manager.go) - EXISTING

**Purpose:** Track volume-to-node attachments with grace period support.

**Key methods for KubeVirt:**
```go
// Track new attachment
TrackAttachment(ctx, volumeID, nodeID) error

// Remove attachment, record detach timestamp
UntrackAttachment(ctx, volumeID) error

// Check if recently detached (grace period)
IsWithinGracePeriod(volumeID, duration) bool

// Clear timestamp after successful handoff
ClearDetachTimestamp(volumeID)
```

**Grace period flow:**
1. UntrackAttachment records `detachTimestamps[volumeID] = time.Now()`
2. Next ControllerPublishVolume calls `IsWithinGracePeriod(volumeID, 30s)`
3. If within grace period, allows attachment to new node
4. After successful attachment, `ClearDetachTimestamp(volumeID)`

### AttachmentReconciler (pkg/attachment/reconciler.go) - EXISTING

**Purpose:** Background cleanup of stale attachments from deleted nodes.

**Reconciliation logic:**
1. Run every 5 minutes (configurable)
2. List all tracked attachments
3. For each, check if node exists via K8s API
4. If node deleted and outside grace period: clear attachment
5. Post Kubernetes event and metric

**Configuration:**
```go
ReconcilerConfig{
    Manager:     attachmentManager,
    K8sClient:   k8sClient,
    Interval:    5 * time.Minute,
    GracePeriod: 30 * time.Second,
    Metrics:     metrics,
    EventPoster: eventPoster,
}
```

### Driver Configuration (pkg/driver/driver.go) - EXISTING

**KubeVirt-related configuration:**
```go
DriverConfig{
    // Attachment reconciler settings
    EnableAttachmentReconciler:  true,
    AttachmentReconcileInterval: 5 * time.Minute,
    AttachmentGracePeriod:       30 * time.Second,

    // VMI serialization settings
    EnableVMISerialization: true,
    VMICacheTTL:            60 * time.Second,
}
```

## New Components Needed

**None required for basic KubeVirt support.** All components are implemented.

### Optional Future Enhancements

| Enhancement | Status | Value | Complexity |
|-------------|--------|-------|------------|
| RDS-side NQN ACLs | Deferred to v0.4 | Storage-level enforcement | Medium |
| Force detach timeout | Deferred to v0.4 | Handle stuck detach | Medium |
| Metrics dashboard | Optional | Operational visibility | Low |
| User documentation | Recommended | User guidance | Low |

## Build Order (For Documentation/Testing)

Since all components exist, the build order is for verification and documentation:

### Phase 1: Verify Existing Implementation
1. **Confirm VMIGrouper functionality**
   - Unit tests exist: `pkg/driver/vmi_grouper_test.go`
   - Verify PVC→VMI resolution works

2. **Confirm AttachmentManager grace period**
   - Unit tests exist: `pkg/attachment/manager_test.go`
   - Verify detach timestamp tracking

3. **Confirm AttachmentReconciler cleanup**
   - Unit tests exist: `pkg/attachment/reconciler_test.go`
   - Verify stale attachment detection

### Phase 2: Integration Testing
1. **VM restart handoff scenario**
   - Deploy VM with RDS volume
   - Simulate node failure
   - Verify VM restarts on new node within grace period

2. **Conflict detection scenario**
   - Attempt dual-attach outside grace period
   - Verify FAILED_PRECONDITION error

### Phase 3: Documentation
1. **User guide section: KubeVirt Integration**
   - Clearly state RWO-only limitation
   - Explain grace period enables fast restart
   - Provide configuration examples

2. **Troubleshooting guide**
   - "VM live migration fails" → Requires RWX storage
   - "Volume attachment conflict" → Check grace period setting

## Anti-Patterns to Avoid

### Anti-Pattern 1: Exposing RWX Capability

**Do NOT add `MULTI_NODE_MULTI_WRITER` capability.**

Why:
- NVMe/TCP doesn't support multi-initiator without clustering
- No SCSI reservations or coordination mechanism
- Would enable silent data corruption

Instead: Keep RWO only, document limitations.

### Anti-Pattern 2: Disabling Grace Period

**Do NOT set `AttachmentGracePeriod = 0`.**

Why:
- Node failures would block VM restart indefinitely
- Grace period doesn't weaken RWO (conflicts still blocked outside window)

Instead: Use default 30s or tune based on cluster characteristics.

### Anti-Pattern 3: Implementing Custom "Migration Protocol"

**Do NOT add driver-specific dual-attachment logic.**

Why:
- Violates CSI spec
- Extremely complex distributed coordination
- Data corruption risk

Instead: Guide users to RWX storage for live migration needs.

## Metrics for Observability

Existing metrics in `pkg/observability/prometheus.go`:

```
rds_csi_attachment_attach_total{status="success|failure"}
rds_csi_attachment_detach_total{status="success|failure"}
rds_csi_attachment_conflicts_total
rds_csi_attachment_grace_period_used_total
rds_csi_attachment_stale_cleared_total
rds_csi_attachment_operation_duration_seconds{operation="attach|detach|reconcile"}
rds_csi_attachment_reconcile_total{action="clear_stale|sync_pv"}
```

**Key metrics for KubeVirt monitoring:**
- `grace_period_used_total` - How often VM restarts use grace period
- `conflicts_total` - Blocked multi-attach attempts
- `stale_cleared_total` - Automatic cleanup from deleted nodes

## KubeVirt Compatibility Summary

| Feature | Supported | Implementation |
|---------|-----------|----------------|
| **Live Migration** | No | Requires RWX, not available |
| **VM Restart Handoff** | Yes | Grace period (30s) |
| **Fast Failover** | Yes | Grace period + reconciler |
| **Hot-plug Volumes** | Yes | Standard CSI flow |
| **ReadWriteOnce** | Yes | AttachmentManager enforcement |
| **ReadWriteMany** | No | NVMe/TCP limitation |
| **Block Mode** | Yes | CSI volumeMode=Block |
| **Filesystem Mode** | Yes | ext4/xfs in NodeStage |
| **Per-VMI Serialization** | Yes | VMIGrouper |
| **Stale Cleanup** | Yes | AttachmentReconciler |

## Sources

### Primary (HIGH Confidence)
- [KubeVirt Live Migration Documentation](https://kubevirt.io/user-guide/compute/live_migration/) - RWX requirement
- [KubeVirt Storage Volume Migration](https://kubevirt.io/user-guide/storage/volume_migration/) - Volume migration strategies
- [CSI Spec - ControllerPublishVolume](https://github.com/container-storage-interface/spec/blob/master/spec.md)
- Codebase: `pkg/driver/controller.go`, `pkg/attachment/manager.go`, `pkg/driver/vmi_grouper.go`

### Secondary (MEDIUM Confidence)
- [OpenEBS KubeVirt Live Migration](https://openebs.io/docs/Solutioning/read-write-many/kubevirt) - RWX patterns
- [Red Hat Storage Considerations](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization)

---

*Architecture research for: KubeVirt Live Migration Integration*
*Researched: 2026-01-30*
*Updated: 2026-02-03*
*Confidence: HIGH - Based on KubeVirt docs, CSI spec, and existing codebase implementation*
