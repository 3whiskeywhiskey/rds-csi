# Phase 2: Stale Mount Detection and Recovery - Research

**Researched:** 2026-01-30
**Domain:** Linux mount management, /proc/mounts parsing, Kubernetes event posting, CSI error codes
**Confidence:** HIGH

## Summary

This phase adds automatic detection and recovery for stale mounts caused by NVMe-oF controller renumbering after reconnections. The driver must detect when a mount's device path no longer matches the NQN-resolved device, recover transparently when possible, and report failures via Kubernetes events.

The research confirms the standard approach: parse `/proc/mounts` (or `/proc/self/mountinfo`) to get the current mount device, resolve symlinks to compare device paths, and use lazy unmount (`umount -l`) as a fallback for stuck mounts. For in-use detection, scanning `/proc/*/fd` is the reliable Linux approach. Kubernetes events are posted via `client-go`'s EventRecorder interface, which is already available in the codebase.

Key findings:
1. **Mount parsing via moby/sys/mountinfo or k8s.io/utils/mount** - Both packages provide reliable `/proc/mountinfo` parsing with the `Source` field containing the mount device
2. **Symlink resolution via filepath.EvalSymlinks** - Standard Go approach, but must handle edge cases with `/dev/disk/by-*` symlinks
3. **In-use detection via /proc/*/fd scanning** - Pure Go approach without requiring `fuser` binary
4. **Lazy unmount semantics are well-defined** - Immediate detachment from namespace, deferred cleanup when file handles close
5. **Event posting via client-go EventRecorder** - Pattern already established in orphan_reconciler.go

**Primary recommendation:** Use `github.com/moby/sys/mountinfo` for mount parsing (well-maintained, used by Docker/containerd), implement in-use detection via `/proc/*/fd` scanning, and follow the established client-go EventRecorder pattern.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/moby/sys/mountinfo` | latest | Parse /proc/mountinfo | Used by Docker/containerd, well-maintained, includes filter functions |
| `k8s.io/client-go v0.28.0` | v0.28.0 | Kubernetes event posting | Already in go.mod, standard for K8s operations |
| `k8s.io/client-go/tools/record` | v0.28.0 | EventRecorder interface | Standard K8s event posting mechanism |
| Go stdlib `filepath` | Go 1.24 | EvalSymlinks for device path comparison | Standard for symlink resolution |
| Go stdlib `os` | Go 1.24 | /proc/*/fd scanning, file operations | Standard for file I/O |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `k8s.io/api/core/v1` | v0.28.0 | Event types, PVC references | Already in go.mod |
| `k8s.io/apimachinery` | v0.28.0 | ObjectReference for events | Already in go.mod |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `moby/sys/mountinfo` | `k8s.io/utils/mount` | k8s.io/utils/mount has `GetDeviceNameFromMount()` but is being deprecated in favor of kubernetes/mount-utils |
| `/proc/*/fd` scan | `fuser` command | fuser requires external binary, /proc scan is pure Go |
| Custom /proc/mounts parser | Standard library only | moby/sys/mountinfo handles edge cases, filtering, optional fields |

**No additional dependencies required.** `moby/sys/mountinfo` is a small module; alternatively, use existing `k8s.io/utils` patterns.

## Architecture Patterns

### Recommended Project Structure
```
pkg/
├── mount/
│   ├── mount.go           # Existing Mounter interface
│   ├── mount_test.go      # Existing tests
│   ├── stale.go           # NEW: Stale mount detection
│   ├── stale_test.go      # NEW: Stale detection tests
│   └── procmounts.go      # NEW: /proc/mounts parsing utilities
├── driver/
│   ├── node.go            # MODIFY: Add stale detection to CSI operations
│   ├── events.go          # NEW: Kubernetes event posting
│   └── events_test.go     # NEW: Event posting tests
```

### Pattern 1: Mount Info Parser
**What:** Parse `/proc/self/mountinfo` to get current mount device for a given path.
**When to use:** Before any CSI node operation to check mount validity.
**Example:**
```go
// Source: github.com/moby/sys/mountinfo pattern
import (
    "github.com/moby/sys/mountinfo"
)

// GetMountDevice returns the source device for a given mount path
func GetMountDevice(mountPath string) (string, error) {
    mounts, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(mountPath))
    if err != nil {
        return "", fmt.Errorf("failed to get mounts: %w", err)
    }
    if len(mounts) == 0 {
        return "", fmt.Errorf("mount not found: %s", mountPath)
    }
    return mounts[0].Source, nil
}
```

### Pattern 2: Stale Mount Detection
**What:** Compare mount device with current NQN-resolved device.
**When to use:** At start of NodePublishVolume, NodeUnpublishVolume, NodeGetVolumeStats.
**Example:**
```go
// Source: Recommended pattern based on research
type StaleMountChecker struct {
    resolver    *nvme.DeviceResolver
    getMountDev func(path string) (string, error) // Injected for testing
}

// IsMountStale checks if mount points to wrong device or missing device
func (c *StaleMountChecker) IsMountStale(mountPath, nqn string) (bool, string, error) {
    // Get current mount device
    mountDevice, err := c.getMountDev(mountPath)
    if err != nil {
        // Mount not found = definitely stale
        return true, "mount_not_found", nil
    }

    // Check if mount device still exists
    resolvedMount, err := filepath.EvalSymlinks(mountDevice)
    if err != nil {
        // Device path can't be resolved (device disappeared)
        return true, "device_disappeared", nil
    }

    // Resolve NQN to current device path
    currentDevice, err := c.resolver.ResolveDevicePath(nqn)
    if err != nil {
        return false, "", fmt.Errorf("failed to resolve NQN: %w", err)
    }

    resolvedCurrent, err := filepath.EvalSymlinks(currentDevice)
    if err != nil {
        return false, "", fmt.Errorf("failed to resolve current device: %w", err)
    }

    // Compare resolved paths
    if resolvedMount != resolvedCurrent {
        return true, "device_path_mismatch", nil
    }

    return false, "", nil
}
```

### Pattern 3: Recovery with Retry
**What:** Attempt recovery with exponential backoff.
**When to use:** When stale mount detected.
**Example:**
```go
// Source: Recommended pattern from CONTEXT.md decisions
type RecoveryConfig struct {
    MaxAttempts       int           // 3
    InitialBackoff    time.Duration // 1s
    BackoffMultiplier float64       // 2.0
    NormalUnmountWait time.Duration // 10s
}

func (r *MountRecoverer) Recover(ctx context.Context, mountPath, nqn string) error {
    backoff := r.config.InitialBackoff

    for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
        err := r.attemptRecovery(ctx, mountPath, nqn)
        if err == nil {
            return nil
        }

        klog.Warningf("Recovery attempt %d/%d failed: %v", attempt, r.config.MaxAttempts, err)

        if attempt < r.config.MaxAttempts {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(backoff):
                backoff = time.Duration(float64(backoff) * r.config.BackoffMultiplier)
            }
        }
    }

    return fmt.Errorf("recovery failed after %d attempts", r.config.MaxAttempts)
}
```

### Pattern 4: Force Unmount with In-Use Check
**What:** Escalate to lazy unmount only if normal unmount fails and mount not in use.
**When to use:** During recovery when normal unmount fails.
**Example:**
```go
// Source: Recommended pattern based on umount(8) semantics
func (m *Mounter) ForceUnmount(path string, timeout time.Duration) error {
    // Try normal unmount first
    if err := m.Unmount(path); err == nil {
        return nil
    }

    // Wait for normal unmount with timeout
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if mounted, _ := m.IsLikelyMountPoint(path); !mounted {
            return nil
        }
        time.Sleep(500 * time.Millisecond)
    }

    // Check if mount is in use before lazy unmount
    inUse, pids, err := m.IsMountInUse(path)
    if err != nil {
        klog.Warningf("Failed to check if mount in use: %v", err)
    }
    if inUse {
        return fmt.Errorf("mount %s is in use by PIDs %v, refusing force unmount", path, pids)
    }

    // Escalate to lazy unmount
    klog.Warningf("Escalating to lazy unmount for %s", path)
    cmd := exec.Command("umount", "-l", path)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("lazy unmount failed: %w, output: %s", err, string(output))
    }

    return nil
}
```

### Pattern 5: Kubernetes Event Posting
**What:** Post events to PVC when mount failures or recovery actions occur.
**When to use:** On unrecoverable failures, per CONTEXT.md decisions.
**Example:**
```go
// Source: client-go EventRecorder pattern
import (
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes"
    typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
    "k8s.io/client-go/tools/record"
)

type EventPoster struct {
    recorder  record.EventRecorder
    clientset kubernetes.Interface
}

func NewEventPoster(clientset kubernetes.Interface, scheme *runtime.Scheme) *EventPoster {
    eventBroadcaster := record.NewBroadcaster()
    eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
        Interface: clientset.CoreV1().Events(""),
    })

    return &EventPoster{
        recorder:  eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "rds-csi-node"}),
        clientset: clientset,
    }
}

// PostMountFailure posts a Warning event to the PVC
func (p *EventPoster) PostMountFailure(ctx context.Context, pvcNamespace, pvcName, volumeID, nodeName, reason, message string) error {
    pvc, err := p.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
    if err != nil {
        return fmt.Errorf("failed to get PVC: %w", err)
    }

    p.recorder.Event(pvc, corev1.EventTypeWarning, reason, message)
    return nil
}
```

### Anti-Patterns to Avoid
- **Checking mount validity only on errors:** Check proactively on every operation to catch stale mounts early
- **Force unmount without checking in-use:** Risk of data loss if processes have open handles
- **Silent recovery without logging:** Operators need visibility even on successful recovery
- **Posting events on successful recovery:** Creates noise; only post on failures per CONTEXT.md
- **Using `fuser` binary for in-use detection:** External dependency; /proc/*/fd scan is pure Go

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Parse /proc/mounts | Custom parser | `moby/sys/mountinfo` | Handles optional fields, separator tokens, escape sequences |
| Symlink resolution | Custom readlink loop | `filepath.EvalSymlinks` | Handles circular symlinks, path limits |
| Event broadcasting | Direct API calls | `client-go/tools/record` | Handles rate limiting, event aggregation, retry |
| Device path comparison | String comparison | Resolved path comparison | `/dev/nvme0n1` vs `/dev/disk/by-id/...` both point to same device |

**Key insight:** Mount parsing and event posting have many edge cases. Use well-tested libraries.

## Common Pitfalls

### Pitfall 1: Mount Source vs Device Path Confusion
**What goes wrong:** Mount source may be a symlink (e.g., `/dev/disk/by-id/nvme-...`) while NQN resolves to `/dev/nvme0n1`.
**Why it happens:** Linux allows mounting via symlinks; mount table preserves original path.
**How to avoid:**
1. Always resolve symlinks with `filepath.EvalSymlinks` before comparison
2. Compare resolved paths, not original paths
3. Handle errors from EvalSymlinks (device may have disappeared)
**Warning signs:** "Stale mount" false positives despite device being correct.

### Pitfall 2: Lazy Unmount Semantics
**What goes wrong:** Lazy unmount returns success but mount still appears in /proc/mounts.
**Why it happens:** Lazy unmount detaches from namespace immediately but defers actual unmount.
**How to avoid:**
1. Don't poll /proc/mounts after lazy unmount expecting it to disappear
2. Understand that mount may linger until all file handles close
3. If you need mount gone, check for in-use first and refuse if busy
**Warning signs:** "Mount still exists" errors after successful lazy unmount.

### Pitfall 3: In-Use Detection Race Conditions
**What goes wrong:** Mount appears not in use, then becomes in use before unmount.
**Why it happens:** Process opens file between check and unmount.
**How to avoid:**
1. Accept this is inherently racy
2. Treat in-use check as advisory, not authoritative
3. Handle EBUSY from unmount gracefully
**Warning signs:** Intermittent "device busy" errors despite in-use check passing.

### Pitfall 4: Event Flood on Transient Failures
**What goes wrong:** Every retry posts an event, flooding the event stream.
**Why it happens:** Events posted on each failure, not just final failure.
**How to avoid:**
1. Post events only on final unrecoverable failure (per CONTEXT.md)
2. Aggregate retries in single event message: "Failed after 3 attempts"
3. Include timestamps and retry counts in event message
**Warning signs:** Hundreds of events for a single mount issue.

### Pitfall 5: PVC Lookup Failure
**What goes wrong:** Can't post event because can't find PVC.
**Why it happens:** PVC may be deleted while volume still mounted; namespace might be terminating.
**How to avoid:**
1. Handle PVC not found gracefully - log warning, don't fail operation
2. Store PVC reference in volume context during NodeStageVolume
3. Accept that some events may not be posted
**Warning signs:** "Failed to post event" errors masking actual mount issues.

### Pitfall 6: /proc/*/fd Permission Errors
**What goes wrong:** Can't read /proc/<pid>/fd for all processes.
**Why it happens:** CSI node plugin may not have permission to read all processes' fd directories.
**How to avoid:**
1. Skip processes where permission denied
2. Log at debug level, not error level
3. Return "unknown" if can't determine in-use status
**Warning signs:** "Permission denied" errors when checking mount usage.

## Code Examples

Verified patterns from official sources:

### Parsing /proc/mountinfo with moby/sys/mountinfo
```go
// Source: github.com/moby/sys/mountinfo documentation
import "github.com/moby/sys/mountinfo"

func getMountInfo(mountPath string) (*mountinfo.Info, error) {
    // Use SingleEntryFilter for specific path lookup
    mounts, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(mountPath))
    if err != nil {
        return nil, err
    }
    if len(mounts) == 0 {
        return nil, fmt.Errorf("no mount found for path: %s", mountPath)
    }
    // Info.Source contains the mount device
    return mounts[0], nil
}
```

### In-Use Detection via /proc/*/fd
```go
// Source: Standard Linux /proc filesystem approach
func isMountInUse(mountPath string) (bool, []int, error) {
    // Resolve mount path to canonical form
    resolvedPath, err := filepath.EvalSymlinks(mountPath)
    if err != nil {
        return false, nil, err
    }

    // Scan /proc for all processes
    procDir, err := os.Open("/proc")
    if err != nil {
        return false, nil, err
    }
    defer procDir.Close()

    entries, err := procDir.Readdirnames(-1)
    if err != nil {
        return false, nil, err
    }

    var inUsePIDs []int
    for _, entry := range entries {
        // Skip non-numeric entries
        pid, err := strconv.Atoi(entry)
        if err != nil {
            continue
        }

        fdDir := filepath.Join("/proc", entry, "fd")
        fds, err := os.ReadDir(fdDir)
        if err != nil {
            // Permission denied is normal for other users' processes
            continue
        }

        for _, fd := range fds {
            link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
            if err != nil {
                continue
            }

            // Check if fd points to file under mount path
            if strings.HasPrefix(link, resolvedPath) || link == resolvedPath {
                inUsePIDs = append(inUsePIDs, pid)
                break // One fd is enough to mark PID as using mount
            }
        }
    }

    return len(inUsePIDs) > 0, inUsePIDs, nil
}
```

### Lazy Unmount
```go
// Source: umount(8) man page, MNT_DETACH behavior
func lazyUnmount(path string) error {
    cmd := exec.Command("umount", "-l", path)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("lazy unmount failed: %w, output: %s", err, string(output))
    }
    return nil
}
```

### Event Recorder Setup
```go
// Source: client-go/tools/record documentation
import (
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/kubernetes/scheme"
    typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
    "k8s.io/client-go/tools/record"
)

func setupEventRecorder(clientset kubernetes.Interface) record.EventRecorder {
    eventBroadcaster := record.NewBroadcaster()
    eventBroadcaster.StartLogging(klog.Infof)
    eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
        Interface: clientset.CoreV1().Events(""),
    })

    return eventBroadcaster.NewRecorder(
        scheme.Scheme,
        corev1.EventSource{Component: "rds-csi-node"},
    )
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Parse /proc/mounts manually | Use moby/sys/mountinfo | Always best practice | Handles escape sequences, optional fields |
| shell out to `fuser` | Scan /proc/*/fd directly | Always best practice | No external dependency, testable |
| k8s.io/kubernetes/pkg/util/mount | k8s.io/utils/mount or kubernetes/mount-utils | 2020+ | Better maintained, smaller dependency |

**Deprecated/outdated:**
- `k8s.io/kubernetes/pkg/util/mount`: Internal Kubernetes package, not for external use
- Manual /proc/mounts parsing: Error-prone, misses edge cases

## Open Questions

Things that couldn't be fully resolved:

1. **Event Rate Limiting**
   - What we know: client-go EventRecorder has built-in rate limiting
   - What's unclear: Exact rate limits, whether they're sufficient for our use case
   - Recommendation: Use EventRecorder's defaults, monitor in production

2. **/proc/*/fd Permission Requirements**
   - What we know: Node plugin runs as root in privileged container
   - What's unclear: Whether all /proc entries are accessible in all K8s configurations
   - Recommendation: Gracefully handle permission errors, assume in-use if can't determine

3. **moby/sys/mountinfo vs k8s.io/utils/mount**
   - What we know: Both work; moby is more actively maintained for mount parsing
   - What's unclear: Whether to add new dependency or use k8s.io/utils which is already indirect
   - Recommendation: Use moby/sys/mountinfo for cleaner API, or implement minimal parser

## Sources

### Primary (HIGH confidence)
- [moby/sys/mountinfo](https://pkg.go.dev/github.com/moby/sys/mountinfo) - Mount info parsing API
- [k8s.io/client-go/tools/record](https://pkg.go.dev/k8s.io/client-go/tools/record) - Event recorder documentation
- [umount(8) man page](https://man7.org/linux/man-pages/man8/umount.8.html) - Lazy unmount semantics
- [umount(2) man page](https://man7.org/linux/man-pages/man2/umount.2.html) - MNT_DETACH behavior
- Go stdlib `filepath.EvalSymlinks` - Symlink resolution

### Secondary (MEDIUM confidence)
- [k8s.io/utils/mount](https://pkg.go.dev/k8s.io/utils/mount) - GetDeviceNameFromMount pattern
- [kubernetes/mount-utils](https://github.com/kubernetes/mount-utils) - Current mount utilities
- CSI spec error codes - gRPC status codes for node operations
- Existing codebase patterns - orphan_reconciler.go for client-go usage

### Tertiary (LOW confidence)
- WebSearch results on CSI stale mount handling - No standard pattern found across drivers
- WebSearch results on in-use detection - Multiple approaches, /proc scan is most portable

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Libraries are well-documented, already in use or well-maintained
- Architecture: HIGH - Patterns based on existing codebase (resolver, security logger) and standard K8s patterns
- Pitfalls: MEDIUM - Based on research and known Linux behaviors; some need production validation

**Research date:** 2026-01-30
**Valid until:** 2026-03-01 (30 days - stable domain, unlikely to change)
