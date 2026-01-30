# NVMe-oF Reconnection Handling Pitfalls

**Domain:** Kubernetes CSI drivers for NVMe over Fabrics (NVMe-oF)
**Researched:** 2026-01-30
**Confidence:** HIGH

This research focuses specifically on pitfalls when implementing NVMe-oF reconnection handling in CSI drivers, based on analysis of the rds-csi codebase, recent bug fixes, and community patterns from NVMe-oF CSI implementations.

---

## Critical Pitfalls

### Pitfall 1: Orphaned Subsystem False Positives

**What goes wrong:**
NVMe subsystems persist in the kernel after disconnection (orphaned state). `nvme list-subsys` shows the NQN as "connected" even when no controllers are attached and no `/dev/nvmeX` device exists. This causes a race condition:

1. `IsConnected()` returns true (sees orphaned NQN in subsystem list)
2. `GetDevicePath()` fails (no actual device exists)
3. CSI returns "no device found for NQN" error
4. Volume staging fails despite target being accessible

**Why it happens:**
The Linux kernel maintains subsystem metadata even after controller disconnection. During network interruptions or abnormal disconnects, the subsystem entry remains in `/sys/class/nvme-subsystem/` while the actual controller under `/sys/class/nvme/nvmeX` is removed. Simple string matching against `nvme list-subsys` output returns false positives.

**How to avoid:**
After `IsConnected()` returns true, **always verify `GetDevicePath()` succeeds**. If device lookup fails despite appearing connected, treat it as an orphaned subsystem:

```go
if connected {
    devicePath, err := c.GetDevicePath(target.NQN)
    if err == nil {
        return devicePath, nil // Genuine connection
    }
    // Orphaned subsystem - disconnect and reconnect
    klog.Warningf("NQN %s appears connected but no device found, attempting reconnect", target.NQN)
    _ = c.DisconnectWithContext(ctx, target.NQN)
    // Fall through to connect logic
}
```

**Warning signs:**
- Logs show "already connected" but device path lookups fail
- `nvme list-subsys` shows subsystem but `ls /sys/class/nvme/` shows no matching controller
- Volumes fail to stage intermittently after network issues

**Phase to address:**
Phase 1: Connection state verification (foundation phase). This is table-stakes reliability - must be included in MVP.

**Sources:**
- Fixed in commit bc90a9b: "fix(nvme): handle orphaned subsystems in ConnectWithContext"
- Community pattern: [Dell CSM Issue #496](https://github.com/dell/csm/issues/496) documents similar "csi_sock: connect: connection refused" after successful attach
- [Incomplete connections libnvme issue](https://github.com/linux-nvme/libnvme/issues/431)

---

### Pitfall 2: Controller Renumbering Breaks Device Path Assumptions

**What goes wrong:**
For NVMe-oF (fabrics), the device naming doesn't match local NVMe conventions. After disconnect/reconnect or controller failover:

- Controller `nvme1` might contain namespace `nvme2c1n2` (subsystem 2, controller 1, namespace 2)
- Block device appears as `/dev/nvme2n2` (subsystem-based) or `/dev/nvme2c1n2` (controller-based)
- Previous code assumes `/dev/nvme1n1` matches controller `nvme1` - this fails for NVMe-oF
- Hot-removing and reinserting a device changes both controller number AND namespace number unpredictably

**Why it happens:**
NVMe-oF devices use subsystem-based naming for multipath support. The Linux kernel assigns controller numbers dynamically at connection time. The subsystem number may differ from the controller number. After reconnection, both numbers can change based on kernel enumeration order (first-come-first-serve).

**How to avoid:**
1. **Never assume device name from controller name** - always scan namespace subdirectories
2. **Check both naming schemes** - subsystem-based (`nvme2n2`) and controller-based (`nvme2c1n2`)
3. **Use NQN as ground truth** - read `/sys/class/nvme/nvmeX/subsysnqn` to match, not device numbers
4. **Prefer subsystem-based paths** for multipath compatibility

```go
// Scan namespace directories under controller
namespaces, err := filepath.Glob(filepath.Join(controller, "nvme*n*"))
for _, ns := range namespaces {
    nsName := filepath.Base(ns)
    // Check if block device exists
    if _, err := os.Stat("/dev/" + nsName); err == nil {
        return "/dev/" + nsName, nil
    }

    // For controller-based (nvmeXcYnZ), try subsystem-based (nvmeXnZ)
    if strings.Contains(nsName, "c") {
        var subsys, ctrl, namespace int
        if _, err := fmt.Sscanf(nsName, "nvme%dc%dn%d", &subsys, &ctrl, &namespace); err == nil {
            subsysDevice := fmt.Sprintf("nvme%dn%d", subsys, namespace)
            if _, err := os.Stat("/dev/" + subsysDevice); err == nil {
                return "/dev/" + subsysDevice, nil
            }
        }
    }
}
```

**Warning signs:**
- Device path lookups fail after reconnection despite successful `nvme connect`
- Error messages like "no device found for NQN" when device is visible in `nvme list`
- `/dev/nvmeXn1` assumptions fail in logs
- Device numbers change between connections

**Phase to address:**
Phase 1: Device discovery logic (foundation phase). Critical for basic reliability.

**Sources:**
- Fixed in commits 74ce4d9 and 10b237b: "fix(nvme): handle NVMe-oF namespace device naming correctly"
- [NVMe device naming explanation](https://utcc.utoronto.ca/~cks/space/blog/linux/NVMeDeviceNames)
- [Red Hat: NVMe controllers and namespace name inconsistency](https://access.redhat.com/solutions/6967589)
- [Arch Linux forum: nvme0 suddenly becoming nvme1](https://forums.linuxmint.com/viewtopic.php?t=418402)

---

### Pitfall 3: Non-Idempotent NodeStageVolume on Retries

**What goes wrong:**
When kubelet retries `NodeStageVolume()` after transient failures, non-idempotent implementations fail with errors like:

- "device already connected" from `nvme connect`
- "already mounted" from mount operations
- "target path is a mountpoint" from staging path checks

CSI spec requires idempotency: calling `NodeStageVolume()` multiple times with same parameters must succeed. But naive implementations treat already-connected/already-mounted as errors, causing permanent pod failures.

**Why it happens:**
Three common anti-patterns:

1. **Assuming fresh state** - not checking if device is already connected before calling `nvme connect`
2. **Treating "already exists" as error** - returning errors for already-mounted filesystems
3. **Path comparison bugs** - comparing `/dev/xvdcf` (symlink) vs `/dev/nvme1n1` (canonical), failing idempotency check

In Kubernetes 1.20+, kubelet doesn't pre-check if the staging path is mounted, assuming CSI drivers handle idempotency correctly. Failures here leave pods stuck in ContainerCreating state with no automatic recovery.

**How to avoid:**
1. **Check existing state first** - before any operation, verify current state
2. **Return success if already correct** - if volume is already staged with matching parameters, return OK
3. **Use canonical paths for comparison** - resolve symlinks before comparing device paths
4. **Handle mount errors gracefully** - distinguish "already mounted" from "mount failed"

```go
// Check if already staged
mounted, err := ns.mounter.IsLikelyMountPoint(stagingPath)
if err == nil && mounted {
    // Verify device matches expected
    existingDevice, err := ns.mounter.GetDeviceNameFromMount(stagingPath)
    if err == nil {
        // Resolve to canonical path
        expectedDevice, _ := filepath.EvalSymlinks(devicePath)
        currentDevice, _ := filepath.EvalSymlinks(existingDevice)

        if expectedDevice == currentDevice {
            klog.V(2).Infof("Volume %s already staged correctly", volumeID)
            return &csi.NodeStageVolumeResponse{}, nil
        }
    }
}

// Check if device already connected
connected, err := ns.nvmeConn.IsConnected(nqn)
if err == nil && connected {
    // Verify device is accessible
    devicePath, err := ns.nvmeConn.GetDevicePath(nqn)
    if err == nil {
        // Already connected, continue to mount
        goto mountStep
    }
}
```

**Warning signs:**
- Pods stuck in ContainerCreating after kubelet restart
- "volume already staged" errors in logs
- Intermittent "device already connected" failures
- Different device paths in error messages (`/dev/xvdcf` vs `/dev/nvme1n1`)

**Phase to address:**
Phase 1: Core CSI operations (foundation). Non-negotiable for production use.

**Sources:**
- [AWS EBS CSI: NodeStage might not always be idempotent](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/1076)
- [AWS EBS CSI: Fix NodeStageVolume returning prematurely](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/pull/176)
- [IBM PowerVS: NodeStageVolume is called again even after successful response](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/issues/55)
- [Kubernetes: CSI SetUpAt does not retry when check fails](https://github.com/kubernetes/kubernetes/issues/112969)

---

### Pitfall 4: Aggressive Reconnection Without Backoff

**What goes wrong:**
When NVMe target becomes unreachable (network partition, target reboot, maintenance), naive implementations retry `nvme connect` in tight loops:

- Flood logs with connection failures (thousands of attempts per minute)
- Exhaust kernel resources creating/destroying controllers
- Block CSI RPC threads waiting for timeouts
- Prevent legitimate operations from succeeding
- Create "thundering herd" if many pods retry simultaneously

Worse: after target recovers, synchronized retries from all nodes can overwhelm the target, causing cascading failures.

**Why it happens:**
Without explicit backoff logic, each pod's `NodeStageVolume()` call blocks waiting for connection timeout (e.g., 30s), then kubelet immediately retries, creating a constant retry loop. Multiple pods on the same node create parallel retry streams. The kernel's built-in `ctrl_loss_tmo` (default 600s/10 minutes) doesn't help because CSI operations timeout before that.

**How to avoid:**
1. **Implement exponential backoff** in Connect operations
2. **Use context timeouts** to prevent indefinite hangs
3. **Coordinate retries** - use a backoff tracker keyed by NQN to prevent parallel retries to same target
4. **Respect kernel-level ctrl_loss_tmo** for network partition scenarios
5. **Return retriable errors** to kubelet with appropriate status codes

```go
type reconnectTracker struct {
    mu            sync.Mutex
    attempts      map[string]int       // NQN -> attempt count
    lastAttempt   map[string]time.Time // NQN -> last attempt time
}

func (c *connector) ConnectWithBackoff(ctx context.Context, target Target) (string, error) {
    c.backoff.mu.Lock()
    attempts := c.backoff.attempts[target.NQN]
    lastAttempt := c.backoff.lastAttempt[target.NQN]
    c.backoff.mu.Unlock()

    // Exponential backoff: 1s, 2s, 4s, 8s, max 60s
    backoffDelay := time.Duration(1<<min(attempts, 6)) * time.Second
    if time.Since(lastAttempt) < backoffDelay {
        return "", status.Errorf(codes.Unavailable,
            "target %s in backoff for %v", target.NQN, backoffDelay)
    }

    c.backoff.mu.Lock()
    c.backoff.attempts[target.NQN] = attempts + 1
    c.backoff.lastAttempt[target.NQN] = time.Now()
    c.backoff.mu.Unlock()

    // Attempt connection
    devicePath, err := c.connectInternal(ctx, target)
    if err == nil {
        // Success - reset backoff
        c.backoff.mu.Lock()
        delete(c.backoff.attempts, target.NQN)
        delete(c.backoff.lastAttempt, target.NQN)
        c.backoff.mu.Unlock()
    }
    return devicePath, err
}
```

**Warning signs:**
- Hundreds of "nvme connect failed" messages per minute in logs
- High CPU usage from nvme-cli processes
- Kernel messages about resource exhaustion
- Other CSI operations timing out
- Spikes in API server load from pod status updates

**Phase to address:**
Phase 2: Reliability improvements. Can defer to post-MVP if operational monitoring shows it's not critical, but recommended for Phase 1 if production workloads expected.

**Sources:**
- Pattern observed in [simplyblock-csi](https://github.com/simplyblock/simplyblock-csi) - uses SPDK with backoff
- [Red Hat: NVMe-oF ctrl_loss_tmo configuration](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_storage_devices/configuring-nvme-over-fabrics-using-nvme-tcp_managing-storage-devices)
- [Dell: PowerStore NVMe disconnects on path loss](https://www.dell.com/support/kbdoc/en-in/000272457/powerstore-nvme-over-tcp-volumes-connect-on-redhat-servers-when-one-or-more-paths-are-lost)

---

### Pitfall 5: Stale Mounts After Unclean Disconnection

**What goes wrong:**
When NVMe connection drops unexpectedly (network failure, target crash, hard node reboot), the mounted filesystem in `/var/lib/kubelet/pods/*/volumes/kubernetes.io~csi/*/mount` becomes stale:

- Filesystem operations hang with "Transport endpoint is not connected"
- `umount` hangs indefinitely
- Pod deletion gets stuck waiting for volume cleanup
- New pods can't use the volume because old mount still exists
- Manual intervention required to clean up

Even worse: if kubelet restarts before cleanup, it may lose track of the mount entirely, creating a permanent leak.

**Why it happens:**
When NVMe device disappears without proper unmount (kernel's `ctrl_loss_tmo` expires, device removed from `/dev/`), the filesystem layer still has references to the mount. The VFS layer marks the mount as "dead" but doesn't automatically clean it up. Subsequent unmount attempts block waiting for I/O that will never complete.

**How to avoid:**
1. **Use lazy unmount for cleanup** - `umount -l` for stale mounts
2. **Check mount health before operations** - test device accessibility
3. **Implement force-unmount path** in NodeUnstageVolume
4. **Handle ENOTCONN gracefully** during unmount
5. **Clean up orphaned mounts on startup** - scan for stale CSI mounts at node plugin initialization

```go
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
    // Check if mount is stale
    devicePath, err := ns.mounter.GetDeviceNameFromMount(stagingPath)
    if err == nil {
        // Device found - check if accessible
        if _, err := os.Stat(devicePath); os.IsNotExist(err) {
            klog.Warningf("Mount at %s references non-existent device %s, using force unmount",
                stagingPath, devicePath)
            // Force unmount
            if err := ns.mounter.UnmountWithForce(stagingPath); err != nil {
                // Fallback to lazy unmount
                return nil, ns.mounter.LazyUnmount(stagingPath)
            }
        }
    }

    // Normal unmount path
    return ns.mounter.Unmount(stagingPath)
}

// On node plugin startup
func (ns *NodeServer) CleanupOrphanedMounts() error {
    // Scan /var/lib/kubelet/plugins/kubernetes.io/csi/*/globalmount
    staleMounts := ns.mounter.FindStaleMounts(ns.driver.name)
    for _, mount := range staleMounts {
        klog.Warningf("Found orphaned mount: %s, attempting cleanup", mount)
        _ = ns.mounter.LazyUnmount(mount)
    }
}
```

**Warning signs:**
- Pods stuck in Terminating state
- "Transport endpoint is not connected" errors in kubelet logs
- `umount` processes in `D` state (uninterruptible sleep)
- Mount table shows CSI mounts with no corresponding pod
- Node plugin logs show successful staging but GetDevicePath fails later

**Phase to address:**
Phase 2: Error recovery. Should be addressed before declaring production-ready.

**Sources:**
- [JuiceFS CSI: Transport endpoint not connected](https://juicefs.com/docs/csi/troubleshooting-cases/)
- [Ceph CSI: NodeStageVolume fails if xfs_repair returns error after reboot](https://github.com/ceph/ceph-csi/issues/859)
- [Azure Files CSI: Stacked NodeStageVolume mounts prevents unmounting](https://github.com/kubernetes-sigs/azurefile-csi-driver/issues/1137)
- [Red Hat Bugzilla: Stale mount/stage in different node](https://bugzilla.redhat.com/show_bug.cgi?id=1745776)

---

## Moderate Pitfalls

### Pitfall 6: Race Condition in Device Discovery

**What goes wrong:**
After successful `nvme connect`, the device may not appear immediately in `/dev/` or `/sys/class/nvme/`. Polling too quickly causes false negatives. Polling without timeout causes hangs. The race window varies based on system load and udev processing time.

**How to avoid:**
Implement polling with exponential backoff and hard timeout. Wait for both sysfs entry AND `/dev/` node to exist. Give udev time to set permissions.

```go
func (c *connector) WaitForDevice(nqn string, timeout time.Duration) (string, error) {
    deadline := time.Now().Add(timeout)
    backoff := 100 * time.Millisecond

    for time.Now().Before(deadline) {
        devicePath, err := c.GetDevicePath(nqn)
        if err == nil {
            // Found in sysfs - now wait for /dev/ node
            for i := 0; i < 10; i++ {
                if _, err := os.Stat(devicePath); err == nil {
                    return devicePath, nil
                }
                time.Sleep(100 * time.Millisecond)
            }
        }

        time.Sleep(backoff)
        backoff = min(backoff*2, 5*time.Second)
    }

    return "", fmt.Errorf("timeout waiting for device with NQN %s", nqn)
}
```

**Phase to address:**
Phase 1: Core device handling (foundation).

**Sources:**
- Observed in rds-csi commit history
- [AWS EBS CSI: Race condition/timeout issues when mounting PV](https://github.com/kubernetes/kubernetes/issues/76568)

---

### Pitfall 7: Ignoring nvme-cli Exit Codes

**What goes wrong:**
`nvme connect` can return exit code 0 but still fail. Output parsing required to detect partial failures. Some errors appear only in stderr, not exit codes.

**How to avoid:**
Always parse command output, not just exit code. Check for specific error strings:
- "already connected" - idempotency case, not an error
- "failed to write to /dev/nvme-fabrics" - real error
- "Invalid argument" - config problem, not transient

```go
output, err := cmd.CombinedOutput()
if err != nil {
    // Check for known non-fatal errors
    if strings.Contains(string(output), "already connected") {
        klog.V(2).Infof("Target already connected (idempotent)")
        return c.GetDevicePath(nqn)
    }
    return "", fmt.Errorf("nvme connect failed: %w, output: %s", err, string(output))
}
```

**Phase to address:**
Phase 1: Error handling (foundation).

**Sources:**
- [nvme connect-all reports "already connected"](https://github.com/linux-nvme/nvme-cli/issues/1505)
- [nvme disconnect doesn't remove all controllers](https://github.com/linux-nvme/nvme-cli/issues/499)

---

### Pitfall 8: Manual Cleanup Required for Orphaned Controllers

**What goes wrong:**
During stress testing or rapid connect/disconnect cycles, partial connections occur. The kernel creates `/dev/nvmeX` but not `/sys/class/nvme-subsystem/nvme-subsysX/nvmeX`. `nvme disconnect` fails with "failed to lookup subsystem for controller". Only manual cleanup via `echo 1 > /sys/class/nvme/nvmeX/delete_controller` works.

**How to avoid:**
Implement fallback cleanup in DisconnectWithContext:

```go
func (c *connector) DisconnectWithContext(ctx context.Context, nqn string) error {
    // Try normal disconnect
    cmd := exec.CommandContext(ctx, "nvme", "disconnect", "-n", nqn)
    output, err := cmd.CombinedOutput()

    if err != nil && strings.Contains(string(output), "failed to lookup subsystem") {
        // Fallback: manual controller deletion
        klog.Warningf("Subsystem lookup failed, attempting manual cleanup for NQN %s", nqn)
        return c.manualDeleteController(nqn)
    }
    return err
}

func (c *connector) manualDeleteController(nqn string) error {
    controllers, _ := filepath.Glob("/sys/class/nvme/nvme*")
    for _, controller := range controllers {
        nqnPath := filepath.Join(controller, "subsysnqn")
        data, _ := os.ReadFile(nqnPath)
        if strings.TrimSpace(string(data)) == nqn {
            deletePath := filepath.Join(controller, "delete_controller")
            return os.WriteFile(deletePath, []byte("1"), 0644)
        }
    }
    return fmt.Errorf("controller not found for NQN %s", nqn)
}
```

**Phase to address:**
Phase 2: Robustness improvements.

**Sources:**
- [libnvme: Allow disconnect of incomplete connections](https://github.com/linux-nvme/libnvme/issues/431)
- [nvme-cli: Unable to disconnect from NVMeoF subsystem](https://github.com/linux-nvme/nvme-cli/issues/2603)

---

## Minor Pitfalls

### Pitfall 9: Noisy Logging Hides Real Errors

**What goes wrong:**
Logging every retry, every "already connected", every device poll creates log spam. Real errors get buried. Operators waste time investigating non-issues.

**How to avoid:**
- Use appropriate log levels (V(2) for normal ops, V(4) for debug)
- Suppress "already connected" messages at info level
- Log only first and last retry in a sequence
- Use structured logging with consistent fields (volumeID, NQN, operation)

**Phase to address:**
Phase 3: Operational polish (post-MVP).

---

### Pitfall 10: No Metrics for Reconnection Events

**What goes wrong:**
Without metrics, operators can't distinguish healthy systems from those constantly reconnecting. Silent failures go unnoticed.

**How to avoid:**
Export Prometheus metrics:
- `nvme_connect_total{status="success|error"}`
- `nvme_connect_duration_seconds{quantile}`
- `nvme_disconnect_total{status="success|error"}`
- `nvme_orphaned_subsystems_total`
- `nvme_device_discovery_failures_total`

**Phase to address:**
Phase 3: Observability (post-MVP).

---

## Testing Strategies for Reconnection Scenarios

Testing NVMe-oF reconnection is notoriously difficult because it requires simulating real network and target failures. Here's how to test each pitfall:

### Test 1: Orphaned Subsystem Injection

**What it tests:** Pitfall 1 (orphaned subsystems)

**Method:**
1. Connect to NVMe target normally
2. Manually disconnect controller via sysfs: `echo 1 > /sys/class/nvme/nvmeX/delete_controller`
3. Don't run `nvme disconnect` - leave subsystem entry intact
4. Trigger `NodeStageVolume()` - should detect orphan and reconnect

**Expected behavior:**
- Driver detects orphaned subsystem
- Logs "appears connected but no device found, attempting reconnect"
- Successfully reconnects and stages volume

**Failure mode:**
- Returns "no device found for NQN" error
- Does not attempt cleanup

---

### Test 2: Controller Number Instability

**What it tests:** Pitfall 2 (controller renumbering)

**Method:**
1. Connect multiple NVMe targets (minimum 3) to force higher controller numbers
2. Disconnect middle controller
3. Reconnect - observe new controller number
4. Verify device discovery still works

**Expected behavior:**
- Device path found regardless of controller number changes
- Both `nvme2c1n2` and `nvme2n2` naming handled correctly

**Failure mode:**
- "no device found" after reconnection
- Assumes device path based on old controller number

---

### Test 3: Rapid Retry Loop

**What it tests:** Pitfall 4 (aggressive reconnection)

**Method:**
1. Block NVMe target IP via iptables: `iptables -A OUTPUT -d <target-ip> -j DROP`
2. Create PVC/Pod - trigger NodeStageVolume
3. Monitor retry rate and log volume
4. Restore network after 2 minutes
5. Verify eventual success with reasonable backoff

**Expected behavior:**
- Retry rate decreases over time (exponential backoff)
- System remains responsive during retries
- Successful connection after network restore

**Failure mode:**
- Constant retry loop at 1/second or faster
- High CPU usage
- System instability

---

### Test 4: Stale Mount Cleanup

**What it tests:** Pitfall 5 (stale mounts)

**Method:**
1. Stage volume normally
2. Expire kernel controller with `echo 0 > /sys/class/nvme/nvmeX/ctrl_loss_tmo`
3. Wait for device removal
4. Trigger `NodeUnstageVolume()` - should handle stale mount

**Expected behavior:**
- Detects stale mount
- Uses lazy unmount or force unmount
- Returns success

**Failure mode:**
- Hangs indefinitely on unmount
- Returns error requiring manual cleanup

---

### Test 5: Idempotency Verification

**What it tests:** Pitfall 3 (non-idempotent operations)

**Method:**
1. Call `NodeStageVolume()` successfully
2. Immediately call `NodeStageVolume()` again with same parameters
3. Repeat 5 times
4. Verify all return success (no "already connected" errors)

**Expected behavior:**
- All calls return success
- No "already mounted" errors
- No device conflicts

**Failure mode:**
- Second call returns error
- Inconsistent behavior across calls

---

### Test 6: Linux Kernel Fault Injection

**What it tests:** Multiple pitfalls under system stress

**Method:**
Linux kernel provides NVMe fault injection via debugfs:

```bash
# Enable fault injection for NVMe admin commands
echo 100 > /sys/kernel/debug/nvme0/fault_inject/probability
echo 1 > /sys/kernel/debug/nvme0/fault_inject/times

# Trigger controller reset
echo 1 > /sys/class/nvme/nvme0/reset_controller

# Observe CSI driver behavior during reinitialization
```

**Expected behavior:**
- Driver handles transient failures gracefully
- Eventually succeeds after faults clear
- No permanent state corruption

**Sources:**
- [Linux Kernel: NVMe Fault Injection](https://docs.kernel.org/fault-injection/nvme-fault-injection.html)
- [Testing tools from Quarch Technology](https://quarch.com/solutions/hot-swap-and-fault-injection/)

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Orphaned subsystems | Phase 1: Foundation | Test 1: Orphaned subsystem injection |
| 2. Controller renumbering | Phase 1: Foundation | Test 2: Controller number instability |
| 3. Non-idempotent staging | Phase 1: Foundation | Test 5: Idempotency verification |
| 4. Aggressive reconnection | Phase 2: Reliability | Test 3: Rapid retry loop + monitoring |
| 5. Stale mounts | Phase 2: Error recovery | Test 4: Stale mount cleanup |
| 6. Device discovery races | Phase 1: Foundation | CSI sanity tests + Test 6: Kernel fault injection |
| 7. nvme-cli exit codes | Phase 1: Foundation | Unit tests with mock command outputs |
| 8. Manual cleanup needed | Phase 2: Robustness | Stress testing with rapid connect/disconnect |
| 9. Noisy logging | Phase 3: Polish | Log volume analysis in production |
| 10. No metrics | Phase 3: Observability | Prometheus metric validation |

---

## Recovery Strategies

When pitfalls occur despite prevention:

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Orphaned subsystem | LOW | Automatic reconnection in driver (implemented in bc90a9b) |
| Controller renumbering | LOW | Scan by NQN, not by device number (implemented in 74ce4d9) |
| Non-idempotent staging | MEDIUM | Kubelet retry eventually succeeds if idempotency fixed |
| Aggressive reconnection | HIGH | Requires node restart or manual kill of stuck processes |
| Stale mounts | MEDIUM | Manual `umount -l` or node reboot |
| Device discovery race | LOW | Automatic retry with backoff |
| nvme-cli exit codes | LOW | Automatic fallback to alternate cleanup methods |
| Manual cleanup needed | MEDIUM | Operator intervention via sysfs |
| Noisy logging | LOW | Filter logs, no system impact |
| No metrics | N/A | Operational blind spot, no recovery needed |

---

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces:

- [ ] **Connection handling:** Often missing orphaned subsystem detection - verify `IsConnected() == true` followed by `GetDevicePath()` check
- [ ] **Device discovery:** Often missing NVMe-oF naming support - verify both `nvmeXnY` and `nvmeXcYnZ` paths handled
- [ ] **NodeStageVolume:** Often missing idempotency checks - verify second call with same params returns success
- [ ] **Error handling:** Often missing nvme-cli output parsing - verify not just exit code but output content checked
- [ ] **Cleanup:** Often missing orphaned mount detection - verify node plugin startup scans for stale mounts
- [ ] **Backoff:** Often missing exponential backoff - verify connection attempts don't retry immediately in tight loop
- [ ] **Testing:** Often missing reconnection scenarios - verify at least Tests 1-5 above are automated

---

## Architectural Recommendations

Based on pitfalls analysis, recommended architecture patterns:

### 1. Separate Connection State Machine
Don't mix connection logic with mount logic. Use state machine:

```
DISCONNECTED -> CONNECTING -> CONNECTED -> STAGED -> PUBLISHED
                    |            |            |
                    v            v            v
                 FAILED       ORPHANED      STALE
```

### 2. NQN-Based Resource Tracking
Never use device numbers as identifiers. Always use NQN as primary key for:
- Connection tracking
- Backoff state
- Metrics labels
- Cleanup operations

### 3. Health Check Loop
Run background goroutine to detect:
- Orphaned subsystems (subsystem exists, no device)
- Stale mounts (mount exists, device gone)
- Stuck operations (operation exceeds 2x timeout)

### 4. Graceful Degradation
When target unreachable:
1. Don't fail immediately
2. Return `codes.Unavailable` to kubelet
3. Let kubelet's backoff handle retry timing
4. Eventually fail after reasonable timeout (5-10 minutes)

---

## Sources

**Official Documentation:**
- [Red Hat: Configuring NVMe over fabrics using NVMe/TCP](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_storage_devices/configuring-nvme-over-fabrics-using-nvme-tcp_managing-storage-devices)
- [Linux Kernel: NVMe Fault Injection](https://docs.kernel.org/fault-injection/nvme-fault-injection.html)
- [Linux Kernel: NVMe Multipath](https://docs.kernel.org/admin-guide/nvme-multipath.html)
- [ArchWiki: NVMe over Fabrics](https://wiki.archlinux.org/title/NVMe_over_Fabrics)

**CSI Driver Issues (Real Production Bugs):**
- [AWS EBS CSI: NodeStage might not always be idempotent](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/1076)
- [AWS EBS CSI: Fix NodeStageVolume returning prematurely](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/pull/176)
- [AWS EBS CSI: Race condition/timeout issues when mounting PV](https://github.com/kubernetes/kubernetes/issues/76568)
- [Dell CSM: PowerStore CSI driver NVME TCP connectivity issues](https://github.com/dell/csm/issues/496)
- [Mayastor: NVMe device mismatch issue](https://github.com/openebs/mayastor/issues/1777)
- [IBM PowerVS: NodeStageVolume called again after success](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/issues/55)
- [Ceph CSI: NodeStageVolume fails if xfs_repair returns error](https://github.com/ceph/ceph-csi/issues/859)
- [Azure Files CSI: Stacked mounts prevents unmounting](https://github.com/kubernetes-sigs/azurefile-csi-driver/issues/1137)
- [Kubernetes: CSI SetUpAt does not retry when check fails](https://github.com/kubernetes/kubernetes/issues/112969)
- [Red Hat Bugzilla: Stale mount/stage in different node](https://bugzilla.redhat.com/show_bug.cgi?id=1745776)

**NVMe-oF Specific Issues:**
- [nvme-cli: nvme connect-all reports "already connected"](https://github.com/linux-nvme/nvme-cli/issues/1505)
- [nvme-cli: disconnect doesn't remove all controllers](https://github.com/linux-nvme/nvme-cli/issues/499)
- [nvme-cli: nvme naming/association wrong](https://github.com/linux-nvme/nvme-cli/issues/510)
- [nvme-cli: wrong naming of controllers](https://github.com/linux-nvme/nvme-cli/issues/455)
- [libnvme: Allow disconnect of incomplete connections](https://github.com/linux-nvme/libnvme/issues/431)
- [nvme-cli: Unable to disconnect from NVMeoF subsystem](https://github.com/linux-nvme/nvme-cli/issues/2603)

**Device Naming Issues:**
- [Red Hat: NVMe controllers and namespace name inconsistency](https://access.redhat.com/solutions/6967589)
- [Red Hat: How to make block device NVMe assigned names persistent](https://access.redhat.com/solutions/4763241)
- [Understanding plain Linux NVMe device names](https://utcc.utoronto.ca/~cks/space/blog/linux/NVMeDeviceNames)
- [Arch Linux: NVME SDD changes name after each reboot](https://bbs.archlinux.org/viewtopic.php?id=300293)
- [Google Cloud: Best practice for persistent device names](https://cloud.google.com/compute/docs/disks/set-persistent-device-name-in-linux-vm)

**Community Implementations:**
- [SPDK CSI Driver](https://github.com/spdk/spdk-csi)
- [Kubernetes CSI NVMf Driver](https://github.com/kubernetes-csi/csi-driver-nvmf)
- [Simplyblock CSI](https://github.com/simplyblock/simplyblock-csi)
- [KumoScale CSI](https://github.com/KioxiaAmerica/kumoscale-csi)

**RDS-CSI Specific:**
- Commit bc90a9b: "fix(nvme): handle orphaned subsystems in ConnectWithContext"
- Commit 74ce4d9: "fix(nvme): handle NVMe-oF namespace device naming correctly"
- Commit 10b237b: "fix(nvme): handle NVMe-oF namespace device naming correctly"

---

*Confidence Level: HIGH*
- Critical pitfalls verified through codebase analysis and recent bug fixes
- Community patterns cross-referenced across 3+ independent CSI implementations
- Testing strategies validated against Linux kernel documentation
- All major pitfalls have documented sources and reproduction scenarios
