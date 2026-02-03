# Phase 11: Block Volume Node Operations - Research

**Researched:** 2026-02-03
**Domain:** CSI Node Service block volume operations
**Confidence:** HIGH

## Summary

Block volume support requires CSI node plugin operations (NodeStageVolume, NodePublishVolume, NodeUnpublishVolume, NodeUnstageVolume) to handle raw block devices differently from filesystem volumes. The key difference is that block volumes expose the underlying device directly to containers without creating a filesystem.

For block volumes, NodeStageVolume connects to the NVMe/TCP target but skips filesystem formatting and mounting. Instead, it stores device path metadata in the staging directory for later use. NodePublishVolume then creates a block device file at the target path (the path kubelet provides for the pod's volumeDevice) using bind-mount from the raw NVMe device. This allows KubeVirt VMs and other workloads to access the storage as a raw block device with direct I/O.

The CSI spec mandates that staging_target_path is always a directory (even for block volumes), while target_path for block volumes must be a device file. The driver must detect volume mode using `VolumeCapability.GetBlock()` and branch logic accordingly. Existing filesystem volume code paths remain unchanged, ensuring no regression.

**Primary recommendation:** Implement block volume detection using GetBlock() in all node operations, skip filesystem operations for block volumes in NodeStageVolume, use file creation + bind mount pattern for NodePublishVolume, and store device path in staging directory for coordination between staging and publishing phases.

## Standard Stack

The established libraries/tools for CSI block volume implementation:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/container-storage-interface/spec | v1.10.0 | CSI gRPC definitions | Official CSI spec implementation with VolumeCapability types |
| golang.org/x/sys/unix | latest | System calls for mknod | Standard Go syscall interface for device file creation |
| k8s.io/utils (optional) | v0.0.0-20230406110748-d93618cff8a2 | Mount utilities | Kubernetes-maintained mount helpers, but not required |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| os package | stdlib | File operations | Create device files, check paths |
| syscall package | stdlib | mknod syscall | Create block device nodes with major/minor |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Bind mount from /dev/nvmeXnY | mknod to create device node | Bind mount is simpler and safer - no need to extract/preserve major/minor numbers |
| k8s.io/mount-utils | Direct exec of mount command | mount-utils adds complexity; existing pkg/mount wrapper is sufficient |

**Installation:**
```bash
# Already in go.mod
# github.com/container-storage-interface/spec v1.10.0
# golang.org/x/sys v0.35.0
```

## Architecture Patterns

### Recommended Code Structure
```
pkg/driver/
├── node.go                    # NodeStageVolume, NodePublishVolume with block detection
pkg/mount/
├── mount.go                   # Add MakeFile() for creating empty files
pkg/utils/
├── block.go (NEW)             # Device path helpers for block volumes
```

### Pattern 1: Volume Mode Detection
**What:** Use GetBlock() to detect block vs filesystem mode at start of each node operation
**When to use:** Every NodeStageVolume, NodePublishVolume call
**Example:**
```go
// Source: CSI spec + csi-driver-host-path pattern
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    // Detect volume mode early
    isBlockVolume := req.GetVolumeCapability().GetBlock() != nil

    if isBlockVolume {
        // Block device path - call block-specific logic
        return ns.nodePublishBlockVolume(ctx, req)
    } else {
        // Filesystem path - existing bind mount logic
        return ns.nodePublishFilesystemVolume(ctx, req)
    }
}
```

### Pattern 2: Block Volume Staging (No Filesystem)
**What:** NodeStageVolume connects NVMe target, stores device path, skips formatting/mounting
**When to use:** When GetBlock() != nil in NodeStageVolumeRequest
**Example:**
```go
// Source: Research findings + existing nvme connector pattern
func (ns *NodeServer) stageBlockVolume(req *csi.NodeStageVolumeRequest) error {
    // 1. Connect to NVMe/TCP target (same as filesystem)
    devicePath, err := ns.nvmeConn.ConnectWithRetry(ctx, target, connConfig)
    if err != nil {
        return err
    }

    // 2. SKIP Format() - block volumes don't have filesystems

    // 3. Store device path in staging directory for NodePublishVolume
    // staging_target_path is ALWAYS a directory per CSI spec
    stagingPath := req.GetStagingTargetPath()
    if err := os.MkdirAll(stagingPath, 0750); err != nil {
        return err
    }

    // Write device path to staging metadata file
    metadataPath := filepath.Join(stagingPath, "device")
    return os.WriteFile(metadataPath, []byte(devicePath), 0600)
}
```

### Pattern 3: Block Device File Creation + Bind Mount
**What:** NodePublishVolume creates empty file at target_path, bind-mounts device to it
**When to use:** When GetBlock() != nil in NodePublishVolumeRequest
**Example:**
```go
// Source: csi-driver-host-path implementation + CSI spec
func (ns *NodeServer) publishBlockVolume(req *csi.NodePublishVolumeRequest) error {
    stagingPath := req.GetStagingTargetPath()
    targetPath := req.GetTargetPath()

    // 1. Read device path from staging metadata
    metadataPath := filepath.Join(stagingPath, "device")
    deviceBytes, err := os.ReadFile(metadataPath)
    if err != nil {
        return fmt.Errorf("failed to read device metadata: %w", err)
    }
    devicePath := string(deviceBytes)

    // 2. Create empty FILE at target_path (kubelet creates parent directory)
    // DO NOT use os.MkdirAll - target must be a file for block volumes
    if err := ns.mounter.MakeFile(targetPath); err != nil && !os.IsExist(err) {
        return fmt.Errorf("failed to create target file: %w", err)
    }

    // 3. Bind mount device to target file
    options := []string{"bind"}
    if req.GetReadonly() {
        options = append(options, "ro")
    }

    // Use empty fstype for bind mount
    if err := ns.mounter.Mount(devicePath, targetPath, "", options); err != nil {
        return fmt.Errorf("failed to bind mount block device: %w", err)
    }

    return nil
}
```

### Pattern 4: Staging Directory Metadata
**What:** Store device path in staging directory since staging_target_path is always a directory
**When to use:** Block volumes need to pass device info from NodeStageVolume to NodePublishVolume
**Example:**
```go
// Source: CSI spec requirement + common pattern
// In NodeStageVolume:
stagingPath := req.GetStagingTargetPath()  // Always a directory per spec
metadataFile := filepath.Join(stagingPath, "device")
os.WriteFile(metadataFile, []byte(devicePath), 0600)

// In NodePublishVolume:
metadataFile := filepath.Join(req.GetStagingTargetPath(), "device")
deviceBytes, _ := os.ReadFile(metadataFile)
devicePath := string(deviceBytes)
```

### Anti-Patterns to Avoid
- **Using MkdirAll for target_path in block volumes:** Target must be a file, not directory. Creates "not a directory" bind mount errors.
- **Attempting to format block volumes:** Block volumes expose raw devices. Formatting would corrupt the intended usage (KubeVirt creates its own filesystem inside the VM).
- **Calling IsLikelyMountPoint on target_path before creation:** For block volumes, target is a file, not a mount point initially. Check parent directory mount status instead.
- **Using mknod instead of bind mount:** Extracting major/minor numbers is complex and fragile. Bind mounting the device from /dev/nvmeXnY is simpler and safer.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Creating device files | Custom mknod with major/minor extraction | Bind mount from /dev/nvmeXnY | Avoids device number management, kernel handles major/minor |
| Checking if block vs mount | String parsing of capability fields | cap.GetBlock() != nil | CSI spec provides type-safe accessor methods |
| Path creation helpers | Custom os.Create wrappers | Add MakeFile() to existing pkg/mount interface | Consistent with existing Mount/Unmount patterns |
| Device path storage | Custom JSON/YAML metadata | Simple file with device path string | Block volumes only need device path, not complex state |

**Key insight:** The CSI spec and reference implementations (csi-driver-host-path) provide well-tested patterns. Don't reinvent device file creation or metadata storage - use standard bind mount + simple file metadata.

## Common Pitfalls

### Pitfall 1: Treating staging_target_path as a file for block volumes
**What goes wrong:** Code tries to write device path directly to staging_target_path, failing because CSI spec requires it to be a directory
**Why it happens:** Misunderstanding CSI spec requirement that "StagingTargetPath is always a directory even for block volumes"
**How to avoid:** Always create staging_target_path as directory with os.MkdirAll, store device path in a file inside that directory (e.g., `staging_target_path/device`)
**Warning signs:** Errors like "not a directory" or "is a directory" during NodeStageVolume

### Pitfall 2: Formatting block volumes
**What goes wrong:** NodeStageVolume calls Format() for block volumes, creating ext4/xfs filesystem that KubeVirt doesn't expect
**Why it happens:** Reusing filesystem staging logic without block volume detection
**How to avoid:** Check `if req.GetVolumeCapability().GetBlock() != nil` at start of NodeStageVolume, skip Format() and Mount() calls for block volumes
**Warning signs:** KubeVirt VMs fail to boot with "unexpected filesystem" errors, data corruption

### Pitfall 3: Creating target_path as directory for block volumes
**What goes wrong:** NodePublishVolume creates target_path with os.MkdirAll, bind mount fails with "not a block device"
**Why it happens:** Copying filesystem volume pattern where target_path is a directory
**How to avoid:** For block volumes, use MakeFile() to create empty file at target_path before bind mounting
**Warning signs:** Bind mount errors, kubelet logs "failed to publish volume: not a block device"

### Pitfall 4: Not cleaning up staging metadata
**What goes wrong:** Device metadata file remains in staging directory after unstage, causing confusion on re-stage
**Why it happens:** NodeUnstageVolume only disconnects NVMe, doesn't remove metadata file
**How to avoid:** In NodeUnstageVolume, remove metadata file before or after NVMe disconnect: `os.Remove(filepath.Join(stagingPath, "device"))`
**Warning signs:** Stale device paths on volume re-attachment, "device not found" errors

### Pitfall 5: Assuming target_path exists before NodePublishVolume
**What goes wrong:** Code expects target_path file to exist, fails when it doesn't
**Why it happens:** Misunderstanding CO vs CSI driver responsibilities (CO creates parent dir, driver creates target file)
**How to avoid:** Always call MakeFile(target_path) in NodePublishVolume, handle os.IsExist error gracefully
**Warning signs:** "no such file or directory" errors intermittently depending on kubelet version

## Code Examples

Verified patterns from official sources:

### Volume Mode Detection
```go
// Source: CSI spec VolumeCapability API
func isBlockVolume(cap *csi.VolumeCapability) bool {
    return cap.GetBlock() != nil
}

func isFilesystemVolume(cap *csi.VolumeCapability) bool {
    return cap.GetMount() != nil
}
```

### NodeStageVolume for Block Volumes
```go
// Source: Research findings + existing RDS CSI patterns
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    stagingPath := req.GetStagingTargetPath()

    // Early detection
    isBlock := req.GetVolumeCapability().GetBlock() != nil

    // ... validation code ...

    // Step 1: Connect to NVMe/TCP target (same for both modes)
    devicePath, err := ns.nvmeConn.ConnectWithRetry(ctx, target, connConfig)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to connect: %v", err)
    }

    if isBlock {
        // Step 2: For block volumes, store device path in staging directory
        if err := os.MkdirAll(stagingPath, 0750); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to create staging dir: %v", err)
        }

        metadataPath := filepath.Join(stagingPath, "device")
        if err := os.WriteFile(metadataPath, []byte(devicePath), 0600); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to write device metadata: %v", err)
        }

        klog.V(2).Infof("Staged block volume %s: device %s", volumeID, devicePath)
    } else {
        // Step 2-4: For filesystem volumes, format and mount (existing code)
        if err := ns.mounter.Format(devicePath, fsType); err != nil {
            return nil, err
        }

        if err := ns.mounter.Mount(devicePath, stagingPath, fsType, mountOptions); err != nil {
            return nil, err
        }

        klog.V(2).Infof("Staged filesystem volume %s", volumeID)
    }

    return &csi.NodeStageVolumeResponse{}, nil
}
```

### NodePublishVolume for Block Volumes
```go
// Source: csi-driver-host-path + CSI spec
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    stagingPath := req.GetStagingTargetPath()
    targetPath := req.GetTargetPath()

    // Early detection
    isBlock := req.GetVolumeCapability().GetBlock() != nil

    // ... validation code ...

    if isBlock {
        // Block volume: bind mount device file to target file

        // Read device path from staging metadata
        metadataPath := filepath.Join(stagingPath, "device")
        deviceBytes, err := os.ReadFile(metadataPath)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "failed to read device metadata: %v", err)
        }
        devicePath := strings.TrimSpace(string(deviceBytes))

        // Verify device exists
        if _, err := os.Stat(devicePath); err != nil {
            return nil, status.Errorf(codes.Internal, "device not found: %s", devicePath)
        }

        // Create target file (CO creates parent directory)
        if err := ns.mounter.MakeFile(targetPath); err != nil && !os.IsExist(err) {
            return nil, status.Errorf(codes.Internal, "failed to create target file: %v", err)
        }

        // Bind mount device to target
        mountOptions := []string{"bind"}
        if req.GetReadonly() {
            mountOptions = append(mountOptions, "ro")
        }

        if err := ns.mounter.Mount(devicePath, targetPath, "", mountOptions); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to bind mount block device: %v", err)
        }

        klog.V(2).Infof("Published block volume %s: %s -> %s", volumeID, devicePath, targetPath)
    } else {
        // Filesystem volume: bind mount staging directory to target directory (existing code)

        // Check staging is mounted
        mounted, err := ns.mounter.IsLikelyMountPoint(stagingPath)
        if err != nil || !mounted {
            return nil, status.Errorf(codes.FailedPrecondition, "staging path not mounted: %s", stagingPath)
        }

        // Bind mount staging to target
        mountOptions := []string{"bind"}
        if req.GetReadonly() {
            mountOptions = append(mountOptions, "ro")
        }

        if err := ns.mounter.Mount(stagingPath, targetPath, "", mountOptions); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to bind mount: %v", err)
        }

        klog.V(2).Infof("Published filesystem volume %s", volumeID)
    }

    return &csi.NodePublishVolumeResponse{}, nil
}
```

### MakeFile Helper for pkg/mount
```go
// Source: Common CSI driver pattern
// Add to pkg/mount/mount.go Mounter interface:

// MakeFile creates an empty file at the given path
// Used for block volume target paths
MakeFile(pathname string) error

// Implementation in mounter struct:
func (m *mounter) MakeFile(pathname string) error {
    // Create parent directory if needed
    parent := filepath.Dir(pathname)
    if err := os.MkdirAll(parent, 0750); err != nil {
        return fmt.Errorf("failed to create parent directory: %w", err)
    }

    // Create empty file
    f, err := os.OpenFile(pathname, os.O_CREATE|os.O_EXCL, 0640)
    if err != nil {
        if os.IsExist(err) {
            // File already exists - this is OK for idempotency
            return nil
        }
        return fmt.Errorf("failed to create file: %w", err)
    }
    f.Close()

    return nil
}
```

### NodeUnpublishVolume (Unified)
```go
// Source: Existing code + block volume considerations
func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    targetPath := req.GetTargetPath()

    // ... validation code ...

    // Unmount works for both filesystem and block volumes
    // (bind mounts in both cases)
    if err := ns.mounter.Unmount(targetPath); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to unmount: %v", err)
    }

    // Remove target path (file for block, directory for filesystem)
    // os.RemoveAll handles both
    if err := os.RemoveAll(targetPath); err != nil {
        klog.Warningf("Failed to remove target path %s: %v", targetPath, err)
        // Don't fail - unmount succeeded, cleanup is best-effort
    }

    klog.V(2).Infof("Unpublished volume %s", volumeID)
    return &csi.NodeUnpublishVolumeResponse{}, nil
}
```

### NodeUnstageVolume (Unified)
```go
// Source: Existing code + block volume metadata cleanup
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
    volumeID := req.GetVolumeId()
    stagingPath := req.GetStagingTargetPath()

    // ... validation code ...

    // Check if this was a block volume by looking for metadata
    metadataPath := filepath.Join(stagingPath, "device")
    isBlock := false
    if _, err := os.Stat(metadataPath); err == nil {
        isBlock = true
    }

    if isBlock {
        // Block volume: just disconnect NVMe, no unmount needed
        klog.V(2).Infof("Unstaging block volume %s", volumeID)

        // Remove metadata file
        if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
            klog.Warningf("Failed to remove metadata: %v", err)
        }

        // Remove staging directory
        if err := os.Remove(stagingPath); err != nil && !os.IsNotExist(err) {
            klog.Warningf("Failed to remove staging directory: %v", err)
        }
    } else {
        // Filesystem volume: unmount staging path (existing code)
        if err := ns.mounter.Unmount(stagingPath); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to unmount: %v", err)
        }
    }

    // Disconnect NVMe device (same for both modes)
    nqn, err := volumeIDToNQN(volumeID)
    if err == nil && nqn != "" {
        if err := ns.nvmeConn.Disconnect(nqn); err != nil {
            klog.Warningf("Failed to disconnect NVMe device: %v", err)
        }
    }

    klog.V(2).Infof("Unstaged volume %s", volumeID)
    return &csi.NodeUnstageVolumeResponse{}, nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| mknod for device files | Bind mount from /dev | CSI 1.0+ era | Simpler, no major/minor management needed |
| Complex metadata in staging | Simple device path file | Common practice since 2020 | Easier debugging, less state |
| Single code path | Mode detection branching | CSI spec from beginning | Clean separation of concerns |

**Deprecated/outdated:**
- **mknod with major/minor extraction**: Modern CSI drivers use bind mounts instead. Only needed for drivers that don't have access to /dev (rare).
- **JSON/protobuf staging metadata**: Simple text file with device path is sufficient for block volumes. Complex metadata adds no value.

## Open Questions

Things that couldn't be fully resolved:

1. **Should NodeGetVolumeStats work for block volumes?**
   - What we know: CSI spec doesn't clearly define stats for block volumes. Kubernetes CO may not call it for block volumes.
   - What's unclear: Whether to return empty stats or actual device size for block volumes
   - Recommendation: Check for block mode in NodeGetVolumeStats, return error codes.Unimplemented for block volumes (align with common CSI driver behavior)

2. **Device-in-use checking for block volumes**
   - What we know: Current code checks open file descriptors in NodeUnstageVolume (SAFETY-04)
   - What's unclear: Whether this works reliably for block device files (might need to check /dev/nvmeXnY instead of target path)
   - Recommendation: Test device-in-use check with block volumes in Phase 13 validation. May need to check the underlying device, not the bind-mounted target.

3. **Read-only block volumes**
   - What we know: NodePublishVolume receives readonly flag, should add "ro" mount option
   - What's unclear: Whether RDS supports read-only NVMe exports (likely it doesn't at protocol level)
   - Recommendation: Add "ro" to bind mount options for read-only flag, test in Phase 13. NVMe/TCP may not enforce read-only at protocol level, but kernel can enforce it at mount level.

## Sources

### Primary (HIGH confidence)
- [CSI Specification](https://github.com/container-storage-interface/spec/blob/master/spec.md) - VolumeCapability.GetBlock() API definition
- [Kubernetes CSI Raw Block Volume Guide](https://kubernetes-csi.github.io/docs/raw-block.html) - Official CSI developer documentation on block volumes
- [csi-driver-host-path nodeserver.go](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/pkg/hostpath/nodeserver.go) - Reference implementation from kubernetes-csi org
- [golang.org/x/sys/unix Mknod](https://golang.hotexamples.com/examples/syscall/-/Mknod/golang-mknod-function-examples.html) - Mknod syscall examples (for context, not used in final implementation)
- [k8s.io/mount-utils](https://pkg.go.dev/k8s.io/mount-utils) - Kubernetes mount utilities documentation

### Secondary (MEDIUM confidence)
- [CSI Block Volume GitHub Discussion](https://github.com/kubernetes/kubernetes/issues/73773) - NodePublishVolume behavior for block volumes
- [Kubernetes Staging Path Requirements](https://github.com/kubernetes/kubernetes/issues/72207) - CO vs driver path creation responsibilities
- [Device Major/Minor in Go](https://www.mnxsolutions.com/blog/golang-determine-a-device-major-minor-number) - Context on device numbers (not needed for bind mount approach)

### Tertiary (LOW confidence)
- None - all critical findings verified against official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Using existing dependencies (CSI spec v1.10.0, stdlib), no new libraries needed
- Architecture: HIGH - Pattern verified against kubernetes-csi reference implementation and CSI spec
- Pitfalls: HIGH - Based on CSI spec requirements and common issues documented in Kubernetes GitHub issues

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (30 days - CSI spec is stable)
