# Hardware Validation Results

## Summary
- **Date:** 2026-02-03
- **Driver version:** ghcr.io/3whiskeywhiskey/rds-csi:dev (commit 3807645)
- **Cluster:** metal (6 worker nodes)
- **Result:** BLOCKED - Critical bug discovered and fixed

## Validation Status

### Completed
- ✅ Task 1: Latest driver deployed to metal cluster (all pods running)
- ✅ Task 2 (partial): PVC created and bound successfully
- 🔧 Bug discovered: Block volume bind mount failure

### In Progress
- ⏳ Waiting for CI/CD build with bug fix (commit 3807645)
- ⏳ Deployment of fixed image pending

### Blocked
- ❌ Task 2: VM boot (blocked by bind mount bug)
- ⏸️ Task 3: I/O validation (blocked - VM won't start)
- ⏸️ Task 4: Live migration (blocked - VM won't start)

## Tests Performed

### VAL-01: VM Boot with Block Volume (BLOCKED)

**Setup:**
- PVC: test-block-vm-disk (5Gi, RWX, Block mode)
- StorageClass: rds-nvme
- VM: test-block-vm (cirros + data disk)
- Applied manifests:
  ```yaml
  # test-block-pvc.yaml
  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: test-block-vm-disk
    namespace: default
  spec:
    accessModes:
      - ReadWriteMany  # RWX for live migration
    volumeMode: Block
    resources:
      requests:
        storage: 5Gi
    storageClassName: rds-nvme
  ```

  ```yaml
  # test-block-vm.yaml
  apiVersion: kubevirt.io/v1
  kind: VirtualMachine
  metadata:
    name: test-block-vm
    namespace: default
  spec:
    running: true
    template:
      metadata:
        labels:
          kubevirt.io/vm: test-block-vm
      spec:
        domain:
          devices:
            disks:
              - name: rootdisk
                disk:
                  bus: virtio
              - name: datadisk
                disk:
                  bus: virtio
            interfaces:
              - name: default
                masquerade: {}
          resources:
            requests:
              memory: 1Gi
              cpu: 1
        networks:
          - name: default
            pod: {}
        volumes:
          - name: rootdisk
            containerDisk:
              image: quay.io/kubevirt/cirros-container-disk-demo:latest
          - name: datadisk
            persistentVolumeClaim:
              claimName: test-block-vm-disk
  ```

**Result:** BLOCKED

**Error:**
```
MapVolume.MapPodDevice failed for volume "pvc-dcf7ed0c-56fd-4608-8651-6e9c5fd73ca8" :
rpc error: code = Internal desc = failed to bind mount block device:
failed to create target directory: mkdir /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/pvc-dcf7ed0c-56fd-4608-8651-6e9c5fd73ca8/d779c559-d09b-49c2-bdcf-604619eabf2e: not a directory
```

**Root cause:**
Bug in `pkg/mount/mount.go` Mount() function:
1. NodePublishVolume calls MakeFile() to create target file ✓
2. NodePublishVolume calls Mount() to bind mount device to file
3. Mount() unconditionally calls `os.MkdirAll(target, 0750)` ✗
4. MkdirAll fails because target is a file, not a directory

**Impact:**
- All block volume publishes fail
- KubeVirt VMs cannot start with RDS block volumes
- This is a regression from Phase 11 implementation

**Notes:**
- PVC bound successfully (controller operations work)
- NodeStageVolume succeeded (NVMe connection established)
- NodePublishVolume failed at bind mount step
- Volume scheduled to node: c4140

### VAL-02: Live Migration (NOT TESTED)
- **Status:** Blocked - cannot test until VM boots
- **Prerequisites:** VAL-01 must pass

### VAL-03: Migration Metrics (NOT TESTED)
- **Status:** Blocked - cannot test until migration works
- **Prerequisites:** VAL-02 must pass

## Issues Encountered

### Issue 1: Incorrect StorageClass Name
- **Description:** Plan specified storageClassName: rds-csi, but cluster uses rds-nvme
- **Impact:** PVC stayed Pending until corrected
- **Resolution:** Updated PVC manifest to use rds-nvme
- **Category:** Minor - documentation/plan mismatch

### Issue 2: Block Volume Bind Mount Failure (CRITICAL BUG)
- **Description:** Mount() function tries to create directory at file path for block volumes
- **Discovered during:** Task 2 - VM creation
- **Root cause:**
  - MakeFile() creates target file successfully
  - Mount() unconditionally calls `os.MkdirAll(target)` for all mount operations
  - For block volumes, target is a file, so MkdirAll fails with "not a directory"
- **Fix:** Commit 3807645 - Skip directory creation when target exists and is a file
- **Fix details:**
  ```go
  // In pkg/mount/mount.go Mount()
  // OLD: Always create directory
  if err := os.MkdirAll(target, 0750); err != nil {
      return fmt.Errorf("failed to create target directory: %w", err)
  }

  // NEW: Check if target is file first
  if stat, err := os.Stat(target); err != nil {
      // Target doesn't exist - create directory
      if os.IsNotExist(err) {
          if err := os.MkdirAll(target, 0750); err != nil {
              return fmt.Errorf("failed to create target directory: %w", err)
          }
      }
  } else if stat.IsDir() {
      // Directory exists - OK
  } else {
      // File exists - OK for block volumes
  }
  ```
- **Status:** Fixed in commit 3807645, awaiting CI/CD build
- **Category:** Critical bug (Rule 1 auto-fix)
- **Affected versions:** Phase 11-12 implementation (first block volume support)
- **No regression:** This bug only affects block volumes (new feature)

## Bug Fix Tracking

### Commit 3807645: Block Volume Mount Fix
- **Commit:** 3807645
- **Message:** fix(13-01): skip directory creation when target is a file for block volumes
- **Files modified:** pkg/mount/mount.go
- **Change summary:** Mount() now detects existing files and skips directory creation
- **Build status:** CI/CD in progress (Dev Branch CI/CD run 21647759615)
- **Deployment status:** Pending - waiting for image build to complete

## Next Steps

1. **Wait for CI/CD:** Build must complete and push updated :dev image
2. **Restart node pods:** Trigger rollout restart to pull fixed image
3. **Retry VM boot:** Delete and recreate VM to test fix
4. **Continue validation:** Complete Tasks 2-4 once VM boots successfully
5. **Document results:** Update this file with complete validation results

## Deployment Information

### Driver Pods (Pre-Fix)
```
NAME                                 READY   STATUS    RESTARTS        AGE
rds-csi-controller-5888c8847-rk9pl   5/5     Running   2 (3m54s ago)   4m35s
rds-csi-node-n5grt                   3/3     Running   0               4m18s   (dpu-r740xd)
rds-csi-node-r9d8d                   3/3     Running   0               3m51s   (dpu-r640)
rds-csi-node-ts7f4                   3/3     Running   0               4m5s    (dpu-c4140)
rds-csi-node-vbptp                   3/3     Running   0               4m10s   (c4140)
rds-csi-node-xxbrh                   3/3     Running   0               121m    (r640)
```

### Images Used
- Controller: ghcr.io/3whiskeywhiskey/rds-csi:dev (commit 511a071 - Phase 12)
- Node: ghcr.io/3whiskeywhiskey/rds-csi:dev (commit 511a071 - Phase 12)
- All pods using :dev tag, updated via rollout restart

### Volume Information
- Volume ID: pvc-dcf7ed0c-56fd-4608-8651-6e9c5fd73ca8
- NVMe device: /dev/nvme2n1 (on node c4140)
- Staging path: /var/lib/kubelet/plugins/kubernetes.io/csi/pvc-dcf7ed0c.../globalmount
- Target path: /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/pvc-dcf7ed0c.../d779c559...

## Recommendations

### For Operators
1. **Do not use block volumes** with versions prior to commit 3807645
2. **Update immediately** to fixed version when available
3. **Test in non-production** before enabling block volume workloads

### For Development
1. **Add integration test** that catches this pattern (MakeFile + Mount on same path)
2. **Consider CSI sanity tests** for block volume path in CI/CD
3. **Document block volume limitations** until fully validated

### For v0.6.0 Release
1. **Hardware validation is essential** - caught critical bug that unit tests missed
2. **Block volume support should be marked beta** until full validation complete
3. **Release notes must mention** that v0.6.0 requires commit 3807645 or later for block volumes

## Test Environment

### Cluster Configuration
- **Platform:** Kubernetes 1.31
- **Worker nodes:** 6 nodes (c4140, r640, r740xd, dpu-r640, dpu-r740xd, dpu-c4140)
- **KubeVirt version:** v1.7.0
- **CSI sidecars:** csi-provisioner, csi-attacher, csi-resizer
- **RDS server:** 10.42.68.1 (MikroTik RouterOS 7.x)

### Node Details (c4140)
- **IP:** 10.42.67.9
- **NVMe device:** /dev/nvme2n1 (5GB RDS volume)
- **CSI node pod:** rds-csi-node-vbptp
- **Test VM scheduled:** test-block-vm (launcher pod: virt-launcher-test-block-vm-5knwr)

---

*Validation paused at: 2026-02-03T21:14:00Z*
*Resume after: CI/CD build completes and driver redeployed*
*Current blocker: Bug fix build in progress*
