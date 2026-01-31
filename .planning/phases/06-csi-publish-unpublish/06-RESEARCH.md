# Phase 6: CSI Publish/Unpublish Implementation - Research

**Researched:** 2026-01-30
**Domain:** CSI Controller ControllerPublishVolume/ControllerUnpublishVolume Implementation
**Confidence:** HIGH

## Summary

Phase 6 implements the CSI ControllerPublishVolume and ControllerUnpublishVolume RPCs to enforce ReadWriteOnce (RWO) semantics. These methods track which nodes have volumes attached, reject conflicting attachment requests, and return NVMe connection information to NodeStageVolume via publish_context.

Building on Phase 5's AttachmentManager (already implemented in `pkg/attachment/`), this phase adds the actual CSI controller methods that use that manager. The implementation follows the CSI spec v1.10.0 for error codes, idempotency, and publish_context format.

**Primary recommendation:** Replace the current stub implementations in controller.go with proper ControllerPublishVolume/ControllerUnpublishVolume that use AttachmentManager for state tracking, return FAILED_PRECONDITION (code 9) for RWO conflicts, include node existence validation for self-healing, and return NVMe connection parameters in publish_context (nvme_address, nvme_port, nvme_nqn, fs_type).

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/container-storage-interface/spec | v1.10.0 | CSI protobuf definitions | Already in use, defines ControllerPublishVolume/ControllerUnpublishVolume RPCs |
| google.golang.org/grpc | v1.69.2 | gRPC status codes | Already in use, provides codes.FailedPrecondition, codes.NotFound, etc. |
| k8s.io/client-go | v0.28.0 | Node existence validation | Already in use for PV annotation updates |
| pkg/attachment | (local) | AttachmentManager from Phase 5 | Already implemented, provides TrackAttachment/UntrackAttachment |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| k8s.io/apimachinery/pkg/api/errors | v0.28.0 | IsNotFound error check | Detecting deleted nodes for self-healing |
| k8s.io/klog/v2 | v2.130.1 | Structured logging | Already in use throughout codebase |
| pkg/driver/events.go | (local) | EventPoster | Posting K8s events for attachment conflicts |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| In-memory + annotations | VolumeAttachment CRD | VolumeAttachments managed by external-attacher, not suitable for internal tracking |
| publish_context snake_case | camelCase | snake_case matches existing volumeContext patterns in codebase |
| Node.Get() validation | Skip validation | Validation enables self-healing when blocking node is deleted |

**Installation:**
```bash
# No new dependencies required - all libraries already in go.mod
go mod verify
```

## Architecture Patterns

### Recommended Code Locations
```
pkg/driver/
├── controller.go          # ControllerPublishVolume/ControllerUnpublishVolume (MODIFY existing stubs)
├── driver.go              # Add PUBLISH_UNPUBLISH_VOLUME capability (MODIFY)
└── events.go              # Add PostAttachmentConflict event method (MODIFY)

pkg/attachment/
├── manager.go             # TrackAttachment/UntrackAttachment (EXISTS from Phase 5)
├── lock.go                # VolumeLockManager (EXISTS from Phase 5)
└── types.go               # AttachmentState (EXISTS from Phase 5)
```

### Pattern 1: CSI Error Code Mapping
**What:** Map operational conditions to correct CSI/gRPC error codes
**When to use:** Any error return from ControllerPublish/Unpublish
**Reference:** CSI Spec v1.10.0

| Condition | Code | gRPC Code Value |
|-----------|------|-----------------|
| Volume already attached to different node (RWO) | FAILED_PRECONDITION | 9 |
| Volume not found | NOT_FOUND | 5 |
| Node not found (target node) | NOT_FOUND | 5 |
| Missing required field | INVALID_ARGUMENT | 3 |
| Internal error (persistence failure) | INTERNAL | 13 |
| Operation already in progress | ABORTED | 10 |

**Example:**
```go
// Source: CSI Spec - ControllerPublishVolume error handling
// Already attached to same node = SUCCESS (idempotent)
if existing.NodeID == nodeID {
    return &csi.ControllerPublishVolumeResponse{
        PublishContext: publishContext,
    }, nil
}

// Already attached to different node = FAILED_PRECONDITION
return nil, status.Errorf(codes.FailedPrecondition,
    "volume %s already attached to node %s, cannot attach to %s",
    volumeID, existing.NodeID, nodeID)
```

### Pattern 2: publish_context Format for NVMe/TCP
**What:** Return connection parameters needed by NodeStageVolume
**When to use:** Successful ControllerPublishVolume response
**Decision:** Use snake_case keys to match existing volumeContext patterns

```go
// Source: CONTEXT.md decision - separate fields, snake_case
func (cs *ControllerServer) buildPublishContext(volume *rds.VolumeInfo, params map[string]string) map[string]string {
    return map[string]string{
        "nvme_address": cs.getNVMEAddress(params),
        "nvme_port":    fmt.Sprintf("%d", volume.NVMETCPPort),
        "nvme_nqn":     volume.NVMETCPNQN,
        "fs_type":      cs.getFSType(params), // Default: ext4
    }
}
```

### Pattern 3: Node Existence Validation with Self-Healing
**What:** Before rejecting attachment, verify the blocking node still exists
**When to use:** When existing attachment blocks new attachment (CONTEXT.md decision)
**Why:** Enables automatic recovery when node is deleted without proper cleanup

```go
// Source: CONTEXT.md - Node Validation Behavior decision
func (cs *ControllerServer) validateBlockingNodeExists(ctx context.Context, nodeID string) (bool, error) {
    _, err := cs.driver.k8sClient.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})
    if err != nil {
        if errors.IsNotFound(err) {
            return false, nil // Node deleted, attachment is stale
        }
        return false, err // API error
    }
    return true, nil
}
```

### Pattern 4: Idempotent Unpublish
**What:** ControllerUnpublishVolume always succeeds, even if volume not attached
**When to use:** All ControllerUnpublishVolume implementations (CONTEXT.md decision)
**Why:** Prevents VolumeAttachment stuck in Terminating state

```go
// Source: CSI Spec idempotency requirement + CONTEXT.md decision
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context,
    req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

    volumeID := req.GetVolumeId()

    // Untrack - idempotent, returns nil even if not tracked
    if err := cs.driver.attachmentManager.UntrackAttachment(ctx, volumeID); err != nil {
        klog.Warningf("Failed to untrack attachment for %s: %v (continuing)", volumeID, err)
    }

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}
```

### Anti-Patterns to Avoid

- **Returning error for idempotent publish:** If volume already attached to same node, return success with publish_context, not error
- **Failing unpublish on state mismatch:** Always return success from ControllerUnpublishVolume, log warnings for inconsistencies
- **Skipping per-volume locks:** Use AttachmentManager's built-in VolumeLockManager to serialize operations
- **Hardcoding NVMe parameters:** Extract from volume info and StorageClass params, don't hardcode
- **Blocking on node validation failure:** On k8s API errors, fail-closed (return error) to prevent data corruption

## Don't Hand-Roll

Problems with existing solutions in the codebase:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Volume locking | Custom mutex per operation | pkg/attachment.VolumeLockManager | Already handles lock creation/cleanup, prevents deadlocks |
| Attachment state | map[string]string in controller | pkg/attachment.AttachmentManager | Already handles persistence, rebuild, thread-safety |
| Error code selection | Switch on string matching | status.Errorf(codes.X, ...) | Type-safe, matches CSI spec exactly |
| PV annotation updates | Direct k8sClient.Update | AttachmentManager.persistAttachment | Already handles RetryOnConflict |
| K8s event posting | Direct recorder.Event | pkg/driver.EventPoster | Already has error handling, metrics integration |

**Key insight:** Phase 5 created the AttachmentManager specifically for this phase. Use its TrackAttachment/UntrackAttachment/GetAttachment methods rather than building parallel state tracking.

## Common Pitfalls

### Pitfall 1: Race Condition in Check-Then-Act
**What goes wrong:** Checking attachment state then acting allows concurrent requests to both succeed
**Why it happens:** Gap between GetAttachment() and TrackAttachment()
**How to avoid:** AttachmentManager.TrackAttachment already handles this atomically with per-volume locks
**Warning signs:** Both pods scheduled on different nodes, both succeed to mount volume

### Pitfall 2: Wrong Error Code for RWO Conflict
**What goes wrong:** Using ALREADY_EXISTS or ABORTED instead of FAILED_PRECONDITION
**Why it happens:** Confusion between CSI error codes
**How to avoid:** FAILED_PRECONDITION (9) for RWO volume attached elsewhere, ALREADY_EXISTS (6) for incompatible capabilities on SAME node
**Warning signs:** External-attacher retries infinitely instead of giving up

### Pitfall 3: Empty publish_context
**What goes wrong:** NodeStageVolume can't find NVMe connection parameters
**Why it happens:** Returning empty map or omitting fields
**How to avoid:** Always populate nvme_address, nvme_port, nvme_nqn, fs_type
**Warning signs:** NodeStageVolume fails with "missing required volume context"

### Pitfall 4: Stale Attachment Blocks New Pod
**What goes wrong:** Node deleted but attachment record remains, blocking new attachments
**Why it happens:** No validation that blocking node still exists
**How to avoid:** Check node existence before rejecting; auto-clear if node deleted (CONTEXT.md decision)
**Warning signs:** Pod stuck in ContainerCreating with "already attached to [deleted-node]"

### Pitfall 5: Missing Capability Declaration
**What goes wrong:** External-attacher doesn't call ControllerPublish/Unpublish
**Why it happens:** PUBLISH_UNPUBLISH_VOLUME capability not declared in ControllerGetCapabilities
**How to avoid:** Add capability to driver.addControllerServiceCapabilities()
**Warning signs:** VolumeAttachment objects never created, node directly calls NodeStageVolume

## Code Examples

### ControllerPublishVolume Implementation
```go
// Source: CSI Spec + CONTEXT.md decisions + existing controller.go patterns
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context,
    req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    klog.V(2).Infof("ControllerPublishVolume called for volume %s to node %s", volumeID, nodeID)

    // Validate request
    if volumeID == "" {
        return nil, status.Error(codes.InvalidArgument, "volume ID is required")
    }
    if nodeID == "" {
        return nil, status.Error(codes.InvalidArgument, "node ID is required")
    }

    // Verify volume exists on RDS
    volume, err := cs.driver.rdsClient.GetVolume(volumeID)
    if err != nil {
        return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
    }

    // Get attachment manager
    am := cs.driver.GetAttachmentManager()
    if am == nil {
        return nil, status.Error(codes.Internal, "attachment manager not initialized")
    }

    // Check existing attachment
    existing, exists := am.GetAttachment(volumeID)
    if exists {
        if existing.NodeID == nodeID {
            // Idempotent: already attached to same node
            klog.V(2).Infof("Volume %s already attached to node %s (idempotent)", volumeID, nodeID)
            return &csi.ControllerPublishVolumeResponse{
                PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
            }, nil
        }

        // Before rejecting, verify blocking node still exists
        nodeExists, err := cs.validateBlockingNodeExists(ctx, existing.NodeID)
        if err != nil {
            // API error - fail closed to prevent data corruption
            return nil, status.Errorf(codes.Internal, "failed to verify node %s: %v", existing.NodeID, err)
        }

        if !nodeExists {
            // Node deleted - auto-clear stale attachment (self-healing)
            klog.Warningf("Volume %s attached to deleted node %s, clearing stale attachment", volumeID, existing.NodeID)
            if err := am.UntrackAttachment(ctx, volumeID); err != nil {
                klog.Warningf("Failed to clear stale attachment: %v", err)
            }
            // Fall through to allow new attachment
        } else {
            // Node exists - genuine RWO conflict
            // Post event for operator visibility (CONTEXT.md decision)
            cs.postAttachmentConflictEvent(ctx, req, existing.NodeID)
            return nil, status.Errorf(codes.FailedPrecondition,
                "volume %s already attached to node %s, cannot attach to %s",
                volumeID, existing.NodeID, nodeID)
        }
    }

    // Track new attachment (uses per-volume lock internally)
    if err := am.TrackAttachment(ctx, volumeID, nodeID); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to track attachment: %v", err)
    }

    klog.V(2).Infof("Successfully published volume %s to node %s", volumeID, nodeID)

    return &csi.ControllerPublishVolumeResponse{
        PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
    }, nil
}
```

### ControllerUnpublishVolume Implementation
```go
// Source: CSI Spec + CONTEXT.md decisions
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context,
    req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

    volumeID := req.GetVolumeId()
    klog.V(2).Infof("ControllerUnpublishVolume called for volume %s", volumeID)

    // Validate request
    if volumeID == "" {
        return nil, status.Error(codes.InvalidArgument, "volume ID is required")
    }

    // Get attachment manager
    am := cs.driver.GetAttachmentManager()
    if am == nil {
        // No attachment manager = nothing to untrack
        return &csi.ControllerUnpublishVolumeResponse{}, nil
    }

    // Untrack attachment (idempotent - succeeds even if not tracked)
    if err := am.UntrackAttachment(ctx, volumeID); err != nil {
        // Log but don't fail - unpublish must be idempotent (CONTEXT.md decision)
        klog.Warningf("Error untracking attachment for %s: %v (returning success)", volumeID, err)
    }

    klog.V(2).Infof("Successfully unpublished volume %s", volumeID)

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}
```

### Capability Declaration Addition
```go
// Source: pkg/driver/driver.go - addControllerServiceCapabilities()
// Add this capability to existing list
{
    Type: &csi.ControllerServiceCapability_Rpc{
        Rpc: &csi.ControllerServiceCapability_RPC{
            Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
        },
    },
},
```

### Build Publish Context Helper
```go
// Source: New helper function in controller.go
func (cs *ControllerServer) buildPublishContext(volume *rds.VolumeInfo, params map[string]string) map[string]string {
    fsType := defaultFSType
    if fs, ok := params[paramFSType]; ok && fs != "" {
        fsType = fs
    }

    return map[string]string{
        "nvme_address": cs.getNVMEAddress(params),
        "nvme_port":    fmt.Sprintf("%d", volume.NVMETCPPort),
        "nvme_nqn":     volume.NVMETCPNQN,
        "fs_type":      fsType,
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Return Unimplemented | Implement with fencing | Phase 6 | Enables RWO enforcement |
| volumeContext only | publish_context preferred | CSI 1.x | publish_context for controller-provided data |
| Fail on any mismatch | Self-heal stale attachments | Phase 6 | Automatic recovery from node failures |

**Current stub in controller.go (lines 336-343):**
```go
// ControllerPublishVolume is not supported (node-local attachment)
func (cs *ControllerServer) ControllerPublishVolume(...) (*csi.ControllerPublishVolumeResponse, error) {
    return nil, status.Error(codes.Unimplemented, "ControllerPublishVolume is not supported")
}
```
This will be replaced with the full implementation.

## Open Questions

### 1. NodeStage: publish_context vs volume_context Priority
**What we know:** Both publish_context and volume_context are available in NodeStageVolume. CONTEXT.md says Claude decides.
**What's unclear:** Whether to prefer publish_context over volume_context when both contain same keys.
**Recommendation:** Prefer publish_context (controller-provided, more current) over volume_context (provisioning-time). NodeStageVolume should check publish_context first, fall back to volume_context. LOW priority - current NodeStageVolume only uses volume_context, works fine.

### 2. RDS SSH Retry Strategy
**What we know:** CONTEXT.md marks this as Claude's discretion.
**What's unclear:** Whether ControllerPublish should retry SSH failures internally or return error to CO.
**Recommendation:** Return error to CO (external-attacher) for retry. Internal retries add latency and complexity. CO has exponential backoff built in. Match existing CreateVolume/DeleteVolume patterns which don't retry internally.

### 3. Kubernetes API Failure Handling
**What we know:** CONTEXT.md marks this as Claude's discretion (safety vs availability).
**What's unclear:** When node validation API call fails, fail-closed (error) or fail-open (allow attach)?
**Recommendation:** Fail-closed (return error). Data corruption is worse than temporary unavailability. Match defensive patterns in existing code. The CO will retry.

## Sources

### Primary (HIGH confidence)
- [CSI Spec v1.10.0](https://github.com/container-storage-interface/spec/blob/master/spec.md) - ControllerPublishVolume/ControllerUnpublishVolume RPCs, error codes, publish_context
- `pkg/attachment/manager.go` - AttachmentManager implementation from Phase 5
- `pkg/driver/controller.go` - Existing CreateVolume/DeleteVolume patterns
- `06-CONTEXT.md` - User decisions for error handling, node validation, idempotency

### Secondary (MEDIUM confidence)
- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver) - ControllerPublishVolume implementation patterns
- [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/) - External-attacher behavior, capability declaration
- `.planning/research/ARCHITECTURE.md` - Prior research on attachment tracking architecture
- `.planning/research/PITFALLS.md` - Prior research on implementation pitfalls

### Tertiary (LOW confidence)
- WebSearch results on node deletion handling - community patterns, varying approaches

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Using existing libraries and Phase 5 components
- Architecture: HIGH - Patterns derived from CSI spec and existing codebase
- Pitfalls: HIGH - Based on prior research in .planning/research/PITFALLS.md

**Research date:** 2026-01-30
**Valid until:** 60 days (CSI spec stable, no breaking changes expected)
