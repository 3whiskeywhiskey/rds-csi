# Stack Research: NVMe-oF Connection Stability

**Domain:** NVMe over Fabrics (NVMe-oF) CSI Driver Reliability
**Researched:** 2026-01-30
**Confidence:** HIGH

## Recommended Stack

### Core Linux Kernel Components

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Native NVMe Multipath (nvme_core.multipath) | Kernel 4.15+ | Automatic path failover and device stability | Default in RHEL 9/SUSE 15+, lower overhead than DM multipath, handles controller renumbering via subsystem-level devices. Provides numa/round-robin/queue-depth policies with ANA (Asymmetric Namespace Access) support. |
| Kernel NVMe-oF Parameters (ctrl_loss_tmo, reconnect_delay) | Kernel 4.x+ | Automatic reconnection on connection loss | Built into kernel NVMe-oF stack. `ctrl_loss_tmo` sets max retry duration, `reconnect_delay` sets retry interval. Essential for handling temporary network disruptions without manual intervention. |
| /sys/class/nvme and /sys/class/nvme-subsystem | Kernel 4.15+ | Device discovery and management | Authoritative source for NVMe device state. Provides subsysnqn, controller state, namespace mappings. More reliable than parsing nvme-cli output. |

### Command-Line Tools

| Tool | Version | Purpose | Why Recommended |
|------|---------|---------|-----------------|
| nvme-cli | 2.x (maintenance) or 3.x (current) | NVMe device management | Official Linux NVMe management tool. Version 2.x is stable and well-documented. Version 3.x integrates libnvme directly (no external dependency). Both support `list-subsys -o json` for programmatic parsing. Minimum kernel 4.15 required. |
| udev | System default | Persistent device naming | Creates `/dev/disk/by-id/` symlinks based on device WWIDs/UUIDs. More stable than `/dev/disk/by-path` which uses PCI addresses that change on reconnection. Use for persistent volume identification. |

### Go Libraries and Approaches

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Direct sysfs parsing | N/A | Read NVMe device state from /sys/class/nvme* | **RECOMMENDED for CSI driver**. Avoids shelling out to nvme-cli. Read `/sys/class/nvme/nvme*/subsysnqn`, `/sys/class/nvme-subsystem/nvme-subsys*/iopolicy`, namespace directories. Reliable and fast. |
| libnvme via CGO | Latest | C library bindings for NVMe management | **Optional, higher complexity**. Provides typed API for NVMe operations. Requires CGO, increases binary size, couples to C library version. Only use if need advanced NVMe admin commands beyond basic connect/disconnect. |
| kubernetes-csi/csi-lib-iscsi | Latest | Multipath device management (iSCSI reference) | **Reference implementation only**. Study `multipath.go` for patterns on handling device mapper multipath, flushing devices, resize operations. Not directly usable for NVMe but shows CSI multipath patterns. |
| Exec nvme-cli commands | nvme-cli 2.x/3.x | Shell out to nvme command | **Current approach, acceptable**. Simple and reliable. Use `nvme list-subsys -o json` and parse JSON for connection state. Use context.Context for timeouts. |

## Installation

### System Requirements

```bash
# Ensure kernel version supports NVMe multipath
uname -r  # Should be >= 4.15

# Install nvme-cli
# Debian/Ubuntu
apt-get install nvme-cli

# RHEL/CentOS
dnf install nvme-cli

# Check nvme-cli version
nvme version
```

### Kernel Configuration

```bash
# Enable native NVMe multipath (usually enabled by default on modern distros)
# Add to kernel boot parameters or set at runtime:

# Boot parameter (persistent):
# Add to /etc/default/grub: nvme_core.multipath=Y

# Runtime (temporary):
echo Y > /sys/module/nvme_core/parameters/multipath

# Set I/O policy (optional, default is numa):
echo "round-robin" > /sys/module/nvme_core/parameters/iopolicy
# OR per-subsystem:
echo "round-robin" > /sys/class/nvme-subsystem/nvme-subsys0/iopolicy
```

### NVMe Connection Parameters for Stability

```bash
# When connecting, use these parameters for automatic recovery:
nvme connect -t tcp \
  -a <target-ip> \
  -s <port> \
  -n <nqn> \
  --ctrl-loss-tmo=60 \      # Max 60s of retries before giving up
  --reconnect-delay=2 \     # Retry every 2s
  --keep-alive-tmo=30       # Keep-alive heartbeat every 30s
```

## Alternatives Considered

| Category | Recommended | Alternative | Why Not Alternative |
|----------|-------------|-------------|---------------------|
| Multipath Stack | Native NVMe Multipath (nvme_core.multipath=Y) | Device Mapper Multipath (DM multipath) | DM multipath has higher CPU overhead, requires disabling native NVMe multipath globally. Use only if need advanced path selection policies beyond numa/round-robin/queue-depth. |
| Device Identification | /dev/disk/by-id/* symlinks | Direct /dev/nvme*n* paths | Direct paths change on controller renumbering (nvme0 → nvme3 after reconnect). /dev/disk/by-id uses WWIDs and persists across reconnections. |
| Device Discovery | Parse /sys/class/nvme* directly | Call `nvme list-subsys -o json` | Both valid. sysfs is faster, no subprocess overhead. nvme-cli JSON is more structured. For CSI driver, sysfs preferred for performance. |
| Go NVMe Library | Direct sysfs + exec nvme-cli | libnvme via CGO | CGO adds complexity, build dependencies, binary size. Only beneficial if need admin commands (format, fw-update). CSI drivers primarily need connect/disconnect/discover which work fine via CLI. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| /dev/disk/by-path/* symlinks | Based on PCI bus addresses which change when controllers reconnect (nvme0 at 0000:01:00.0 → nvme3 at same PCI address after disconnect/reconnect cycle). | /dev/disk/by-id/* (WWN/UUID-based) or NQN-based lookup via /sys/class/nvme/*/subsysnqn |
| Parsing `nvme list` plain text output | Output format changes between versions, not designed for parsing. | `nvme list-subsys -o json` for structured, parseable output |
| Disabling native NVMe multipath (nvme_core.multipath=N) | Causes device path instability. Each controller gets separate nvme* number. On reconnect, new controller number assigned, breaking mounts. | Keep native multipath enabled (Y), use subsystem-level devices (nvme0n1 instead of nvme0c0n1) |
| Hardcoded device paths in mount code | Paths change on reconnection. | Look up device by NQN via sysfs or use persistent symlinks from udev |

## Stack Patterns by Scenario

### Scenario 1: Handle Controller Renumbering (Current Bug)

**Problem:** After network disruption, controller reconnects as nvme3 instead of nvme0, but mount still references /dev/nvme0n1.

**Solution:**
- Enable native NVMe multipath (nvme_core.multipath=Y)
- Use subsystem-level device naming (nvme0n1) instead of controller-specific (nvme0c0n1)
- Look up device path by NQN, not hardcoded path:

```go
// Read subsysnqn for all controllers
controllers, _ := filepath.Glob("/sys/class/nvme/nvme*")
for _, ctrl := range controllers {
    nqnBytes, _ := os.ReadFile(filepath.Join(ctrl, "subsysnqn"))
    if strings.TrimSpace(string(nqnBytes)) == targetNQN {
        // Found matching controller, now find namespace
        namespaces, _ := filepath.Glob(filepath.Join(ctrl, "nvme*n*"))
        // Return first namespace block device
        return "/dev/" + filepath.Base(namespaces[0])
    }
}
```

### Scenario 2: Automatic Reconnection After Network Outage

**Problem:** Storage network has brief outage (5-10 seconds), need volumes to survive without pod restart.

**Solution:**
- Set `ctrl_loss_tmo=60` and `reconnect_delay=2` when connecting
- Kernel will automatically retry for up to 60 seconds, reconnecting every 2 seconds
- Application I/O blocks during reconnection, resumes when connection restored
- No CSI driver intervention needed

```bash
nvme connect -t tcp -a 10.42.68.1 -s 4420 -n nqn.2000-02.com.mikrotik:pvc-12345 \
  --ctrl-loss-tmo=60 --reconnect-delay=2
```

### Scenario 3: Detecting Stale Connections (Orphaned Subsystems)

**Problem:** Subsystem shows as "connected" (`nvme list-subsys` finds NQN) but no controllers/devices exist.

**Solution:**
- Check not just subsystem existence but also device existence:

```go
func isReallyConnected(nqn string) (bool, error) {
    // 1. Check subsystem exists
    subsysConnected, _ := nvmeListSubsysContains(nqn)
    if !subsysConnected {
        return false, nil
    }

    // 2. Verify actual device path exists
    devicePath, err := getDevicePathByNQN(nqn)
    if err != nil {
        // Subsystem exists but no device - orphaned!
        return false, fmt.Errorf("orphaned subsystem: %w", err)
    }

    // 3. Verify device node accessible
    if _, err := os.Stat(devicePath); err != nil {
        return false, fmt.Errorf("device node not accessible: %w", err)
    }

    return true, nil
}
```

### Scenario 4: Persistent Volume Identification Across Reconnects

**Problem:** After reconnect, need to find the same volume even if device path changed.

**Solution:**
- Store NQN in volume context (already doing this)
- Use udev-generated `/dev/disk/by-id/nvme-eui.*` or `/dev/disk/by-id/wwn-*` symlinks
- OR look up by NQN via sysfs (most reliable)

```go
func getStableDevicePath(volumeID string) (string, error) {
    nqn := volumeIDToNQN(volumeID)

    // Option 1: Lookup by NQN via sysfs (recommended)
    return getDevicePathByNQN(nqn)

    // Option 2: Use udev persistent symlinks (if available)
    // Note: Requires device to have published EUI-64 or NGUID
    // symlinks, _ := filepath.Glob("/dev/disk/by-id/nvme-*")
    // for each symlink, check if target matches our NQN...
}
```

## Version Compatibility

| Package | Minimum Version | Tested With | Notes |
|---------|-----------------|-------------|-------|
| Linux Kernel | 4.15 | 5.15, 6.1 | Kernel 4.15+ required for /sys/class/nvme-subsystem. Native multipath default in 5.3+. |
| nvme-cli | 1.x (legacy), 2.0+ (stable) | 2.3, 2.11, 3.x | Version 2.x in maintenance mode. Version 3.x integrates libnvme. Both support required features. |
| libnvme | 1.0+ | 1.9 | Only if using CGO approach. Not required for basic CSI operations. |
| Go | 1.18+ | 1.24 | For generics, improved error handling. Current project uses 1.24. |

## Implementation Roadmap Implications

Based on this research, the reliability milestone should focus on:

1. **Phase 1: sysfs-based device path lookup** (HIGH priority)
   - Replace current `/dev/nvme*n*` path assumptions with NQN-based lookup via `/sys/class/nvme/*/subsysnqn`
   - Implement `GetDevicePathByNQN()` that scans sysfs
   - Handles controller renumbering transparently

2. **Phase 2: Connection parameter tuning** (MEDIUM priority)
   - Add `ctrl_loss_tmo` and `reconnect_delay` parameters to `nvme connect` calls
   - Make configurable via StorageClass parameters
   - Default: ctrl_loss_tmo=60, reconnect_delay=2

3. **Phase 3: Orphaned subsystem detection** (MEDIUM priority)
   - Enhance `IsConnected()` to check device existence, not just subsystem presence
   - Implement reconnect logic if orphaned subsystem detected

4. **Phase 4: Native multipath enablement verification** (LOW priority, informational)
   - Check `/sys/module/nvme_core/parameters/multipath` at startup
   - Log warning if disabled, provide guidance to enable
   - Not critical if doing sysfs lookups, but improves stability

5. **Phase 5: Persistent device identification** (OPTIONAL)
   - Use `/dev/disk/by-id/*` symlinks if available
   - Fallback to NQN-based lookup via sysfs

## Sources

**Kernel Documentation (HIGH confidence):**
- [Linux NVMe multipath — The Linux Kernel documentation](https://docs.kernel.org/admin-guide/nvme-multipath.html)

**Official Red Hat Documentation (HIGH confidence):**
- [Chapter 4. Enabling multipathing on NVMe devices | RHEL 9](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/configuring_device_mapper_multipath/enabling-multipathing-on-nvme-devices_configuring-device-mapper-multipath)
- [Chapter 6. Overview of persistent naming attributes | RHEL 9](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_file_systems/assembly_overview-of-persistent-naming-attributes_managing-file-systems)

**nvme-cli Documentation (HIGH confidence):**
- [nvme-connect man page](https://www.mankier.com/1/nvme-connect)
- [nvme-list-subsys man page](https://www.mankier.com/1/nvme-list-subsys)
- [linux-nvme/nvme-cli GitHub](https://github.com/linux-nvme/nvme-cli)

**Kubernetes CSI References (MEDIUM confidence):**
- [kubernetes-csi/csi-lib-iscsi multipath management](https://pkg.go.dev/github.com/kubernetes-csi/csi-lib-iscsi/iscsi)
- [Multipath Management | kubernetes-csi/csi-lib-iscsi](https://deepwiki.com/kubernetes-csi/csi-lib-iscsi/2.3-multipath-management)

**Community and Issue Tracking (MEDIUM confidence):**
- [systemd/systemd #22692: udev by-path device names for NVMe disks are not persistent](https://github.com/systemd/systemd/issues/22692)
- [linux-nvme/nvme-cli issues and documentation](https://github.com/linux-nvme/nvme-cli/issues)
- [longhorn/longhorn #3602: Create golang API for mounting NVMeoF targets](https://github.com/longhorn/longhorn/issues/3602)

**Red Hat Solutions (MEDIUM confidence):**
- [How to configure Native NVME Multipath device names](https://access.redhat.com/solutions/5384031)
- [How to make block device NVMe assigned names persistent?](https://access.redhat.com/solutions/4763241)

---
*Stack research for: NVMe-oF CSI Driver Reliability Milestone*
*Researched: 2026-01-30*
