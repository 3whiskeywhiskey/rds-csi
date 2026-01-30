# Phase 1: Foundation - Device Path Resolution - Research

**Researched:** 2026-01-30
**Domain:** Linux NVMe sysfs, device path resolution, Go caching patterns
**Confidence:** HIGH

## Summary

This phase addresses a critical reliability problem in the RDS CSI driver: NVMe device paths (e.g., `/dev/nvme0n1`) can change after controller reconnections or system events, but the NQN (NVMe Qualified Name) remains constant. The driver must resolve device paths dynamically using NQN lookups instead of storing hardcoded paths.

The research confirms that Linux exposes NVMe subsystem information through sysfs at `/sys/class/nvme/nvmeN/subsysnqn`, and block devices can be discovered via `/sys/class/block/`. The standard approach is to scan sysfs, match by NQN, and resolve the corresponding block device. This is exactly what the existing `GetDevicePath()` function does, but it lacks caching, TTL validation, and proper orphan detection.

Key findings:
1. **Sysfs scanning is the correct approach** - NQN is exposed at `/sys/class/nvme/nvmeN/subsysnqn`
2. **Orphaned subsystems are real** - Subsystems can appear connected but have no active controllers/devices
3. **Caching with TTL is essential** - Sysfs scanning on every call is expensive; use TTL-based caching with validation
4. **Device paths change on reconnection** - Never store device paths; always resolve from NQN

**Primary recommendation:** Refactor `GetDevicePath()` into a `DeviceResolver` component with NQN-based caching, TTL validation, and orphan detection.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `os` | Go 1.24 | Sysfs file reading | Standard for file I/O |
| Go stdlib `filepath` | Go 1.24 | Path operations, globbing | Standard for path manipulation |
| Go stdlib `sync` | Go 1.24 | Thread-safe caching | Standard for concurrent access |
| Go stdlib `time` | Go 1.24 | TTL management | Standard for time operations |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `k8s.io/klog/v2` | v2.x | Structured logging | Already used in project |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `sync.Map` | `map` + `sync.RWMutex` | `sync.Map` is simpler but harder to implement TTL cleanup; use map + mutex |
| Custom sysfs scanner | `github.com/prometheus/procfs` | procfs is for /proc, not /sys; custom scanner is correct |

**No additional dependencies required.** All implementation uses Go standard library.

## Architecture Patterns

### Recommended Project Structure
```
pkg/
├── nvme/
│   ├── nvme.go           # Connector interface (existing)
│   ├── resolver.go       # NEW: DeviceResolver with caching
│   ├── sysfs.go          # NEW: Sysfs scanning functions
│   ├── resolver_test.go  # NEW: Resolver unit tests
│   └── sysfs_test.go     # NEW: Sysfs scanning tests
```

### Pattern 1: DeviceResolver with TTL Cache
**What:** A dedicated resolver component that caches NQN-to-device-path mappings with TTL validation.
**When to use:** Any operation needing to resolve device path from NQN.
**Example:**
```go
// Source: Recommended pattern based on research
type DeviceResolver struct {
    cache     map[string]*cacheEntry
    mu        sync.RWMutex
    ttl       time.Duration
    sysfsRoot string // "/sys" in production, test dir in tests
}

type cacheEntry struct {
    devicePath string
    resolvedAt time.Time
    validated  bool
}

func (r *DeviceResolver) ResolveDevicePath(nqn string) (string, error) {
    r.mu.RLock()
    entry, exists := r.cache[nqn]
    r.mu.RUnlock()

    if exists && time.Since(entry.resolvedAt) < r.ttl {
        // Validate device still exists
        if _, err := os.Stat(entry.devicePath); err == nil {
            return entry.devicePath, nil
        }
        // Device gone - invalidate and re-resolve
    }

    // Scan sysfs for device
    devicePath, err := r.scanSysfs(nqn)
    if err != nil {
        return "", err
    }

    r.mu.Lock()
    r.cache[nqn] = &cacheEntry{
        devicePath: devicePath,
        resolvedAt: time.Now(),
        validated:  true,
    }
    r.mu.Unlock()

    return devicePath, nil
}
```

### Pattern 2: Sysfs Scanning
**What:** Scan `/sys/class/nvme/` for controllers, read `subsysnqn`, find matching device.
**When to use:** Cache miss or TTL expiration.
**Example:**
```go
// Source: Based on existing GetDevicePath() and Linux sysfs structure
func (r *DeviceResolver) scanSysfs(targetNQN string) (string, error) {
    controllers, err := filepath.Glob(filepath.Join(r.sysfsRoot, "class/nvme/nvme*"))
    if err != nil {
        return "", fmt.Errorf("failed to scan nvme controllers: %w", err)
    }

    for _, controller := range controllers {
        // Read subsysnqn
        nqnBytes, err := os.ReadFile(filepath.Join(controller, "subsysnqn"))
        if err != nil {
            continue
        }

        nqn := strings.TrimSpace(string(nqnBytes))
        if nqn != targetNQN {
            continue
        }

        // Found matching controller - find block device
        return r.findBlockDevice(controller)
    }

    return "", fmt.Errorf("no device found for NQN: %s", targetNQN)
}
```

### Pattern 3: Orphan Detection
**What:** Detect when subsystem appears connected but has no usable device.
**When to use:** Before any device operation.
**Example:**
```go
// Source: Research on NVMe-oF orphaned subsystems
func (r *DeviceResolver) IsOrphanedSubsystem(nqn string) (bool, error) {
    // Check if subsystem exists in nvme list-subsys output
    connected, err := r.connector.IsConnected(nqn)
    if err != nil {
        return false, err
    }

    if !connected {
        return false, nil // Not connected, not orphaned
    }

    // Subsystem appears connected - check if we can find device
    _, err = r.scanSysfs(nqn)
    if err != nil {
        // Subsystem connected but no device = orphaned
        return true, nil
    }

    return false, nil
}
```

### Anti-Patterns to Avoid
- **Storing device paths in staging metadata:** Device paths change on reconnection. Store NQN, resolve path on demand.
- **Assuming device path stability:** Never cache device paths without TTL and validation.
- **Parsing nvme-cli output for device discovery:** Use sysfs directly; nvme-cli output format can vary.
- **Ignoring controller renumbering:** `/dev/nvme0n1` today might be `/dev/nvme1n1` after reconnection.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| NQN validation | Custom regex | `utils.ValidateNQN()` | Already exists, prevents command injection |
| Sysfs scanning | Shell scripts | Go filepath.Glob | Portable, testable, no exec overhead |
| TTL cache | External library | `sync.RWMutex` + `map` + `time.Duration` | Simple enough, no dependency needed |

**Key insight:** The sysfs interface is stable and well-documented. Direct file I/O is preferable to parsing nvme-cli output, which can vary between versions.

## Common Pitfalls

### Pitfall 1: NVMe-oF Device Naming Variations
**What goes wrong:** NVMe-oF devices may appear as `nvmeXnY` (subsystem-based) or `nvmeXcYnZ` (controller-based), causing device lookup failures.
**Why it happens:** NVMe multipath and NVMe-oF use different naming conventions than local NVMe.
**How to avoid:**
1. Prefer `nvmeXnY` format when available (subsystem-based)
2. Fall back to `nvmeXcYnZ` if simple format not found
3. Check both `/sys/class/block/` and controller subdirectories
**Warning signs:** Tests pass locally but fail on NVMe-oF targets.

### Pitfall 2: Race Between Connect and Device Appearance
**What goes wrong:** Device path resolution fails immediately after `nvme connect` because device hasn't appeared yet.
**Why it happens:** udev and kernel need time to create device nodes after connection.
**How to avoid:**
1. Use `WaitForDevice()` with appropriate timeout after connect
2. Implement retry logic in `ResolveDevicePath()` with backoff
3. Verify device node exists with `os.Stat()` before returning
**Warning signs:** Intermittent "device not found" errors after successful connect.

### Pitfall 3: Cache Staleness After Disconnect/Reconnect
**What goes wrong:** Cached device path points to old device after reconnection.
**Why it happens:** Device renumbered on reconnect, but cache still has old path.
**How to avoid:**
1. Always validate cached path with `os.Stat()` before use
2. Use short TTL (5-30 seconds recommended)
3. Invalidate cache explicitly on disconnect
**Warning signs:** I/O errors despite device appearing healthy.

### Pitfall 4: Orphaned Subsystem Detection Failure
**What goes wrong:** Driver thinks volume is connected because NQN appears in `nvme list-subsys`, but no device exists.
**Why it happens:** Controller lost connection but subsystem entry persists until timeout.
**How to avoid:**
1. Check for orphan condition: subsystem exists but no device path
2. On orphan detection, force disconnect and reconnect
3. Log warning for operator visibility
**Warning signs:** Pods stuck in ContainerCreating with "device not found" errors.

### Pitfall 5: Testing Sysfs Scanning
**What goes wrong:** Tests require root access or mock filesystem fails.
**Why it happens:** Sysfs scanning hits real `/sys` directory.
**How to avoid:**
1. Make sysfs root path configurable (`sysfsRoot` field)
2. Create mock sysfs structure in temp directory for tests
3. Use `t.TempDir()` for automatic cleanup
**Warning signs:** Tests skipped or marked as integration-only.

## Code Examples

Verified patterns from research and existing codebase:

### Reading subsysnqn from sysfs
```go
// Source: Linux sysfs structure, existing GetDevicePath() pattern
func readSubsysNQN(controllerPath string) (string, error) {
    nqnPath := filepath.Join(controllerPath, "subsysnqn")
    data, err := os.ReadFile(nqnPath)
    if err != nil {
        return "", fmt.Errorf("failed to read subsysnqn: %w", err)
    }
    return strings.TrimSpace(string(data)), nil
}
```

### Finding block device for NVMe-oF controller
```go
// Source: Existing GetDevicePath() with improvements from research
func findBlockDevice(controllerPath, sysfsRoot string) (string, error) {
    controllerName := filepath.Base(controllerPath) // e.g., "nvme0"

    // Method 1: Look for namespaces under controller
    namespaces, _ := filepath.Glob(filepath.Join(controllerPath, "nvme*n*"))
    for _, ns := range namespaces {
        nsName := filepath.Base(ns)
        devicePath := "/dev/" + nsName
        if _, err := os.Stat(devicePath); err == nil {
            return devicePath, nil
        }

        // For nvmeXcYnZ, also try nvmeXnZ (subsystem-based)
        if strings.Contains(nsName, "c") {
            var subsys, ctrl, namespace int
            if _, err := fmt.Sscanf(nsName, "nvme%dc%dn%d", &subsys, &ctrl, &namespace); err == nil {
                altPath := fmt.Sprintf("/dev/nvme%dn%d", subsys, namespace)
                if _, err := os.Stat(altPath); err == nil {
                    return altPath, nil
                }
            }
        }
    }

    // Method 2: Check /sys/class/block for controller-named devices
    blockDevices, _ := filepath.Glob(filepath.Join(sysfsRoot, "class/block", controllerName+"n*"))
    for _, bd := range blockDevices {
        deviceName := filepath.Base(bd)
        // Prefer simple nvmeXnY over nvmeXcYnZ
        if !strings.Contains(deviceName, "c") {
            return "/dev/" + deviceName, nil
        }
    }

    // Method 3: Return any matching device
    if len(blockDevices) > 0 {
        return "/dev/" + filepath.Base(blockDevices[0]), nil
    }

    return "", fmt.Errorf("no block device found for controller %s", controllerName)
}
```

### TTL Cache with Validation
```go
// Source: Go caching patterns research
type DeviceCache struct {
    entries map[string]*deviceEntry
    mu      sync.RWMutex
    ttl     time.Duration
}

type deviceEntry struct {
    path       string
    resolvedAt time.Time
}

func (c *DeviceCache) Get(nqn string) (string, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, exists := c.entries[nqn]
    if !exists {
        return "", false
    }

    // Check TTL
    if time.Since(entry.resolvedAt) > c.ttl {
        return "", false
    }

    // Validate device still exists
    if _, err := os.Stat(entry.path); err != nil {
        return "", false
    }

    return entry.path, true
}

func (c *DeviceCache) Set(nqn, path string) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.entries[nqn] = &deviceEntry{
        path:       path,
        resolvedAt: time.Now(),
    }
}

func (c *DeviceCache) Invalidate(nqn string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    delete(c.entries, nqn)
}
```

### Orphan Detection
```go
// Source: Research on NVMe-oF subsystem behavior
func (r *DeviceResolver) DetectOrphan(nqn string) (bool, error) {
    // Step 1: Check if subsystem appears connected
    connected, err := r.isSubsystemPresent(nqn)
    if err != nil {
        return false, err
    }

    if !connected {
        return false, nil // Not present = not orphaned
    }

    // Step 2: Try to find actual device
    devicePath, err := r.scanSysfs(nqn)
    if err != nil {
        // Present but no device = orphaned
        klog.Warningf("Orphaned subsystem detected: NQN %s appears connected but has no device", nqn)
        return true, nil
    }

    // Step 3: Verify device is accessible
    if _, err := os.Stat(devicePath); err != nil {
        return true, nil
    }

    return false, nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Hardcoded `/dev/nvme0n1` paths | NQN-based sysfs lookup | Always best practice | Reliability across reconnections |
| `nvme list` output parsing | Direct sysfs file reading | Always best practice | No nvme-cli dependency for discovery |
| Store device path in PV | Store NQN, resolve on demand | Always best practice | Handles controller renumbering |

**Deprecated/outdated:**
- Parsing `nvme list` JSON output for device discovery: Direct sysfs is faster and more reliable
- Assuming device numbering stability: Controller renumbering is common with NVMe-oF

## Open Questions

Things that couldn't be fully resolved:

1. **Optimal TTL Value**
   - What we know: TTL should be short enough to catch reconnections, long enough to avoid scan overhead
   - What's unclear: Exact optimal value depends on workload patterns
   - Recommendation: Start with 10 seconds, make configurable via environment variable or flag

2. **Cache Cleanup Strategy**
   - What we know: Need to clean up stale entries periodically
   - What's unclear: Whether background goroutine vs lazy cleanup is better
   - Recommendation: Use lazy cleanup on Get() for simplicity; add background cleanup if memory becomes issue

3. **Multipath Behavior**
   - What we know: NVMe multipath creates virtual subsystem devices
   - What's unclear: RDS single-controller setup may not trigger multipath behavior
   - Recommendation: Design for multipath compatibility but defer testing until needed

## Sources

### Primary (HIGH confidence)
- Linux kernel NVMe sysfs structure: `/sys/class/nvme/nvmeN/subsysnqn`
- Existing codebase: `pkg/nvme/nvme.go:GetDevicePath()` - Current implementation pattern
- Go standard library documentation - `filepath`, `os`, `sync` packages

### Secondary (MEDIUM confidence)
- [ArchWiki - NVMe](https://wiki.archlinux.org/title/Solid_state_drive/NVMe) - Device naming conventions
- [ArchWiki - NVMe over Fabrics](https://wiki.archlinux.org/title/NVMe_over_Fabrics) - NVMe-oF connection patterns
- [linux-nvme/nvme-cli issue #989](https://github.com/linux-nvme/nvme-cli/issues/989) - Subsystem/controller/namespace relationships
- [nvme-cli documentation](https://github.com/linux-nvme/nvme-cli/blob/master/Documentation/nvme-list-subsys.txt) - list-subsys output format
- [Alibaba Cloud CSI Driver](https://pkg.go.dev/github.com/kubernetes-sigs/alibaba-cloud-csi-driver/pkg/disk) - Sysfs scanning patterns

### Tertiary (LOW confidence)
- WebSearch results on orphan detection - Limited specific documentation; validated against kernel behavior
- TTL cache patterns - General Go patterns, not NVMe-specific

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Uses only Go standard library, patterns well-established
- Architecture: HIGH - Based on existing working code and verified Linux sysfs structure
- Pitfalls: MEDIUM - Based on research and existing code comments; some items need production validation

**Research date:** 2026-01-30
**Valid until:** 2026-03-01 (30 days - stable domain, unlikely to change)
