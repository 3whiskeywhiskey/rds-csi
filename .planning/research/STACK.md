# Stack Research: v0.3.0 Volume Fencing

**Domain:** CSI ControllerPublishVolume/ControllerUnpublishVolume with Attachment Tracking
**Researched:** 2026-01-30
**Confidence:** HIGH (using existing codebase patterns + official Kubernetes documentation)

## Executive Summary

Volume fencing implementation requires **zero new dependencies**. The existing stack (client-go v0.28.0, sync primitives, CSI spec v1.10.0) provides everything needed. This document specifies the exact APIs, patterns, and concurrency primitives to use.

## Recommended Stack Additions

### No New Dependencies Required

| Category | Technology | Version | Already In go.mod | Notes |
|----------|------------|---------|-------------------|-------|
| PV Annotation CRUD | k8s.io/client-go | v0.28.0 | **Yes** | PersistentVolumes().Update() |
| Concurrency | sync.RWMutex | stdlib | **Yes** | Match existing codebase patterns |
| CSI Capability | CSI Spec | v1.10.0 | **Yes** | PUBLISH_UNPUBLISH_VOLUME already defined |
| Retry on Conflict | k8s.io/client-go/util/retry | v0.28.0 | **Yes** | Part of client-go |

**Rationale:** The v0.2.0 codebase already uses client-go v0.28.0 for the orphan reconciler (`pkg/reconciler/orphan_reconciler.go` line 168). Volume fencing uses the same client for PV annotation updates.

## Kubernetes client-go Patterns for PV Annotation CRUD

### 1. Get-Modify-Update Pattern

Use the standard Kubernetes pattern: Get resource, modify in memory, Update resource.

```go
import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/util/retry"
)

// SetAttachmentAnnotation sets the node attachment annotation on a PV
func SetAttachmentAnnotation(ctx context.Context, client kubernetes.Interface, pvName, nodeID string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // 1. Get current PV
        pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
        if err != nil {
            return err
        }

        // 2. Modify annotations in memory
        if pv.Annotations == nil {
            pv.Annotations = make(map[string]string)
        }
        pv.Annotations["rds.csi.srvlab.io/attached-node"] = nodeID
        pv.Annotations["rds.csi.srvlab.io/attached-at"] = time.Now().UTC().Format(time.RFC3339)

        // 3. Update PV
        _, err = client.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
        return err
    })
}
```

**Why `retry.RetryOnConflict`:** PV updates can fail with HTTP 409 Conflict if another process (kubelet, controller) modified the PV between Get and Update. The retry wrapper handles this automatically with exponential backoff.

### 2. Annotation Key Format

| Annotation | Value | Purpose |
|------------|-------|---------|
| `rds.csi.srvlab.io/attached-node` | Node ID (e.g., `metal-1`) | Which node owns the attachment |
| `rds.csi.srvlab.io/attached-at` | RFC3339 timestamp | When attachment was created |

**Rationale:** Use driver-specific prefix to avoid conflicts. RFC3339 timestamps are human-readable and parseable.

### 3. Clear Attachment Annotation

```go
func ClearAttachmentAnnotation(ctx context.Context, client kubernetes.Interface, pvName string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
        if err != nil {
            return err
        }

        // Remove annotation (delete from map)
        delete(pv.Annotations, "rds.csi.srvlab.io/attached-node")
        delete(pv.Annotations, "rds.csi.srvlab.io/attached-at")

        _, err = client.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
        return err
    })
}
```

### 4. Get Attachment State

```go
func GetAttachmentState(ctx context.Context, client kubernetes.Interface, pvName string) (nodeID string, attachedAt time.Time, err error) {
    pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
    if err != nil {
        return "", time.Time{}, err
    }

    nodeID = pv.Annotations["rds.csi.srvlab.io/attached-node"]
    if ts := pv.Annotations["rds.csi.srvlab.io/attached-at"]; ts != "" {
        attachedAt, _ = time.Parse(time.RFC3339, ts)
    }
    return nodeID, attachedAt, nil
}
```

### 5. PV Name from Volume ID

CSI ControllerPublishVolume receives `VolumeId` (the CSI volume handle), not the PV name. However, for this driver:

```go
// The volume ID IS the PV name (e.g., "pvc-abc123")
// This is set in CreateVolume: volumeID := req.GetName()
pvName := req.GetVolumeId()
```

**Verification:** See `pkg/driver/controller.go` line 90: `volumeID := req.GetName()` - the external-provisioner passes the PV name as the volume name.

## Concurrency Primitives for In-Memory Attachment State

### Pattern: sync.RWMutex + map (Codebase Standard)

The codebase uses `sync.RWMutex` + `map` consistently (not `sync.Map`). Follow this pattern:

| File | Pattern | Usage |
|------|---------|-------|
| `pkg/nvme/resolver.go:27` | `mu sync.RWMutex` | Device path cache |
| `pkg/nvme/nvme.go:121` | `mu sync.RWMutex` | Connection state |
| `pkg/rds/pool.go:77` | `mu sync.RWMutex` | SSH connection pool |
| `pkg/security/metrics.go:11` | `mu sync.RWMutex` | Metrics tracking |

### AttachmentTracker Implementation

```go
package driver

import (
    "sync"
    "time"
)

// AttachmentInfo tracks volume attachment state
type AttachmentInfo struct {
    NodeID     string
    AttachedAt time.Time
}

// AttachmentTracker provides thread-safe volume attachment tracking
type AttachmentTracker struct {
    mu          sync.RWMutex
    attachments map[string]AttachmentInfo // key: volumeID
}

// NewAttachmentTracker creates a new attachment tracker
func NewAttachmentTracker() *AttachmentTracker {
    return &AttachmentTracker{
        attachments: make(map[string]AttachmentInfo),
    }
}

// Get returns attachment info for a volume (read lock)
func (t *AttachmentTracker) Get(volumeID string) (AttachmentInfo, bool) {
    t.mu.RLock()
    defer t.mu.RUnlock()
    info, ok := t.attachments[volumeID]
    return info, ok
}

// Set records an attachment (write lock)
func (t *AttachmentTracker) Set(volumeID, nodeID string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.attachments[volumeID] = AttachmentInfo{
        NodeID:     nodeID,
        AttachedAt: time.Now(),
    }
}

// Delete removes an attachment (write lock)
func (t *AttachmentTracker) Delete(volumeID string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    delete(t.attachments, volumeID)
}

// IsAttachedToOtherNode checks if volume is attached to a different node
func (t *AttachmentTracker) IsAttachedToOtherNode(volumeID, nodeID string) bool {
    t.mu.RLock()
    defer t.mu.RUnlock()
    if info, ok := t.attachments[volumeID]; ok {
        return info.NodeID != nodeID
    }
    return false
}
```

**Why `sync.RWMutex` over `sync.Map`:**
- Codebase consistency (all 7 existing usages use RWMutex)
- Better for mixed read/write workloads (CSI has both)
- Explicit locking makes code easier to understand and debug
- `sync.Map` optimized for append-only or disjoint key access patterns, not updates

## ControllerPublishVolume/ControllerUnpublishVolume Implementation

### CSI Capability Declaration

Add to `pkg/driver/driver.go` in `addControllerServiceCapabilities()`:

```go
{
    Type: &csi.ControllerServiceCapability_Rpc{
        Rpc: &csi.ControllerServiceCapability_RPC{
            Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
        },
    },
},
```

### ControllerPublishVolume Flow

```go
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    // 1. Check in-memory state first (fast path)
    if cs.attachments.IsAttachedToOtherNode(volumeID, nodeID) {
        existing, _ := cs.attachments.Get(volumeID)
        return nil, status.Errorf(codes.FailedPrecondition,
            "volume %s is already attached to node %s", volumeID, existing.NodeID)
    }

    // 2. Check PV annotation (authoritative state)
    pvNodeID, _, err := GetAttachmentState(ctx, cs.k8sClient, volumeID)
    if err != nil && !errors.IsNotFound(err) {
        return nil, status.Errorf(codes.Internal, "failed to get PV: %v", err)
    }
    if pvNodeID != "" && pvNodeID != nodeID {
        return nil, status.Errorf(codes.FailedPrecondition,
            "volume %s is already attached to node %s (from PV annotation)", volumeID, pvNodeID)
    }

    // 3. Record attachment in both places
    if err := SetAttachmentAnnotation(ctx, cs.k8sClient, volumeID, nodeID); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to set attachment annotation: %v", err)
    }
    cs.attachments.Set(volumeID, nodeID)

    // 4. Return success (no publish_context needed for NVMe/TCP)
    return &csi.ControllerPublishVolumeResponse{}, nil
}
```

### ControllerUnpublishVolume Flow

```go
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    // 1. Verify caller is the attached node (optional safety check)
    if info, ok := cs.attachments.Get(volumeID); ok {
        if info.NodeID != nodeID {
            klog.Warningf("Unpublish request from %s but volume attached to %s", nodeID, info.NodeID)
            // Still proceed - CO knows best
        }
    }

    // 2. Clear attachment from PV annotation
    if err := ClearAttachmentAnnotation(ctx, cs.k8sClient, volumeID); err != nil {
        if !errors.IsNotFound(err) {
            return nil, status.Errorf(codes.Internal, "failed to clear attachment annotation: %v", err)
        }
        // PV not found is OK (volume might be deleted)
    }

    // 3. Clear in-memory state
    cs.attachments.Delete(volumeID)

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}
```

## Startup Synchronization

On controller startup, sync in-memory state from PV annotations:

```go
func (cs *ControllerServer) syncAttachmentState(ctx context.Context) error {
    pvList, err := cs.k8sClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return fmt.Errorf("failed to list PVs: %w", err)
    }

    for _, pv := range pvList.Items {
        if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != DriverName {
            continue
        }
        if nodeID := pv.Annotations["rds.csi.srvlab.io/attached-node"]; nodeID != "" {
            cs.attachments.Set(pv.Spec.CSI.VolumeHandle, nodeID)
            klog.V(2).Infof("Restored attachment: %s -> %s", pv.Spec.CSI.VolumeHandle, nodeID)
        }
    }
    return nil
}
```

## What NOT to Add

| Rejected Option | Why Not |
|-----------------|---------|
| ConfigMap for state persistence | PV annotations are more appropriate; ConfigMaps have size limits and would require separate cleanup |
| External database (etcd, Redis) | Overkill for homelab; adds operational complexity; PV annotations are already highly available |
| sync.Map for attachments | Codebase uses sync.RWMutex consistently; sync.Map optimized for different access patterns |
| Custom CRD for attachments | Unnecessary complexity; PV annotations are standard CSI practice |
| Informer/Watch for PV changes | Not needed; controller handles all attachment changes through direct CSI calls |
| Leader election | Out of scope for v0.3.0; single controller replica is acceptable for homelab |
| RDS-side ACLs | PROJECT.md explicitly decided against: "Standard CSI approach (not RDS-side ACLs)" |

## Version Compatibility

| Package | Current Version | Required Features | Notes |
|---------|-----------------|-------------------|-------|
| k8s.io/client-go | v0.28.0 | PersistentVolumes().Update(), retry.RetryOnConflict | Already satisfied |
| k8s.io/api | v0.28.0 | corev1.PersistentVolume with Annotations | Already satisfied |
| CSI Spec | v1.10.0 | PUBLISH_UNPUBLISH_VOLUME capability | Already satisfied |
| Go | 1.24 | sync.RWMutex, context.Context | Already satisfied |

**No version upgrades required.**

## Testing Considerations

### Unit Testing

```go
// Use fake clientset for unit tests
import "k8s.io/client-go/kubernetes/fake"

func TestSetAttachmentAnnotation(t *testing.T) {
    client := fake.NewSimpleClientset(&corev1.PersistentVolume{
        ObjectMeta: metav1.ObjectMeta{Name: "pvc-test"},
    })

    err := SetAttachmentAnnotation(context.Background(), client, "pvc-test", "node-1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    pv, _ := client.CoreV1().PersistentVolumes().Get(context.Background(), "pvc-test", metav1.GetOptions{})
    if pv.Annotations["rds.csi.srvlab.io/attached-node"] != "node-1" {
        t.Errorf("expected annotation node-1, got %s", pv.Annotations["rds.csi.srvlab.io/attached-node"])
    }
}
```

### Concurrency Testing

```go
func TestAttachmentTrackerConcurrency(t *testing.T) {
    tracker := NewAttachmentTracker()
    var wg sync.WaitGroup

    // Concurrent writes
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            tracker.Set(fmt.Sprintf("vol-%d", n), fmt.Sprintf("node-%d", n%5))
        }(i)
    }

    // Concurrent reads while writes happening
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            tracker.Get(fmt.Sprintf("vol-%d", n))
        }(i)
    }

    wg.Wait()
}
```

## Sources

**Kubernetes Documentation (HIGH confidence):**
- [Persistent Volumes | Kubernetes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [client-go PersistentVolume interface](https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/persistentvolume.go)

**CSI Specification (HIGH confidence):**
- [CSI Spec - ControllerPublishVolume](https://github.com/container-storage-interface/spec/blob/master/spec.md)
- [Developing a CSI Driver | kubernetes-csi](https://kubernetes-csi.github.io/docs/developing.html)

**Go Concurrency (HIGH confidence):**
- [sync package | Go Packages](https://pkg.go.dev/sync)
- [Go sync.Map: The Right Tool for the Right Job | VictoriaMetrics](https://victoriametrics.com/blog/go-sync-map/)

**Codebase Patterns (HIGH confidence):**
- `pkg/reconciler/orphan_reconciler.go` - PV listing with client-go
- `pkg/nvme/resolver.go` - sync.RWMutex + map pattern
- `pkg/driver/events.go` - K8s API patterns with context

---
*Stack research for: v0.3.0 Volume Fencing Milestone*
*Researched: 2026-01-30*
