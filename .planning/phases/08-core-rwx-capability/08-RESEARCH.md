# Phase 8: Core RWX Capability - Research

**Researched:** 2026-02-03
**Domain:** CSI multi-node access modes, KubeVirt live migration, NVMe/TCP concurrent access
**Confidence:** HIGH

## Summary

ReadWriteMany (RWX) support for block volumes enables KubeVirt VM live migration by allowing a volume to be attached to exactly two nodes simultaneously (source and destination during migration). The CSI spec defines MULTI_NODE_MULTI_WRITER access mode for this purpose, but most block storage CSI drivers do NOT support RWX due to data corruption risks with standard filesystems.

The key insight is that RWX for block volumes is safe ONLY when the application layer (QEMU in our case) coordinates I/O access. Standard filesystems (ext4, xfs) are NOT designed for concurrent multi-node access and WILL corrupt data. KubeVirt handles this by using QEMU's cache=none mode and raw block devices, trusting QEMU to coordinate I/O between source and destination during migration.

This phase implements driver-side RWX support (capability declaration, dual-attach tracking, validation) while explicitly rejecting RWX filesystem volumes to prevent corruption. Migration safety and I/O coordination are handled by KubeVirt/QEMU, not the driver.

**Primary recommendation:** Implement strict validation that accepts RWX+block but rejects RWX+filesystem, track up to 2 concurrent attachments per volume, and provide migration-aware error messages to guide operators.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/container-storage-interface/spec | v1.10.0+ | CSI gRPC definitions | Official CSI spec implementation, defines VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER |
| k8s.io/client-go | v0.28.0+ | Kubernetes client | Required for querying node existence during attachment validation |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| sync.Map | stdlib | Concurrent-safe map | Track multiple attachments per volume (volumeID -> []nodeID) |
| time.Time | stdlib | Timestamp tracking | Track attachment order (primary vs secondary node) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Map of slices | Map of structs with node list | Struct approach cleaner but map-of-slices sufficient for 2-node limit |
| External state store | In-memory tracking | External store adds complexity, in-memory sufficient for controller HA scenario |

**Installation:**
No new dependencies required - all libraries already in use.

## Architecture Patterns

### Recommended Data Structure for Dual-Attach Tracking

Extend existing AttachmentManager to support multiple nodes per volume:

```go
// AttachmentState tracks volume-to-node(s) binding
type AttachmentState struct {
    VolumeID   string
    Nodes      []NodeAttachment  // Ordered: [0]=primary, [1]=secondary
    AttachedAt time.Time         // When first node attached
}

type NodeAttachment struct {
    NodeID     string
    AttachedAt time.Time  // Track order for migration debugging
}
```

**Key design decision:** Use ordered slice (not map) to distinguish primary (first attached) from secondary (second attached during migration). This helps debugging and provides clear semantics for "which node attached first."

### Pattern 1: Access Mode Detection in CreateVolume

**What:** Validate volume capabilities early in CreateVolume to reject unsupported combinations
**When to use:** Always - fail fast before volume provisioning

**Example:**
```go
// Source: CSI spec validation pattern + driver requirements
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
    // Extract access mode and volume mode from capabilities
    for _, cap := range req.GetVolumeCapabilities() {
        accessMode := cap.GetAccessMode().GetMode()
        isBlock := cap.GetBlock() != nil
        isFilesystem := cap.GetMount() != nil

        // ROADMAP-4: RWX block-only, reject RWX filesystem
        if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
            if isFilesystem {
                return nil, status.Error(codes.InvalidArgument,
                    "RWX access mode requires volumeMode: Block. Filesystem volumes risk data corruption with multi-node access. Use Block mode for KubeVirt live migration.")
            }
            klog.V(2).Infof("Creating RWX block volume %s (KubeVirt live migration)", volumeID)
        }
    }

    // Proceed with volume creation
    return &csi.CreateVolumeResponse{...}, nil
}
```

### Pattern 2: Dual-Attach Enforcement in ControllerPublishVolume

**What:** Track up to 2 node attachments per volume, reject 3rd attempt
**When to use:** Every ControllerPublishVolume call for RWX volumes

**Example:**
```go
// Source: Derived from existing RWO attachment tracking + RWX requirements
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    am := cs.driver.GetAttachmentManager()

    // Check existing attachments
    existing, exists := am.GetAttachment(volumeID)
    if exists {
        // Already attached to requesting node (idempotent)
        if am.IsAttachedToNode(volumeID, nodeID) {
            klog.V(2).Infof("Volume %s already attached to node %s (idempotent)", volumeID, nodeID)
            return &csi.ControllerPublishVolumeResponse{...}, nil
        }

        // Check attachment limit based on access mode
        accessMode := cs.getAccessMode(req.GetVolumeCapabilities())

        if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
            // ROADMAP-5: 2-node limit during migration
            if len(existing.Nodes) >= 2 {
                // ROADMAP decision: migration-aware error message
                return nil, status.Errorf(codes.FailedPrecondition,
                    "Volume %s already attached to 2 nodes (migration limit). Wait for migration to complete. Attached nodes: %v",
                    volumeID, existing.GetNodeIDs())
            }
            // Allow second attachment (migration target)
            klog.V(2).Infof("Allowing second attachment of RWX volume %s to node %s (migration)", volumeID, nodeID)
        } else {
            // RWO: reject second attachment with hint about RWX
            return nil, status.Errorf(codes.FailedPrecondition,
                "Volume %s already attached to node %s. For multi-node access, use RWX with block volumes.",
                volumeID, existing.Nodes[0].NodeID)
        }
    }

    // Track new attachment
    if err := am.TrackAttachment(ctx, volumeID, nodeID); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to track attachment: %v", err)
    }

    return &csi.ControllerPublishVolumeResponse{
        PublishContext: cs.buildPublishContext(volume, req.GetVolumeContext()),
    }, nil
}
```

### Pattern 3: Capability Declaration

**What:** Declare MULTI_NODE_MULTI_WRITER in ControllerGetCapabilities
**When to use:** Driver initialization

**Example:**
```go
// Source: CSI driver capability declaration pattern
func (d *Driver) addVolumeCapabilities() {
    d.vcaps = []*csi.VolumeCapability_AccessMode{
        {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
        {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
        {Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},  // NEW
    }
}
```

**Note:** ROADMAP decision allows advertising MULTI_NODE_MULTI_WRITER unconditionally since validation happens at volume creation time.

### Anti-Patterns to Avoid

- **Tracking unlimited attachments:** Don't use unbounded slice or map of nodes - enforce 2-node limit explicitly with clear error messages
- **Backend coordination:** Don't attempt to coordinate I/O at driver level - that's QEMU's responsibility (ROADMAP-6: trust QEMU)
- **Filesystem RWX:** Never allow RWX with filesystem volumes - guaranteed data corruption
- **Silent attachment:** Don't log RWX attachments at same level as RWO conflicts - use distinct log entries for debugging (CONTEXT decision)

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Multi-node I/O coordination | Custom locking, distributed locks, fencing | Trust QEMU cache=none | QEMU handles I/O coordination during migration; driver just permits dual-attach |
| Clustered filesystem | Custom filesystem layer | Reject RWX filesystem entirely | Standard filesystems (ext4/xfs) not designed for concurrent access; KubeVirt uses raw block |
| Attachment ordering | Complex state machine | Simple ordered slice | Two-node limit doesn't need complex state; first-attached = primary, second = secondary |
| Node existence validation | Custom polling/watching | k8s client-go Node API | Kubernetes already tracks node lifecycle; use existing API |

**Key insight:** Multi-node block access is ONLY safe when application layer coordinates I/O. The driver's job is to permit dual-attach and validate safe configurations, not to coordinate access itself.

## Common Pitfalls

### Pitfall 1: Allowing RWX with Filesystem Volumes

**What goes wrong:** User creates PVC with `accessModes: [ReadWriteMany]` and `volumeMode: Filesystem`. Two pods on different nodes mount the volume as ext4. Both write concurrently. Filesystem metadata corrupts. Data loss occurs.

**Why it happens:** CSI spec allows drivers to support both block and filesystem RWX. Developers assume filesystem support is safe since NFS works that way. But NFS has built-in coordination; local filesystems don't.

**How to avoid:**
- Validate in CreateVolume: reject RWX + filesystem combination
- Error message must explain WHY: "Filesystem volumes risk data corruption with multi-node access"
- Guide user to correct usage: "Use volumeMode: Block for KubeVirt live migration"

**Warning signs:**
- User reports "corrupted filesystem" after migration
- `fsck` errors in node logs
- Ext4/xfs journal corruption messages

### Pitfall 2: No Attachment Limit Enforcement

**What goes wrong:** Driver tracks attachments but doesn't enforce 2-node limit. User misconfigures workload to use RWX volume across 3+ nodes. All nodes write concurrently. Even QEMU can't coordinate I/O across more than 2 nodes during migration. Data corruption or undefined behavior.

**Why it happens:** Developer implements RWX but doesn't understand KubeVirt's specific use case (2-node migration only). Assumes "multi-writer" means "unlimited writers."

**How to avoid:**
- Hard-code 2-node limit (don't make it configurable via StorageClass)
- Check `len(existing.Nodes) >= 2` before allowing attachment
- Error message must reference migration: "already attached to 2 nodes (migration limit)"
- Log warning if RWX volume used outside KubeVirt context

**Warning signs:**
- More than 2 VolumeAttachment objects for single volume
- User reports "unexpected I/O errors" during multi-pod workloads
- Migration fails with "volume in use" errors

### Pitfall 3: Confusing RWX Dual-Attach with RWO Conflict

**What goes wrong:** Both RWX dual-attach and RWO conflict log at same level. Operator sees "volume attached to multiple nodes" and can't distinguish expected behavior (migration) from problem (RWO violation). Wastes time investigating false alarms.

**Why it happens:** Developer uses same error/warning message for all multi-attach scenarios without considering context.

**How to avoid:**
- RWX dual-attach: `klog.V(2)` (info level) - expected during migration
- RWO conflict: `klog.Warningf` - unexpected, needs attention
- Different error messages:
  - RWX: "Allowing second attachment of RWX volume %s to node %s (migration)"
  - RWO: "Volume %s already attached to node %s, rejecting attachment to %s"
- Post K8s events for RWO conflicts only, not RWX dual-attach

**Warning signs:**
- Operator says "logs full of attachment warnings during migrations"
- Can't distinguish real issues from expected behavior
- Too many false-positive alerts

### Pitfall 4: Missing Node Existence Check

**What goes wrong:** Node crashes during migration. Volume remains tracked as attached to dead node. New attachment attempt rejected with "already attached to 2 nodes." Volume stuck until manual intervention. Migration can't complete.

**Why it happens:** Developer assumes Kubernetes always cleans up attachments when node dies. But network partition or sudden node loss can leave stale state.

**How to avoid:**
- Before rejecting 3rd attach attempt, verify attached nodes still exist
- Use `k8sClient.CoreV1().Nodes().Get(ctx, nodeID, metav1.GetOptions{})`
- If node deleted, auto-clear stale attachment (self-healing)
- Log warning: "Volume %s attached to deleted node %s, clearing stale attachment"

**Warning signs:**
- User reports "can't migrate VM after node failure"
- Attachment count stuck at 2 even though only 1 node running
- Manual PV/PVC deletion required to recover

### Pitfall 5: Access Mode Detection in Wrong Place

**What goes wrong:** Developer validates access mode in ControllerPublishVolume instead of CreateVolume. Volume provisions successfully. User creates deployment, pod scheduled, CSI tries to attach, THEN rejects with "RWX filesystem not supported." PVC stuck in Pending. User confused why volume creation succeeded but attachment failed.

**Why it happens:** Developer puts validation logic near usage point instead of creation point. Seems logical but creates poor UX.

**How to avoid:**
- Validate in CreateVolume (fail fast)
- Also validate in ValidateVolumeCapabilities (used by pre-provisioned volumes)
- ControllerPublishVolume can re-validate but should never be first place user sees error
- Error at creation time: clear cause-effect relationship

**Warning signs:**
- User says "PVC created successfully but pods won't start"
- Events show "failed to attach" but no indication during provisioning
- Support tickets ask "why didn't it tell me earlier?"

## Code Examples

Verified patterns from official sources and existing codebase:

### Access Mode + Volume Mode Detection

```go
// Source: CSI spec pattern + existing validateVolumeCapabilities
func detectVolumeMode(caps []*csi.VolumeCapability) (accessMode csi.VolumeCapability_AccessMode_Mode, isBlock bool, err error) {
    if len(caps) == 0 {
        return 0, false, fmt.Errorf("volume capabilities required")
    }

    // Use first capability (CSI provisioner ensures consistent access mode across all caps)
    cap := caps[0]

    accessMode = cap.GetAccessMode().GetMode()
    isBlock = cap.GetBlock() != nil

    // Validate access type specified
    if cap.GetBlock() == nil && cap.GetMount() == nil {
        return 0, false, fmt.Errorf("volume capability must specify either block or mount")
    }

    return accessMode, isBlock, nil
}
```

### RWX Validation with Actionable Error Messages

```go
// Source: CONTEXT.md error message requirements + Pure Storage/HPE patterns
func validateRWXCapability(accessMode csi.VolumeCapability_AccessMode_Mode, isBlock bool, volumeID string) error {
    if accessMode != csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
        return nil // Not RWX, no validation needed
    }

    if !isBlock {
        // ROADMAP-4: Reject RWX filesystem
        return status.Error(codes.InvalidArgument,
            "RWX access mode requires volumeMode: Block. Filesystem volumes risk data corruption with multi-node access. "+
            "For KubeVirt VM live migration, use volumeMode: Block in your PVC.")
    }

    // Log RWX usage (helps detect misuse outside KubeVirt)
    klog.V(2).Infof("Volume %s requested with RWX block mode (KubeVirt live migration use case)", volumeID)

    return nil
}
```

### Dual-Attach Tracking with Ordered Nodes

```go
// Source: Extends existing pkg/attachment/manager.go pattern
// Modified TrackAttachment to support multiple nodes

func (am *AttachmentManager) TrackAttachment(ctx context.Context, volumeID, nodeID string) error {
    am.volumeLocks.Lock(volumeID)
    defer am.volumeLocks.Unlock(volumeID)

    am.mu.RLock()
    existing, exists := am.attachments[volumeID]
    am.mu.RUnlock()

    if exists {
        // Check if already attached to this node (idempotent)
        for _, na := range existing.Nodes {
            if na.NodeID == nodeID {
                klog.V(2).Infof("Volume %s already attached to node %s (idempotent)", volumeID, nodeID)
                return nil
            }
        }

        // Already attached to different node(s) - caller must check limit
        return fmt.Errorf("volume %s already attached to %d node(s)", volumeID, len(existing.Nodes))
    }

    // Create new attachment state with first node
    state := &AttachmentState{
        VolumeID: volumeID,
        Nodes: []NodeAttachment{
            {NodeID: nodeID, AttachedAt: time.Now()},
        },
        AttachedAt: time.Now(),
    }

    am.mu.Lock()
    am.attachments[volumeID] = state
    am.mu.Unlock()

    klog.V(2).Infof("Tracked attachment: volume=%s, node=%s (primary)", volumeID, nodeID)
    return nil
}

func (am *AttachmentManager) AddSecondaryAttachment(ctx context.Context, volumeID, nodeID string) error {
    am.volumeLocks.Lock(volumeID)
    defer am.volumeLocks.Unlock(volumeID)

    am.mu.Lock()
    defer am.mu.Unlock()

    existing, exists := am.attachments[volumeID]
    if !exists {
        return fmt.Errorf("volume %s not attached", volumeID)
    }

    if len(existing.Nodes) >= 2 {
        return fmt.Errorf("volume %s already attached to 2 nodes", volumeID)
    }

    existing.Nodes = append(existing.Nodes, NodeAttachment{
        NodeID:     nodeID,
        AttachedAt: time.Now(),
    })

    klog.V(2).Infof("Tracked secondary attachment: volume=%s, node=%s (migration target)", volumeID, nodeID)
    return nil
}
```

### Migration-Aware Error Messages

```go
// Source: CONTEXT.md error message requirements
func (cs *ControllerServer) checkAttachmentLimit(volumeID string, accessMode csi.VolumeCapability_AccessMode_Mode, existing *AttachmentState) error {
    if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
        if len(existing.Nodes) >= 2 {
            // ROADMAP-5: 2-node limit with migration-aware message
            return status.Errorf(codes.FailedPrecondition,
                "Volume %s already attached to 2 nodes (migration limit). Wait for migration to complete. Attached nodes: %v",
                volumeID, getNodeIDs(existing.Nodes))
        }
    } else {
        // RWO: include hint about RWX
        return status.Errorf(codes.FailedPrecondition,
            "Volume %s already attached to node %s. For multi-node access, use RWX with block volumes.",
            volumeID, existing.Nodes[0].NodeID)
    }
    return nil
}

func getNodeIDs(nodes []NodeAttachment) []string {
    ids := make([]string, len(nodes))
    for i, n := range nodes {
        ids[i] = n.NodeID
    }
    return ids
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Single attachment per volume | Multiple attachments for RWX | CSI spec v1.0+ (2018) | Enables live migration but requires careful validation |
| Accept all access mode combinations | Reject unsafe combinations (RWX+filesystem) | 2020+ (production experience) | Prevents data corruption, better UX |
| Generic "already attached" errors | Migration-aware error messages | Recent (KubeVirt adoption) | Operators understand context, faster troubleshooting |
| No attachment tracking | In-memory attachment manager | v0.3 (this project) | RWO enforcement, foundation for RWX |

**Deprecated/outdated:**
- Filesystem-based RWX for block storage: Proven to cause corruption, modern drivers reject
- Unlimited multi-attach: Early RWX implementations allowed any number of attachments; production experience shows controlled limits (2 for migration) prevent misuse
- No access mode validation: Early CSI drivers accepted all modes; now best practice to validate at CreateVolume time

## Open Questions

Things that couldn't be fully resolved:

1. **How should driver handle 3rd attachment attempt during brief network partition?**
   - What we know: CONTEXT.md allows "immediate rejection vs brief wait" as discretion area
   - What's unclear: Optimal wait duration if we implement brief wait (100ms? 1s?)
   - Recommendation: Start with immediate rejection (simpler), add brief wait in Phase 9 if operators report false failures

2. **Should driver emit metrics for RWX usage patterns?**
   - What we know: Observability added in v0.3, metrics framework exists
   - What's unclear: Whether to track "RWX volume creation count" and "dual-attach duration"
   - Recommendation: Yes, add metrics - helps detect misuse and understand migration frequency

3. **How to detect if RWX volume actually used by KubeVirt vs other workload?**
   - What we know: CONTEXT.md suggests "warning logs on RWX usage" as discretion area
   - What's unclear: Can we inspect PVC annotations or labels to identify KubeVirt VMs?
   - Recommendation: Check for `kubevirt.io/vm` label on PVC; log warning if RWX but no VM label

4. **Should ValidateVolumeCapabilities also reject RWX+filesystem for pre-provisioned volumes?**
   - What we know: ValidateVolumeCapabilities used for existing volumes
   - What's unclear: Whether to reject or just return "not confirmed" for RWX+filesystem
   - Recommendation: Return unconfirmed (don't error) but log warning - matches CSI spec semantics

## Sources

### Primary (HIGH confidence)
- [KubeVirt Live Migration User Guide](https://kubevirt.io/user-guide/compute/live_migration/) - RWX requirement for live migration
- [QEMU Migration with Shared Storage](https://wiki.qemu.org/Documentation/Migration_with_shared_storage) - cache=none requirement
- [CSI Spec v1.7.0](https://github.com/container-storage-interface/spec/blob/v1.7.0/spec.md) - MULTI_NODE_MULTI_WRITER definition
- [Kubernetes CSI Raw Block Volume Documentation](https://kubernetes-csi.github.io/docs/raw-block.html) - Block vs filesystem capabilities

### Secondary (MEDIUM confidence)
- [Pure Storage CSI RWX Documentation](https://github.com/purestorage/helm-charts/blob/master/docs/csi-read-write-many.md) - RWX block implementation example
- [AWS EBS CSI Driver Controller](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/pkg/driver/controller.go) - Attachment tracking pattern
- [HPE SCOD CSI Documentation](https://scod.hpedev.io/csi_driver/using.html) - Access mode validation
- [Portworx KubeVirt RWX Block Documentation](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration) - KubeVirt-specific RWX usage

### Tertiary (LOW confidence)
- [NVMe/TCP Data Corruption Issue #844](https://github.com/linux-nvme/nvme-cli/issues/844) - Multi-device corruption report (specific bug, not general RWX issue)
- Various Stack Overflow discussions on RWX limitations - anecdotal, not authoritative

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - CSI spec and Go stdlib well-documented
- Architecture: HIGH - Patterns verified in production CSI drivers (Pure, HPE, AWS EBS)
- Pitfalls: HIGH - Based on real production issues documented in GitHub issues and vendor documentation
- KubeVirt requirements: HIGH - Official KubeVirt documentation explicit about RWX+block requirement
- QEMU I/O coordination: MEDIUM - QEMU docs confirm cache=none requirement but limited detail on dual-attach coordination internals

**Research date:** 2026-02-03
**Valid until:** 2026-03-05 (30 days - stable domain, CSI spec changes slowly)

**Key decisions from CONTEXT.md validated:**
- ROADMAP-4 (RWX block-only): Confirmed by Pure Storage, HPE, and data corruption reports
- ROADMAP-5 (2-node limit): Confirmed by KubeVirt migration model (source + destination only)
- ROADMAP-6 (Trust QEMU): Confirmed by QEMU cache=none documentation and CSI driver patterns (drivers don't coordinate I/O)

**Implementation complexity estimate:** Medium
- Attachment tracking extension: Straightforward (add slice instead of single node)
- Validation logic: Simple (check access mode + volume mode)
- Error messages: Careful wording required for UX
- Testing: Moderate complexity (need to simulate dual-attach scenarios)
