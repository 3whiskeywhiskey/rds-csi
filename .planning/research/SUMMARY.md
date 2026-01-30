# Project Research Summary

**Project:** RDS CSI Driver NVMe-oF Reliability Enhancement
**Domain:** Kubernetes CSI Storage Driver - NVMe over Fabrics
**Researched:** 2026-01-30
**Confidence:** HIGH

## Executive Summary

The RDS CSI driver currently suffers from NVMe-oF device path instability after network disruptions or target reconnections. Research confirms this is a well-known problem in NVMe-oF CSI implementations with proven solutions. The root cause is storing ephemeral device paths (`/dev/nvme2n1`) instead of persistent identifiers (NQNs) combined with inadequate connection state verification. When NVMe controllers reconnect after network disruptions, they receive new device numbers (nvme0 → nvme3), breaking existing mounts and causing I/O errors.

The recommended approach is **reactive device path resolution**: store NQNs as persistent identifiers, resolve device paths on-demand via sysfs scanning, detect stale mounts during CSI operations, and leverage Kubernetes' built-in retry mechanisms for recovery. This aligns with CSI idempotency requirements and is production-proven in AWS EBS CSI, Longhorn, and other mature drivers. The architecture should implement cached resolution with validation (not background monitoring) to handle orphaned subsystems, controller renumbering, and stale mounts transparently.

Critical risks include orphaned subsystem false positives (subsystem exists but no device), controller renumbering breaking device path assumptions, non-idempotent operations causing permanent failures, and stale mounts after unclean disconnections. All have well-documented mitigation strategies: verify device existence after IsConnected(), use NQN-based sysfs lookups instead of hardcoded paths, implement proper idempotency checks with canonical path comparison, and use lazy unmount for stale filesystem cleanup. The research shows high confidence across all areas with multiple production CSI drivers demonstrating successful implementations.

## Key Findings

### Recommended Stack

NVMe-oF reliability depends on proper Linux kernel configuration and sysfs-based device management rather than additional libraries. Native NVMe multipath (kernel 4.15+) provides automatic path failover and device stability through subsystem-level devices that survive controller renumbering. The nvme-cli tool (versions 2.x or 3.x) offers stable device management with JSON output for programmatic parsing. For CSI drivers, direct sysfs parsing is recommended over CGO bindings to libnvme because it avoids build complexity while providing reliable access to `/sys/class/nvme` and `/sys/class/nvme-subsystem` hierarchies.

**Core technologies:**
- **Native NVMe Multipath (nvme_core.multipath)**: Handles controller renumbering via subsystem-level devices - essential for device path stability across reconnections
- **Kernel NVMe-oF Parameters (ctrl_loss_tmo, reconnect_delay)**: Automatic reconnection on connection loss - prevents manual intervention during transient network issues
- **Direct sysfs parsing**: Read NVMe device state from `/sys/class/nvme/*/subsysnqn` - avoids subprocess overhead and provides authoritative device information
- **Persistent device naming via udev**: `/dev/disk/by-id/*` symlinks - more stable than `/dev/disk/by-path` which uses PCI addresses that change on reconnection

**Critical configuration:**
- Enable native NVMe multipath (usually default on modern distros)
- Use `--ctrl-loss-tmo=60 --reconnect-delay=2` on nvme connect commands
- Rely on sysfs for device discovery, not nvme-cli output parsing
- Use NQN as ground truth for device identification, never device numbers

### Expected Features

Production CSI drivers implement comprehensive reliability features beyond basic volume provisioning. The research distinguishes table stakes features (expected by users, missing them raises production readiness concerns), differentiators (competitive advantages for production-grade drivers), and anti-features (commonly requested but problematic in practice). The foundation includes basic liveness probes, NodeStageVolume idempotency, mount point validation, connection timeout configuration, and structured logging. These are non-negotiable for CSI spec compliance and Kubernetes integration.

**Must have (table stakes):**
- **NodeStageVolume Idempotency**: CSI spec v1.5.0+ requires checking if volume already staged and verifying mount validity - kubelet no longer validates, driver must
- **Mount Point Validation**: Detect corrupted/stale mounts before use by checking `/proc/mounts` and comparing source device - critical after node restarts or NVMe reconnections
- **NodeGetVolumeStats**: Required by kubelet for volume health monitoring and PVC usage metrics
- **Connection Timeout Configuration**: Prevent indefinite hangs during NVMe operations with explicit timeouts (30s is common)
- **Graceful Cleanup on Errors**: Prevent orphaned NVMe connections by disconnecting on format/mount failures

**Should have (competitive):**
- **Volume Condition in NodeGetVolumeStats**: Node-side mount health detection for proactive problem reporting - enables CSIVolumeHealth feature
- **Automatic NVMe Reconnection**: Configure `--reconnect-delay=2 --ctrl-loss-tmo=60 --keep-alive-tmo=5` for handling transient network issues
- **Prometheus Metrics Endpoint**: Expose `/metrics` for operation durations, connection failures, mount failures - essential for production observability
- **Kubernetes Events for Storage Issues**: Post events to PVC/Pod on connection failures, mount failures, health check failures - user-visible feedback
- **NVMe Connection Status Checking**: Check `/sys/class/nvme-subsystem/*/subsysnqn` to find existing connections before reconnecting - prevents duplicate connections

**Defer (v2+):**
- **Volume Health Monitoring (external-health-monitor)**: Deploy sidecar controller for proactive detection - adds complexity, defer until basic health validation proves insufficient
- **NVMe Multipath Support**: Requires RDS dual-controller architecture (hardware limitation) - not feasible without infrastructure changes
- **Stale Mount Recovery**: Persist mount cache and reload on plugin restart - HIGH complexity, only needed if pods frequently restart without deletion

**Anti-features to avoid:**
- **Automatic Mount Repair Without Validation**: Remounting without data integrity checks can cause corruption
- **Aggressive Automatic Reconnection**: Too-frequent reconnection can mask persistent issues and cause I/O storms
- **Health Monitor Deleting Pods**: CSI drivers should report conditions, not make policy decisions
- **Complex State Machines for Recovery**: Increases bug surface area, prefer idempotent stateless operations

### Architecture Approach

The recommended architecture uses reactive device path resolution aligned with CSI idempotency principles. A new DevicePathResolver component maintains a cached NQN → device path mapping with TTL-based validation. This resolver sits between CSI operation handlers and the existing NVMe Connector, intercepting device path lookups to detect staleness before operations proceed. The pattern stores NQNs (persistent identifiers) in staging metadata instead of device paths, resolves paths on-demand during NodePublishVolume, detects stale mounts by comparing mount source device with current device, and remounts automatically when staleness detected.

**Major components:**
1. **Device Path Resolver** (NEW - `pkg/nvme/resolver.go`) — Maintains NQN → device path cache with validation; re-resolves on cache miss or device non-existence
2. **NodeStageVolume Handler** (ENHANCED) — Stores NQN in metadata instead of device path; uses resolver for device lookups; implements proper idempotency checks
3. **NodePublishVolume Handler** (ENHANCED) — Validates staging mount before bind mount; resolves current device path; remounts if stale detected
4. **Mount Validator** (NEW - `pkg/mount/mount.go`) — Reads `/proc/mounts` to detect stale mounts; implements `IsMountStale()` by comparing expected vs actual device
5. **NVMe Connector** (EXISTING, enhanced) — Already implements `GetDevicePath()` via sysfs scanning; needs enhancement for orphaned subsystem cleanup

**Key patterns:**
- **Cached Resolution**: Cache device paths with 30-second TTL; validate device exists before using cached path
- **Reactive Recovery**: Detect and fix issues during CSI operations rather than background monitoring
- **NQN as Source of Truth**: All lookups keyed by NQN; never assume device paths are persistent
- **Idempotent Operations**: All CSI operations check current state and return success if already in desired state

**Architecture decision: Reactive vs. Proactive**
Chose reactive (on-demand resolution) over proactive (background monitoring) because it aligns with CSI lifecycle expectations, has simpler implementation with no coordination complexity, uses fewer resources, fails fast for easier debugging, and is production-proven by AWS EBS and Longhorn CSI drivers. Background monitoring deferred to Phase 2+ if reactive approach proves insufficient.

### Critical Pitfalls

Research identified 10 pitfalls with varying severity based on production CSI driver issues and recent rds-csi bug fixes. The critical pitfalls are all in the foundation category and must be addressed in the MVP phase.

1. **Orphaned Subsystem False Positives** — NVMe subsystems persist after disconnection; `nvme list-subsys` shows "connected" when no device exists. After `IsConnected()` returns true, always verify `GetDevicePath()` succeeds. If device lookup fails, disconnect subsystem and reconnect. This is table-stakes reliability. (Fixed in commit bc90a9b)

2. **Controller Renumbering Breaks Device Path Assumptions** — NVMe-oF uses subsystem-based naming; after reconnect, controller nvme1 might contain namespace nvme2c1n2, and device appears as `/dev/nvme2n2` not `/dev/nvme1n1`. Never assume device name from controller name; scan namespace subdirectories; check both naming schemes (nvme2n2 and nvme2c1n2); use NQN as ground truth by reading `/sys/class/nvme/nvmeX/subsysnqn`. (Fixed in commits 74ce4d9, 10b237b)

3. **Non-Idempotent NodeStageVolume on Retries** — Naive implementations fail with "device already connected" or "already mounted" errors when kubelet retries. Check existing state first; return success if already staged with matching parameters; use canonical paths (resolve symlinks) for device comparison; distinguish "already mounted" from "mount failed".

4. **Aggressive Reconnection Without Backoff** — Tight retry loops flood logs, exhaust kernel resources, block CSI threads, create thundering herd. Implement exponential backoff (1s, 2s, 4s, 8s, max 60s); use context timeouts; coordinate retries keyed by NQN; return retriable errors to kubelet.

5. **Stale Mounts After Unclean Disconnection** — When NVMe device disappears unexpectedly, filesystem operations hang with "Transport endpoint is not connected"; umount hangs indefinitely; pod deletion gets stuck. Use lazy unmount (`umount -l`) for stale mounts; check mount health before operations; implement force-unmount path; clean up orphaned mounts on node plugin startup.

## Implications for Roadmap

Based on research, suggested four-phase structure aligned with architectural patterns and pitfall mitigation priorities:

### Phase 1: Foundation - Device Path Resolution (Week 1)
**Rationale:** Must establish reliable device discovery before any higher-level reliability features can work. Addresses critical pitfalls 1-3 which cause immediate operational failures. These are table-stakes features without which the driver cannot claim production readiness.

**Delivers:**
- DevicePathResolver with cached NQN → device path mapping
- NQN persistence in staging metadata
- Enhanced GetDevicePath() supporting NVMe-oF naming (both nvmeXnY and nvmeXcYnZ)
- Orphaned subsystem detection and cleanup

**Addresses features:**
- Mount Point Validation (table stakes)
- NVMe Connection Status Checking (table stakes)
- Connection Timeout Configuration (table stakes)

**Avoids pitfalls:**
- Pitfall 1: Orphaned subsystems
- Pitfall 2: Controller renumbering
- Pitfall 6: Device discovery races

**Implementation order:**
1. Create DevicePathResolver (`pkg/nvme/resolver.go`)
2. Add NQN persistence to staging metadata
3. Update NodeServer to use resolver
4. Enhance GetDevicePath for NVMe-oF naming

**Validation:** Existing tests pass; no behavior changes (pure refactoring)

### Phase 2: Stale Mount Detection and Recovery (Week 2)
**Rationale:** With reliable device discovery in place, can now detect and repair stale mounts. Implements core reliability improvement that fixes the reported bug. Builds on Phase 1 infrastructure to implement automatic recovery.

**Delivers:**
- Mount validation logic (reads `/proc/mounts`, compares devices)
- Stale mount detection in NodePublishVolume
- Automatic remounting when staleness detected
- Kubernetes events for mount failures

**Uses stack:**
- Sysfs parsing for device validation
- Resolver cache for device lookup

**Implements architecture:**
- Mount Validator component
- Reactive recovery pattern
- Enhanced NodePublishVolume handler

**Addresses features:**
- NodeStageVolume Idempotency (table stakes)
- Kubernetes Events for Storage Issues (should have)

**Avoids pitfalls:**
- Pitfall 3: Non-idempotent staging
- Pitfall 5: Stale mounts after unclean disconnection

**Implementation order:**
1. Add mount validation helpers (`pkg/mount/mount.go`)
2. Implement staleness detection
3. Add remounting logic with retry
4. Post Kubernetes events on failures

**Validation:** Manual NVMe reconnection triggers detection; automatic recovery succeeds

### Phase 3: Reconnection Resilience (Week 3)
**Rationale:** With detection and recovery working, add proper backoff and error handling to prevent thundering herd and resource exhaustion. Makes the driver production-ready under adverse conditions.

**Delivers:**
- Exponential backoff for connection retries
- Coordinated retry tracking by NQN
- Configurable ctrl_loss_tmo and reconnect_delay parameters
- Force unmount and lazy unmount for cleanup

**Addresses features:**
- Automatic NVMe Reconnection (should have)
- Configurable Retry Parameters (should have)
- Graceful Cleanup on Errors (table stakes)

**Avoids pitfalls:**
- Pitfall 4: Aggressive reconnection without backoff
- Pitfall 7: Ignoring nvme-cli exit codes
- Pitfall 8: Manual cleanup required for orphaned controllers

**Implementation order:**
1. Implement reconnect tracker with backoff
2. Add StorageClass parameters for connection tuning
3. Enhance DisconnectWithContext with fallback cleanup
4. Implement orphaned mount cleanup on startup

**Validation:** Network partition test shows exponential backoff; recovery after network restore

### Phase 4: Observability (Week 4)
**Rationale:** With core reliability features complete, add production monitoring capabilities. Enables operators to detect issues proactively and measure driver health.

**Delivers:**
- Prometheus metrics endpoint at `:8095/metrics`
- Metrics for connection operations, mount operations, orphan detection
- Enhanced structured logging
- Log level tuning

**Addresses features:**
- Prometheus Metrics Endpoint (should have)
- Structured Logging (table stakes)

**Avoids pitfalls:**
- Pitfall 9: Noisy logging
- Pitfall 10: No metrics

**Implementation order:**
1. Create metrics package with Prometheus collectors
2. Add metrics to resolver, connector, mount handler
3. Expose `/metrics` endpoint
4. Create ServiceMonitor for Prometheus Operator

**Validation:** Metrics appear in Prometheus after operations; events visible in kubectl describe

### Phase Ordering Rationale

- **Phase 1 before Phase 2**: Can't detect stale mounts without reliable device path resolution
- **Phase 2 before Phase 3**: Need working remount before adding backoff (otherwise backoff delays recovery unnecessarily)
- **Phase 3 before Phase 4**: Observability is most useful once core reliability works
- **No background monitoring phase**: Deferred indefinitely; reactive approach is sufficient and simpler

**Dependencies discovered:**
- Mount validation requires device path resolution (NQN lookup)
- Remounting requires both validation AND current device path
- Backoff requires connection state tracking
- Metrics require all operations to be instrumented

**Architecture pattern alignment:**
- All phases use reactive approach (no proactive monitoring)
- Each phase builds on previous phase's infrastructure
- Phase boundaries align with component boundaries
- Each phase can be validated independently

### Research Flags

**Phases likely needing deeper research during planning:**
- None - all four phases use standard CSI patterns with clear implementation paths. Research has provided sufficient detail for implementation.

**Phases with standard patterns (skip research-phase):**
- **Phase 1**: Device path resolution is well-documented in kernel docs and CSI driver implementations
- **Phase 2**: Mount validation follows standard Unix patterns; CSI idempotency is spec-defined
- **Phase 3**: Exponential backoff is a solved problem; nvme-cli exit code handling documented in recent bug fixes
- **Phase 4**: Prometheus integration is standard Go pattern; metrics design follows CSI community practices

**Validation approach:**
- Phase 1: Unit tests + CSI sanity tests
- Phase 2: Integration tests with forced reconnection
- Phase 3: Stress tests with network partition
- Phase 4: End-to-end tests with Prometheus scraping

**Implementation risk assessment:**
- **Low risk**: Phases 1 and 4 (refactoring and instrumentation)
- **Medium risk**: Phase 2 (remounting logic has edge cases)
- **Medium risk**: Phase 3 (backoff coordination across goroutines)

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Verified against official kernel docs, Red Hat documentation, nvme-cli manpages. Native multipath and sysfs parsing are production-standard. |
| Features | HIGH | Cross-referenced with Kubernetes CSI documentation, AWS EBS CSI implementation, SPDK CSI, Ceph CSI. Feature categorization validated across multiple production drivers. |
| Architecture | MEDIUM-HIGH | Reactive pattern proven in AWS EBS and Longhorn CSI drivers. Device path resolver pattern documented in multiple implementations. Some edge cases (concurrent operations) need testing. |
| Pitfalls | HIGH | Critical pitfalls verified through codebase analysis (bc90a9b, 74ce4d9, 10b237b commits). All major pitfalls have documented community issues and reproduction scenarios. Testing strategies validated against kernel documentation. |

**Overall confidence:** HIGH

The research is comprehensive with multiple independent sources corroborating findings. All critical architectural decisions have production precedent. The main areas of uncertainty are operational tuning values (cache TTL, backoff parameters) which can be adjusted based on testing.

### Gaps to Address

Research has identified clear implementation paths for all phases, but some details require validation during implementation:

- **Cache TTL tuning**: Research suggests 30-60 seconds but optimal value depends on production workload characteristics - start with 30s, add metric for cache hit rate, adjust based on data
- **Backoff parameters**: Research shows exponential backoff starting at 1s with 60s max, but may need tuning for RDS-specific behavior - make configurable via flags, document recommendations
- **Orphaned mount detection on startup**: Pattern exists in Ceph CSI but integration with kubelet mount tracking needs testing - implement conservative approach (log only, don't auto-cleanup) initially
- **Concurrent operation safety**: Resolver cache needs proper locking but research doesn't specify granularity - use RWMutex per-cache, validate with stress tests
- **StorageClass parameter names**: Need to choose parameter names for ctrl_loss_tmo, reconnect_delay - follow nvme-cli naming conventions for consistency

All gaps are implementation details rather than architectural uncertainties. The recommended patterns are clear and production-proven.

## Sources

### Primary (HIGH confidence)
- [Linux NVMe Multipath - Kernel Documentation](https://docs.kernel.org/admin-guide/nvme-multipath.html) - Authoritative source on native multipath and subsystem devices
- [Red Hat: Enabling multipathing on NVMe devices](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/configuring_device_mapper_multipath/enabling-multipathing-on-nvme-devices_configuring-device-mapper-multipath) - Production configuration patterns
- [nvme-connect man page](https://www.mankier.com/1/nvme-connect) - Official connection parameter documentation
- [Kubernetes CSI Volume Health Monitoring](https://kubernetes-csi.github.io/docs/volume-health-monitor.html) - Official CSI feature documentation
- [CSI Driver Object - requiresRepublish](https://kubernetes-csi.github.io/docs/csi-driver-object.html) - CSI lifecycle documentation

### Secondary (MEDIUM confidence)
- [AWS EBS CSI Driver Implementation](https://github.com/kubernetes-sigs/aws-ebs-csi-driver) - Production CSI driver patterns for idempotency and mount validation
- [Longhorn CSI patterns](https://github.com/longhorn/longhorn) - Device path management and recovery strategies
- [SPDK CSI Driver](https://github.com/spdk/spdk-csi) - NVMe-oF specific implementation patterns
- [kubernetes-csi/csi-lib-iscsi](https://github.com/kubernetes-csi/csi-lib-iscsi) - Multipath device management reference (iSCSI but applicable patterns)

### Tertiary (verified through multiple sources)
- [AWS EBS CSI Issue #1076: NodeStage idempotency](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/1076) - Real-world idempotency challenges
- [Dell CSM Issue #496: NVMe-oF connectivity issues](https://github.com/dell/csm/issues/496) - Orphaned subsystem patterns
- [Ceph CSI Issue #859: NodeStageVolume failures after reboot](https://github.com/ceph/ceph-csi/issues/859) - Stale mount recovery patterns
- [Kubernetes Issue #112969: CSI SetUpAt retry behavior](https://github.com/kubernetes/kubernetes/issues/112969) - kubelet retry semantics

### Codebase-specific (HIGH confidence)
- Commit bc90a9b: "fix(nvme): handle orphaned subsystems in ConnectWithContext" - Validates pitfall 1
- Commit 74ce4d9, 10b237b: "fix(nvme): handle NVMe-oF namespace device naming correctly" - Validates pitfall 2
- `pkg/nvme/nvme.go`: Current Connector implementation with orphaned subsystem handling
- `pkg/driver/node.go`: Current NodeStageVolume/NodePublishVolume implementation

---
*Research completed: 2026-01-30*
*Ready for roadmap: yes*
