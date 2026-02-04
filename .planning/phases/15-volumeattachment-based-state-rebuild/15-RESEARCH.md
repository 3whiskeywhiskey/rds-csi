# Phase 15: VolumeAttachment-Based State Rebuild - Research

**Researched:** 2026-02-04
**Domain:** Kubernetes CSI VolumeAttachment API, controller state rebuild patterns
**Confidence:** HIGH

## Summary

This research investigates how to replace the current PV annotation-based state rebuild with VolumeAttachment-based rebuild. The current implementation reads `rds.csi.srvlab.io/attached-node` and `rds.csi.srvlab.io/attached-at` annotations from PersistentVolumes during controller startup to reconstruct in-memory attachment state. This approach has a fundamental flaw: PV annotations can become stale if clearing fails, or if manual kubectl operations modify them, leading to false-positive attachment conflicts after controller restart.

VolumeAttachment objects are the authoritative source for attachment state because they are managed by the external-attacher sidecar and represent the actual Kubernetes-level attachment intent. When a VolumeAttachment exists and has `status.attached=true`, the volume is definitively attached to that node from Kubernetes' perspective. When no VolumeAttachment exists for a volume, it is definitively not attached.

**Primary recommendation:** Replace `RebuildState()` to list VolumeAttachment objects filtered by `spec.attacher=rds.csi.srvlab.io` and rebuild in-memory state from them. Keep PV annotations as informational-only (write-through, never read on rebuild).

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| k8s.io/client-go/informers/storage/v1 | v0.35.0 | VolumeAttachment informer/lister | Official client-go, shared informer pattern |
| k8s.io/client-go/listers/storage/v1 | v0.35.0 | VolumeAttachmentLister interface | Type-safe caching with List/Get |
| k8s.io/apimachinery/pkg/labels | v0.35.0 | Label selector for List operations | Standard Kubernetes label filtering |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| k8s.io/client-go/tools/cache | v0.35.0 | SharedIndexInformer, ResourceEventHandler | Event-driven updates (optional, for watcher) |
| k8s.io/apimachinery/pkg/api/errors | v0.35.0 | Error type checking (IsNotFound, etc.) | API error handling |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Informer-based lister | Direct API calls | Informer caches locally, reduces API load; direct calls always fresh but more expensive |
| Full informer + watcher | Periodic list rebuild | Watcher gives real-time updates but adds complexity; periodic rebuild is simpler |

**Installation:**
Already available in go.mod:
```go
require (
    k8s.io/client-go v0.35.0
    k8s.io/apimachinery v0.35.0
)
```

## Architecture Patterns

### Recommended Project Structure
```
pkg/attachment/
├── manager.go           # AttachmentManager (existing)
├── types.go             # AttachmentState types (existing)
├── persist.go           # PV annotation write-through (existing, keep for debug)
├── rebuild.go           # RebuildState() - MODIFY to use VolumeAttachment
└── va_lister.go         # NEW: VolumeAttachment listing helpers
```

### Pattern 1: VolumeAttachment-Based Rebuild
**What:** On controller startup, list all VolumeAttachment objects for our driver and rebuild in-memory state
**When to use:** Controller initialization (replaces current PV annotation-based rebuild)
**Example:**
```go
// Source: Kubernetes VolumeAttachment API specification
// https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume-attachment-v1/

func (am *AttachmentManager) RebuildStateFromVolumeAttachments(ctx context.Context) error {
    if am.k8sClient == nil {
        klog.V(2).Info("Skipping state rebuild (no k8s client)")
        return nil
    }

    klog.Info("Rebuilding attachment state from VolumeAttachment objects")

    // List all VolumeAttachments (non-namespaced resource)
    vaList, err := am.k8sClient.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
    if err != nil {
        return fmt.Errorf("failed to list VolumeAttachments: %w", err)
    }

    am.mu.Lock()
    defer am.mu.Unlock()

    // Clear existing state
    am.attachments = make(map[string]*AttachmentState)

    rebuiltCount := 0
    for _, va := range vaList.Items {
        // Filter: only process VolumeAttachments for our driver
        if va.Spec.Attacher != driverName {
            continue
        }

        // Filter: only process attached volumes
        if !va.Status.Attached {
            continue
        }

        // Extract volume ID from source
        volumeID := ""
        if va.Spec.Source.PersistentVolumeName != nil {
            volumeID = *va.Spec.Source.PersistentVolumeName
        }
        if volumeID == "" {
            klog.Warningf("VolumeAttachment %s has no PersistentVolumeName, skipping", va.Name)
            continue
        }

        nodeID := va.Spec.NodeName

        // Check if we already have an attachment for this volume (RWX case)
        if existing, exists := am.attachments[volumeID]; exists {
            // Dual-attach scenario (RWX migration) - add second node
            existing.Nodes = append(existing.Nodes, NodeAttachment{
                NodeID:     nodeID,
                AttachedAt: va.CreationTimestamp.Time,
            })
            klog.V(2).Infof("Rebuilt secondary attachment: volume=%s, node=%s (multi-attach)", volumeID, nodeID)
        } else {
            // New primary attachment
            state := &AttachmentState{
                VolumeID:   volumeID,
                NodeID:     nodeID,
                AttachedAt: va.CreationTimestamp.Time,
                Nodes: []NodeAttachment{
                    {NodeID: nodeID, AttachedAt: va.CreationTimestamp.Time},
                },
            }
            am.attachments[volumeID] = state
            klog.V(2).Infof("Rebuilt attachment: volume=%s, node=%s", volumeID, nodeID)
        }
        rebuiltCount++
    }

    klog.Infof("State rebuild complete: %d attachments recovered from VolumeAttachment objects", rebuiltCount)
    return nil
}
```

### Pattern 2: Grace Period Detection from Multiple VolumeAttachments
**What:** Detect KubeVirt migration grace period by counting VolumeAttachments per volume
**When to use:** During rebuild to restore migration state
**Example:**
```go
// Source: KubeVirt live migration pattern
// Multiple VolumeAttachments for same volume indicate active migration

// Build volume -> VolumeAttachments map
vaByVolume := make(map[string][]*storagev1.VolumeAttachment)
for i := range vaList.Items {
    va := &vaList.Items[i]
    if va.Spec.Attacher != driverName || !va.Status.Attached {
        continue
    }
    volumeID := *va.Spec.Source.PersistentVolumeName
    vaByVolume[volumeID] = append(vaByVolume[volumeID], va)
}

// Rebuild with migration detection
for volumeID, attachments := range vaByVolume {
    if len(attachments) == 1 {
        // Single attachment - normal case
        rebuildSingleAttachment(volumeID, attachments[0])
    } else if len(attachments) == 2 {
        // Dual attachment - migration in progress
        rebuildMigrationAttachment(volumeID, attachments)
    } else if len(attachments) > 2 {
        // Anomaly - more than 2 attachments
        klog.Warningf("Volume %s has %d VolumeAttachments (expected max 2), rebuilding first 2",
            volumeID, len(attachments))
        rebuildMigrationAttachment(volumeID, attachments[:2])
    }
}
```

### Pattern 3: Write-Through Annotations (Informational Only)
**What:** Keep writing PV annotations for debugging/observability but never read them
**When to use:** During ControllerPublishVolume (write), never during rebuild (no read)
**Example:**
```go
// Source: Current implementation pattern (persist.go)
// KEEP: Write-through for debugging
func (am *AttachmentManager) persistAttachmentAnnotation(ctx context.Context, volumeID, nodeID string) error {
    // ... existing implementation ...
    // This is informational only - annotations are NEVER read during rebuild
}

// REMOVE from RebuildState:
// nodeID, hasNode := pv.Annotations[AnnotationAttachedNode]  // DELETE THIS LINE
```

### Anti-Patterns to Avoid
- **Reading PV annotations during rebuild:** Annotations can become stale; VolumeAttachment is authoritative
- **Ignoring VolumeAttachment status.attached:** Must check `status.attached=true` before trusting attachment
- **Assuming single attachment:** RWX volumes can have multiple VolumeAttachments during migration
- **Blocking startup on API errors:** Log warning and continue; reconciler will retry later

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| List filtering | Manual iteration | FieldSelector/LabelSelector | API-level filtering is more efficient |
| Watch events | Custom loop | SharedIndexInformer | client-go handles reconnection, resyncs |
| Error handling | Simple retry | k8s.io/client-go/util/retry | Exponential backoff, conflict handling |
| Type assertions | Direct cast | runtime.Object conversion | Handles unknown object types safely |

**Key insight:** The external-attacher already solves the hard problem of reconciling VolumeAttachment state with actual driver state. Our job is simpler: just read what external-attacher has already determined is the authoritative state.

## Common Pitfalls

### Pitfall 1: Ignoring Detached VolumeAttachments
**What goes wrong:** VolumeAttachment exists but `status.attached=false` - volume is being detached
**Why it happens:** External-attacher creates VA first, then calls ControllerPublishVolume; during error/detach, VA exists but isn't attached
**How to avoid:** Always check `va.Status.Attached == true` before considering it an active attachment
**Warning signs:** False positives where volume appears attached but ControllerPublishVolume was never successful

### Pitfall 2: Field Selector Limitations
**What goes wrong:** Trying to filter VolumeAttachments by `spec.source.persistentVolumeName` in List call
**Why it happens:** Not all VolumeAttachment fields are valid fieldSelector fields
**How to avoid:** Filter in client code after listing, or use `spec.nodeName` which IS a valid fieldSelector
**Warning signs:** API errors when using unsupported field selectors
```go
// WORKS: spec.nodeName is a valid fieldSelector
vaList, _ := client.List(ctx, metav1.ListOptions{
    FieldSelector: "spec.nodeName=worker-1",
})

// DOES NOT WORK: persistentVolumeName is NOT a valid fieldSelector
// Must filter in code after List
```

### Pitfall 3: Race Condition During Controller Restart
**What goes wrong:** VolumeAttachment created/deleted between List and ControllerPublishVolume call
**Why it happens:** No atomic "list and lock" operation; state can change
**How to avoid:** Treat rebuilt state as best-effort; idempotent handlers handle duplicates
**Warning signs:** Occasional "volume already attached" errors after restart that self-heal

### Pitfall 4: Migration Timeout State Not Preserved
**What goes wrong:** After rebuild, volumes in migration don't have MigrationStartedAt timestamp
**Why it happens:** VolumeAttachment doesn't store our custom timeout tracking
**How to avoid:** Use VolumeAttachment CreationTimestamp as proxy; or accept timeout resets on restart
**Warning signs:** Migration timeout not enforced after controller restart

### Pitfall 5: Backward Compatibility with Stale Annotations
**What goes wrong:** Old volumes have stale annotations that contradict VolumeAttachment state
**Why it happens:** Bug (62197ce) left annotations behind; manual kubectl edits
**How to avoid:** VolumeAttachment is source of truth; ignore annotations entirely on rebuild
**Warning signs:** Logged warnings about annotation/VA mismatch (informational only)

## Code Examples

Verified patterns from official sources:

### List VolumeAttachments for Driver
```go
// Source: https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume-attachment-v1/
func (am *AttachmentManager) listDriverVolumeAttachments(ctx context.Context) ([]*storagev1.VolumeAttachment, error) {
    vaList, err := am.k8sClient.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, err
    }

    var filtered []*storagev1.VolumeAttachment
    for i := range vaList.Items {
        va := &vaList.Items[i]
        if va.Spec.Attacher == driverName {
            filtered = append(filtered, va)
        }
    }
    return filtered, nil
}
```

### Check if Volume Attached to Node via VolumeAttachment
```go
// Source: VolumeAttachment spec structure
func (am *AttachmentManager) isVolumeAttachedToNode(ctx context.Context, volumeID, nodeID string) (bool, error) {
    vaList, err := am.k8sClient.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{
        FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeID),
    })
    if err != nil {
        return false, err
    }

    for _, va := range vaList.Items {
        if va.Spec.Attacher != driverName {
            continue
        }
        if va.Spec.Source.PersistentVolumeName == nil {
            continue
        }
        if *va.Spec.Source.PersistentVolumeName == volumeID && va.Status.Attached {
            return true, nil
        }
    }
    return false, nil
}
```

### VolumeAttachment Watcher (Optional Enhancement)
```go
// Source: https://pkg.go.dev/k8s.io/client-go/informers/storage/v1
// For real-time updates instead of periodic rebuild

import (
    storageinformers "k8s.io/client-go/informers/storage/v1"
    "k8s.io/client-go/tools/cache"
)

func (am *AttachmentManager) startVolumeAttachmentWatcher(ctx context.Context) error {
    factory := informers.NewSharedInformerFactory(am.k8sClient, 10*time.Minute)
    vaInformer := factory.Storage().V1().VolumeAttachments()

    vaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            va := obj.(*storagev1.VolumeAttachment)
            if va.Spec.Attacher == driverName && va.Status.Attached {
                am.handleVolumeAttachmentAdd(va)
            }
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            newVA := newObj.(*storagev1.VolumeAttachment)
            if newVA.Spec.Attacher == driverName {
                am.handleVolumeAttachmentUpdate(newVA)
            }
        },
        DeleteFunc: func(obj interface{}) {
            va := obj.(*storagev1.VolumeAttachment)
            if va.Spec.Attacher == driverName {
                am.handleVolumeAttachmentDelete(va)
            }
        },
    })

    factory.Start(ctx.Done())
    factory.WaitForCacheSync(ctx.Done())
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PV annotation-based rebuild | VolumeAttachment-based rebuild | Phase 15 (planned) | Eliminates stale state risk |
| Read annotations on startup | Read VolumeAttachments on startup | Phase 15 (planned) | Authoritative source of truth |
| Annotations as primary state | Annotations as informational | Phase 15 (planned) | No behavior change from stale annotations |

**Deprecated/outdated:**
- **PV annotation reading during rebuild:** Keep writing for debugging, but never read during rebuild
- **AnnotationAttachedNode as source of truth:** VolumeAttachment replaces it as authoritative

## RBAC Requirements

The controller already has the necessary RBAC permissions (verified in `deploy/kubernetes/rbac.yaml`):

```yaml
# Already granted to rds-csi-controller-role:
- apiGroups: ["storage.k8s.io"]
  resources: ["volumeattachments"]
  verbs: ["get", "list", "watch", "patch"]

- apiGroups: ["storage.k8s.io"]
  resources: ["volumeattachments/status"]
  verbs: ["patch"]
```

No RBAC changes needed for Phase 15.

## Open Questions

Things that couldn't be fully resolved:

1. **Migration timeout preservation across restarts**
   - What we know: VolumeAttachment doesn't store custom timeout fields; MigrationStartedAt lost on restart
   - What's unclear: Acceptable to use VA CreationTimestamp as proxy? Or accept timeout reset?
   - Recommendation: Use older VA's CreationTimestamp as MigrationStartedAt proxy; document this behavior

2. **Informer vs Direct API calls**
   - What we know: Informers cache and reduce API load; direct calls always fresh
   - What's unclear: Is real-time VA watching needed, or is startup rebuild sufficient?
   - Recommendation: Start with direct List on startup; add informer-based watcher if needed (Phase 15+)

3. **AccessMode preservation**
   - What we know: VolumeAttachment doesn't store RWO vs RWX; must infer from PV
   - What's unclear: Need to look up PV to get access mode during rebuild?
   - Recommendation: Yes, look up PV.Spec.AccessModes to determine if RWX (allows dual-attach)

## Sources

### Primary (HIGH confidence)
- [Kubernetes VolumeAttachment API Reference](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume-attachment-v1/) - API spec, field definitions
- [client-go storage/v1 informers](https://pkg.go.dev/k8s.io/client-go/informers/storage/v1) - VolumeAttachmentInformer interface
- [kubernetes-csi/external-attacher](https://github.com/kubernetes-csi/external-attacher) - Authoritative VA management
- [external-attacher development.md](https://github.com/kubernetes-csi/external-attacher/blob/master/doc/development.md) - Architecture and design patterns

### Secondary (MEDIUM confidence)
- [kubernetes-csi/external-attacher PR #184](https://github.com/kubernetes-csi/external-attacher/pull/184) - ListVolumes reconciliation pattern
- [How Kubernetes VolumeAttachments Named](https://sklar.rocks/k8s-volumeattachment-names/) - VA naming conventions

### Tertiary (LOW confidence)
- WebSearch results on CSI driver rebuild patterns - general guidance, not driver-specific

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Using official client-go libraries, well-documented API
- Architecture: HIGH - Pattern derived from external-attacher design, verified with Kubernetes docs
- Pitfalls: MEDIUM - Based on common CSI driver issues and project-specific context

**Research date:** 2026-02-04
**Valid until:** 90 days (VolumeAttachment API is stable v1, unlikely to change)
