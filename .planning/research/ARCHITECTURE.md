# Architecture Research: Volume Fencing via ControllerPublish/Unpublish

**Domain:** CSI Controller Service - Volume Attachment Tracking
**Researched:** 2026-01-30
**Confidence:** HIGH

## Problem Context

The RDS CSI driver currently does not implement ControllerPublishVolume/ControllerUnpublishVolume. Without these methods:

- **No volume fencing**: A ReadWriteOnce volume can be attached to multiple nodes simultaneously during pod migration or failover
- **No attachment tracking**: Controller has no visibility into which nodes have volumes attached
- **Data corruption risk**: Two nodes writing to the same NVMe/TCP target concurrently

Current controller returns `codes.Unimplemented` for both methods (lines 336-343 in controller.go).

## Integration with Existing Architecture

### Current Controller Structure

```
pkg/driver/
├── driver.go          # Driver initialization, capability declaration
├── controller.go      # ControllerServer implementation
├── node.go            # NodeServer implementation
├── server.go          # gRPC server setup
├── events.go          # Kubernetes event posting
└── params.go          # StorageClass parameter parsing

pkg/rds/
├── client.go          # RDS SSH client interface
├── pool.go            # Connection pool with circuit breaker
├── commands.go        # RouterOS command execution
└── types.go           # VolumeInfo, CapacityInfo structs

pkg/reconciler/
└── orphan_reconciler.go  # Orphaned volume cleanup
```

### Integration Points for AttachmentManager

The new AttachmentManager component integrates at these points:

```
┌──────────────────────────────────────────────────────────────────────┐
│ ControllerServer (controller.go)                                      │
│                                                                        │
│  ┌────────────────────┐  ┌────────────────────┐                      │
│  │ CreateVolume       │  │ DeleteVolume       │                      │
│  │ - Uses rdsClient   │  │ - Uses rdsClient   │                      │
│  │ - No attachment    │  │ - No attachment    │                      │
│  │   awareness        │  │   awareness        │                      │
│  └────────────────────┘  └────────────────────┘                      │
│                                                                        │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ NEW: ControllerPublishVolume / ControllerUnpublishVolume       │  │
│  │                                                                  │  │
│  │  ┌──────────────────────────────────────────────────────────┐  │  │
│  │  │ AttachmentManager (NEW COMPONENT)                        │  │  │
│  │  │                                                           │  │  │
│  │  │  - In-memory state: map[volumeID]nodeID                  │  │  │
│  │  │  - PV annotation persistence: csi.rds.srvlab.io/node-id  │  │  │
│  │  │  - ReadWriteOnce enforcement                              │  │  │
│  │  │  - Thread-safe operations (sync.RWMutex)                 │  │  │
│  │  └──────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  Existing components:                                                  │
│  ┌────────────────────┐  ┌────────────────────┐                      │
│  │ rdsClient          │  │ k8sClient          │                      │
│  │ - SSH pool         │  │ - PV read/update   │                      │
│  │ - Volume commands  │  │ - Event posting    │                      │
│  └────────────────────┘  └────────────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘
```

## Recommended Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│ CSI Controller Plugin (Deployment)                                    │
│                                                                        │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ ControllerServer                                                │  │
│  │                                                                  │  │
│  │  CreateVolume()   DeleteVolume()   ControllerExpandVolume()    │  │
│  │       │                │                     │                  │  │
│  │       └────────────────┴─────────────────────┘                  │  │
│  │                        │                                         │  │
│  │                   rdsClient (existing)                          │  │
│  │                                                                  │  │
│  │  ControllerPublishVolume()          ControllerUnpublishVolume() │  │
│  │       │                                      │                  │  │
│  │       └──────────────┬───────────────────────┘                  │  │
│  │                      │                                           │  │
│  │  ┌───────────────────▼───────────────────────────────────────┐  │  │
│  │  │ AttachmentManager (NEW)                                    │  │  │
│  │  │                                                             │  │  │
│  │  │  ┌─────────────────────┐  ┌─────────────────────────────┐ │  │  │
│  │  │  │ In-Memory State     │  │ PV Annotation Persistence   │ │  │  │
│  │  │  │                     │  │                              │ │  │  │
│  │  │  │ map[volumeID]Attach │  │ k8sClient.CoreV1().PVs()    │ │  │  │
│  │  │  │  - nodeID           │  │  - Get/Update annotations   │ │  │  │
│  │  │  │  - attachTime       │  │  - csi.rds.srvlab.io/node  │ │  │  │
│  │  │  │  - readonly         │  │  - csi.rds.srvlab.io/time  │ │  │  │
│  │  │  └─────────────────────┘  └─────────────────────────────┘ │  │  │
│  │  │                                                             │  │  │
│  │  │  Methods:                                                   │  │  │
│  │  │  - Attach(volumeID, nodeID, readonly) error                │  │  │
│  │  │  - Detach(volumeID, nodeID) error                          │  │  │
│  │  │  - GetAttachment(volumeID) (*Attachment, bool)             │  │  │
│  │  │  - LoadFromAnnotations(ctx) error  (startup recovery)      │  │  │
│  │  └───────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Location | New/Modified |
|-----------|----------------|----------|--------------|
| **AttachmentManager** | Track volume-to-node attachments; enforce RWO; persist to PV annotations | `pkg/attachment/manager.go` | NEW |
| **ControllerPublishVolume** | Validate RWO constraints; record attachment; return publish context | `pkg/driver/controller.go` | NEW (replace stub) |
| **ControllerUnpublishVolume** | Remove attachment record; update PV annotation | `pkg/driver/controller.go` | NEW (replace stub) |
| **Driver.addControllerServiceCapabilities** | Declare PUBLISH_UNPUBLISH_VOLUME capability | `pkg/driver/driver.go` | MODIFIED |
| **ControllerServer** | Add attachmentManager field | `pkg/driver/controller.go` | MODIFIED |

### New Package: pkg/attachment

```
pkg/attachment/
├── manager.go        # AttachmentManager implementation
├── manager_test.go   # Unit tests
└── types.go          # Attachment struct, AttachmentState enum
```

## Data Flow

### ControllerPublishVolume Flow

```
external-attacher sidecar
         │
         │ ControllerPublishVolume(volumeID, nodeID, readonly, volumeCapability)
         ▼
┌────────────────────────────────────────────────────────────────────┐
│ ControllerServer.ControllerPublishVolume()                          │
│                                                                      │
│  1. Validate request (volumeID, nodeID required)                    │
│         │                                                            │
│  2. Validate volume exists on RDS                                   │
│         │  rdsClient.GetVolume(volumeID)                            │
│         │                                                            │
│  3. Check attachment state                                          │
│         │  attachmentManager.GetAttachment(volumeID)                │
│         │                                                            │
│         ├─► If attached to SAME node: return success (idempotent)   │
│         │                                                            │
│         ├─► If attached to DIFFERENT node AND RWO:                  │
│         │      return FailedPrecondition error                      │
│         │      "volume already attached to node X"                  │
│         │                                                            │
│         └─► If not attached: proceed                                │
│                                                                      │
│  4. Record attachment                                               │
│         │  attachmentManager.Attach(volumeID, nodeID, readonly)     │
│         │    ├─► Update in-memory map                               │
│         │    └─► Update PV annotation (async, best-effort)          │
│         │                                                            │
│  5. Return PublishContext                                           │
│         │  map[string]string{                                       │
│         │    "nvmeAddress": volume.NVMEAddress,                     │
│         │    "nvmePort":    volume.NVMETCPPort,                     │
│         │    "nqn":         volume.NVMETCPNQN,                      │
│         │  }                                                         │
│         ▼                                                            │
│  Return ControllerPublishVolumeResponse                             │
└────────────────────────────────────────────────────────────────────┘
```

### ControllerUnpublishVolume Flow

```
external-attacher sidecar
         │
         │ ControllerUnpublishVolume(volumeID, nodeID)
         ▼
┌────────────────────────────────────────────────────────────────────┐
│ ControllerServer.ControllerUnpublishVolume()                        │
│                                                                      │
│  1. Validate request (volumeID required; nodeID optional)           │
│         │                                                            │
│  2. Get current attachment                                          │
│         │  attachmentManager.GetAttachment(volumeID)                │
│         │                                                            │
│         ├─► If not attached: return success (idempotent)            │
│         │                                                            │
│         ├─► If attached to DIFFERENT node (and nodeID specified):   │
│         │      Log warning, return success (defensive)              │
│         │                                                            │
│         └─► If attached to specified node: proceed                  │
│                                                                      │
│  3. Remove attachment record                                        │
│         │  attachmentManager.Detach(volumeID, nodeID)               │
│         │    ├─► Remove from in-memory map                          │
│         │    └─► Remove PV annotation (async, best-effort)          │
│         │                                                            │
│  4. Return success                                                  │
│         ▼                                                            │
│  Return ControllerUnpublishVolumeResponse{}                         │
└────────────────────────────────────────────────────────────────────┘
```

### Controller Restart Recovery Flow

```
Controller Pod Starts
         │
         ▼
┌────────────────────────────────────────────────────────────────────┐
│ Driver.Run()                                                        │
│                                                                      │
│  1. Initialize AttachmentManager                                    │
│         │  attachmentManager = NewAttachmentManager(k8sClient)      │
│         │                                                            │
│  2. Load state from PV annotations                                  │
│         │  attachmentManager.LoadFromAnnotations(ctx)               │
│         │                                                            │
│         │  For each PV with CSI driver = "rds.csi.srvlab.io":       │
│         │    ├─► Read annotation: csi.rds.srvlab.io/attached-node   │
│         │    ├─► Read annotation: csi.rds.srvlab.io/attach-time     │
│         │    └─► Populate in-memory map if annotations present      │
│         │                                                            │
│  3. Start gRPC server                                               │
│         │  (AttachmentManager now has recovered state)              │
│         ▼                                                            │
│  Server ready to handle ControllerPublish/Unpublish requests        │
└────────────────────────────────────────────────────────────────────┘
```

## AttachmentManager Design

### Data Structures

```go
// pkg/attachment/types.go

// Attachment represents a volume's attachment state
type Attachment struct {
    VolumeID   string    // CSI volume ID (e.g., pvc-uuid)
    NodeID     string    // Kubernetes node name
    Readonly   bool      // Whether attached as read-only
    AttachTime time.Time // When attachment was recorded
}

// AttachmentManager tracks volume attachments
type AttachmentManager struct {
    attachments map[string]*Attachment // volumeID -> Attachment
    mu          sync.RWMutex
    k8sClient   kubernetes.Interface
    driverName  string // For PV annotation filtering
}
```

### PV Annotation Schema

```yaml
# Annotations added to PersistentVolume objects
metadata:
  annotations:
    # Node where volume is currently attached
    csi.rds.srvlab.io/attached-node: "worker-1"

    # Timestamp of attachment (RFC3339)
    csi.rds.srvlab.io/attach-time: "2026-01-30T12:34:56Z"

    # Whether attached read-only
    csi.rds.srvlab.io/readonly: "false"
```

### Thread Safety

```go
// All public methods use RWMutex for thread safety

func (m *AttachmentManager) Attach(volumeID, nodeID string, readonly bool) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Check existing attachment
    if existing, ok := m.attachments[volumeID]; ok {
        if existing.NodeID != nodeID {
            return fmt.Errorf("volume %s already attached to node %s",
                volumeID, existing.NodeID)
        }
        // Idempotent: already attached to same node
        return nil
    }

    // Record new attachment
    m.attachments[volumeID] = &Attachment{
        VolumeID:   volumeID,
        NodeID:     nodeID,
        Readonly:   readonly,
        AttachTime: time.Now(),
    }

    // Persist to PV annotation (async, best-effort)
    go m.persistAttachment(volumeID, nodeID, readonly)

    return nil
}

func (m *AttachmentManager) GetAttachment(volumeID string) (*Attachment, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    attachment, ok := m.attachments[volumeID]
    if !ok {
        return nil, false
    }
    // Return copy to prevent mutation
    copy := *attachment
    return &copy, true
}
```

## Architectural Patterns

### Pattern 1: In-Memory State with Annotation Persistence

**What:** Primary state in memory; PV annotations as durable backup for controller restarts

**Trade-offs:**
- **Pro:** Fast attachment lookups (no API calls in hot path)
- **Pro:** State survives controller restarts via annotation reload
- **Pro:** No external database dependency
- **Con:** Brief window where state could diverge if annotation update fails
- **Con:** Must handle annotation update failures gracefully

**Mitigation for annotation failures:**
- Log warning but don't fail the Attach operation
- Retry annotation update on next operation
- LoadFromAnnotations on startup reconciles state

**CONFIDENCE:** HIGH - This pattern is used by several production CSI drivers (AWS EBS, GCE PD).

### Pattern 2: ReadWriteOnce Enforcement at Controller Level

**What:** Reject ControllerPublishVolume if volume is already attached to a different node

**When to use:** For volumes with `VolumeCapability_AccessMode_SINGLE_NODE_WRITER`

**Implementation:**

```go
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context,
    req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId()

    // Check existing attachment
    if existing, ok := cs.attachmentManager.GetAttachment(volumeID); ok {
        if existing.NodeID == nodeID {
            // Idempotent: already attached to this node
            klog.V(4).Infof("Volume %s already attached to node %s", volumeID, nodeID)
            return &csi.ControllerPublishVolumeResponse{
                PublishContext: cs.buildPublishContext(volumeID),
            }, nil
        }

        // Check if access mode allows multi-node
        accessMode := req.GetVolumeCapability().GetAccessMode().GetMode()
        if accessMode == csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER ||
           accessMode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
            return nil, status.Errorf(codes.FailedPrecondition,
                "volume %s is already attached to node %s, cannot attach to node %s",
                volumeID, existing.NodeID, nodeID)
        }
    }

    // Record new attachment
    readonly := req.GetReadonly()
    if err := cs.attachmentManager.Attach(volumeID, nodeID, readonly); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to record attachment: %v", err)
    }

    // ... build and return response
}
```

**CONFIDENCE:** HIGH - This is the standard pattern for RWO enforcement.

**Sources:**
- [CSI Spec - ControllerPublishVolume](https://github.com/container-storage-interface/spec/blob/master/spec.md)
- [AWS EBS CSI Driver - ControllerPublishVolume](https://github.com/kubernetes-sigs/aws-ebs-csi-driver)

### Pattern 3: Defensive Unpublish Handling

**What:** ControllerUnpublishVolume succeeds even if state is inconsistent

**Rationale:**
- Kubernetes may retry unpublish multiple times
- Node may have crashed, leaving stale state
- Blocking unpublish causes volume stuck in terminating state

**Implementation:**

```go
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context,
    req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

    volumeID := req.GetVolumeId()
    nodeID := req.GetNodeId() // May be empty in some cases

    existing, ok := cs.attachmentManager.GetAttachment(volumeID)
    if !ok {
        // Not attached - idempotent success
        klog.V(4).Infof("Volume %s not attached, returning success", volumeID)
        return &csi.ControllerUnpublishVolumeResponse{}, nil
    }

    // If nodeID specified and doesn't match, log warning but still detach
    if nodeID != "" && existing.NodeID != nodeID {
        klog.Warningf("Volume %s attached to node %s, but unpublish requested for node %s; detaching anyway",
            volumeID, existing.NodeID, nodeID)
    }

    if err := cs.attachmentManager.Detach(volumeID, existing.NodeID); err != nil {
        // Log error but don't fail - state will reconcile eventually
        klog.Errorf("Failed to remove attachment record for volume %s: %v", volumeID, err)
    }

    return &csi.ControllerUnpublishVolumeResponse{}, nil
}
```

**CONFIDENCE:** HIGH - Defensive handling prevents stuck volumes.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Blocking on Annotation Updates

**What people do:** Make ControllerPublishVolume wait for PV annotation update to succeed

**Why it's wrong:**
- API server unavailability blocks all publish operations
- Increases latency for every attach operation
- In-memory state is authoritative; annotation is backup

**Do this instead:** Update annotation asynchronously; log failures but don't block

### Anti-Pattern 2: Stateless Controller (No Attachment Tracking)

**What people do:** Rely solely on Kubernetes VolumeAttachment objects

**Why it's wrong:**
- VolumeAttachment is managed by external-attacher, not controller
- Controller has no way to enforce RWO without its own state
- Race conditions during pod migration

**Do this instead:** Maintain attachment state in controller; use as source of truth for RWO enforcement

### Anti-Pattern 3: Failing Unpublish on State Mismatch

**What people do:** Return error if unpublish nodeID doesn't match recorded nodeID

**Why it's wrong:**
- Causes volume to be stuck if node crashed
- Kubernetes retries indefinitely, never succeeds
- Manual intervention required

**Do this instead:** Log warning but allow unpublish to succeed; state will reconcile

## Capability Declaration

### Current Capabilities (driver.go)

```go
func (d *Driver) addControllerServiceCapabilities() {
    d.cscaps = []*csi.ControllerServiceCapability{
        {Type: &csi.ControllerServiceCapability_Rpc{
            Rpc: &csi.ControllerServiceCapability_RPC{
                Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
            }}},
        {Type: &csi.ControllerServiceCapability_Rpc{
            Rpc: &csi.ControllerServiceCapability_RPC{
                Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
            }}},
        {Type: &csi.ControllerServiceCapability_Rpc{
            Rpc: &csi.ControllerServiceCapability_RPC{
                Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
            }}},
    }
}
```

### Required Addition

```go
// ADD this capability to enable ControllerPublish/Unpublish
{Type: &csi.ControllerServiceCapability_Rpc{
    Rpc: &csi.ControllerServiceCapability_RPC{
        Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
    }}},
```

### CSIDriver Manifest Update

```yaml
# deploy/kubernetes/csi-driver.yaml
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: rds.csi.srvlab.io
spec:
  attachRequired: true  # CHANGE from false to true
  podInfoOnMount: true
  volumeLifecycleModes:
    - Persistent
```

When `attachRequired: true`:
- external-attacher creates VolumeAttachment objects
- Kubernetes waits for attachment before calling NodeStageVolume
- ControllerPublishVolume is called before node operations

## Suggested Build Order

### Phase 1: AttachmentManager Core (2-3 tasks)

1. **Create pkg/attachment/types.go**
   - Define Attachment struct
   - Define AttachmentManager interface

2. **Create pkg/attachment/manager.go**
   - Implement in-memory state with RWMutex
   - Implement Attach/Detach/GetAttachment methods
   - Unit tests for concurrent operations

3. **Add PV annotation persistence**
   - Implement persistAttachment (async)
   - Implement LoadFromAnnotations (sync on startup)
   - Integration test with fake k8s client

### Phase 2: ControllerPublish Implementation (2-3 tasks)

4. **Wire AttachmentManager into ControllerServer**
   - Add field to ControllerServer struct
   - Initialize in NewControllerServer
   - Call LoadFromAnnotations on startup

5. **Implement ControllerPublishVolume**
   - Request validation
   - RWO enforcement logic
   - Return PublishContext with NVMe connection info
   - Unit tests with mock AttachmentManager

6. **Implement ControllerUnpublishVolume**
   - Defensive handling
   - State cleanup
   - Unit tests

### Phase 3: Capability and Deployment (1-2 tasks)

7. **Update capability declarations**
   - Add PUBLISH_UNPUBLISH_VOLUME to cscaps
   - Update CSIDriver manifest (attachRequired: true)

8. **E2E testing**
   - Test pod migration (RWO enforcement)
   - Test controller restart (state recovery)
   - Test node failure (unpublish handling)

## Integration with Existing Components

### ControllerServer Modifications

```go
// pkg/driver/controller.go

type ControllerServer struct {
    csi.UnimplementedControllerServer
    driver            *Driver
    attachmentManager *attachment.AttachmentManager // NEW
}

func NewControllerServer(driver *Driver) *ControllerServer {
    return &ControllerServer{
        driver:            driver,
        attachmentManager: attachment.NewAttachmentManager(driver.k8sClient, DriverName),
    }
}
```

### Driver Initialization

```go
// pkg/driver/driver.go - Run() method

func (d *Driver) Run(endpoint string) error {
    // ... existing initialization ...

    // Initialize controller service if enabled
    if d.rdsClient != nil {
        klog.Info("Controller service enabled")
        d.cs = NewControllerServer(d)

        // Load attachment state from PV annotations
        if d.k8sClient != nil {
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            if err := d.cs.(*ControllerServer).attachmentManager.LoadFromAnnotations(ctx); err != nil {
                klog.Warningf("Failed to load attachment state from annotations: %v", err)
                // Non-fatal: continue with empty state
            }
        }
    }

    // ... rest of initialization ...
}
```

### RBAC Updates Required

```yaml
# deploy/kubernetes/controller-rbac.yaml
# ADD to controller ClusterRole:
- apiGroups: [""]
  resources: ["persistentvolumes"]
  verbs: ["get", "list", "watch", "update", "patch"]  # ADD update, patch
```

## Scaling Considerations

| Scale Factor | Impact | Mitigation |
|--------------|--------|------------|
| **Many volumes (1000+)** | In-memory map grows; startup load time increases | Use efficient map; paginate PV list on startup |
| **Frequent attach/detach** | Annotation update load on API server | Batch annotation updates; rate limit |
| **Controller restarts** | Brief period with incomplete state | LoadFromAnnotations is fast; external-attacher retries |
| **Large clusters (100+ nodes)** | More concurrent publish requests | RWMutex handles concurrency well |

## Sources

### CSI Specification (HIGH Confidence)
- [CSI Spec - ControllerPublishVolume](https://github.com/container-storage-interface/spec/blob/master/spec.md) - Official method specification
- [CSIDriver Object](https://kubernetes-csi.github.io/docs/csi-driver-object.html) - attachRequired field documentation

### CSI Driver Examples (MEDIUM-HIGH Confidence)
- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver) - Production attachment tracking implementation
- [DigitalOcean CSI Driver](https://pkg.go.dev/github.com/digitalocean/csi-digitalocean/driver) - ControllerPublishVolume example
- [CSI Host Path Driver](https://github.com/kubernetes-csi/csi-driver-host-path/pkg/hostpath) - Reference implementation

### Kubernetes Documentation (HIGH Confidence)
- [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/developing.html)
- [External Attacher](https://github.com/kubernetes-csi/external-attacher) - Sidecar interaction patterns

### Codebase Analysis (HIGH Confidence)
- `pkg/driver/controller.go` - Current stub implementation (lines 336-343)
- `pkg/driver/driver.go` - Capability declaration patterns
- `pkg/reconciler/orphan_reconciler.go` - PV annotation access patterns

---

*Architecture research for: Volume Fencing via ControllerPublish/Unpublish*
*Researched: 2026-01-30*
*Confidence: HIGH - Based on CSI spec, production driver examples, and existing codebase patterns*
