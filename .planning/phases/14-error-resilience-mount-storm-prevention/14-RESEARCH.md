# Phase 14: Error Resilience and Mount Storm Prevention - Research

**Researched:** 2026-02-03
**Domain:** Error handling, mount resilience, circuit breakers, graceful shutdown
**Confidence:** MEDIUM

## Summary

This phase implements critical defensive safeguards to prevent two production failure modes discovered during Phase 13: mount storms from corrupted filesystems (thousands of duplicate mounts overwhelming `/proc/mounts`) and system volume disconnection (driver interfering with NixOS diskless node boot volumes).

The standard approach combines NQN filtering (explicit namespace management), procmounts parsing with timeouts (defense against filesystem corruption), filesystem health checks before mount operations (detect corruption early), and circuit breaker patterns (prevent retry storms). Go's context package provides robust timeout handling, the moby/sys/mountinfo library offers production-ready mount parsing, and multiple circuit breaker libraries exist with exponential backoff support.

**Primary recommendation:** Use configurable NQN prefix validation at startup (fail-fast), implement duplicate mount detection during procmounts parsing with context timeout (10s max), add filesystem health checks before NodeStageVolume mount operations, and use sony/gobreaker or mercari/go-circuitbreaker for circuit breaker pattern with exponential backoff.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| context | stdlib | Timeout/cancellation | Go standard, goroutine-safe, battle-tested |
| moby/sys/mountinfo | latest | Parse mount info | Used by Docker/containerd, handles procmounts edge cases |
| sony/gobreaker | v0.5.0+ | Circuit breaker | Simple API, production-proven, 6.8k stars |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| mercari/go-circuitbreaker | v0.2.0+ | Circuit breaker | Need exponential backoff built-in, context-aware errors |
| cenkalti/backoff/v4 | v4.2.0+ | Exponential backoff | Standalone backoff logic, configurable algorithms |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| sony/gobreaker | mercari/go-circuitbreaker | More features (backoff, context errors) but more complex |
| moby/sys/mountinfo | Manual /proc/mounts parsing | Simpler but fragile (format changes, edge cases) |

**Installation:**
```bash
go get github.com/moby/sys/mountinfo
go get github.com/sony/gobreaker  # Simple option
go get github.com/mercari/go-circuitbreaker  # Feature-rich option
```

## Architecture Patterns

### Recommended Code Structure
```
pkg/
├── driver/
│   ├── node.go                  # Add NQN prefix validation, health checks
│   └── driver.go                # Add startup validation
├── mount/
│   ├── mount.go                 # Add duplicate detection
│   ├── health.go                # NEW: Filesystem health checks
│   └── procmounts.go            # NEW: Safe procmounts parsing with timeout
├── circuitbreaker/              # NEW: Circuit breaker logic
│   ├── breaker.go               # Circuit breaker wrapper
│   └── volume_breaker.go        # Per-volume circuit breaker state
└── nvme/
    └── orphan.go                # Fix NQN filtering bug (line 52)
```

### Pattern 1: NQN Prefix Validation at Startup
**What:** Validate configurable NQN prefix at driver startup, fail fast if invalid or missing
**When to use:** Driver initialization (controller and node plugins)
**Example:**
```go
// Source: Phase 14 design requirements + NVMe NQN spec
const (
    envManagedNQNPrefix = "CSI_MANAGED_NQN_PREFIX"
)

func validateNQNPrefix(prefix string) error {
    if prefix == "" {
        return fmt.Errorf("managed NQN prefix is required (set %s)", envManagedNQNPrefix)
    }

    // NQN max length is 223 bytes per NVMe spec
    if len(prefix) > 223 {
        return fmt.Errorf("NQN prefix exceeds maximum length (223 bytes): %d", len(prefix))
    }

    // Validate NQN format: nqn.yyyy-mm.domain:identifier
    if !strings.HasPrefix(prefix, "nqn.") {
        return fmt.Errorf("NQN prefix must start with 'nqn.': %s", prefix)
    }

    return nil
}

// In driver.go initialization:
func NewDriver(...) (*Driver, error) {
    nqnPrefix := os.Getenv(envManagedNQNPrefix)
    if err := validateNQNPrefix(nqnPrefix); err != nil {
        return nil, fmt.Errorf("startup validation failed: %w", err)
    }

    klog.Infof("Driver managing volumes with NQN prefix: %s", nqnPrefix)
    return &Driver{managedNQNPrefix: nqnPrefix}, nil
}
```

### Pattern 2: Procmounts Parsing with Context Timeout
**What:** Parse /proc/mounts with 10-second timeout to prevent hangs on corrupted filesystems
**When to use:** Any operation that reads mount information
**Example:**
```go
// Source: moby/sys/mountinfo + Go context patterns
import (
    "context"
    "time"
    "github.com/moby/sys/mountinfo"
)

const procmountsTimeout = 10 * time.Second

func getMountsWithTimeout(ctx context.Context) ([]*mountinfo.Info, error) {
    ctx, cancel := context.WithTimeout(ctx, procmountsTimeout)
    defer cancel()

    // Channel to receive result or error
    type result struct {
        mounts []*mountinfo.Info
        err    error
    }
    resultCh := make(chan result, 1)

    go func() {
        mounts, err := mountinfo.GetMounts(nil)
        resultCh <- result{mounts: mounts, err: err}
    }()

    select {
    case res := <-resultCh:
        return res.mounts, res.err
    case <-ctx.Done():
        return nil, fmt.Errorf("procmounts parsing timed out after %v: %w",
            procmountsTimeout, ctx.Err())
    }
}
```

### Pattern 3: Duplicate Mount Detection
**What:** Detect excessive duplicate mounts for same device, prevent mount storms
**When to use:** Before mount operations and during procmounts parsing
**Example:**
```go
// Source: Production incident analysis + defense-in-depth pattern
const maxDuplicateMountsPerDevice = 100

func detectDuplicateMounts(mounts []*mountinfo.Info, devicePath string) (int, error) {
    count := 0
    for _, mount := range mounts {
        if mount.Source == devicePath {
            count++
        }
    }

    if count >= maxDuplicateMountsPerDevice {
        return count, fmt.Errorf(
            "duplicate mount storm detected: device %s has %d mount entries (threshold: %d). "+
            "This indicates filesystem corruption. Manual cleanup required.",
            devicePath, count, maxDuplicateMountsPerDevice)
    }

    return count, nil
}
```

### Pattern 4: Filesystem Health Check Before Mount
**What:** Run fsck in read-only mode before mounting to detect corruption early
**When to use:** NodeStageVolume before first mount attempt
**Example:**
```go
// Source: Red Hat filesystem docs + fsck best practices
func checkFilesystemHealth(devicePath, fsType string) error {
    var cmd *exec.Cmd

    switch fsType {
    case "ext4", "ext3", "ext2":
        // fsck.ext4 -n: read-only check, no modifications
        cmd = exec.Command("fsck.ext4", "-n", devicePath)
    case "xfs":
        // xfs_repair -n: dry-run check only
        cmd = exec.Command("xfs_repair", "-n", devicePath)
    default:
        // Unknown filesystem - skip check
        klog.V(2).Infof("Skipping health check for unsupported filesystem: %s", fsType)
        return nil
    }

    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("filesystem health check failed: %w (output: %s)", err, output)
    }

    return nil
}
```

### Pattern 5: Circuit Breaker for Mount Operations
**What:** Prevent retry storms on repeatedly failing volumes with exponential backoff
**When to use:** Wrap mount operations in NodeStageVolume
**Example:**
```go
// Source: sony/gobreaker documentation
import "github.com/sony/gobreaker"

type VolumeCircuitBreaker struct {
    breakers map[string]*gobreaker.CircuitBreaker
    mu       sync.RWMutex
}

func (vcb *VolumeCircuitBreaker) getBreaker(volumeID string) *gobreaker.CircuitBreaker {
    vcb.mu.RLock()
    cb, exists := vcb.breakers[volumeID]
    vcb.mu.RUnlock()

    if exists {
        return cb
    }

    vcb.mu.Lock()
    defer vcb.mu.Unlock()

    // Double-check after acquiring write lock
    if cb, exists := vcb.breakers[volumeID]; exists {
        return cb
    }

    // Create new circuit breaker for this volume
    settings := gobreaker.Settings{
        Name:        volumeID,
        MaxRequests: 1,  // Only 1 request in half-open state
        Interval:    time.Minute,
        Timeout:     5 * time.Minute,  // Stay open for 5 minutes after failure
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            // Open circuit after 3 consecutive failures
            return counts.ConsecutiveFailures >= 3
        },
        OnStateChange: func(name string, from, to gobreaker.State) {
            klog.Infof("Circuit breaker for volume %s: %s -> %s", name, from, to)
        },
    }

    cb = gobreaker.NewCircuitBreaker(settings)
    vcb.breakers[volumeID] = cb
    return cb
}

// Usage in NodeStageVolume:
func (ns *NodeServer) NodeStageVolume(...) {
    cb := ns.circuitBreaker.getBreaker(volumeID)

    _, err := cb.Execute(func() (interface{}, error) {
        // Health check
        if err := checkFilesystemHealth(devicePath, fsType); err != nil {
            return nil, err
        }

        // Mount operation
        if err := ns.mounter.Mount(devicePath, stagingPath, fsType, options); err != nil {
            return nil, err
        }

        return nil, nil
    })

    if err == gobreaker.ErrOpenState {
        return nil, status.Errorf(codes.Unavailable,
            "Volume %s circuit breaker open due to repeated failures. "+
            "Filesystem may be corrupted. Add annotation 'rds.csi.srvlab.io/reset-circuit-breaker=true' "+
            "to PV to retry.", volumeID)
    }

    return err
}
```

### Pattern 6: Graceful Shutdown with Timeout
**What:** Handle SIGTERM with 30s timeout, allow in-flight operations to complete
**When to use:** Driver main function
**Example:**
```go
// Source: Kubernetes CSI shutdown patterns + Go graceful shutdown guides
import (
    "os"
    "os/signal"
    "syscall"
)

const shutdownTimeout = 30 * time.Second

func main() {
    // Create shutdown context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Setup signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

    // Start driver
    driver, err := NewDriver(...)
    if err != nil {
        klog.Fatalf("Failed to create driver: %v", err)
    }

    go func() {
        <-sigCh
        klog.Info("Received shutdown signal, initiating graceful shutdown")
        cancel()
    }()

    // Run driver with context
    if err := driver.Run(ctx); err != nil && err != context.Canceled {
        klog.Fatalf("Driver failed: %v", err)
    }

    // Wait for shutdown with timeout
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
    defer shutdownCancel()

    if err := driver.Shutdown(shutdownCtx); err != nil {
        klog.Errorf("Shutdown did not complete within timeout: %v", err)
        os.Exit(1)
    }

    klog.Info("Driver stopped gracefully")
}
```

### Anti-Patterns to Avoid
- **Hard-coded NQN prefix:** Always use configurable env var, never hard-code `pvc-` prefix (breaks flexibility)
- **Blocking procmounts parse:** Always wrap with context timeout, filesystem corruption can hang reads indefinitely
- **Ignoring circuit breaker state:** Don't bypass circuit breaker on retry, respect open state and require manual intervention
- **Silent health check failures:** Log health check results, don't swallow errors that indicate corruption

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Mount info parsing | Custom /proc/mounts parser | moby/sys/mountinfo | Handles format variations, edge cases (spaces in paths, special chars), used by Docker/containerd |
| Circuit breaker | Custom retry counter | sony/gobreaker or mercari/go-circuitbreaker | State machine complexity, thread safety, metrics, OnStateChange hooks |
| Exponential backoff | Manual sleep doubling | cenkalti/backoff or gobreaker Timeout | Jitter, max delay, elapsed time tracking, cancellation |
| Graceful shutdown | Custom signal handler | Standard Go pattern (context + sync.WaitGroup) | Timeout handling, cleanup coordination, tested pattern |

**Key insight:** Mount operations interact with kernel state that can be corrupted or unresponsive. Production-tested libraries handle edge cases (EINTR, EAGAIN, malformed procmounts) that custom code typically misses.

## Common Pitfalls

### Pitfall 1: Procmounts Parse Hangs on Corrupted Filesystem
**What goes wrong:** Reading `/proc/mounts` blocks indefinitely when filesystem metadata is corrupted (thousands of duplicate entries, circular references)
**Why it happens:** `/proc/mounts` is a kernel pseudo-file that performs I/O to gather mount state; corruption can cause kernel to hang during read
**How to avoid:** Always wrap procmounts parsing with context timeout (10s), run in separate goroutine with result channel
**Warning signs:** CSI driver stops responding to all requests, kubectl get pods hangs, node becomes NotReady

### Pitfall 2: NQN Filtering with Hard-Coded Prefix
**What goes wrong:** Driver disconnects system volumes (e.g., diskless node `/var` mounts using `nqn.2000-02.com.mikrotik:nixos-*`), bricking the node
**Why it happens:** Code hard-codes `pvc-` prefix check using `strings.HasSuffix` on NQN instead of checking full managed prefix
**How to avoid:** Validate full NQN prefix (including `nqn.2000-02.com.mikrotik:` base) from env var at startup, fail fast if not configured
**Warning signs:** Orphan cleaner logs show "skipping non-CSI volume" but code has hard-coded prefix check (pkg/nvme/orphan.go line 52)

### Pitfall 3: Circuit Breaker Without Manual Reset
**What goes wrong:** Volume with transient corruption remains permanently unavailable after circuit breaker opens, even after filesystem repair
**Why it happens:** Circuit breaker state stored in memory, no way for operator to signal "try again now"
**How to avoid:** Check PV annotations for reset signal (e.g., `rds.csi.srvlab.io/reset-circuit-breaker=true`) before checking circuit breaker state
**Warning signs:** Pods stuck in ContainerCreating with "circuit breaker open" error, no way to recover without driver restart

### Pitfall 4: Health Check on Mounted Filesystem
**What goes wrong:** fsck.ext4 returns false positives or corrupts filesystem when run on mounted volume
**Why it happens:** fsck expects filesystem to be unmounted; running on mounted filesystem sees inconsistent state
**How to avoid:** Only run health checks before initial mount (in NodeStageVolume before first Mount call), use `-n` (read-only) flag, skip check if already mounted
**Warning signs:** fsck logs show "filesystem is mounted" warnings, health check reports errors on known-good filesystem

### Pitfall 5: Kubernetes Shutdown Timing
**What goes wrong:** CSI driver receives SIGTERM and exits before volumes finish unmounting, leaving orphaned NVMe connections
**Why it happens:** Kubernetes sends SIGTERM to CSI driver before workload pods finish terminating
**How to avoid:** Set terminationGracePeriodSeconds to 60s (double default), implement graceful shutdown that waits for in-flight operations
**Warning signs:** Node plugin logs "received SIGTERM" followed immediately by exit, orphaned NVMe devices after node shutdown

## Code Examples

Verified patterns from official sources:

### NQN Format Validation
```go
// Source: NVMe spec section 7.9 + SPDK validation
// NQN format: nqn.yyyy-mm.domain:identifier
// Maximum length: 223 bytes (excluding null terminator)
// Case-sensitive byte comparison (no normalization)

func isValidNQN(nqn string) bool {
    // Check length
    if len(nqn) > 223 {
        return false
    }

    // Check prefix
    if !strings.HasPrefix(nqn, "nqn.") {
        return false
    }

    // Check for required colon separator
    if !strings.Contains(nqn, ":") {
        return false
    }

    return true
}
```

### Context-Safe Mount Info Retrieval
```go
// Source: moby/sys/mountinfo v0.7.0+ documentation
import "github.com/moby/sys/mountinfo"

func getMountInfo(path string) (*mountinfo.Info, error) {
    // Get all mounts with single-entry filter
    mounts, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(path))
    if err != nil {
        return nil, fmt.Errorf("failed to get mount info: %w", err)
    }

    if len(mounts) == 0 {
        return nil, fmt.Errorf("no mount found at %s", path)
    }

    return mounts[0], nil
}
```

### Exponential Backoff Circuit Breaker
```go
// Source: mercari/go-circuitbreaker v0.2.0+ examples
import (
    "github.com/mercari/go-circuitbreaker"
    "github.com/cenkalti/backoff/v4"
)

func newVolumeCircuitBreaker(volumeID string) *circuitbreaker.CircuitBreaker {
    expBackoff := backoff.NewExponentialBackOff()
    expBackoff.InitialInterval = 1 * time.Minute
    expBackoff.MaxInterval = 30 * time.Minute
    expBackoff.Multiplier = 2.0

    return circuitbreaker.New(
        circuitbreaker.WithOpenTimeoutBackOff(expBackoff),
        circuitbreaker.WithTripFunc(
            // Open after 3 consecutive failures
            circuitbreaker.NewTripFuncConsecutiveFailures(3),
        ),
        circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
            klog.Infof("Volume %s circuit breaker: %s -> %s", volumeID, from, to)
        }),
    )
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual /proc/mounts parsing | moby/sys/mountinfo library | 2020+ | Handles edge cases, goroutine-safe |
| Global retry logic | Per-volume circuit breakers | 2021+ | Isolates failures, prevents cascading |
| Blocking fsck | Read-only health checks | 2022+ | No filesystem modification risk |
| Hard-coded timeouts | Context-based cancellation | Go 1.7+ (2016) | Composable, deadline propagation |
| Process-wide circuit breaker | Per-operation breakers | 2023+ | Granular failure isolation |

**Deprecated/outdated:**
- Hard-coded 60s SIGTERM wait: Use configurable terminationGracePeriodSeconds (Kubernetes 1.21+)
- fsck without -n flag: Always use read-only mode for health checks (corrupts mounted filesystems)
- Synchronous procmounts read: Always use timeout to prevent hangs (filesystem corruption reality)

## Open Questions

Things that couldn't be fully resolved:

1. **Optimal duplicate mount threshold**
   - What we know: Phase 13 incident had "thousands" of duplicates, 100 seems reasonable
   - What's unclear: Industry standard threshold, how other CSI drivers handle this
   - Recommendation: Start with 100, make configurable via env var, monitor metrics to tune

2. **Circuit breaker backoff parameters**
   - What we know: mercari/go-circuitbreaker supports exponential backoff, typical values are 1min initial → 30min max
   - What's unclear: Optimal values for storage operations (longer than HTTP services?)
   - Recommendation: Initial 1min, max 30min, multiplier 2.0, make configurable

3. **Graceful shutdown with stuck mount operations**
   - What we know: Kubernetes default terminationGracePeriodSeconds is 30s, CSI drivers often need more time
   - What's unclear: What to do if mount operations don't complete within timeout (force kill? log and exit?)
   - Recommendation: Wait 30s, log operations still in progress, exit cleanly (Kubernetes will SIGKILL if needed)

4. **Filesystem health check performance impact**
   - What we know: fsck.ext4 -n is read-only and relatively fast, xfs_repair -n can be slower
   - What's unclear: Performance impact on large volumes (multi-TB), whether to make this optional
   - Recommendation: Always run for initial mount, make timeout configurable (default 60s), log if check takes >10s

5. **NQN prefix validation strictness**
   - What we know: NVMe spec requires "nqn." prefix, max 223 bytes, case-sensitive
   - What's unclear: Whether to enforce full RFC format (yyyy-mm.domain:identifier) or just basic checks
   - Recommendation: Basic validation (prefix + length + has colon), strict RFC validation can break compatibility

## Sources

### Primary (HIGH confidence)
- [moby/sys/mountinfo package documentation](https://pkg.go.dev/github.com/moby/sys/mountinfo) - Mount info parsing
- [sony/gobreaker package documentation](https://pkg.go.dev/github.com/sony/gobreaker) - Circuit breaker patterns
- [mercari/go-circuitbreaker package documentation](https://pkg.go.dev/github.com/mercari/go-circuitbreaker) - Circuit breaker with backoff
- [Go context package documentation](https://pkg.go.dev/context) - Timeout and cancellation
- [SPDK NVMe-oF Target documentation](https://spdk.io/doc/nvmf.html) - NQN format and validation

### Secondary (MEDIUM confidence)
- [Red Hat: Checking and repairing a file system](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/8/html/managing_file_systems/checking-and-repairing-a-file-system__managing-file-systems) - fsck best practices
- [Linuxize: Fsck Command in Linux](https://linuxize.com/post/fsck-command-in-linux/) - fsck usage and flags
- [OneUpTime: How to Implement Graceful Shutdown in Go for Kubernetes (2026-01-07)](https://oneuptime.com/blog/post/2026-01-07-go-graceful-shutdown-kubernetes/view) - Graceful shutdown patterns
- [DevOpsCube: Kubernetes Pod Graceful Shutdown with SIGTERM](https://devopscube.com/kubernetes-pod-graceful-shutdown/) - K8s termination behavior
- [Kubernetes Issue #115148: Graceful node shutdown doesn't wait for volume teardown](https://github.com/kubernetes/kubernetes/issues/115148) - Known CSI timing issues

### Tertiary (LOW confidence)
- WebSearch results: "mount storm" terminology not widely documented (likely project-specific term for duplicate mount proliferation)
- WebSearch results: Duplicate mount thresholds not standardized (100 chosen based on reasoning, not industry standard)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - moby/sys/mountinfo and gobreaker are production-proven in container ecosystems
- Architecture: MEDIUM - Patterns are sound but implementation details need validation (thresholds, timeouts)
- Pitfalls: HIGH - Based on actual production incidents (Phase 13) and verified documentation

**Research date:** 2026-02-03
**Valid until:** 30 days (stable domain - mount operations and circuit breakers are mature patterns)
