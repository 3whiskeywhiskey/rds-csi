# Volume Fencing Pitfalls: ControllerPublishVolume/ControllerUnpublishVolume

**Domain:** CSI Controller Attachment Tracking for ReadWriteOnce Volumes
**Researched:** 2026-01-30
**Confidence:** HIGH
**Target:** v0.3.0 - Volume Fencing to fix volume ping-pong

This research focuses on pitfalls when implementing `ControllerPublishVolume` and `ControllerUnpublishVolume` for attachment tracking, specifically for KubeVirt workloads with ReadWriteOnce volumes using in-memory state + PV annotations.

---

## Critical Pitfalls

### Pitfall 1: In-Memory State Loss on Controller Restart

**What goes wrong:**
Controller maintains attachment tracking in memory (map of volumeID -> nodeID). When controller pod restarts, crashes, or is rescheduled:

1. In-memory state is completely lost
2. Controller returns to "clean slate" - doesn't know which volumes are attached where
3. Next `ControllerPublishVolume` call for already-attached volume succeeds (no fencing)
4. Volume now "published" to two nodes simultaneously
5. Data corruption if both nodes perform I/O

**Why it happens:**
Pure in-memory tracking without persistence. Common anti-pattern: relying on "controller will rebuild state from external-attacher calls" - but external-attacher doesn't re-call `ControllerPublishVolume` for already-attached volumes after controller restart.

**Consequences:**
- **Silent data corruption** - worst case, both nodes write to same volume
- **Multi-attach errors** - Kubernetes eventually detects and blocks new pods
- **Inconsistent state** - Driver thinks volume is available, but it's in use

**Warning signs:**
- After controller restart, `kubectl get volumeattachment` shows volumes attached, but controller has empty state
- Multi-attach errors appear shortly after controller pod restart
- Logs show "volume not found in publish context" when unpublishing

**Prevention strategy:**

```go
// Option A: Persist to PV annotations (recommended for your use case)
func (cs *ControllerServer) persistAttachment(volumeID, nodeID string) error {
    pv, err := cs.kubeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, metav1.GetOptions{})
    if err != nil {
        return err
    }

    if pv.Annotations == nil {
        pv.Annotations = make(map[string]string)
    }
    pv.Annotations["rds.csi.srvlab.io/attached-to"] = nodeID
    pv.Annotations["rds.csi.srvlab.io/attached-at"] = time.Now().Format(time.RFC3339)

    _, err = cs.kubeClient.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
    return err
}

// Rebuild state on startup
func (cs *ControllerServer) rebuildStateFromPVs() error {
    pvs, err := cs.kubeClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return err
    }

    for _, pv := range pvs.Items {
        if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != cs.driverName {
            continue
        }
        if nodeID, ok := pv.Annotations["rds.csi.srvlab.io/attached-to"]; ok {
            cs.attachments[pv.Name] = nodeID
            klog.Infof("Recovered attachment: %s -> %s", pv.Name, nodeID)
        }
    }
    return nil
}
```

**Phase to address:**
Phase 1: Core implementation. This is the FIRST thing to implement before any attachment logic.

**Sources:**
- [Azure Disk CSI: CRI Recovery Enhancement](https://github.com/kubernetes-sigs/azuredisk-csi-driver/issues/1648) - describes state recovery challenges
- [vSphere CSI: ControllerPublishVolume called twice](https://github.com/kubernetes-sigs/vsphere-csi-driver/issues/580) - shows duplicate attach after upgrade

---

### Pitfall 2: Race Condition Between Concurrent ControllerPublishVolume Calls

**What goes wrong:**
Two pods scheduled simultaneously on different nodes for the same RWO volume:

```
Timeline:
T0: Pod A scheduled on Node1, triggers ControllerPublishVolume(vol, node1)
T1: Pod B scheduled on Node2, triggers ControllerPublishVolume(vol, node2)
T2: Controller checks attachments[vol] -> empty (neither completed yet)
T3: Controller allows BOTH publications
T4: Both nodes connect to same NVMe target
T5: Data corruption
```

**Why it happens:**
No locking around attachment check-and-set. The check ("is volume attached?") and set ("mark as attached") are not atomic. With concurrent requests, both pass the check before either completes the set.

**Consequences:**
- RWO volume attached to multiple nodes simultaneously
- NVMe/TCP allows multiple initiators (no built-in SCSI-like reservation)
- Filesystem corruption guaranteed

**Warning signs:**
- Multi-attach detected by kubelet but after initial I/O already occurred
- Rapid pod scheduling (e.g., deployment scale-up) causes sporadic failures
- Race window is small (milliseconds) so hard to reproduce reliably

**Prevention strategy:**

```go
type ControllerServer struct {
    // Per-volume lock to serialize ControllerPublishVolume calls
    volumeLocks sync.Map // map[string]*sync.Mutex

    // Attachment state
    attachments     map[string]string
    attachmentsLock sync.RWMutex
}

func (cs *ControllerServer) getVolumeLock(volumeID string) *sync.Mutex {
    lock, _ := cs.volumeLocks.LoadOrStore(volumeID, &sync.Mutex{})
    return lock.(*sync.Mutex)
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    // Serialize all publish operations for this volume
    lock := cs.getVolumeLock(volumeID)
    lock.Lock()
    defer lock.Unlock()

    // Now safe to check and set
    cs.attachmentsLock.RLock()
    existingNode, exists := cs.attachments[volumeID]
    cs.attachmentsLock.RUnlock()

    if exists {
        if existingNode == nodeID {
            // Idempotent: already attached to same node
            return &csi.ControllerPublishVolumeResponse{}, nil
        }
        // Attached to different node - REJECT
        return nil, status.Errorf(codes.FailedPrecondition,
            "volume %s already attached to node %s, cannot attach to %s",
            volumeID, existingNode, nodeID)
    }

    // Perform actual attachment tracking
    cs.attachmentsLock.Lock()
    cs.attachments[volumeID] = nodeID
    cs.attachmentsLock.Unlock()

    // Persist to PV annotation
    if err := cs.persistAttachment(volumeID, nodeID); err != nil {
        // Rollback in-memory state
        cs.attachmentsLock.Lock()
        delete(cs.attachments, volumeID)
        cs.attachmentsLock.Unlock()
        return nil, status.Errorf(codes.Internal, "failed to persist attachment: %v", err)
    }

    return &csi.ControllerPublishVolumeResponse{}, nil
}
```

**Phase to address:**
Phase 1: Core implementation. Must be present from day one.

**Sources:**
- [CSI Spec: ControllerPublishVolume](https://github.com/container-storage-interface/spec/blob/master/spec.md) - requires FAILED_PRECONDITION for already-published volumes
- [Kubernetes optimistic concurrency](https://kyungho.me/en/posts/kubernetes-concurrency-control) - explains why Kubernetes doesn't prevent this at scheduler level

---

### Pitfall 3: Dangling VolumeAttachments After Node Deletion

**What goes wrong:**
Node is forcefully removed from cluster (hardware failure, `kubectl delete node --force`):

1. VolumeAttachment objects remain in Terminating state
2. external-attacher cannot call ControllerUnpublishVolume (node is gone)
3. Controller's attachment state still shows volume -> deleted_node
4. New pod scheduled on healthy node
5. ControllerPublishVolume rejects with "already attached to [deleted_node]"
6. Pod stuck in ContainerCreating forever

**Why it happens:**
Controller trusts its attachment state without validating node existence. When node is deleted, the attachment state becomes stale but is never cleaned up.

**Consequences:**
- Volumes become permanently unavailable until manual intervention
- Operators must manually clear annotations or restart controller
- Workloads cannot failover after node failures

**Warning signs:**
- `kubectl get volumeattachment` shows attachments to non-existent nodes
- Pods stuck in ContainerCreating with "already attached" errors
- Errors reference nodes that don't appear in `kubectl get nodes`

**Prevention strategy:**

```go
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    lock := cs.getVolumeLock(volumeID)
    lock.Lock()
    defer lock.Unlock()

    cs.attachmentsLock.RLock()
    existingNode, exists := cs.attachments[volumeID]
    cs.attachmentsLock.RUnlock()

    if exists && existingNode != nodeID {
        // Before rejecting, verify the existing node still exists
        _, err := cs.kubeClient.CoreV1().Nodes().Get(ctx, existingNode, metav1.GetOptions{})
        if err != nil {
            if errors.IsNotFound(err) {
                klog.Warningf("Volume %s was attached to deleted node %s, allowing reattachment to %s",
                    volumeID, existingNode, nodeID)
                // Clear stale attachment
                cs.attachmentsLock.Lock()
                delete(cs.attachments, volumeID)
                cs.attachmentsLock.Unlock()
                // Fall through to allow new attachment
            } else {
                return nil, status.Errorf(codes.Internal, "failed to verify node existence: %v", err)
            }
        } else {
            // Node exists - genuine conflict
            return nil, status.Errorf(codes.FailedPrecondition,
                "volume %s already attached to node %s", volumeID, existingNode)
        }
    }

    // Continue with attachment...
}

// Also: background reconciliation loop
func (cs *ControllerServer) reconcileStaleAttachments() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        cs.attachmentsLock.RLock()
        attachmentsCopy := make(map[string]string)
        for k, v := range cs.attachments {
            attachmentsCopy[k] = v
        }
        cs.attachmentsLock.RUnlock()

        for volumeID, nodeID := range attachmentsCopy {
            _, err := cs.kubeClient.CoreV1().Nodes().Get(context.Background(), nodeID, metav1.GetOptions{})
            if errors.IsNotFound(err) {
                klog.Warningf("Cleaning up stale attachment: %s -> %s (node deleted)", volumeID, nodeID)
                cs.attachmentsLock.Lock()
                delete(cs.attachments, volumeID)
                cs.attachmentsLock.Unlock()
                cs.clearPVAnnotation(volumeID)
            }
        }
    }
}
```

**Phase to address:**
Phase 2: Robustness. Can launch without this but must add before production use with node failures.

**Sources:**
- [Kubernetes Issue #67853](https://github.com/kubernetes/kubernetes/issues/67853) - VolumeAttachment not recreated after node deletion
- [VMware CSI Issue #245](https://github.com/vmware-archive/cloud-director-named-disk-csi-driver/issues/245) - Multi-attach due to dangling attachments
- [Dell CSI KB](https://www.dell.com/support/kbdoc/en-us/000200778/container-storage-interface-csi-drivers-family-when-a-node-goes-down-due-to-node-crash-node-down-power-off-scenario-pods-cannot-come-up-on-a-new-node-because-storage-volumes-cannot-be-attached) - Node down volume attachment issues

---

### Pitfall 4: ControllerUnpublishVolume Called Before NodeUnstageVolume Completes

**What goes wrong:**
During pod deletion or node drain:

1. kubelet calls `NodeUnpublishVolume` (unmount from pod path)
2. kubelet calls `NodeUnstageVolume` (unmount staging, NVMe disconnect)
3. **Before step 2 completes**, external-attacher calls `ControllerUnpublishVolume`
4. Controller clears attachment state
5. New pod scheduled immediately on different node
6. Controller allows new attachment (old attachment cleared)
7. Two nodes now accessing same volume - one disconnecting, one connecting

**Why it happens:**
The external-attacher doesn't wait for `NodeUnstageVolume` to complete. It watches VolumeAttachment deletion and immediately calls `ControllerUnpublishVolume`. The CSI spec says drivers MUST handle this, but many don't.

**Consequences:**
- Brief window where two nodes access volume
- Less severe than Pitfall 2 (one node is disconnecting), but still dangerous for certain workloads
- KubeVirt live migration particularly vulnerable

**Warning signs:**
- Logs show ControllerUnpublishVolume before NodeUnstageVolume
- Occasional filesystem corruption during rapid pod rescheduling
- Works in testing (slow operations) but fails in production (fast operations)

**Prevention strategy:**

```go
// Option A: Grace period before allowing re-attachment
const reattachmentGracePeriod = 30 * time.Second

type attachmentInfo struct {
    NodeID       string
    UnpublishAt  time.Time // When ControllerUnpublishVolume was called
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()

    lock := cs.getVolumeLock(volumeID)
    lock.Lock()
    defer lock.Unlock()

    // Don't immediately clear - mark as "unpublishing" with timestamp
    cs.attachmentsLock.Lock()
    cs.attachments[volumeID] = attachmentInfo{
        NodeID:      req.GetNodeId(),
        UnpublishAt: time.Now(),
    }
    cs.attachmentsLock.Unlock()

    // Background cleanup after grace period
    go func() {
        time.Sleep(reattachmentGracePeriod)
        lock := cs.getVolumeLock(volumeID)
        lock.Lock()
        defer lock.Unlock()

        cs.attachmentsLock.Lock()
        if info, ok := cs.attachments[volumeID].(attachmentInfo); ok {
            if time.Since(info.UnpublishAt) >= reattachmentGracePeriod {
                delete(cs.attachments, volumeID)
            }
        }
        cs.attachmentsLock.Unlock()
    }()

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    lock := cs.getVolumeLock(volumeID)
    lock.Lock()
    defer lock.Unlock()

    cs.attachmentsLock.RLock()
    existing := cs.attachments[volumeID]
    cs.attachmentsLock.RUnlock()

    switch v := existing.(type) {
    case string:
        // Attached to a node
        if v == nodeID {
            return &csi.ControllerPublishVolumeResponse{}, nil // Idempotent
        }
        return nil, status.Errorf(codes.FailedPrecondition, "volume attached to %s", v)

    case attachmentInfo:
        // In grace period
        if v.NodeID == nodeID {
            // Re-attaching to same node - likely kubelet retry, allow it
            return &csi.ControllerPublishVolumeResponse{}, nil
        }
        // Different node - enforce grace period
        remaining := reattachmentGracePeriod - time.Since(v.UnpublishAt)
        if remaining > 0 {
            return nil, status.Errorf(codes.Unavailable,
                "volume %s in detachment grace period, retry in %v", volumeID, remaining)
        }
        // Grace period expired, allow new attachment
    }

    // Continue with attachment...
}
```

**Phase to address:**
Phase 2: KubeVirt support. Critical for live migration scenarios.

**Sources:**
- [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) - "This RPC MUST be called after all NodeUnstageVolume and NodeUnpublishVolume"
- [Longhorn Volume Live Migration](https://github.com/longhorn/longhorn/blob/master/enhancements/20210216-volume-live-migration.md) - Detailed attach/detach flow for migration

---

### Pitfall 5: Incorrect FAILED_PRECONDITION vs ABORTED Error Codes

**What goes wrong:**
CSI spec requires specific error codes for specific conditions:

- `FAILED_PRECONDITION` (9): Volume attached to different node with incompatible access mode
- `ABORTED` (10): Operation already in progress for this volume
- `ALREADY_EXISTS` (6): Volume already attached to same node with same capabilities (should return success, not error)

Wrong error codes cause wrong CO behavior:
- Returning `ABORTED` when it should be `FAILED_PRECONDITION` causes infinite retries
- Returning error when it should be success causes stuck pods

**Why it happens:**
Developers don't read CSI spec carefully. "Already attached" seems like an error, so they return an error.

**Consequences:**
- Pods stuck in retry loops
- Unnecessary load on controller
- Confusing error messages for operators

**Warning signs:**
- Logs show constant retries for the same volume
- `ControllerPublishVolume` errors for idempotent calls
- external-attacher logs show unexpected error handling

**Prevention strategy:**

```go
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    cs.attachmentsLock.RLock()
    existingNode, exists := cs.attachments[volumeID]
    cs.attachmentsLock.RUnlock()

    if exists {
        if existingNode == nodeID {
            // CSI Spec: "If this RPC failed, or the CO does not know if it failed,
            // it MAY call this RPC again. The Plugin SHOULD ensure that when this
            // call is repeated, the result remains the same."
            // This is IDEMPOTENT - return SUCCESS, not error
            klog.V(4).Infof("Volume %s already published to node %s (idempotent)", volumeID, nodeID)
            return &csi.ControllerPublishVolumeResponse{
                PublishContext: map[string]string{
                    "attachedNode": nodeID,
                },
            }, nil
        }

        // CSI Spec: "Indicates that a volume corresponding to the specified
        // volume_id has already been published at the node corresponding to
        // the specified node_id but is incompatible with the specified
        // volume_capability or readonly flag."
        // Use FAILED_PRECONDITION for RWO volume on different node
        return nil, status.Errorf(codes.FailedPrecondition,
            "volume %s is already exclusively attached to node %s, cannot attach to %s (ReadWriteOnce access mode)",
            volumeID, existingNode, nodeID)
    }

    // Check if operation already in progress
    if cs.isOperationInProgress(volumeID) {
        // Use ABORTED for concurrent operation
        return nil, status.Errorf(codes.Aborted,
            "operation already in progress for volume %s", volumeID)
    }

    // Continue with attachment...
}
```

**Phase to address:**
Phase 1: Core implementation. Get it right from the start.

**Sources:**
- [CSI Spec v1.7.0](https://github.com/container-storage-interface/spec/blob/v1.7.0/spec.md) - Complete error code requirements
- [Kadalu CSI Sanity Failures](https://github.com/kadalu/kadalu/issues/494) - Spec conformance issues

---

## Moderate Pitfalls

### Pitfall 6: PV Annotation Update Conflicts

**What goes wrong:**
When using PV annotations for persistence, concurrent updates can conflict:

1. Controller A reads PV (resourceVersion: 100)
2. Controller B reads PV (resourceVersion: 100)
3. Controller A updates annotation, write succeeds (resourceVersion: 101)
4. Controller B updates annotation, write fails with "conflict"
5. Controller B doesn't retry, attachment state inconsistent

**Why it happens:**
Kubernetes uses optimistic concurrency. If you read a resource, modify it, and write it back, another writer may have updated it in between. Your write fails with StatusConflict (409).

**Prevention strategy:**

```go
func (cs *ControllerServer) persistAttachment(volumeID, nodeID string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        pv, err := cs.kubeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, metav1.GetOptions{})
        if err != nil {
            return err
        }

        if pv.Annotations == nil {
            pv.Annotations = make(map[string]string)
        }
        pv.Annotations["rds.csi.srvlab.io/attached-to"] = nodeID

        _, err = cs.kubeClient.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
        return err // If conflict, retry.RetryOnConflict will retry
    })
}
```

**Phase to address:**
Phase 1: Core implementation.

**Sources:**
- [Kubernetes Operators Best Practices](https://alenkacz.medium.com/kubernetes-operators-best-practices-understanding-conflict-errors-d05353dff421) - Conflict handling

---

### Pitfall 7: Missing Publish Context in Response

**What goes wrong:**
`ControllerPublishVolume` should return `PublishContext` map that is passed to `NodeStageVolume`. If empty or missing:

1. NodeStageVolume doesn't receive controller-provided metadata
2. Node service must duplicate discovery logic
3. State desync between controller and node

**Prevention strategy:**

```go
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    // ... attachment logic ...

    return &csi.ControllerPublishVolumeResponse{
        PublishContext: map[string]string{
            // Pass through to NodeStageVolume
            "nvmeAddress":  cs.config.NVMeAddress,
            "nvmePort":     cs.config.NVMePort,
            "attachedNode": nodeID,
            "attachedAt":   time.Now().Format(time.RFC3339),
        },
    }, nil
}
```

**Phase to address:**
Phase 1: Core implementation.

---

### Pitfall 8: Kubelet 6-Minute Force Detach Timeout

**What goes wrong:**
Kubernetes has hardcoded 6-minute `maxWaitForUnmountDuration`. After this:

1. AttachDetach controller force-detaches volume even if still mounted
2. VolumeAttachment marked as detached
3. New pod can attach to volume
4. Old node still has I/O in flight

**Why it happens:**
This is Kubernetes behavior, not CSI driver behavior. The driver must handle it gracefully.

**Prevention strategy:**

```go
// In node service - ensure unmount completes quickly
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
    // Use shorter timeouts to avoid hitting 6-minute limit
    unmountCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    defer cancel()

    if err := ns.mounter.UnmountWithContext(unmountCtx, stagingPath); err != nil {
        // Try lazy unmount as fallback
        klog.Warningf("Normal unmount failed, trying lazy unmount: %v", err)
        if err := ns.mounter.LazyUnmount(stagingPath); err != nil {
            return nil, status.Errorf(codes.Internal, "unmount failed: %v", err)
        }
    }

    // Ensure NVMe disconnect completes before returning
    if err := ns.nvmeConn.DisconnectWithContext(unmountCtx, nqn); err != nil {
        klog.Warningf("NVMe disconnect failed (may be force-detached): %v", err)
        // Don't fail - volume may already be detached by force
    }

    return &csi.NodeUnstageVolumeResponse{}, nil
}
```

**Phase to address:**
Phase 2: Robustness. Important for production reliability.

**Sources:**
- [Longhorn Issue #3584](https://github.com/longhorn/longhorn/issues/3584) - Configurable detach timeout
- [Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/5562994/failedattachvolume-kubernetes-pods-not-reattaching) - Force detach behavior

---

### Pitfall 9: VolumeAttachment Finalizer Stuck

**What goes wrong:**
VolumeAttachment has finalizer `external-attacher/rds.csi.srvlab.io`. If ControllerUnpublishVolume fails repeatedly:

1. VolumeAttachment stuck in Terminating
2. PV stuck in Terminating (has finalizer referencing VolumeAttachment)
3. PVC stuck in Terminating
4. Namespace stuck in Terminating (if PVC has finalizer)
5. Manual intervention required

**Prevention strategy:**

```go
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    // ALWAYS return success if volume not attached
    // This allows finalizer removal even if state is inconsistent
    cs.attachmentsLock.RLock()
    existingNode, exists := cs.attachments[volumeID]
    cs.attachmentsLock.RUnlock()

    if !exists {
        klog.V(4).Infof("Volume %s not in attachment state, returning success (idempotent)", volumeID)
        return &csi.ControllerUnpublishVolumeResponse{}, nil
    }

    if existingNode != nodeID && nodeID != "" {
        // Volume attached to different node - still return success
        // The nodeID might be stale, and we need to allow cleanup
        klog.Warningf("Unpublish request for volume %s from node %s, but attached to %s. Allowing unpublish.",
            volumeID, nodeID, existingNode)
    }

    // Clear state
    cs.attachmentsLock.Lock()
    delete(cs.attachments, volumeID)
    cs.attachmentsLock.Unlock()

    // Clear PV annotation (best effort - don't fail on this)
    if err := cs.clearPVAnnotation(volumeID); err != nil {
        klog.Warningf("Failed to clear PV annotation for %s: %v (continuing)", volumeID, err)
    }

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}
```

**Phase to address:**
Phase 1: Core implementation. Must be idempotent from the start.

**Sources:**
- [External-Attacher Issue #66](https://github.com/kubernetes-csi/external-attacher/issues/66) - Failing to delete PV after attach failure
- [External-Attacher csi_handler.go](https://github.com/kubernetes-csi/external-attacher/blob/master/pkg/controller/csi_handler.go) - Finalizer handling

---

## Minor Pitfalls

### Pitfall 10: No NVMe-Level Fencing (Storage-Level SCSI-3 PR Equivalent)

**What goes wrong:**
Controller-level fencing (tracking which node has volume) is "soft fencing". It relies on the CSI driver being the only path to the storage. If:

1. Direct NVMe connection made outside CSI (debugging, manual recovery)
2. Controller state corrupted
3. Split-brain scenario

Then storage-level protection is bypassed.

**Current state:**
MikroTik RDS NVMe/TCP does not support NVMe Reservations (equivalent to SCSI-3 Persistent Reservations). AWS EBS added NVMe Reservations support in 2023, but this is not available on RDS.

**Mitigation:**
Document this limitation. For mission-critical workloads, consider using SCSI-based storage with fence_scsi support.

**Phase to address:**
Document in Phase 1. Hardware limitation, not software.

**Sources:**
- [AWS EBS NVMe Reservations](https://www.infoq.com/news/2023/10/aws-ebs-fencing-nvme/) - Storage-level fencing with NVMe
- [Red Hat fence_scsi](https://access.redhat.com/articles/530533) - SCSI PR fencing

---

### Pitfall 11: Not Handling Volume Not Found in Unpublish

**What goes wrong:**
`ControllerUnpublishVolume` called for volume that was never created or already deleted:

```go
// Bad
func (cs *ControllerServer) ControllerUnpublishVolume(...) {
    pv, err := cs.kubeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, ...)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to get PV: %v", err)
    }
}
```

This causes VolumeAttachment stuck in Terminating.

**Prevention strategy:**

```go
// Good - return success if PV doesn't exist
func (cs *ControllerServer) ControllerUnpublishVolume(...) {
    pv, err := cs.kubeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, ...)
    if err != nil {
        if errors.IsNotFound(err) {
            // Volume doesn't exist, nothing to unpublish
            return &csi.ControllerUnpublishVolumeResponse{}, nil
        }
        return nil, status.Errorf(codes.Internal, "failed to get PV: %v", err)
    }
}
```

**Phase to address:**
Phase 1: Core implementation.

---

## KubeVirt-Specific Pitfalls

### Pitfall 12: Live Migration Volume Handoff

**What goes wrong:**
KubeVirt live migration requires both source and target nodes to access volume simultaneously during memory copy phase. RWO access mode blocks this.

**Why it happens:**
Live migration is a multi-step process:
1. Target VM starts on new node
2. Memory copied from source to target (both VMs running)
3. Final cutover (source stops, target becomes primary)

During step 2, both nodes need volume access.

**Current limitation:**
RDS-CSI with RWO volumes does not support live migration. Use RWX volumes or storage migration instead.

**Mitigation:**
- Document limitation clearly
- Consider implementing RWX support for live migration use cases
- Use [volume migration](https://kubevirt.io/user-guide/storage/volume_migration/) instead of live migration

**Phase to address:**
Future enhancement (post-v0.3.0). Document limitation in v0.3.0.

**Sources:**
- [KubeVirt Live Migration](https://kubevirt.io/user-guide/compute/live_migration/) - Requires RWX
- [Longhorn Volume Live Migration](https://github.com/longhorn/longhorn/blob/master/enhancements/20210216-volume-live-migration.md) - Implementation approach

---

## Phase-to-Pitfall Mapping

| Pitfall | Severity | Phase | Verification |
|---------|----------|-------|--------------|
| 1. In-memory state loss | CRITICAL | Phase 1 | Controller restart test |
| 2. Concurrent publish race | CRITICAL | Phase 1 | Parallel pod scheduling test |
| 3. Dangling attachments | CRITICAL | Phase 2 | Node deletion test |
| 4. Unpublish before unstage | HIGH | Phase 2 | Rapid reschedule test |
| 5. Wrong error codes | HIGH | Phase 1 | CSI sanity tests |
| 6. PV annotation conflicts | MEDIUM | Phase 1 | Concurrent update test |
| 7. Missing publish context | MEDIUM | Phase 1 | Integration test |
| 8. 6-minute force detach | MEDIUM | Phase 2 | Slow unmount test |
| 9. Stuck finalizer | HIGH | Phase 1 | Unpublish failure test |
| 10. No storage-level fencing | LOW | Document | N/A (hardware limitation) |
| 11. Volume not found | MEDIUM | Phase 1 | Deleted volume unpublish test |
| 12. Live migration | LOW | Document | N/A (RWX required) |

---

## Testing Checklist

### Unit Tests

- [ ] `ControllerPublishVolume` with new volume returns success
- [ ] `ControllerPublishVolume` to same node is idempotent (returns success, not error)
- [ ] `ControllerPublishVolume` to different node returns `FAILED_PRECONDITION`
- [ ] `ControllerUnpublishVolume` clears attachment state
- [ ] `ControllerUnpublishVolume` for non-existent volume returns success
- [ ] PV annotation persisted on publish
- [ ] PV annotation cleared on unpublish
- [ ] State rebuilt from PV annotations on controller init

### Integration Tests

- [ ] Controller restart doesn't lose attachment state
- [ ] Concurrent `ControllerPublishVolume` calls are serialized
- [ ] Node deletion allows volume reattachment
- [ ] Full volume lifecycle with fencing enabled

### E2E Tests

- [ ] Pod rescheduling after node drain works
- [ ] Pod fails to schedule if volume attached elsewhere
- [ ] Volume cleanup after PVC deletion

---

## Implementation Order

Based on pitfall severity and dependencies:

1. **Phase 1: Foundation** (must-have for any release)
   - Per-volume locking (Pitfall 2)
   - PV annotation persistence (Pitfall 1)
   - State rebuild on startup (Pitfall 1)
   - Correct error codes (Pitfall 5)
   - Idempotent unpublish (Pitfall 9, 11)
   - Conflict retry for annotations (Pitfall 6)
   - Publish context (Pitfall 7)

2. **Phase 2: Robustness** (before production use)
   - Node existence validation (Pitfall 3)
   - Background stale attachment cleanup (Pitfall 3)
   - Reattachment grace period (Pitfall 4)
   - Unmount timeout handling (Pitfall 8)

3. **Documentation** (at release)
   - No storage-level fencing (Pitfall 10)
   - Live migration limitation (Pitfall 12)

---

## Sources

**CSI Specification:**
- [CSI Spec v1.7.0](https://github.com/container-storage-interface/spec/blob/v1.7.0/spec.md) - ControllerPublishVolume/ControllerUnpublishVolume requirements
- [CSI Spec v1.3.0](https://github.com/container-storage-interface/spec/blob/v1.3.0/spec.md) - Error code definitions

**Kubernetes Issues:**
- [Issue #67853](https://github.com/kubernetes/kubernetes/issues/67853) - VolumeAttachment not recreated after node deletion
- [Issue #77324](https://github.com/kubernetes/kubernetes/issues/77324) - Garbage collect VolumeAttachment objects
- [Issue #106710](https://github.com/kubernetes/kubernetes/issues/106710) - Volumes detached while still mounted
- [Issue #65392](https://github.com/kubernetes/kubernetes/issues/65392) - Force detach on pod deletion

**CSI Driver Issues:**
- [Azure Disk CSI #1648](https://github.com/kubernetes-sigs/azuredisk-csi-driver/issues/1648) - CRI Recovery Enhancement
- [vSphere CSI #580](https://github.com/kubernetes-sigs/vsphere-csi-driver/issues/580) - ControllerPublishVolume called twice
- [vSphere CSI #221](https://github.com/kubernetes-sigs/vsphere-csi-driver/issues/221) - Volume can't be detached from deleted node
- [AWS EBS CSI #833](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/833) - VolumeAttachment not deleted for 20+ minutes
- [Longhorn #3584](https://github.com/longhorn/longhorn/issues/3584) - Configurable detach timeout
- [Longhorn #7258](https://github.com/longhorn/longhorn/issues/7258) - Volumes stuck on attachment deletion

**External-Attacher:**
- [Issue #66](https://github.com/kubernetes-csi/external-attacher/issues/66) - Failing to delete PV after attach failure
- [Issue #416](https://github.com/kubernetes-csi/external-attacher/issues/416) - VolumeAttachment status mismatch
- [PR #184](https://github.com/kubernetes-csi/external-attacher/pull/184) - Reconcile with ListVolumes

**Multi-Attach Analysis:**
- [Medium: Multi-Attach Error Explained](https://medium.com/@golusstyle/demystifying-the-multi-attach-error-for-volume-causes-and-solutions-595a19316a0c)
- [KodeKloud: Multi-Attach Volume Errors](https://notes.kodekloud.com/docs/Kubernetes-Troubleshooting-for-Application-Developers/Troubleshooting-Scenarios/Multi-Attach-Volume-Errors)
- [Portworx: Volume Exclusively Attached](https://portworx.com/knowledge-hub/volume-is-already-exclusively-attached-to-one-node-and-cant-be-attached-to-another/)

**KubeVirt:**
- [KubeVirt Live Migration](https://kubevirt.io/user-guide/compute/live_migration/)
- [KubeVirt Volume Migration](https://kubevirt.io/user-guide/storage/volume_migration/)
- [Longhorn Volume Live Migration Enhancement](https://github.com/longhorn/longhorn/blob/master/enhancements/20210216-volume-live-migration.md)

**NVMe Reservations/Fencing:**
- [AWS EBS NVMe Reservations](https://www.infoq.com/news/2023/10/aws-ebs-fencing-nvme/)
- [Red Hat fence_scsi](https://access.redhat.com/articles/530533)
- [SCSI SPC-3 Persistent Reservations](https://grimoire.carcano.ch/blog/spc-3-persistent-reservations-and-fencing/)

---

*Confidence Level: HIGH*
- Critical pitfalls derived from CSI specification requirements
- Race conditions verified against Kubernetes controller behavior
- Production issues cross-referenced from 5+ CSI driver implementations
- KubeVirt limitations verified against official documentation
