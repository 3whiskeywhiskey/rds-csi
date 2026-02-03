# Domain Pitfalls: KubeVirt Live Migration with Block Storage CSI

**Domain:** KubeVirt VM live migration with ReadWriteOnce block storage
**Researched:** 2026-02-03
**Context:** RDS CSI Driver using NVMe/TCP, no storage-level locking/reservations
**Target:** v0.5.0 - KubeVirt Live Migration Support

## Executive Summary

Adding live migration support to a CSI driver that currently enforces strict RWO semantics is **fraught with data corruption risks**. The core challenge: allowing temporary dual-attachment during migration without creating filesystem corruption from simultaneous writes. NVMe/TCP provides no storage-level fencing, so all safety must come from careful orchestration and timing.

**Critical insight:** This is NOT just about relaxing attachment validation. Data corruption happens at the filesystem layer when two nodes mount the same block device simultaneously. The CSI driver must coordinate the handoff window precisely.

**Key requirement clarification:** KubeVirt requires **ReadWriteMany (RWX) access mode** for standard live migration. RWO block volumes cannot be live migrated without storage-level block migration (copying data from source to destination). The challenge is implementing RWX-like behavior on NVMe/TCP storage that natively supports multi-initiator access but lacks filesystem-level coordination.

## Critical Pitfalls

These mistakes cause data corruption, VM crashes, or require complete rewrites.

### Pitfall 1: Filesystem Mounted on Two Nodes Simultaneously

**What goes wrong:** The filesystem (ext4, xfs) inside the VM disk image gets corrupted when mounted read-write on two nodes at the same time. Both kernels cache metadata, make conflicting writes, and neither knows about the other's changes.

**Why it happens:** Developer assumes that because the migration is "temporary," brief dual-mounting is safe. It is not. Filesystems are explicitly designed for single-writer semantics.

**Consequences:**
- Filesystem corruption requiring fsck or complete data loss
- VM crashes with I/O errors during or after migration
- Silent corruption that appears later (worst case)

**Prevention:**
```
KubeVirt live migration with block volumes works DIFFERENTLY than filesystem mounts:

For KubeVirt VMs:
- QEMU accesses the block device directly (no filesystem mount by Linux)
- QEMU can coordinate I/O pause during migration handoff
- The VM's INTERNAL filesystem is protected by QEMU, not Linux

The CSI driver must:
1. Allow RWX block volume mode (volumeMode: Block)
2. NOT mount a filesystem on the block device
3. Trust KubeVirt/QEMU to coordinate I/O during migration

For filesystem volumes (non-KubeVirt):
- NEVER allow dual filesystem mounts
- Timeline enforcement still required if used outside KubeVirt
```

**Detection:**
- Monitor for mount count on same volume (should be 0 for block mode, 1 for filesystem)
- Check for filesystem errors in kernel logs on both nodes
- VM shows I/O errors or read-only filesystem remount

**Warning signs during implementation:**
- Code path allows NodePublishVolume with mount while RWX is enabled
- No distinction between block mode and filesystem mode volumes

**Phase to address:** Implementation - Volume capability validation

**Sources:**
- [Red Hat: Storage considerations for OpenShift Virtualization](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization)
- [Longhorn Issue #3597: Corruption using XFS after node restart](https://github.com/longhorn/longhorn/issues/3597)

---

### Pitfall 2: Advertising RWX Capability Without Proper Multi-Initiator Support

**What goes wrong:** CSI driver advertises `MULTI_NODE_MULTI_WRITER` capability to enable RWX volumes, but the underlying storage or driver doesn't actually handle concurrent access safely.

**Why it happens:** Developer focuses on the Kubernetes/CSI layer and forgets the storage layer. NVMe/TCP supports multi-initiator, but the driver's attachment tracking and namespace management may not.

**Consequences:**
- KubeVirt allows migration (sees RWX support)
- Both nodes connect to NVMe target simultaneously
- Without QEMU coordination, concurrent writes corrupt data
- With QEMU coordination, migration may still fail due to CSI-level rejections

**Prevention:**
```go
// Volume capability validation must differentiate:

func validateVolumeCapabilities(caps []*csi.VolumeCapability) error {
    for _, cap := range caps {
        accessMode := cap.GetAccessMode().GetMode()

        // RWX with Block = OK for KubeVirt live migration
        if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
            if cap.GetBlock() == nil {
                // RWX with Mount (filesystem) = NOT SAFE
                return fmt.Errorf("MULTI_NODE_MULTI_WRITER only supported for block volumes")
            }
            // Block mode is OK - QEMU coordinates access
        }
    }
    return nil
}

// Capability advertisement:
d.vcaps = []*csi.VolumeCapability_AccessMode{
    {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
    // Add RWX ONLY for block mode
    {Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
}
```

**Detection:**
- Test: Create RWX filesystem PVC, try to mount on two nodes - should fail
- Test: Create RWX block PVC for KubeVirt VM - should allow dual attachment

**Warning signs during implementation:**
- Adding MULTI_NODE_MULTI_WRITER to vcaps without access type validation
- No test cases for filesystem vs block mode with RWX

**Phase to address:** Research/Planning - Must decide capability scope before implementation

**Sources:**
- [Portworx: Raw Block for Live Migration](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration)
- [GitHub Issue: Azure Disk MULTI_NODE_MULTI_WRITER not supported](https://github.com/kubernetes-sigs/azuredisk-csi-driver/issues/827)

---

### Pitfall 3: Race Between Attachment Validation and Actual Device Access

**What goes wrong:** ControllerPublishVolume checks attachment state, returns OK for dual-attachment, but by the time NodeStageVolume runs, the device access conflicts with source node.

**Why it happens:** There's a time gap between CSI calls. The attachment manager tracks "intent," but NVMe connections are the "reality." Developer assumes attachment state equals connection state.

**Consequences:**
- Both nodes have active NVMe connections
- NVMe namespace may show different controller IDs
- Depending on RDS implementation, one connection may fail or data corruption

**Prevention:**
```
The attachment manager must track:
1. Attachment INTENT (what ControllerPublishVolume approved)
2. Connection STATE (what NodeStageVolume achieved)
3. Mount STATE (what NodePublishVolume completed)

For live migration:
- Source: ATTACHED, CONNECTED, MOUNTED (block device to QEMU)
- Destination: ATTACHED (approved), CONNECTING...
- Source: UNMOUNTING (QEMU pauses I/O)
- Source: DISCONNECTING
- Destination: CONNECTED, MOUNTING

The CSI driver cannot enforce this ordering - KubeVirt does.
But the driver CAN:
- Track all states for observability
- Refuse connection if attachment not approved
- Refuse disconnect if still marked as primary
```

**Detection:**
- Metrics show attachment count != connection count for same volume
- Node logs show "NVMe already connected" errors
- Multiple nvme subsystems connected to same NQN from different nodes

**Warning signs during implementation:**
- ControllerPublishVolume returns success without tracking
- No mechanism to verify source actually released before destination acquires

**Phase to address:** Implementation - Attachment manager enhancement

---

### Pitfall 4: NVMe Disconnect Timing - Volume Still In Use

**What goes wrong:** NodeUnstageVolume calls `nvme disconnect` while QEMU still has the block device open on the source node. Kernel panics, or I/O errors cause VM crash.

**Why it happens:** Developer assumes NodeUnpublishVolume always runs first and QEMU closes the device. In failure scenarios or timing issues, QEMU may still have file descriptor open.

**Consequences:**
- Node kernel panic or filesystem errors
- VM crash on source node
- Data loss from in-flight writes
- Stale NVMe subsystem state prevents reconnection

**Prevention:**
```bash
# In NodeUnstageVolume, ALWAYS verify device not in use:

# 1. Check if device has any open file descriptors
device_path=$(get_device_by_nqn "$nqn")
if lsof "$device_path" 2>/dev/null | grep -q .; then
    return FAILED_PRECONDITION "device still in use, cannot unstage"
fi

# 2. Check if device is mounted (paranoid check for block mode)
if findmnt --source "$device_path"; then
    return FAILED_PRECONDITION "device mounted, cannot disconnect"
fi

# 3. Flush any pending I/O
sync
echo 1 > /proc/sys/vm/drop_caches  # Optional: clear page cache

# 4. ONLY THEN disconnect
nvme disconnect -n "$nqn"
```

**Detection:**
- Kernel logs show "I/O error, dev nvmeXnY, sector ..." after disconnect
- QEMU process crashes with block device errors
- `dmesg` shows NVMe controller removal while in use

**Warning signs during implementation:**
- NodeUnstageVolume doesn't check for open file descriptors
- No synchronization between QEMU unmount and NVMe disconnect

**Phase to address:** Implementation - NodeUnstageVolume enhancement

**Sources:**
- [Dell CSI: When a Node Goes Down, Block Volumes Cannot be Attached to Another Node](https://www.dell.com/support/kbdoc/en-us/000200778/)

---

### Pitfall 5: Grace Period Configuration Misuse

**What goes wrong:** Migration takes longer than expected (network slow, large memory). Grace period expires, attachment reconciler forcibly detaches source, migration fails, VM crashes.

**Alternative problem:** Grace period is applied universally, allowing genuine conflicts to proceed for grace period duration.

**Why it happens:** Developer sets grace period based on "typical" migration time without accounting for variability. Or uses single grace period for both migration and conflict detection.

**Consequences:**
- Migration fails mid-stream
- VM experiences I/O errors and crashes
- OR: Legitimate conflicts allowed for grace period window

**Prevention:**
```
TWO SEPARATE CONCERNS:

1. Migration Handoff Window (should be long):
   - Allow dual-attachment for configured duration
   - Configurable per-StorageClass or globally
   - Default: 5 minutes (allows for slow migrations)
   - Must be renewable if migration still in progress

2. Conflict Detection (should be immediate):
   - Non-migration dual-attach attempts
   - Should fail IMMEDIATELY with FailedPrecondition
   - No grace period for conflicts

How to distinguish:
- Migration: ControllerPublishVolume from virt-launcher pod owner
- Conflict: Any other dual-attach attempt

Implementation:
func ControllerPublishVolume(req) {
    if existingAttachment.NodeID != req.NodeID {
        if isKubeVirtMigration(req) {
            // Apply migration grace period
            return allowMigrationHandoff(req)
        }
        // Not migration - reject immediately
        return FAILED_PRECONDITION
    }
}
```

**Detection:**
- Metrics show `attachment_conflicts_total` during known migrations
- Events on PVC: "Detached from node-A due to timeout" during active migration
- OR: Metrics show conflicts allowed that should have been rejected

**Warning signs during implementation:**
- Single `gracePeriod` config for all attachment decisions
- No way to identify migration requests vs conflict requests

**Phase to address:** Planning - Architecture of attachment manager

---

### Pitfall 6: Node Failure During Migration - Split-Brain Attachment

**What goes wrong:** Source node crashes mid-migration. Kubernetes thinks it's down, but it's actually network-partitioned and still has NVMe connection active. Destination node connects to same volume. Both write simultaneously.

**Why it happens:** Without storage-level fencing (NVMe/TCP has none via RDS), there's no way to guarantee source node released the volume.

**Consequences:**
- Silent data corruption (worst case scenario)
- Both nodes write to volume simultaneously
- No immediate error - corruption appears later
- Most dangerous when network partition resolves

**Prevention:**
```
This is the HARDEST problem for RWO block storage without fencing.

Options (in order of safety):

1. Require manual intervention:
   - If source node status = NotReady, refuse migration
   - Operator must verify node is truly down before force-detach
   - Document: "Network partitions can cause corruption"

2. Implement application-level fencing:
   - Source node must actively confirm unmount before destination mounts
   - Requires out-of-band communication channel (not CSI spec)
   - Could use RDS SSH connection to verify state

3. Use RDS-level protection (if available):
   - MikroTik RDS might support namespace reservations
   - Would need to research RouterOS capabilities
   - Most robust but requires storage cooperation

For RDS CSI (no fencing), recommend Option 1:
- AttachmentReconciler detects NotReady nodes
- Refuses migration if source node is NotReady
- Requires confirmed node deletion or manual override
- Logs aggressive warnings
```

**Detection:**
- Both nodes show NVMe connection to same NQN
- Attachment manager shows volume attached to multiple nodes
- Source node returns from partition with stale data

**Warning signs during implementation:**
- No check for source node status before allowing migration
- No documentation of split-brain risks

**Phase to address:** Research - Investigate RDS capabilities for fencing

**Sources:**
- [AIStore: Split-brain is Inevitable](https://aistore.nvidia.com/blog/2025/02/16/split-brain-blog)
- [Kubernetes on SAN Storage: Fencing for Persistent Storage](https://datamattsson.tumblr.com/post/182297931146/highly-available-stateful-workloads-on-kubernetes)

---

### Pitfall 7: Attachment State Persisted After Migration Failure

**What goes wrong:** Migration starts, attachment manager marks volume as "migrating to node-B". Migration fails. State persists. Future legitimate attachments rejected because volume still marked as migrating.

**Why it happens:** Developer adds migration state but forgets cleanup path for failure cases.

**Consequences:**
- Volume stuck in "migrating" state
- Future pod scheduling fails with "volume unavailable"
- Requires manual annotation cleanup or controller restart

**Prevention:**
```
Attachment state machine must handle ALL transitions:

States:
- AVAILABLE: No attachment
- ATTACHED(nodeA): Single attachment
- MIGRATING(nodeA -> nodeB): Dual attachment for migration
- DETACHING(nodeA): Cleanup in progress

Transitions:
1. AVAILABLE -> ATTACHED: Normal attach
2. ATTACHED -> MIGRATING: Migration start (ControllerPublish to new node)
3. MIGRATING -> ATTACHED: Migration complete (ControllerUnpublish from old node)
4. MIGRATING -> ATTACHED(original): Migration failed (rollback)
5. ATTACHED -> DETACHING: Normal detach start
6. DETACHING -> AVAILABLE: Detach complete

Failure recovery:
- Reconciler detects stale MIGRATING state (timestamp > max_migration_time)
- Checks actual NVMe connections on both nodes
- Resolves to correct single-attachment state
- Posts event explaining resolution
```

**Detection:**
- PV annotation shows migration in progress but no migration running
- Volume attach fails with "migration in progress" error
- Reconciler logs show repeated "stale migration state detected"

**Warning signs during implementation:**
- State machine only handles happy path
- No reconciler logic for stale migration cleanup

**Phase to address:** Implementation - Attachment state machine

---

## Moderate Pitfalls

These cause delays, debugging sessions, or technical debt.

### Pitfall 8: CSIDriver Object Configuration Mismatch

**What goes wrong:** CSIDriver object in Kubernetes has incorrect `attachRequired` setting. If false, Kubernetes skips ControllerPublishVolume entirely, breaking attachment tracking.

**Why it happens:** Developer changes driver behavior but forgets to update CSIDriver manifest.

**Consequences:**
- Attachment tracking bypassed
- Grace period logic never invoked
- Volumes appear to work but without proper fencing

**Prevention:**
```yaml
# CSIDriver must have attachRequired: true for attachment tracking
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: rds.csi.srvlab.io
spec:
  attachRequired: true  # CRITICAL: Must be true for ControllerPublish/Unpublish
  podInfoOnMount: true  # Recommended for VMI grouper
  volumeLifecycleModes:
    - Persistent
```

**Detection:**
- ControllerPublishVolume never called (no logs)
- Attachment metrics always zero
- Migration "works" but attachment state is empty

**Warning signs during implementation:**
- Changing deployment manifests without testing attachment flow
- Copy-pasting CSIDriver from driver that doesn't need attachment

**Phase to address:** Deployment - Manifest validation

---

### Pitfall 9: VolumeAttachment Object Cleanup Delay

**What goes wrong:** Kubernetes takes 6 minutes to clean up VolumeAttachment after node failure. During this time, volume cannot be attached to new node. Migration appears stuck.

**Why it happens:** This is Kubernetes default behavior (`maxWaitForUnmountDuration = 6m`). Not a driver bug, but impacts migration UX.

**Prevention:**
```
Cannot change Kubernetes behavior, but can:

1. Document expected delay in operator runbooks
2. Provide kubectl command for force cleanup:
   kubectl delete volumeattachment csi-<volume-id>
3. Add health check endpoint that detects this state
4. Post events explaining the wait period
5. Consider implementing force-detach with warnings
```

**Detection:**
- Migration stuck at "Waiting for volume detachment"
- VolumeAttachment object exists for deleted pod
- 6-minute timer in progress

**Warning signs during implementation:**
- No documentation of this Kubernetes limitation
- Tests don't account for cleanup delays

**Phase to address:** Documentation - Operator runbook

**Sources:**
- [Medium: Demystifying the Multi-Attach Error for Volume](https://medium.com/@golusstyle/demystifying-the-multi-attach-error-for-volume-causes-and-solutions-595a19316a0c)

---

### Pitfall 10: NVMe Multipath Interference

**What goes wrong:** If NVMe multipath is enabled on nodes, kernel may try to combine connections from different nodes into a multipath device. This breaks the single-initiator model.

**Why it happens:** Default kernel NVMe settings may enable multipath. Developer doesn't explicitly disable it.

**Consequences:**
- Unexpected device naming (/dev/nvmeXcYnZ vs /dev/nvmeXnY)
- Device path resolution fails
- Potential I/O routing issues

**Prevention:**
```
For RDS CSI driver (single controller, no multipath):

1. Deployment requirements:
   - Kernel parameter: nvme_core.multipath=N
   - OR modprobe.d: options nvme_core multipath=N

2. Driver validation at startup:
   - Check /sys/module/nvme_core/parameters/multipath
   - Log warning if multipath=Y
   - Consider failing startup with clear error

3. Documentation:
   - Add to deployment prerequisites
   - Explain why multipath should be disabled
```

**Detection:**
- Device appears as /dev/dm-* instead of /dev/nvmeXnY
- Multiple nvme controllers shown for same subsystem
- `/sys/class/nvme-subsystem/*/nvme*/` shows unexpected entries

**Warning signs during implementation:**
- No validation of multipath setting
- Device path code assumes simple naming

**Phase to address:** Deployment - Prerequisites documentation

---

### Pitfall 11: VMI Serialization Unaware of Migration

**What goes wrong:** Project has VMI serialization to prevent concurrent operations on same VMI. Migration adds second VMI (source + destination) operating on same volume. Serialization doesn't account for this.

**Why it happens:** VMI grouper keys by single VMI name. During migration, two VMIs (source launcher + destination launcher) both operate on volume, bypassing serialization.

**Prevention:**
```
Extend VMI grouper to handle migration:

Option A: Track by volume, not VMI
- Lock on volume ID for all operations
- Works regardless of how many VMIs access volume
- May be too coarse-grained for multi-volume VMIs

Option B: Detect migration and lock both VMIs
- Identify migration scenario from pod labels/ownership
- Acquire locks on both source and destination VMI
- More complex but more precise

Option C: Separate migration coordination
- Migration operations use different lock namespace
- AttachmentManager handles migration coordination
- VMI grouper continues to handle single-VMI case

Recommended: Option C (cleanest separation of concerns)
```

**Detection:**
- Metrics show concurrent ControllerPublishVolume for same volume during migration
- VMI serialization locks don't prevent dual-attachment
- Race conditions reappear despite serialization being enabled

**Warning signs during implementation:**
- No review of VMI grouper behavior during migration
- Tests only cover single-VMI scenarios

**Phase to address:** Implementation - Migration-aware locking

**Related:** Current implementation at driver.go:203-211 only handles single VMI case.

---

### Pitfall 12: CSI Idempotency Violations During Migration

**What goes wrong:** During migration, kubelet retries CSI calls. Driver assumes it's a duplicate call for same operation, returns success, but parameters have changed (different target node).

**Why it happens:** CSI spec requires idempotency, but "same operation" is ambiguous during migration.

**Prevention:**
```go
func ControllerPublishVolume(req) {
    // Check for existing attachment
    existing := getAttachment(req.VolumeId)

    if existing != nil {
        if existing.NodeID == req.NodeID {
            // TRUE idempotency: same volume, same node
            return existingResponse()
        }

        // Different node - NOT idempotent, this is new operation
        // Must validate migration is allowed
        if !isMigrationAllowed(existing, req) {
            return FAILED_PRECONDITION
        }
        // Process as migration
    }

    // No existing attachment, process normally
}
```

**Detection:**
- Same request retried returns different results
- Attachment state inconsistent after retries
- Kubelet logs show unexpected responses

**Warning signs during implementation:**
- Idempotency check only looks at volume ID, not node ID
- No test cases for retry during migration

**Phase to address:** Implementation - ControllerPublishVolume

---

## Minor Pitfalls

These cause annoyance or confusion but are easily fixable.

### Pitfall 13: Migration Events Not Posted

**What goes wrong:** Migration starts, runs, completes, but no events posted to PVC. Operators have no visibility into what happened.

**Why it happens:** Developer focuses on functionality, forgets observability.

**Prevention:**
```
Add migration-specific events:

1. MigrationStarted - ControllerPublishVolume for second node
2. SourceDetachInitiated - KubeVirt starts cleanup
3. SourceDetachCompleted - ControllerUnpublishVolume for source
4. MigrationCompleted - Successfully attached to destination only
5. MigrationFailed - Migration rolled back or timed out

Event posting (example):
poster.PostEvent(ctx, pvc, corev1.EventTypeNormal, "MigrationStarted",
    fmt.Sprintf("Volume %s migration started: %s -> %s", volumeID, sourceNode, destNode))
```

**Detection:**
- No events on PVC during migration
- Operators can't tell if migration is in progress

**Warning signs during implementation:**
- No event posting in migration code paths
- Events only posted for errors

**Phase to address:** Implementation - Observability

---

### Pitfall 14: Metrics Don't Distinguish Migration

**What goes wrong:** `attachment_conflicts_total` metric increases during migrations. Alerts fire. Operators think there's a problem when there isn't.

**Why it happens:** Same metric used for conflicts and migrations.

**Prevention:**
```
Separate metrics for migration:

# Conflicts (bad)
rds_csi_attachment_conflicts_total{reason="rwo_violation"}

# Migrations (expected)
rds_csi_migrations_total{status="started|completed|failed"}
rds_csi_migration_duration_seconds{phase="attach|detach|total"}
rds_csi_migration_grace_period_usage{used="true|false"}
```

**Detection:**
- Alerts during known maintenance windows
- Metrics dashboard shows "conflicts" during migrations

**Warning signs during implementation:**
- Reusing conflict metric for migration tracking
- No migration-specific metrics

**Phase to address:** Implementation - Metrics

---

### Pitfall 15: Testing Only Fast Migrations

**What goes wrong:** Tests pass with small VMs that migrate in seconds. Production has VMs with large memory that take minutes. Timing bugs only appear in production.

**Why it happens:** Test VMs are minimal to speed up CI.

**Prevention:**
```
Migration test scenarios:

1. Fast migration (baseline): Small VM, completes in <10s
2. Slow migration: Throttle network to 10MB/s, 2GB VM = 200s
3. Very slow migration: Throttle to 1MB/s, exceeds grace period
4. Failed migration: Inject error mid-migration
5. Node failure during migration: Kill source node process
6. Network partition during migration: iptables DROP

Test validation:
- Checksum VM disk before and after migration
- Verify no I/O errors in VM during migration
- Verify attachment state correct after all scenarios
```

**Detection:**
- Tests pass, production fails
- Timing-related bugs only in logs, not test failures

**Warning signs during implementation:**
- All test VMs are 256MB memory
- No network throttling in tests
- No fault injection testing

**Phase to address:** Testing - E2E test scenarios

---

## Phase-Specific Warnings

| Phase | Topic | Likely Pitfall | Mitigation |
|-------|-------|----------------|------------|
| Research | Live migration requirements | Assuming RWX filesystem is only option | Verify KubeVirt supports RWX block mode migration |
| Research | Storage capabilities | Not investigating RDS multi-initiator support | SSH to RDS, test concurrent nvme connect from 2 nodes |
| Planning | Capability advertisement | Adding RWX for both block and filesystem | Only advertise MULTI_NODE_MULTI_WRITER for block volumes |
| Planning | State machine | Missing failure transitions | Design ALL state transitions including failures |
| Implementation | Dual-attachment | Using single grace period for migration + conflict | Separate grace period logic from conflict detection |
| Implementation | VMI serialization | Not handling two-VMI migration scenario | Extend or bypass VMI grouper for migration |
| Implementation | NodeUnstageVolume | Not checking for open file descriptors | Verify device not in use before nvme disconnect |
| Testing | Migration timing | Testing only fast migrations | Test slow, failed, and fault-injected migrations |
| Testing | Corruption detection | Assuming corruption is obvious | Add checksum verification, fsck after migration |
| Deployment | CSIDriver manifest | Wrong attachRequired setting | Validate CSIDriver has attachRequired: true |
| Deployment | NVMe multipath | Not disabling multipath | Document and validate kernel parameter |
| Production | Monitoring | Only monitoring CSI metrics | Monitor actual NVMe connection state on nodes |

---

## Common Mistakes Summary

1. **Allowing dual filesystem mounts** - Always fatal for mounted filesystems; block mode is different
2. **Advertising RWX without proper validation** - Must restrict to block volumes only
3. **Trusting attachment state without connection verification** - Race conditions between CSI calls
4. **Disconnecting NVMe while in use** - Node crashes or I/O errors
5. **Single grace period for migration + conflict** - Conflicts should fail immediately
6. **No split-brain protection** - Network partitions cause silent corruption
7. **State machine missing failure paths** - Volume stuck in migration state
8. **Idempotency violations during migration** - Same volume, different node is NOT idempotent
9. **VMI serialization unaware of migration** - Concurrent operations bypass locks
10. **Poor observability** - Can't debug migration failures

---

## Success Criteria for Avoiding These Pitfalls

- [ ] **RWX only for block volumes** - Capability validation enforces this
- [ ] **Zero dual filesystem mounts** - Verified via monitoring on both nodes
- [ ] **Grace period separation** - Migration handoff (5m) separate from conflict detection (immediate)
- [ ] **Device-in-use check** - NodeUnstageVolume verifies no open FDs before disconnect
- [ ] **Split-brain protection** - Manual intervention required for NotReady node migrations
- [ ] **Complete state machine** - All transitions including failures documented and implemented
- [ ] **Idempotency tests** - Retry scenarios during migration produce correct results
- [ ] **Migration state cleanup** - Reconciler resolves stale migration markers
- [ ] **Multipath disabled** - Kernel parameter documented and validated
- [ ] **Migration-specific metrics** - Separate from conflict metrics
- [ ] **Slow migration tests** - E2E tests with network throttling
- [ ] **Checksum verification** - Test suite validates data integrity post-migration

---

## References

### Data Corruption & Filesystem Safety
- [Red Hat: Mounting Ext2/3/4 or XFS Filesystem from Multiple Systems Causes Corruption](https://access.redhat.com/solutions/353833)
- [Longhorn Issue #3597: XFS Corruption After Node Restart](https://github.com/longhorn/longhorn/issues/3597)

### KubeVirt Live Migration
- [KubeVirt User Guide: Live Migration](https://kubevirt.io/user-guide/compute/live_migration/)
- [KubeVirt Issue #10642: Allow block live migration of PVCs without RWX](https://github.com/kubevirt/kubevirt/issues/10642)
- [KubeVirt: Volume Migration](https://kubevirt.io/user-guide/storage/volume_migration/)
- [Red Hat: Storage considerations for OpenShift Virtualization](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization)

### CSI Driver Implementation
- [Kubernetes CSI: Developing a CSI Driver](https://kubernetes-csi.github.io/docs/developing.html)
- [Kubernetes CSI: CSIDriver Object](https://kubernetes-csi.github.io/docs/csi-driver-object.html)
- [CSI Spec: MULTI_NODE_MULTI_WRITER](https://github.com/container-storage-interface/spec/blob/master/spec.md)

### Multi-Attach Error and Recovery
- [Medium: Demystifying the Multi-Attach Error for Volume](https://medium.com/@golusstyle/demystifying-the-multi-attach-error-for-volume-causes-and-solutions-595a19316a0c)
- [Dell CSI: Block Volumes Cannot be Attached After Node Goes Down](https://www.dell.com/support/kbdoc/en-us/000200778/)
- [Portworx: Volume Already Exclusively Attached](https://portworx.com/knowledge-hub/volume-is-already-exclusively-attached-to-one-node-and-cant-be-attached-to-another/)

### Block Storage for KubeVirt
- [Portworx: Raw Block for Live Migration](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration)
- [HPE CSI: KubeVirt Integration](https://scod.hpedev.io/csi_driver/using.html)
- [OpenEBS: KubeVirt VM Live Migration](https://openebs.io/docs/Solutioning/read-write-many/kubevirt)

### Split-Brain and Fencing
- [AIStore: Split-brain is Inevitable](https://aistore.nvidia.com/blog/2025/02/16/split-brain-blog)
- [Kubernetes on SAN Storage: Highly-Available Stateful Workloads](https://datamattsson.tumblr.com/post/182297931146/highly-available-stateful-workloads-on-kubernetes)

### NVMe/TCP
- [SUSE: NVMe-oF Storage Administration](https://documentation.suse.com/sles/15-SP7/html/SLES-all/cha-nvmeof.html)
- [Linux Kernel: NVMe Multipath](https://docs.kernel.org/admin-guide/nvme-multipath.html)

---

**Confidence Level:** HIGH for critical pitfalls (verified via official documentation and real-world issues), MEDIUM for moderate pitfalls (implementation details based on CSI spec), LOW for minor pitfalls (based on general CSI development experience).

**Research gaps:**
- Exact RDS multi-initiator behavior needs testing (can two nodes connect to same NQN simultaneously?)
- KubeVirt virt-launcher pod lifecycle timing during migration (source code review needed)
- Whether RDS supports any form of namespace reservation for fencing
