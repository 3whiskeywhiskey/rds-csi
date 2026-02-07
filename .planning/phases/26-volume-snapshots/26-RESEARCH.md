# Phase 26: Volume Snapshots - Research

**Researched:** 2026-02-06
**Domain:** CSI Snapshot Operations + MikroTik RouterOS Btrfs Snapshots
**Confidence:** HIGH (CSI spec and codebase), MEDIUM (RouterOS Btrfs commands)

## Summary

This research covers implementing CSI volume snapshot operations (CreateSnapshot, DeleteSnapshot, ListSnapshots) and volume restore from snapshot (CreateVolume with VolumeContentSource) for the RDS CSI driver. The driver uses MikroTik RouterOS Btrfs snapshots via SSH as the underlying storage mechanism.

The CSI spec v1.12.0 (already in go.mod) fully supports snapshot operations. The implementation requires: (1) adding three controller RPC methods, (2) extending CreateVolume to handle VolumeContentSource with snapshot source, (3) adding snapshot-related RDS client commands using `/disk/btrfs/subvolume` RouterOS commands, (4) deploying the external-snapshotter sidecar (csi-snapshotter) alongside the controller, and (5) installing VolumeSnapshot CRDs cluster-wide. The stub implementations for CreateSnapshot, DeleteSnapshot, and ListSnapshots already exist in `controller.go` returning `Unimplemented`.

The RDS storage uses Btrfs, which natively supports snapshots via subvolume operations. RouterOS exposes these through `/disk/btrfs/subvolume/add` (with `read-only=yes` and `parent=` for snapshots) and `/disk/btrfs/subvolume/remove`. For restore, the standard Btrfs approach is to create a new writable snapshot-of-snapshot from the read-only snapshot. This aligns with the user's question about cascade delete implications.

**Primary recommendation:** Implement CSI snapshot operations using RouterOS `/disk/btrfs/subvolume` commands for snapshot lifecycle, with restore via writable snapshot-of-snapshot creation. Deploy csi-snapshotter v8.2.0 sidecar and install v1 VolumeSnapshot CRDs.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library/Tool | Version | Purpose | Why Standard |
|-------------|---------|---------|--------------|
| CSI Spec Go lib | v1.12.0 | Snapshot protobuf types (CreateSnapshotRequest, Snapshot, etc.) | Already in go.mod, all snapshot types available |
| csi-snapshotter sidecar | v8.2.0 | Watches VolumeSnapshotContent CRDs, triggers CSI RPCs | Official Kubernetes SIG-Storage sidecar, deploy per-driver |
| snapshot-controller | v8.2.0 | Watches VolumeSnapshot CRDs, orchestrates snapshot workflow | Official Kubernetes SIG-Storage, deploy once per cluster |
| VolumeSnapshot CRDs v1 | v8.2.0 | VolumeSnapshot, VolumeSnapshotContent, VolumeSnapshotClass | Required CRDs, installed cluster-wide |
| google.golang.org/protobuf | (existing) | `timestamppb.Now()` for snapshot creation_time | Already in go.mod, needed for Timestamp proto field |

### Supporting
| Library/Tool | Version | Purpose | When to Use |
|-------------|---------|---------|-------------|
| kubernetes-csi/csi-test | v5.4.0 | Sanity tests with snapshot validation | Already in go.mod, configure `TestSnapshotParameters` |
| google/uuid | v1.6.0 | Generate `snap-<uuid>` snapshot IDs | Already in go.mod |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| csi-snapshotter v8.2.0 | v8.4.0 (latest) | v8.4.0 adds VolumeGroupSnapshot v1beta2; not needed, v8.2.0 is stable |
| RouterOS Btrfs subvolume snapshot | RouterOS file copy | File copy is slow/space-inefficient; Btrfs snapshot is instant and CoW |

**Installation:**
```bash
# CRDs (cluster-wide, one time)
kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/client/config/crd | kubectl create -f -

# Snapshot controller (cluster-wide, one time)
kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller | kubectl create -f -

# No go module changes needed - CSI spec v1.12.0 already has snapshot support
```

## Architecture Patterns

### Snapshot Data Model

```
Kubernetes API                         RDS (RouterOS)
=============                         ==============

VolumeSnapshot ---------.
                         \
VolumeSnapshotContent ----> CSI CreateSnapshot ---> /disk/btrfs/subvolume/add
  annotations:                  |                     read-only=yes
    source-volume-id            |                     parent=<source-subvol>
    creation-timestamp          |                     fs=<filesystem-label>
    size-bytes                  |                     name=snap-<uuid>
    rds-disk-slot               v
                           Snapshot response:
                             snapshot_id = "snap-<uuid>"
                             source_volume_id = "pvc-<uuid>"
                             creation_time = now
                             size_bytes = source volume size
                             ready_to_use = true
```

### Recommended Code Structure
```
pkg/
  driver/
    controller.go          # Add CreateSnapshot, DeleteSnapshot, ListSnapshots
                           # Modify CreateVolume for VolumeContentSource
    driver.go              # Add CREATE_DELETE_SNAPSHOT, LIST_SNAPSHOTS capabilities
  rds/
    client.go              # Add snapshot methods to RDSClient interface
    types.go               # Add SnapshotInfo type
    commands.go            # Add CreateSnapshot, DeleteSnapshot, GetSnapshot, ListSnapshots
    mock.go                # Add mock snapshot operations
  utils/
    snapshotid.go          # Snapshot ID generation and validation (snap-<uuid>)
deploy/
  kubernetes/
    controller.yaml        # Add csi-snapshotter sidecar container
    rbac.yaml              # Add snapshot CRD RBAC permissions
    snapshotclass.yaml     # VolumeSnapshotClass definition (NEW file)
```

### Pattern 1: Snapshot ID Format and Validation
**What:** Snapshot IDs use `snap-<uuid>` format, parallel to volume `pvc-<uuid>` pattern.
**When to use:** All snapshot operations.
```go
// Snapshot ID validation - mirrors ValidateVolumeID pattern
const SnapshotIDPrefix = "snap-"
var snapshotIDPattern = regexp.MustCompile(`^snap-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func ValidateSnapshotID(snapshotID string) error {
    if snapshotID == "" {
        return fmt.Errorf("snapshot ID cannot be empty")
    }
    if !snapshotIDPattern.MatchString(snapshotID) {
        // Allow safe alphanumeric IDs for sanity tests
        if !safeSlotPattern.MatchString(snapshotID) {
            return fmt.Errorf("invalid snapshot ID: %s", snapshotID)
        }
    }
    return nil
}

func GenerateSnapshotID() string {
    return SnapshotIDPrefix + uuid.New().String()
}
```

### Pattern 2: CreateSnapshot Implementation Flow
**What:** Create a Btrfs read-only snapshot via RouterOS SSH.
**When to use:** CreateSnapshot RPC handler.
```go
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
    // 1. Validate request (name required, source_volume_id required)
    // 2. Check idempotency: snapshot with same name already exists?
    //    - Same source volume -> return existing snapshot (idempotent)
    //    - Different source volume -> return AlreadyExists error
    // 3. Verify source volume exists on RDS
    // 4. Generate snapshot ID: snap-<uuid>
    // 5. Create Btrfs snapshot via SSH:
    //    /disk/btrfs/subvolume/add read-only=yes parent=<source-subvol-name> fs=<fs-label> name=<snap-id>
    // 6. Verify snapshot was created
    // 7. Return response with Snapshot{
    //      SnapshotId: snapshotID,
    //      SourceVolumeId: req.SourceVolumeId,
    //      CreationTime: timestamppb.Now(),
    //      SizeBytes: sourceVolume.FileSizeBytes,
    //      ReadyToUse: true,  // Btrfs snapshots are instant
    //    }
}
```

### Pattern 3: CreateVolume from Snapshot (Restore)
**What:** Handle `VolumeContentSource.Snapshot` in CreateVolume to create a new volume from snapshot.
**When to use:** CreateVolume when VolumeContentSource is present.
```go
// In CreateVolume, after standard validation:
if req.GetVolumeContentSource() != nil {
    if snapshot := req.GetVolumeContentSource().GetSnapshot(); snapshot != nil {
        snapshotID := snapshot.GetSnapshotId()
        // 1. Verify snapshot exists
        // 2. Create writable snapshot-of-snapshot:
        //    /disk/btrfs/subvolume/add parent=<snap-id> fs=<fs-label> name=<new-vol-id>
        //    (Note: no read-only=yes -> creates writable subvolume from snapshot)
        // 3. Create /disk add type=file referencing the new subvolume
        // 4. Return volume with ContentSource set
    }
}
```

### Pattern 4: RouterOS Btrfs Snapshot Commands
**What:** SSH commands for snapshot lifecycle on RDS.
**When to use:** RDS client snapshot operations.
```bash
# Create read-only snapshot
/disk/btrfs/subvolume/add read-only=yes parent=<source-subvol> fs=<fs-label> name=<snap-id>

# List snapshots (filter by snap- prefix)
/disk/btrfs/subvolume/print where name~"snap-"

# Get specific snapshot
/disk/btrfs/subvolume/print detail where name=<snap-id>

# Delete snapshot
/disk/btrfs/subvolume/remove [find where name=<snap-id>]

# Create writable snapshot-of-snapshot (for restore)
/disk/btrfs/subvolume/add parent=<snap-id> fs=<fs-label> name=<new-vol-name>
```

### Pattern 5: Capability Registration
**What:** Advertise snapshot capabilities in ControllerGetCapabilities.
**When to use:** Driver initialization.
```go
// In addControllerServiceCapabilities(), add:
{
    Type: &csi.ControllerServiceCapability_Rpc{
        Rpc: &csi.ControllerServiceCapability_RPC{
            Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
        },
    },
},
{
    Type: &csi.ControllerServiceCapability_Rpc{
        Rpc: &csi.ControllerServiceCapability_RPC{
            Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
        },
    },
},
```

### Anti-Patterns to Avoid
- **Storing snapshot state only in-memory:** Snapshots must survive controller restarts. Store metadata in VolumeSnapshotContent annotations (as per user decision) and query RDS for ground truth via ListSnapshots.
- **Blocking on snapshot creation:** Btrfs snapshots are instant (CoW), so `ready_to_use` should always be `true`. Do not implement async polling.
- **Cascade-deleting snapshots when source volume is deleted:** CSI spec does not require this. Snapshots are independent after creation. Btrfs snapshots remain valid after parent subvolume deletion.
- **Using file copy for restore:** Always use Btrfs snapshot-of-snapshot (writable clone from read-only snapshot). It is instant and space-efficient.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Snapshot CRD management | Custom CRD controllers | external-snapshotter sidecar + snapshot-controller | Complex reconciliation, leader election, finalizers |
| VolumeSnapshotContent lifecycle | Custom webhook/controller | VolumeSnapshot CRDs v1 + snapshot-controller | Handles binding, status updates, deletion policy |
| Pagination for ListSnapshots | Custom token scheme | Integer-based token (index into sorted list) | CSI sanity tests validate this exact pattern |
| Snapshot timestamp | Custom time format | `timestamppb.Now()` from google/protobuf | CSI spec requires `google.protobuf.Timestamp` |
| Snapshot-to-volume restore plumbing | Custom PVC controller | VolumeContentSource in CreateVolume + external-provisioner | Provisioner automatically passes snapshot source |

**Key insight:** The Kubernetes snapshot ecosystem is split into cluster-wide components (snapshot-controller, CRDs) and per-driver components (csi-snapshotter sidecar). The CSI driver only needs to implement the gRPC methods -- all Kubernetes object management is handled by the sidecars.

## Common Pitfalls

### Pitfall 1: Forgetting CreateSnapshot Idempotency
**What goes wrong:** Duplicate snapshots created when external-snapshotter retries after timeout.
**Why it happens:** CSI spec requires CreateSnapshot to be idempotent by snapshot name + source volume ID.
**How to avoid:** Before creating, query RDS for existing snapshot with matching name. If found with same source volume, return existing snapshot. If found with different source volume, return `AlreadyExists` error.
**Warning signs:** CSI sanity test "should be idempotent" fails.

### Pitfall 2: DeleteSnapshot Not Returning Success for Non-Existent Snapshots
**What goes wrong:** Snapshot deletion gets stuck in loop or errors.
**Why it happens:** CSI spec requires DeleteSnapshot to return OK (not NOT_FOUND) when snapshot does not exist.
**How to avoid:** If snapshot not found on RDS, return `&csi.DeleteSnapshotResponse{}` with nil error.
**Warning signs:** VolumeSnapshotContent stuck in "Deleting" state.

### Pitfall 3: Missing VolumeSnapshotContent Annotations for Metadata
**What goes wrong:** After controller restart, snapshot metadata (source volume, size) is lost.
**Why it happens:** CSI driver returns metadata in CreateSnapshotResponse but doesn't persist it.
**How to avoid:** The external-snapshotter stores snapshot metadata in VolumeSnapshotContent status. For the RDS driver, query RDS to reconstruct metadata (ListSnapshots must return accurate data from RDS ground truth). No driver-side persistence needed beyond RDS.
**Warning signs:** ListSnapshots returns stale or empty metadata after restart.

### Pitfall 4: Not Validating Snapshot Exists Before Restore
**What goes wrong:** CreateVolume from non-existent snapshot creates empty/corrupt volume.
**Why it happens:** Skipping pre-flight check on snapshot existence.
**How to avoid:** In CreateVolume, when VolumeContentSource has snapshot source, verify snapshot exists on RDS before creating writable clone. Return NOT_FOUND if snapshot missing.
**Warning signs:** Restored PVC has wrong data or fails to mount.

### Pitfall 5: RBAC Missing for Snapshot CRDs
**What goes wrong:** csi-snapshotter sidecar cannot create/update VolumeSnapshotContent objects.
**Why it happens:** Existing RBAC doesn't include snapshot.storage.k8s.io API group.
**How to avoid:** Add RBAC rules for `volumesnapshotclasses`, `volumesnapshotcontents`, `volumesnapshotcontents/status`, and `volumesnapshots` in the controller ClusterRole.
**Warning signs:** Permission denied errors in csi-snapshotter sidecar logs.

### Pitfall 6: RouterOS Btrfs Subvolume Name vs /disk Slot Confusion
**What goes wrong:** Snapshot operations target wrong resource type.
**Why it happens:** Volume creation uses `/disk add` (disk-level), but snapshots use `/disk/btrfs/subvolume` (filesystem-level). These are different command hierarchies.
**How to avoid:** Clearly separate volume operations (`/disk` commands) from snapshot operations (`/disk/btrfs/subvolume` commands) in the RDS client. Document the relationship: volumes are file-backed disks on Btrfs, snapshots are Btrfs subvolume snapshots of the parent subvolume.
**Warning signs:** "no such item" errors from RouterOS.

### Pitfall 7: ListSnapshots Pagination Token Validation
**What goes wrong:** CSI sanity tests fail with ABORTED error.
**Why it happens:** Invalid starting_token not handled properly.
**How to avoid:** Parse starting_token as integer index. Return ABORTED if token is invalid or out of range. Return empty result (not error) if no snapshots match filter.
**Warning signs:** ListSnapshots pagination sanity tests fail.

## Code Examples

### CreateSnapshot Response Construction
```go
// Source: CSI spec csi.proto + hostpath driver pattern
import "google.golang.org/protobuf/types/known/timestamppb"

return &csi.CreateSnapshotResponse{
    Snapshot: &csi.Snapshot{
        SnapshotId:     snapshotID,          // "snap-<uuid>"
        SourceVolumeId: req.GetSourceVolumeId(), // "pvc-<uuid>"
        CreationTime:   timestamppb.Now(),
        SizeBytes:      sourceVolume.FileSizeBytes,
        ReadyToUse:     true, // Btrfs snapshots are instant
    },
}, nil
```

### ListSnapshots with Pagination
```go
// Source: kubernetes-csi/csi-driver-host-path pattern
func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
    // Handle single snapshot lookup by ID
    if req.GetSnapshotId() != "" {
        snap, err := cs.driver.rdsClient.GetSnapshot(req.GetSnapshotId())
        if err != nil {
            // Not found -> return empty (not error)
            return &csi.ListSnapshotsResponse{}, nil
        }
        return &csi.ListSnapshotsResponse{
            Entries: []*csi.ListSnapshotsResponse_Entry{
                {Snapshot: snap.ToCSISnapshot()},
            },
        }, nil
    }

    // Handle filtering by source volume
    // Handle pagination with starting_token and max_entries
    // Sort snapshots by ID for deterministic pagination
    // Return next_token if more entries exist
}
```

### RDS Client Snapshot Interface Addition
```go
// Add to RDSClient interface in client.go:
type RDSClient interface {
    // ... existing methods ...

    // Snapshot operations
    CreateSnapshot(opts CreateSnapshotOptions) (*SnapshotInfo, error)
    DeleteSnapshot(snapshotID string) error
    GetSnapshot(snapshotID string) (*SnapshotInfo, error)
    ListSnapshots() ([]SnapshotInfo, error)
    RestoreSnapshot(snapshotID string, newVolumeOpts CreateVolumeOptions) error
}
```

### SnapshotInfo Type
```go
// Add to types.go:
type SnapshotInfo struct {
    Name           string    // Snapshot name (snap-<uuid>)
    SourceVolume   string    // Source volume slot (pvc-<uuid>)
    FileSizeBytes  int64     // Size of snapshot (same as source)
    CreatedAt      time.Time // Creation timestamp
    ReadOnly       bool      // Should always be true for snapshots
    FSLabel        string    // Btrfs filesystem label
}

type CreateSnapshotOptions struct {
    Name           string // snap-<uuid>
    SourceVolume   string // pvc-<uuid> (parent subvolume)
    FSLabel        string // Btrfs filesystem label
}
```

### VolumeSnapshotClass YAML
```yaml
# deploy/kubernetes/snapshotclass.yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: rds-csi-snapclass
driver: rds.csi.srvlab.io
deletionPolicy: Delete
# parameters: {} # No special parameters needed for Btrfs snapshots
```

### Controller Deployment with csi-snapshotter Sidecar
```yaml
# Add to controller.yaml containers:
- name: csi-snapshotter
  image: registry.k8s.io/sig-storage/csi-snapshotter:v8.2.0
  args:
    - "--csi-address=$(ADDRESS)"
    - "--v=5"
    - "--timeout=300s"
    - "--leader-election=true"
    - "--leader-election-namespace=rds-csi"
  env:
    - name: ADDRESS
      value: /var/lib/csi/sockets/pluginproxy/csi.sock
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
  resources:
    requests:
      cpu: 10m
      memory: 64Mi
    limits:
      cpu: 100m
      memory: 128Mi
```

### RBAC Additions for Snapshots
```yaml
# Add to rds-csi-controller-role in rbac.yaml:
# VolumeSnapshotContent access (for csi-snapshotter sidecar)
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshotclasses"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshotcontents"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshotcontents/status"]
  verbs: ["update", "patch"]
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshots"]
  verbs: ["get", "list"]
```

### CSI Sanity Test Configuration for Snapshots
```go
// In test/sanity/sanity_test.go, add to config:
config.TestSnapshotParameters = map[string]string{
    // No special parameters needed for Btrfs snapshots
}
```

## Industry Standard: In-Use Volume Snapshots

**Research finding (user requested):** CSI drivers universally support taking snapshots of mounted/in-use volumes. This is crash-consistent, not application-consistent.

| Driver | In-Use Snapshot | Consistency | Notes |
|--------|----------------|-------------|-------|
| AWS EBS CSI | Yes | Crash-consistent | Takes snapshot of attached/mounted volume; recommends flush before snapshot for app consistency |
| Longhorn | Yes | Crash-consistent | Supports CSI snapshots on mounted volumes; app-consistent requires quiesce |
| GCE PD CSI | Yes | Crash-consistent | Google Cloud snapshots work on attached disks |
| Ceph CSI | Yes | Crash-consistent | RBD snapshots on mapped volumes |

**Recommendation:** Allow snapshots of in-use (mounted) volumes. Do NOT require volume to be detached. Btrfs snapshots are atomic at the filesystem level, providing crash-consistent state. This matches the user decision of "crash-consistent snapshots only, no freeze/thaw."

## Snapshot Deletion and Dependencies

**CSI spec guidance:** The CSI spec says "creating a snapshot from a snapshot is not supported" (source_volume_id must be a volume, not a snapshot). However, restoring creates a writable snapshot-of-snapshot on the Btrfs level.

**Btrfs behavior:** Btrfs snapshots are independent after creation. Deleting a source volume or parent snapshot does NOT invalidate child snapshots. Btrfs uses copy-on-write, so data blocks shared between parent and snapshot are reference-counted.

**User concern about cascade delete:** Since restore creates a snapshot-of-snapshot internally, cascade delete would destroy restored volumes. This confirms the user's intuition.

**Recommendation:**
- Do NOT cascade-delete snapshots when source volume is deleted
- Do NOT cascade-delete child subvolumes when snapshot is deleted
- Let Btrfs reference counting handle shared data blocks
- DeleteSnapshot only removes the specific read-only snapshot subvolume
- If a snapshot has dependent writable clones (restored volumes), Btrfs handles the reference counting transparently

## Volume Sizing During Restore

**Research finding (user requested):** Expand-during-restore is NOT a common CSI pattern. The standard behavior is:

1. CreateVolume from snapshot creates a volume with the same size as the snapshot
2. If user wants a larger volume, they expand it AFTER creation using ControllerExpandVolume
3. CSI spec requires: "The size of the volume MUST NOT be less than the size of the source snapshot"

**Recommendation:** During restore, create the volume at snapshot size. If requested capacity is larger than snapshot, either: (a) create at requested size and let the filesystem expand later, or (b) create at snapshot size and let the normal expand flow handle it. Option (a) is simpler since Btrfs handles this transparently.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| v1beta1 VolumeSnapshot API | v1 VolumeSnapshot API | Kubernetes 1.20 GA | Must use snapshot.storage.k8s.io/v1 |
| Snapshot CRDs auto-installed by sidecar | CRDs must be manually installed | external-snapshotter v4.0+ | Must install CRDs before deploying sidecar |
| Validation webhook for snapshots | CEL validation rules | external-snapshotter v7.0+ | No webhook needed; CRDs have built-in validation |
| SnapshotStatus enum (alpha) | ReadyToUse boolean | CSI spec v1.0 | Always use `ready_to_use` bool field |

**Deprecated/outdated:**
- v1beta1 VolumeSnapshot API: deprecated, use v1
- Snapshot validation webhook: replaced by CEL rules in CRDs
- Auto-installation of CRDs by sidecar: removed in v4.0+

## Open Questions

### 1. RouterOS Btrfs Subvolume-to-Disk Relationship
- **What we know:** Volumes are created via `/disk add type=file file-path=<path>` where the file lives on a Btrfs filesystem. Snapshots are created via `/disk/btrfs/subvolume/add`. These are different command hierarchies.
- **What's unclear:** When restoring from a snapshot (creating a writable clone), does the new subvolume need a separate `/disk add type=file` entry to be exported via NVMe/TCP? The current volume creation uses file-backed disks, not Btrfs subvolumes directly. The restore path may need to: (1) create writable Btrfs clone, (2) create a file-backed disk referencing the clone, (3) export via NVMe/TCP.
- **Recommendation:** This is the most critical implementation question. Test on actual RouterOS hardware during implementation. The implementation should try the simplest approach first: create writable snapshot-of-snapshot, then create a `/disk add type=file` pointing to the snapshot data. If Btrfs subvolumes cannot be directly used as file-backed disk paths, an alternative approach (e.g., copying snapshot data to a new file) may be needed. **Flag this for hardware validation early in implementation.**

### 2. RouterOS Btrfs Filesystem Label Discovery
- **What we know:** `/disk/btrfs/subvolume/add` requires an `fs=<filesystem-label>` parameter. The filesystem label identifies which Btrfs filesystem to operate on.
- **What's unclear:** How to programmatically discover the correct Btrfs filesystem label for the storage pool. The current driver uses `/storage-pool/metal-csi` as a path, not a Btrfs label.
- **Recommendation:** Query `/disk print` to find the Btrfs filesystem label for the storage pool disk. Alternatively, make it a configurable parameter in the StorageClass or driver config.

### 3. Snapshot Name-to-Subvolume Mapping
- **What we know:** Volumes use `pvc-<uuid>` as the `/disk` slot name. Snapshots will use `snap-<uuid>` as the Btrfs subvolume name. The user decided to store source volume ID in annotations.
- **What's unclear:** How to query Btrfs subvolumes by name to find a specific snapshot. The `/disk/btrfs/subvolume/print where name=<snap-id>` command format needs hardware validation.
- **Recommendation:** Validate the exact RouterOS command syntax on hardware. The `where name=` filter should work based on documented patterns for `/disk/btrfs/subvolume/print`.

### 4. Mock RDS Client Snapshot Support
- **What we know:** The MockClient in `pkg/rds/mock.go` currently only supports volume operations.
- **What's unclear:** Nothing unclear, but the mock needs to be extended with snapshot operations for unit tests and sanity tests.
- **Recommendation:** Add snapshot maps and methods to MockClient, following the same pattern as volume operations.

## Sources

### Primary (HIGH confidence)
- CSI spec v1.12.0 protobuf (csi.proto) - CreateSnapshot/DeleteSnapshot/ListSnapshots/Snapshot/VolumeContentSource message definitions
- kubernetes-csi/csi-driver-host-path - Reference implementation for snapshot operations in Go
- kubernetes-csi/external-snapshotter README - Sidecar architecture, CRD requirements, deployment model
- Existing RDS CSI codebase - controller.go, commands.go, types.go, mock.go patterns

### Secondary (MEDIUM confidence)
- MikroTik RouterOS Btrfs documentation (help.mikrotik.com) - `/disk/btrfs/subvolume` command syntax
- kubernetes-csi.github.io snapshot-restore-feature - VolumeSnapshotClass, restore workflow
- Btrfs official documentation - Snapshot-of-snapshot for restore, CoW behavior, reference counting

### Tertiary (LOW confidence)
- MikroTik forum posts - Storage features, Btrfs behavior on RouterOS
- Btrfs subvolume-to-disk-file relationship on RouterOS - Not documented, needs hardware validation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - CSI spec v1.12.0 well-documented, sidecar versions verified from GitHub releases
- Architecture: HIGH (CSI side) / MEDIUM (RouterOS side) - CSI patterns well-established from hostpath driver; RouterOS Btrfs subvolume-to-NVMe-export path needs hardware validation
- Pitfalls: HIGH - Well-documented in CSI spec (idempotency, error codes, pagination) and confirmed by sanity test requirements

**Research date:** 2026-02-06
**Valid until:** 2026-03-08 (30 days - stable domain, CSI spec rarely changes)
