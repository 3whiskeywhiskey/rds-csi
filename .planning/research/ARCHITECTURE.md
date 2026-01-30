# Architecture Research: NVMe Device Path Stability in CSI Drivers

**Domain:** Kubernetes CSI Node Driver Reliability
**Researched:** 2026-01-30
**Confidence:** MEDIUM

## Problem Context

The RDS CSI driver currently stores device paths (e.g., `/dev/nvme2n1`) during `NodeStageVolume` and assumes they remain valid for the volume's lifetime. However, NVMe-oF devices can reconnect with different device paths after network disruptions or target restarts, causing:

- **Stale mounts**: Bind mounts from staging to target path reference non-existent devices
- **I/O errors**: Running pods lose access to storage without visibility
- **Orphaned subsystems**: NQN appears connected but no usable device exists

Current architecture identifies the subsystem by NQN but doesn't handle device path changes after initial staging.

## Recommended Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│ CSI Node Plugin (DaemonSet Pod)                                 │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ NodeStageVolume / NodePublishVolume (CSI RPC Handlers)   │  │
│  │ - Validate requests                                       │  │
│  │ - Trigger connection/mount operations                     │  │
│  │ - Return success/failure to kubelet                       │  │
│  └─────────────┬────────────────────────────────────────────┘  │
│                │                                                 │
│  ┌─────────────▼────────────────────────────────────────────┐  │
│  │ Device Path Resolver (NEW)                               │  │
│  │ - Maintains NQN → current device path mapping            │  │
│  │ - Detects stale paths (device doesn't exist)             │  │
│  │ - Re-scans /sys/class/nvme on path resolution failure    │  │
│  │ - Thread-safe cache with RWMutex                         │  │
│  └─────────────┬────────────────────────────────────────────┘  │
│                │                                                 │
│  ┌─────────────▼────────────────────────────────────────────┐  │
│  │ NVMe Connector (EXISTING, enhanced)                      │  │
│  │ - Connect/Disconnect operations                          │  │
│  │ - GetDevicePath: queries /sys/class/nvme                 │  │
│  │ - IsConnected: checks subsystem existence                │  │
│  └─────────────┬────────────────────────────────────────────┘  │
│                │                                                 │
│  ┌─────────────▼────────────────────────────────────────────┐  │
│  │ Mount Handler (EXISTING)                                 │  │
│  │ - Format/Mount/Unmount operations                        │  │
│  │ - IsMounted: checks /proc/mounts                         │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Optional (Future):                                             │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Background Reconciler (NEW, deferred)                    │  │
│  │ - Periodic health check (every 30s)                      │  │
│  │ - Detects disconnected NVMe targets                      │  │
│  │ - Triggers reconnection attempts                         │  │
│  │ - Logs events for monitoring                             │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Where It Lives | New or Enhanced |
|-----------|----------------|----------------|-----------------|
| **Device Path Resolver** | Maintains NQN → device path mapping; detects stale paths; re-resolves on demand | `pkg/nvme/resolver.go` | NEW |
| **NodeStageVolume Handler** | Stores NQN (not device path) in metadata; uses resolver for current device | `pkg/driver/node.go` | ENHANCED (change what gets stored) |
| **NodePublishVolume Handler** | Validates staging path is mounted; resolves current device before bind mount | `pkg/driver/node.go` | ENHANCED (add validation) |
| **GetDevicePath** | Scans `/sys/class/nvme` for current device path given NQN | `pkg/nvme/nvme.go` | EXISTING (already implements this) |
| **Mount Handler** | Detects if mount is stale (device no longer exists); remounts if needed | `pkg/mount/mount.go` | ENHANCED (add staleness detection) |

## Data Flow for Device Path Resolution

### Initial Volume Stage (Happy Path)

```
1. kubelet → NodeStageVolume(volumeID, stagingPath, volumeContext)
2. NodeServer extracts NQN from volumeContext
3. NodeServer calls Connector.Connect(target) with NQN
4. Connector executes `nvme connect -t tcp -a <ip> -s <port> -n <nqn>`
5. Connector calls GetDevicePath(nqn) → scans /sys/class/nvme → returns /dev/nvme2n1
6. NodeServer formats device (if needed) and mounts to stagingPath
7. NodeServer stores NQN in staging metadata (NOT device path)
8. Return success to kubelet
```

**Key change:** Store NQN instead of device path in staging metadata. Current implementation already extracts NQN from volumeContext; we just need to persist it.

### Volume Publish with Stale Device Handling

```
1. kubelet → NodePublishVolume(volumeID, stagingPath, targetPath)
2. NodeServer checks if stagingPath is mounted (EXISTING check)
3. NEW: NodeServer resolves current device path from NQN
   a. Read NQN from staging metadata
   b. Call resolver.GetCurrentDevicePath(nqn)
   c. Resolver checks cache for recent path
   d. If cache miss or path doesn't exist:
      - Call Connector.GetDevicePath(nqn)
      - Validate device exists at /dev/<path>
      - Update cache
   e. Return current device path
4. NEW: Check if staging mount points to current device
   a. If mismatch detected (mount points to /dev/nvme2n1 but current is /dev/nvme3n1):
      - Log warning
      - Unmount staging path
      - Re-mount using current device path
5. Bind mount stagingPath to targetPath (EXISTING)
6. Return success to kubelet
```

**Key pattern:** Lazy re-resolution on NodePublishVolume. This aligns with Kubernetes CSI lifecycle where NodePublishVolume is idempotent and can be called multiple times.

### Reconnection Detection (On-Demand)

```
NodeExpandVolume or subsequent NodePublishVolume:
1. NodeServer needs device path for operation
2. Call resolver.GetCurrentDevicePath(nqn)
3. Resolver checks if cached path still exists:
   - If /dev/nvme2n1 exists → return from cache
   - If not found:
     a. Call Connector.IsConnected(nqn) - checks subsystem
     b. If subsystem exists but no device:
        - Log "orphaned subsystem detected"
        - Call Connector.Disconnect(nqn)
        - Return error (allow kubelet to retry)
     c. If subsystem doesn't exist:
        - Return error "device not connected"
     d. If subsystem exists with device:
        - Update cache with new path
        - Return new path
```

## Architectural Patterns

### Pattern 1: Cached Device Path Resolution

**What:** Cache NQN → device path mappings with validation before use

**When to use:** Every time we need to access the device (mount, unmount, expand, stats)

**Trade-offs:**
- **Pro:** Avoids expensive `/sys/class/nvme` scan on every operation
- **Pro:** Detects stale paths immediately when cache validation fails
- **Con:** Introduces state that must be kept in sync
- **Con:** Cache invalidation timing is critical

**Implementation:**

```go
type DevicePathResolver struct {
    connector Connector
    cache     map[string]*cacheEntry  // NQN → device path
    mu        sync.RWMutex
    ttl       time.Duration
}

type cacheEntry struct {
    devicePath string
    lastCheck  time.Time
}

func (r *DevicePathResolver) GetCurrentDevicePath(nqn string) (string, error) {
    r.mu.RLock()
    entry, exists := r.cache[nqn]
    r.mu.RUnlock()

    // Cache hit and fresh - validate device still exists
    if exists && time.Since(entry.lastCheck) < r.ttl {
        if deviceExists(entry.devicePath) {
            return entry.devicePath, nil
        }
        // Cache was stale, invalidate
        klog.Warningf("Cached device path %s for NQN %s no longer exists",
            entry.devicePath, nqn)
    }

    // Cache miss or stale - re-resolve
    devicePath, err := r.connector.GetDevicePath(nqn)
    if err != nil {
        return "", fmt.Errorf("failed to resolve device path: %w", err)
    }

    // Update cache
    r.mu.Lock()
    r.cache[nqn] = &cacheEntry{
        devicePath: devicePath,
        lastCheck:  time.Now(),
    }
    r.mu.Unlock()

    return devicePath, nil
}
```

**CONFIDENCE:** HIGH - This pattern is standard in storage drivers and aligns with CSI idempotency requirements.

### Pattern 2: Reactive Reconnection on Demand

**What:** Detect and recover from device path changes when CSI operations are called, rather than proactively monitoring

**When to use:** NodePublishVolume, NodeExpandVolume, NodeGetVolumeStats - any operation needing device access

**Trade-offs:**
- **Pro:** No background goroutines or periodic polling overhead
- **Pro:** Aligns with CSI idempotency model (operations can be retried)
- **Pro:** Simple - detection and recovery happen in same code path
- **Con:** Pod may experience I/O errors until next CSI operation triggers recovery
- **Con:** Relies on kubelet retry behavior for recovery

**Implementation approach:**

```go
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    // ... validation ...

    // Read NQN from staging metadata (stored during NodeStageVolume)
    nqn, err := ns.readNQNFromStaging(stagingPath)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to read NQN: %v", err)
    }

    // Resolve current device path
    devicePath, err := ns.resolver.GetCurrentDevicePath(nqn)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to resolve device: %v", err)
    }

    // Check if staging mount points to current device
    if err := ns.validateStagingMount(stagingPath, devicePath); err != nil {
        klog.Warningf("Staging mount is stale, remounting: %v", err)
        if err := ns.remountStaging(stagingPath, devicePath, fsType); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to remount: %v", err)
        }
    }

    // Proceed with bind mount
    // ... existing bind mount logic ...
}
```

**CONFIDENCE:** HIGH - Matches CSI spec expectation that operations are idempotent and retriable.

**Sources:**
- [CSI block: Why is NodePublishVolume called in SetUpDevice?](https://github.com/kubernetes/kubernetes/issues/73773) - Documents idempotency expectations
- [NodeStageVolume is called before each call to NodePublishVolume](https://github.com/kubernetes-csi/external-provisioner/issues/90) - Confirms retry patterns

### Pattern 3: Background Health Monitoring (DEFERRED)

**What:** Periodic goroutine checks NVMe connection health and preemptively reconnects

**When to use:** Production deployments where I/O errors are unacceptable (deferred to Phase 2)

**Trade-offs:**
- **Pro:** Detects failures before kubelet operations
- **Pro:** Can emit metrics and events for monitoring
- **Con:** Adds complexity and resource usage
- **Con:** Risk of unnecessary reconnections (thundering herd)
- **Con:** Must coordinate with CSI operation locks

**Recommendation:** DO NOT implement in initial reliability phase. Focus on reactive recovery first, add proactive monitoring only if reactive approach proves insufficient in production.

**CONFIDENCE:** MEDIUM - This pattern exists in some CSI drivers but adds significant complexity. Document as future enhancement.

**Sources:**
- [Pods using CSI driver sidecar fail to recover after Node failure](https://github.com/GoogleCloudPlatform/gcs-fuse-csi-driver/issues/113) - Shows recovery challenges
- [Critical: Node reboot could lead to data loss](https://github.com/kubernetes/kubernetes/issues/120853) - Documents Kubernetes lifecycle issues

## Anti-Patterns to Avoid

### Anti-Pattern 1: Storing Device Path in Persistent Metadata

**What people do:** Store `/dev/nvme2n1` in JSON file at staging path, use it for all subsequent operations

**Why it's wrong:**
- Device paths are ephemeral and change on reconnection
- No mechanism to update stored path when device changes
- Creates stale reference that causes I/O errors

**Do this instead:** Store NQN (persistent identifier) and resolve device path on each use

**CONFIDENCE:** HIGH - This is the root cause of the current issue.

### Anti-Pattern 2: Using `requiresRepublish=true` for Remounting

**What people do:** Set CSIDriver spec `requiresRepublish: true` to trigger periodic NodePublishVolume calls for remounting

**Why it's wrong:**
- Kubelet calls NodePublishVolume every 100ms when enabled (hardcoded reconcilerLoopSleepPeriod)
- Any error during republish causes immediate unmount by kubelet
- Unmount doesn't propagate to running pod mount namespace (pod keeps stale mount)
- Designed for credential rotation, not device path changes
- Extremely high overhead for all volumes

**Do this instead:** Use on-demand resolution in NodePublishVolume; rely on natural kubelet retries for recovery

**CONFIDENCE:** HIGH - Documented Kubernetes issues show this approach is problematic.

**Sources:**
- [Kubelet deletes CSI mount points if requiresRepublish call returns error](https://github.com/kubernetes/kubernetes/issues/121271)
- [CSIDriver Object - requiresRepublish documentation](https://kubernetes-csi.github.io/docs/csi-driver-object.html)

### Anti-Pattern 3: Reconnecting on Every GetDevicePath Call

**What people do:** Call `nvme disconnect && nvme connect` on every device path lookup to ensure fresh connection

**Why it's wrong:**
- Causes I/O disruption for running pods
- Expensive operation (connect takes 2-5 seconds)
- Unnecessary when device path is still valid
- Creates thundering herd if multiple pods stage simultaneously

**Do this instead:** Only reconnect when device path resolution fails AND subsystem is orphaned

**CONFIDENCE:** HIGH - Performance and reliability impact is clear.

### Anti-Pattern 4: Background Thread Remounting Without Coordination

**What people do:** Background goroutine periodically checks mounts and remounts if stale

**Why it's wrong:**
- Race conditions with CSI operations (NodeUnstageVolume during remount)
- Can remount while kubelet is trying to unmount (violates CSI state machine)
- No way to propagate remount errors to kubelet/scheduler
- Difficult to debug (operations happen outside request context)

**Do this instead:** Handle remounting within CSI operation handlers where errors can be returned properly

**CONFIDENCE:** HIGH - CSI spec assumes single-threaded operation per volume.

## Integration Points

### Kubelet Integration

| Scenario | Kubelet Behavior | CSI Driver Response |
|----------|------------------|---------------------|
| **Pod starts, volume needs mounting** | Calls NodeStageVolume (if not already staged), then NodePublishVolume | Connect NVMe (if needed), mount to staging, bind mount to target |
| **Mount validation fails (stale device)** | Retries NodePublishVolume with exponential backoff | Detect stale mount, remount with current device, return success |
| **Device path changed between operations** | Not detected by kubelet; relies on CSI driver | Resolver detects change, updates cache, uses new path transparently |
| **Node reboot, staging path lost** | Calls NodeStageVolume again to restage | Re-connect NVMe, mount to staging (idempotent operation) |

**CONFIDENCE:** HIGH - Tested behavior documented in CSI spec and Kubernetes issues.

**Sources:**
- [BUG: Missed NodeStageVolume after reboot](https://github.com/longhorn/longhorn/issues/8009)
- [NodeStageVolume called before NodePublishVolume](https://github.com/kubernetes-csi/external-provisioner/issues/90)

### NVMe Subsystem Integration

| State | Detection Method | Recovery Action |
|-------|------------------|-----------------|
| **Connected with valid device** | `IsConnected(nqn)` returns true, `GetDevicePath(nqn)` returns path, device exists | Use cached path |
| **Orphaned subsystem (connected but no device)** | `IsConnected(nqn)` returns true, `GetDevicePath(nqn)` fails | Disconnect subsystem, return error to trigger retry |
| **Disconnected** | `IsConnected(nqn)` returns false | Return error to kubelet; kubelet will call NodeStageVolume again |
| **Device path changed** | Old path doesn't exist, new scan finds device under different path | Update cache, use new path |

**CONFIDENCE:** MEDIUM - Based on current implementation's orphaned subsystem handling (lines 495-504 in nvme.go).

**Sources:**
- Codebase analysis: `pkg/nvme/nvme.go` ConnectWithContext implementation

## Implementation Order (Phase Structure)

### Phase 1: Foundation (Week 1)
**Goal:** Introduce device path resolver without changing CSI behavior

1. **Create DevicePathResolver** (`pkg/nvme/resolver.go`)
   - Implement cached resolution with TTL (30 seconds)
   - Add `GetCurrentDevicePath(nqn)` method
   - Add `InvalidateCache(nqn)` for explicit invalidation
   - Unit tests for cache hit/miss scenarios

2. **Update NodeServer to use resolver** (`pkg/driver/node.go`)
   - Instantiate resolver in `NewNodeServer`
   - Replace direct `Connector.GetDevicePath` calls with `resolver.GetCurrentDevicePath`
   - No behavior changes yet, just refactoring

3. **Add NQN persistence** (metadata storage)
   - Store NQN in JSON file at staging path (e.g., `/var/lib/kubelet/plugins/.../staging/volume-metadata.json`)
   - Implement `readNQNFromStaging` helper
   - Keep backward compatibility (if metadata missing, derive from volumeID)

**Validation:** Existing tests pass; no behavior changes observed

### Phase 2: Stale Mount Detection (Week 2)
**Goal:** Detect but don't automatically fix stale mounts

4. **Add mount validation** (`pkg/mount/mount.go`)
   - Implement `GetMountDevicePath(mountPath)` - reads /proc/mounts
   - Implement `IsMountStale(mountPath, expectedDevice)` - compares devices
   - Unit tests with mock /proc/mounts

5. **Add staleness detection to NodePublishVolume**
   - Before bind mount, check if staging mount points to current device
   - Log warning if stale detected
   - Return error with `codes.FailedPrecondition` if stale
   - Add metric for stale mount detection count

**Validation:** Manually trigger NVMe reconnection, verify error is returned and logged

### Phase 3: Automatic Remounting (Week 3)
**Goal:** Automatically recover from stale mounts

6. **Implement remounting logic**
   - Add `remountStaging(stagingPath, newDevicePath, fsType)` method
   - Unmount staging path
   - Re-mount with current device path
   - Log operation with duration metrics

7. **Integrate remounting into NodePublishVolume**
   - When stale mount detected, attempt remount
   - If remount succeeds, continue with bind mount
   - If remount fails, return error to kubelet
   - Add retry with exponential backoff (3 attempts)

8. **Add orphaned subsystem handling**
   - If device path resolution fails but subsystem exists
   - Disconnect and return error (trigger kubelet retry)
   - Log event for monitoring

**Validation:** Integration test with forced reconnection; verify automatic recovery

### Phase 4: Observability (Week 4)
**Goal:** Add metrics and events for production monitoring

9. **Add Prometheus metrics** (`pkg/metrics/nvme.go`)
   - `rds_csi_device_path_resolutions_total{result="success|failure"}`
   - `rds_csi_stale_mounts_detected_total`
   - `rds_csi_remount_operations_total{result="success|failure"}`
   - `rds_csi_orphaned_subsystems_detected_total`

10. **Add Kubernetes events**
    - Emit event when stale mount detected
    - Emit event when remount succeeds/fails
    - Emit event when orphaned subsystem cleaned up

**Validation:** Deploy to test cluster, trigger failures, verify metrics and events appear

## Decision: Reactive vs. Proactive Architecture

**CHOSEN APPROACH: Reactive (On-Demand Resolution)**

**Rationale:**
1. **Aligns with CSI lifecycle**: Kubelet already retries failed operations; leverage existing retry mechanism
2. **Simpler implementation**: No background threads, no coordination complexity
3. **Lower resource usage**: No periodic scanning or health checks
4. **Fail-fast**: Errors surface immediately during CSI operations (easier to debug)
5. **Production-proven**: Other CSI drivers (AWS EBS, Longhorn) use reactive patterns successfully

**DEFERRED: Proactive (Background Monitoring)**

**Reasons to defer:**
1. Adds complexity without clear benefit over reactive approach
2. Risk of race conditions with CSI operations
3. Requires careful coordination with locks
4. Thundering herd risk (multiple nodes reconnecting simultaneously)
5. Can be added later if reactive approach proves insufficient

**Decision criteria for adding proactive monitoring:**
- Metrics show frequent I/O errors between CSI operations
- Customer demand for zero-downtime reconnection
- Evidence that kubelet retry delays are unacceptable

**CONFIDENCE:** HIGH - Based on research of existing CSI drivers and Kubernetes issue discussions.

**Sources:**
- Analysis of Longhorn CSI driver patterns (reactive remounting in NodePublishVolume)
- Kubernetes CSI spec emphasizes idempotency over proactive monitoring

## Scaling Considerations

| Scale Factor | Impact | Mitigation |
|--------------|--------|------------|
| **Many volumes per node (100+)** | Resolver cache size grows; memory usage increases | Implement cache eviction based on LRU; limit cache size to 1000 entries |
| **Frequent reconnections** | Cache thrashing; repeated /sys scans | Increase TTL to 60 seconds; add metric for cache hit rate |
| **High pod churn** | Many NodePublishVolume calls; resolver pressure | Cache helps here; ensure GetDevicePath is efficient |
| **Network instability** | Many orphaned subsystems; cleanup overhead | Log but don't block operations; track cleanup latency metric |

## Sources

### Official Documentation (HIGH Confidence)
- [CSI Driver Object - requiresRepublish](https://kubernetes-csi.github.io/docs/csi-driver-object.html) - Official Kubernetes CSI documentation
- [Deploying a CSI Driver on Kubernetes](https://kubernetes-csi.github.io/docs/deploying.html) - Architecture patterns
- [How the CSI Works](https://sklar.rocks/how-container-storage-interface-works/) - CSI lifecycle explanation

### Kubernetes Issues (MEDIUM-HIGH Confidence)
- [Issue #121271: Kubelet deletes CSI mount points if requiresRepublish returns error](https://github.com/kubernetes/kubernetes/issues/121271) - RequiresRepublish pitfalls
- [Issue #120853: Node reboot data loss due to broken lifecycle](https://github.com/kubernetes/kubernetes/issues/120853) - CSI lifecycle issues
- [Issue #8009: Missed NodeStageVolume after reboot](https://github.com/longhorn/longhorn/issues/8009) - Real-world reboot handling
- [Issue #3744: Staging path no longer valid](https://github.com/longhorn/longhorn/issues/3744) - Stale staging path patterns
- [Issue #73773: CSI block NodePublishVolume in SetUpDevice](https://github.com/kubernetes/kubernetes/issues/73773) - NodePublish idempotency

### NVMe-oF CSI Implementations (MEDIUM Confidence)
- [kubernetes-csi/csi-driver-nvmf](https://github.com/kubernetes-csi/csi-driver-nvmf) - Reference implementation
- [spdk/spdk-csi](https://github.com/spdk/spdk-csi) - SPDK architecture patterns
- [csi-lib-iscsi Multipath Management](https://deepwiki.com/kubernetes-csi/csi-lib-iscsi/2.3-multipath-management) - Device path handling patterns

### Codebase Analysis (HIGH Confidence)
- `pkg/nvme/nvme.go` - Current Connector implementation with orphaned subsystem handling
- `pkg/driver/node.go` - Current NodeStageVolume/NodePublishVolume implementation
- `.planning/codebase/ARCHITECTURE.md` - Existing architecture documentation
- `.planning/codebase/CONCERNS.md` - Known issues with NVMe device paths

---

*Architecture research for: NVMe-oF device path stability*
*Researched: 2026-01-30*
