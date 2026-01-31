# Feature Landscape: Volume Fencing (v0.3.0)

**Domain:** CSI ControllerPublish/Unpublish for RWO Volume Fencing
**Researched:** 2026-01-30
**Context:** RDS CSI Driver experiencing volume ping-pong (~7 min migration cycles) without attachment tracking

## Problem Statement

Without `ControllerPublishVolume`/`ControllerUnpublishVolume`:
- Kubernetes cannot enforce single-node attachment for RWO volumes
- Volumes migrate between nodes every ~7 minutes (ping-pong behavior)
- KubeVirt VMs experience I/O errors and pause from concurrent access
- No fencing guarantees exist to prevent data corruption

---

## Table Stakes

Features users expect. Missing = fencing does not work.

| Feature | Why Expected | Complexity | Dependencies |
|---------|--------------|------------|--------------|
| **PUBLISH_UNPUBLISH_VOLUME capability** | External-attacher requires this to call Publish/Unpublish RPCs | Low | Capability declaration in ControllerGetCapabilities |
| **ControllerPublishVolume implementation** | Makes volume available on target node before NodeStage | Medium | Attachment state tracking |
| **ControllerUnpublishVolume implementation** | Revokes volume availability from node after NodeUnstage | Medium | Attachment state tracking |
| **Attachment state tracking** | Must track which node has volume attached to detect conflicts | Medium | In-memory or persistent state store |
| **Idempotent Publish (same node)** | Retries with same volume+node must succeed immediately | Low | State lookup |
| **Idempotent Unpublish** | Unpublish of already-unpublished volume returns success | Low | State lookup |
| **FAILED_PRECONDITION on multi-attach attempt** | Return gRPC code 9 when RWO volume already attached elsewhere | Low | State lookup |
| **NOT_FOUND for missing volume** | Return gRPC code 5 when volume_id doesn't exist | Low | RDS volume lookup |
| **NOT_FOUND for missing node** | Return gRPC code 5 when node_id doesn't exist | Low | Node validation |
| **Volume capability validation** | Reject publish if capabilities incompatible | Low | Capability checking |
| **publish_context return** | Return metadata for NodeStage (NVMe connection info) | Low | VolumeContext assembly |

### CSI Specification Requirements (HIGH Confidence)

Per the [CSI Specification v1.7.0](https://github.com/container-storage-interface/spec/blob/v1.7.0/spec.md):

**ControllerPublishVolume:**
- MUST be idempotent - if already published at node with compatible capabilities, return OK
- MUST return `FAILED_PRECONDITION` if volume published at different node without MULTI_NODE capability
- SHOULD specify node_id of current attachment in error message when returning FAILED_PRECONDITION
- MUST return `NOT_FOUND` if volume_id or node_id doesn't exist
- Called BEFORE `NodeStageVolume` - establishes attachment at controller level

**ControllerUnpublishVolume:**
- MUST be idempotent - if already unpublished, return OK
- Called AFTER all `NodeUnstageVolume` and `NodeUnpublishVolume` complete
- MUST complete before `DeleteVolume` can be called

---

## Differentiators

Features that improve robustness beyond minimum spec compliance.

| Feature | Value Proposition | Complexity | Dependencies |
|---------|-------------------|------------|--------------|
| **publish_context with NVMe parameters** | Pass ctrl_loss_tmo, reconnect_delay to NodeStage for resilient connections | Low | Existing NVMe param parsing |
| **Graceful concurrent request handling** | Return ABORTED (code 10) for in-flight operations on same volume | Medium | Request tracking/locking |
| **Attachment reconciliation with LIST_VOLUMES** | Periodically sync attachments with external-attacher for self-healing | High | LIST_VOLUMES_PUBLISHED_NODES capability |
| **Persistent attachment state** | Survive controller restarts without losing attachment knowledge | Medium | ConfigMap or CRD storage |
| **Prometheus metrics for attachments** | Expose attachment counts, durations, failure rates | Low | Existing metrics infrastructure |
| **Kubernetes Events on attach/detach** | Post events for visibility into attachment lifecycle | Low | Existing event posting |
| **Force-detach with timeout** | Allow forced attachment override after configurable timeout | Medium | Timestamp tracking, safety checks |
| **Node health integration** | Consider node Ready status before allowing attachment | Medium | Kubernetes API client |

### Recommended Differentiators for v0.3.0

1. **publish_context with NVMe parameters** - Low effort, high value for NVMe resilience
2. **Kubernetes Events** - Already have event infrastructure, adds visibility
3. **Prometheus metrics** - Already have metrics infrastructure, adds observability

Defer to later versions:
- Persistent state (adds complexity, in-memory sufficient for single-replica controller)
- Attachment reconciliation (requires additional capability, adds complexity)
- Force-detach (safety concerns, requires careful design)

---

## Anti-Features

Features to explicitly NOT build. Common mistakes in this domain.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **Storage-level locking** | NVMe/TCP has no native reservation mechanism; would require RDS firmware changes | Rely on Kubernetes VolumeAttachment tracking |
| **Automatic force-detach on node failure** | Can cause data corruption if node is actually still running (split-brain) | Wait for Kubernetes non-graceful shutdown taint, then allow reattachment |
| **Multi-attach for RWO volumes** | Violates CSI spec, causes data corruption | Return FAILED_PRECONDITION, never allow |
| **Blocking on NVMe disconnect in Unpublish** | Node may be unreachable; cannot guarantee disconnect | Track attachment state only; let NodeUnstage handle NVMe disconnect |
| **Distributed state for attachments** | Overkill for single-controller deployment; adds failure modes | Use controller-local state (in-memory or single ConfigMap) |
| **Custom fencing protocols** | Reinventing wheel; Kubernetes already handles this via VolumeAttachment | Implement standard CSI Publish/Unpublish, let external-attacher manage |
| **Skipping ControllerUnpublish on node failure** | Creates orphaned attachments that block future attachments | Always require Unpublish before re-Publish to different node |

### Critical Anti-Pattern: Node-Level Fencing

**Do NOT implement:**
- SCSI reservations (not applicable to NVMe/TCP)
- Storage-side I/O fencing (RDS doesn't support)
- Network-level fencing (out of scope for CSI driver)

**Why:** The [CSI-Addons fencing specification](https://github.com/csi-addons/spec/blob/main/fence/README.md) defines storage-level fencing for specialized storage systems. RDS (file-backed NVMe/TCP) doesn't have these capabilities. Kubernetes VolumeAttachment tracking provides sufficient fencing for single-writer semantics.

---

## Edge Cases in Attachment Tracking

### Case 1: Timeout During ControllerPublishVolume

**Scenario:** RPC times out while attachment is in progress.

**Expected Behavior:**
- External-attacher retries with exponential backoff (1s default start, 5m max)
- Driver must check if attachment already completed
- Return success if volume already attached to target node

**Implementation:**
```go
// Check existing attachment before performing new attach
if currentNode := getAttachedNode(volumeID); currentNode == targetNode {
    return &ControllerPublishVolumeResponse{PublishContext: ...}, nil
}
```

**Source:** [AWS EBS CSI Driver Design](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/docs/design.md)

### Case 2: Concurrent Publish to Different Nodes

**Scenario:** Two ControllerPublishVolume calls arrive nearly simultaneously for same volume but different nodes.

**Expected Behavior:**
- First request acquires lock, proceeds with attachment
- Second request must wait or return ABORTED
- Once first completes, second returns FAILED_PRECONDITION (volume now attached elsewhere)

**Implementation:**
```go
// Use per-volume mutex
volumeLock := getVolumeLock(volumeID)
if !volumeLock.TryLock() {
    return nil, status.Error(codes.Aborted, "operation in progress")
}
defer volumeLock.Unlock()
```

**Source:** [CSI Spec - Concurrency](https://github.com/container-storage-interface/spec/blob/master/spec.md)

### Case 3: Node Deleted While Volume Attached

**Scenario:** Kubernetes node object deleted while volume still shows attached.

**Expected Behavior:**
- ControllerUnpublishVolume called for deleted node
- Driver should allow unpublish even if node doesn't exist (idempotent)
- Clear attachment state to allow reattachment elsewhere

**Common Mistake:** Returning NOT_FOUND for deleted node blocks volume migration.

**Implementation:**
```go
// Don't fail if node is gone - just clear attachment
if !nodeExists(nodeID) {
    klog.Warningf("Node %s no longer exists, clearing attachment for volume %s", nodeID, volumeID)
}
clearAttachment(volumeID)
return &ControllerUnpublishVolumeResponse{}, nil
```

**Source:** [Dell CSI Drivers Knowledge Base](https://www.dell.com/support/kbdoc/en-us/000200778/container-storage-interface-csi-drivers-family-when-a-node-goes-down-due-to-node-crash-node-down-power-off-scenario-pods-cannot-come-up-on-a-new-node-because-storage-volumes-cannot-be-attached)

### Case 4: Controller Restart with In-Memory State

**Scenario:** Controller pod restarts, losing attachment state.

**Expected Behavior:**
- External-attacher has authoritative state in VolumeAttachment objects
- On startup, driver can rebuild state from LIST_VOLUMES if supported
- Or, driver can treat all volumes as "unknown attachment state"

**Trade-off:**
- **Conservative (recommended for v0.3.0):** Require Unpublish before Publish after restart
- **Aggressive:** Allow Publish to any node, risk brief dual-attachment during transition

**Implementation for Conservative Approach:**
```go
// After restart, attachment state is unknown
// Return FAILED_PRECONDITION if we don't know current state
// External-attacher will call Unpublish first if needed
```

### Case 5: ControllerUnpublish Before NodeUnstage Completes

**Scenario:** Pod deleted, ControllerUnpublish called while NVMe still connected.

**Expected Behavior (per CSI spec):**
- ControllerUnpublishVolume MUST be called after NodeUnstageVolume
- Kubernetes orchestrates this ordering via kubelet -> external-attacher

**Reality:**
- Non-graceful shutdown may violate this ordering
- Driver should NOT block on NVMe disconnect status
- Clear attachment state; let NodeUnstage handle NVMe cleanup separately

**Source:** [CSI Spec - RPC Ordering](https://github.com/container-storage-interface/spec/blob/master/spec.md)

### Case 6: VolumeAttachment Finalizer Stuck

**Scenario:** VolumeAttachment object has finalizer but ControllerUnpublish keeps failing.

**Expected Behavior:**
- External-attacher retries with backoff
- If node is gone, ControllerUnpublish should succeed (idempotent)
- Manual intervention: remove finalizer to unblock

**Prevention:**
- Ensure Unpublish is always idempotent
- Don't require node to be reachable for Unpublish
- Log clearly when clearing attachment for missing node

**Source:** [Kubernetes External-Attacher](https://github.com/kubernetes-csi/external-attacher)

### Case 7: Publish After Non-Graceful Node Shutdown

**Scenario:** Node powered off unexpectedly, pods rescheduled, new Publish arrives.

**Expected Behavior:**
- Old VolumeAttachment may still exist (node not responding to Unpublish)
- Manual taint `node.kubernetes.io/out-of-service` signals safe to proceed
- Driver should check for out-of-service taint before forcing reattachment

**Kubernetes Feature:** Non-Graceful Node Shutdown (GA in v1.28)

**Source:** [Kubernetes Non-Graceful Node Shutdown](https://kubernetes.io/blog/2023/08/16/kubernetes-1-28-non-graceful-node-shutdown-ga/)

---

## Feature Dependencies

```
                    ┌─────────────────────────┐
                    │ PUBLISH_UNPUBLISH_VOLUME│
                    │     Capability          │
                    └───────────┬─────────────┘
                                │
              ┌─────────────────┼─────────────────┐
              ▼                 ▼                 ▼
    ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
    │ControllerPublish│ │ControllerUnpub- │ │   Attachment    │
    │    Volume       │ │   lishVolume    │ │   State Store   │
    └────────┬────────┘ └────────┬────────┘ └────────┬────────┘
             │                   │                   │
             └─────────────┬─────┴───────────────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
    ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
    │ Idempotency  │ │ FAILED_PRE-  │ │ publish_     │
    │   Checks     │ │  CONDITION   │ │   context    │
    └──────────────┘ └──────────────┘ └──────────────┘
```

### Existing Features to Leverage

| Existing Feature | How It Helps |
|-----------------|--------------|
| `GetVolume` in RDS client | Volume existence check for NOT_FOUND |
| `VolumeContext` in CreateVolume | Template for publish_context structure |
| NVMe connection params parsing | Reuse for publish_context NVMe settings |
| Prometheus metrics | Add attachment metrics |
| Kubernetes event posting | Add attach/detach events |
| Security audit logging | Log attach/detach operations |

---

## MVP Recommendation

For v0.3.0 MVP, prioritize:

### Must Have (Table Stakes)
1. **PUBLISH_UNPUBLISH_VOLUME capability declaration**
2. **ControllerPublishVolume with in-memory state tracking**
3. **ControllerUnpublishVolume with idempotent cleanup**
4. **FAILED_PRECONDITION for RWO multi-attach attempts**
5. **Idempotent behavior for same-node publish**

### Should Have (High-Value Differentiators)
6. **publish_context with NVMe connection parameters**
7. **Prometheus metrics for attachment operations**
8. **Kubernetes Events for attach/detach**

### Defer to v0.4.0+
- Persistent attachment state (ConfigMap/CRD)
- LIST_VOLUMES_PUBLISHED_NODES capability
- Attachment reconciliation
- Force-detach with timeout
- Node health integration

---

## Complexity Estimates

| Feature | Complexity | Effort | Risk |
|---------|-----------|--------|------|
| Capability declaration | Low | 1 hour | None |
| ControllerPublishVolume (basic) | Medium | 4 hours | Low |
| ControllerUnpublishVolume (basic) | Medium | 2 hours | Low |
| In-memory attachment state | Medium | 2 hours | Medium (lost on restart) |
| FAILED_PRECONDITION logic | Low | 1 hour | Low |
| publish_context assembly | Low | 1 hour | None |
| Metrics integration | Low | 2 hours | None |
| Event posting | Low | 1 hour | None |
| Unit tests | Medium | 4 hours | None |
| Integration testing | High | 8 hours | Medium |

**Total Estimate:** 25-30 hours for MVP

---

## Sources

### CSI Specification (HIGH Confidence)
- [CSI Spec v1.7.0](https://github.com/container-storage-interface/spec/blob/v1.7.0/spec.md)
- [CSI Spec Master](https://github.com/container-storage-interface/spec/blob/master/spec.md)

### Kubernetes CSI Documentation (HIGH Confidence)
- [External-Attacher Sidecar](https://kubernetes-csi.github.io/docs/external-attacher.html)
- [CSI Driver Object](https://kubernetes-csi.github.io/docs/csi-driver-object.html)
- [Developing a CSI Driver](https://kubernetes-csi.github.io/docs/developing.html)

### Production Driver Implementations (MEDIUM Confidence)
- [AWS EBS CSI Driver Design](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/docs/design.md)
- [vSphere CSI Driver](https://docs.okd.io/latest/storage/container_storage_interface/persistent-storage-csi-vsphere.html)

### Kubernetes Features (HIGH Confidence)
- [Non-Graceful Node Shutdown GA](https://kubernetes.io/blog/2023/08/16/kubernetes-1-28-non-graceful-node-shutdown-ga/)
- [Node Shutdowns Documentation](https://kubernetes.io/docs/concepts/cluster-administration/node-shutdown/)

### Edge Cases and Troubleshooting (MEDIUM Confidence)
- [Dell CSI Drivers - Node Down Scenarios](https://www.dell.com/support/kbdoc/en-us/000200778/container-storage-interface-csi-drivers-family-when-a-node-goes-down-due-to-node-crash-node-down-power-off-scenario-pods-cannot-come-up-on-a-new-node-because-storage-volumes-cannot-be-attached)
- [Kubernetes PR #96617 - Dangling Attachments](https://github.com/kubernetes/kubernetes/pull/96617)
- [Portworx Multi-Attach Troubleshooting](https://portworx.com/knowledge-hub/volume-is-already-exclusively-attached-to-one-node-and-cant-be-attached-to-another/)

### KubeVirt Integration (MEDIUM Confidence)
- [KubeVirt Volume Migration](https://kubevirt.io/user-guide/storage/volume_migration/)
- [KubeVirt Disks and Volumes](https://kubevirt.io/user-guide/storage/disks_and_volumes/)
