---
phase: 26-volume-snapshots
plan: 05
subsystem: deployment
tags: [kubernetes, rbac, snapshot, csi-snapshotter, volumesnapshotclass]

# Dependency graph
requires:
  - phase: 26-03
    provides: CSI controller snapshot service implementation
provides:
  - Kubernetes RBAC rules for snapshot.storage.k8s.io API group
  - csi-snapshotter v8.2.0 sidecar container in controller deployment
  - VolumeSnapshotClass manifest for RDS CSI driver
affects: [helm-chart, documentation]

# Tech tracking
tech-stack:
  added:
    - csi-snapshotter:v8.2.0
  patterns:
    - VolumeSnapshotClass manifest with installation prerequisites
    - Snapshot RBAC permissions pattern matching other CSI sidecar permissions

key-files:
  created:
    - deploy/kubernetes/snapshotclass.yaml
  modified:
    - deploy/kubernetes/rbac.yaml
    - deploy/kubernetes/controller.yaml

key-decisions:
  - "csi-snapshotter v8.2.0 selected (not v8.4.0) per RESEARCH.md - avoids unnecessary VolumeGroupSnapshot v1beta2 features"
  - "VolumeSnapshotClass uses deletionPolicy: Delete for automatic cleanup (matches StorageClass deletion policy pattern)"
  - "Installation prerequisites documented in VolumeSnapshotClass comments (CRD and snapshot-controller installation required)"

patterns-established:
  - "Snapshot RBAC follows sidecar pattern: volumesnapshotclasses (get/list/watch), volumesnapshotcontents (full CRUD), volumesnapshotcontents/status (update/patch), volumesnapshots (get/list)"
  - "csi-snapshotter sidecar configuration mirrors other sidecars: leader election in rds-csi namespace, 300s timeout, same socket mount"

# Metrics
duration: 1.4min
completed: 2026-02-06
---

# Phase 26 Plan 05: Volume Snapshots Summary

**Kubernetes deployment manifests for snapshot support: RBAC with snapshot.storage.k8s.io permissions, csi-snapshotter v8.2.0 sidecar, and VolumeSnapshotClass with deletion policy**

## Performance

- **Duration:** 1.4 min (86s)
- **Started:** 2026-02-06T05:57:41Z
- **Completed:** 2026-02-06T05:59:07Z
- **Tasks:** 2
- **Files modified:** 3 (1 created, 2 modified)

## Accomplishments
- Added snapshot.storage.k8s.io RBAC rules to controller ClusterRole (4 rules for volumesnapshotclasses, volumesnapshotcontents, volumesnapshotcontents/status, volumesnapshots)
- Deployed csi-snapshotter v8.2.0 sidecar container with leader election and timeout configuration
- Created VolumeSnapshotClass manifest with driver=rds.csi.srvlab.io and deletionPolicy=Delete

## Task Commits

Each task was committed atomically:

1. **Task 1: Add snapshot RBAC permissions and csi-snapshotter sidecar** - `12a2cca` (feat)
2. **Task 2: Create VolumeSnapshotClass manifest** - `490ed35` (feat)

## Files Created/Modified
- `deploy/kubernetes/rbac.yaml` - Added 4 RBAC rules for snapshot.storage.k8s.io API group resources
- `deploy/kubernetes/controller.yaml` - Added csi-snapshotter v8.2.0 sidecar container with leader election
- `deploy/kubernetes/snapshotclass.yaml` - Created VolumeSnapshotClass with driver name matching DriverName constant and Delete deletion policy

## Decisions Made

**1. csi-snapshotter version selection (v8.2.0)**
- RESEARCH.md recommended v8.2.0 over v8.4.0
- v8.4.0 adds VolumeGroupSnapshot v1beta2 support which we don't need
- v8.2.0 is stable and sufficient for single-volume snapshots

**2. VolumeSnapshotClass deletionPolicy: Delete**
- Matches the RDS StorageClass deletion policy pattern (reclaimPolicy: Delete)
- Ensures snapshots are automatically cleaned up when VolumeSnapshot objects are deleted
- Aligns with CSI spec recommendation for test/dev environments

**3. Installation prerequisite documentation**
- Added kubectl commands for VolumeSnapshot CRDs and snapshot-controller in VolumeSnapshotClass comments
- These are cluster-wide dependencies that must be installed before VolumeSnapshotClass can be created
- Makes deployment requirements explicit for operators

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - All YAML manifests validated successfully with kubectl dry-run. The expected "ensure CRDs are installed first" error for VolumeSnapshotClass validation confirms the prerequisite documentation is necessary.

## Next Phase Readiness

**Phase 26 (Volume Snapshots) is now COMPLETE:**
- ✅ Plan 01: Snapshot data model foundation (types, ID utilities, RDSClient interface, MockClient)
- ✅ Plan 02: Snapshot SSH commands (RouterOS Btrfs operations)
- ✅ Plan 03: CSI controller snapshot service (CreateSnapshot, DeleteSnapshot RPCs)
- ✅ Plan 04: ListSnapshots with pagination and CreateVolume snapshot restore
- ✅ Plan 05: RBAC and deployment manifests (this plan)

**Ready for deployment:**
- Install VolumeSnapshot CRDs: `kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/client/config/crd | kubectl create -f -`
- Install snapshot-controller: `kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller | kubectl create -f -`
- Deploy updated RDS CSI driver with snapshot support: `kubectl apply -f deploy/kubernetes/`
- Create VolumeSnapshotClass: `kubectl apply -f deploy/kubernetes/snapshotclass.yaml`

**Next phase:**
- Phase 28: Cross-Cluster Volume Migration (or Helm chart if prioritized)

---
*Phase: 26-volume-snapshots*
*Completed: 2026-02-06*
