# Feature Research: NVMe-oF CSI Driver Reliability

**Domain:** Kubernetes CSI Driver - NVMe/TCP Storage Health Monitoring
**Researched:** 2026-01-30
**Confidence:** MEDIUM

## Feature Landscape

### Table Stakes (Users Expect These)

Features production CSI drivers implement for reliability. Missing these = production readiness concerns.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Basic Liveness Probe** | Standard Kubernetes pattern for CSI drivers | LOW | Already implemented (v2.12.0 sidecar). Uses `/healthz` endpoint on port 9808 with configurable probe intervals. |
| **NodeStageVolume Idempotency** | Required by CSI spec v1.5.0+ | MEDIUM | CSI drivers MUST check if volume already staged, verify mount validity themselves (kubelet no longer does this). Return success if already staged with matching capability. |
| **NodeGetVolumeStats** | Kubelet requires for volume health monitoring | LOW | Returns filesystem usage (bytes/inodes). Required for PVC usage metrics and health checks. Already implemented in current codebase. |
| **Mount Point Validation** | Detect corrupted/stale mounts before use | MEDIUM | CSI drivers must validate mount paths exist and are valid before reusing. Check `/proc/mounts` and compare source device. Critical after node restarts or NVMe reconnections. |
| **Connection Timeout Configuration** | Prevent indefinite hangs during NVMe operations | LOW | NVMe connect operations should have explicit timeouts (30s is common). Current codebase has 30s device discovery timeout. |
| **Graceful Cleanup on Errors** | Prevent orphaned NVMe connections | MEDIUM | On format/mount failures, must disconnect NVMe target. Current code has this in NodeStageVolume. |
| **Structured Logging** | Debugging in production requires correlation | LOW | Use klog with volume ID, operation type, node ID fields. Already present in current implementation. |

### Differentiators (Competitive Advantage)

Features that set production-grade CSI drivers apart. Not required, but valuable for reliability.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Volume Health Monitoring (external-health-monitor)** | Proactive detection of abnormal volume conditions | MEDIUM | Deploy external-health-monitor-controller sidecar. Implements ListVolumes or ControllerGetVolume RPC to check health (5min interval for ListVolumes, 1min for ControllerGetVolume). Reports events on PVC when abnormal. Kubernetes 1.21+ Alpha feature. |
| **Volume Condition in NodeGetVolumeStats** | Node-side mount health detection | MEDIUM | Extend NodeGetVolumeStats to return volume condition (VOLUME_CONDITION node capability). Kubelet checks this (CSIVolumeHealth feature gate) and logs events on Pods when abnormal. Requires implementing `VolumeCondition` field in response. |
| **Automatic NVMe Reconnection** | Handles transient network issues without manual intervention | HIGH | Configure nvme-cli connection parameters: `--reconnect-delay=2 --ctrl-loss-tmo=60 --keep-alive-tmo=5`. Native multipath enabled by default on Linux 5.0+. Simplyblock and similar CSI drivers handle this automatically. |
| **NVMe Multipath Support** | Redundant paths for high availability | HIGH | Native NVMe multipath (`nvme_core multipath=y`) recommended over device-mapper multipath for NVMe. Requires multiple NVMe controllers/IPs. ANA (Asymmetric Namespace Access) protocol for path failover. Lower overhead than DM-MPIO. |
| **Stale Mount Recovery** | Auto-recover from CSI plugin crashes | HIGH | Persist mount cache entries to node local directory. On CSI plugin startup, load cache, verify volume IDs exist in cluster, check staging paths exist, remount if needed. Enables auto-recovery when pods restart without needing pod deletion. |
| **Prometheus Metrics Endpoint** | Observability for operations teams | MEDIUM | Expose `/metrics` endpoint (typically port 8095). Metrics: volume provision duration, attach/detach errors, connection counts, NVMe connection failures. Create ServiceMonitor for Prometheus Operator or configure scrape job. |
| **Kubernetes Events for Storage Issues** | User-visible feedback on volume problems | LOW | Post events to PVC/Pod on: connection failures, mount failures, health check failures, orphan detection. Current orphan reconciler has this pattern. |
| **NVMe Connection Status Checking** | Detect existing connections before reconnecting | MEDIUM | Check `/sys/class/nvme-subsystem/*/subsysnqn` to find existing connections by NQN. Return early if already connected (idempotency). Already present in current nvme.go implementation. |
| **Configurable Retry Parameters** | Tune resilience for different environments | LOW | Make retry counts, backoff intervals, timeouts configurable via flags or StorageClass parameters. Current SSH client has hardcoded 3 retries with exponential backoff. |
| **Volume Attachment Fencing** | Prevent split-brain scenarios | HIGH | Track which node has volume attached. Refuse attachment if already attached elsewhere (unless multi-attach supported). Use PV.Status.Phase and annotations. Critical for ReadWriteOnce volumes. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems in CSI driver reliability implementations.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Automatic Mount Repair Without Validation** | "Just fix it automatically" | Remounting without validating data integrity can cause corruption. Silent failures mask underlying issues (bad NVMe connection, network problems). | Detect stale mounts, post Kubernetes event, require explicit pod restart. Log detailed diagnostic info for troubleshooting. |
| **Aggressive Automatic Reconnection** | Minimize downtime | Reconnecting too frequently during maintenance can cause I/O storms. May mask persistent issues (bad NIC, misconfigured network). Can trigger Kubernetes pod restarts if health checks fail. | Use exponential backoff. Set `ctrl-loss-tmo` to reasonable value (60-300s). Let kernel's native reconnect handle transient issues. Post events for persistent failures. |
| **Health Monitor Deleting Pods** | "Self-healing" | CSI drivers should never delete pods - that's a policy decision. Could cause cascading failures. Violates separation of concerns. | Report abnormal conditions via events/metrics. Let cluster admin or policy controller (like Descheduler) make deletion decisions. |
| **Synchronous Health Checks in Data Path** | "Real-time validation" | Adding health checks to NodePublishVolume/NodeStageVolume slows critical path. Can cause timeouts during volume provisioning. | Use asynchronous periodic health monitoring (external-health-monitor). Health checks in NodeGetVolumeStats are ok (called periodically by kubelet). |
| **Complex State Machines for Recovery** | Handle every edge case | Increases code complexity and bug surface area. Hard to test all state transitions. May still miss edge cases. | Keep operations idempotent and stateless. Rely on Kubernetes reconciliation loop. Clear error messages for manual intervention. |
| **Custom Mount Retry Logic in CSI Driver** | "Be resilient" | Kubernetes already retries failed mounts. CSI driver retrying adds unpredictable delays. Kubelet has timeout enforcement - driver retries fight with this. | Let individual operations fail fast with clear errors. Kubernetes volume attach/mount loop handles retries at pod level. |

## Feature Dependencies

```
[NVMe Connection Status Checking]
    ├──requires──> [Mount Point Validation]
    └──requires──> [NodeStageVolume Idempotency]

[Stale Mount Recovery]
    ├──requires──> [Mount Point Validation]
    └──requires──> [Connection Status Checking]

[Volume Health Monitoring]
    ├──requires──> [NodeGetVolumeStats]
    └──enhances──> [Prometheus Metrics Endpoint]

[Volume Condition in NodeGetVolumeStats]
    └──requires──> [Mount Point Validation]

[Automatic NVMe Reconnection]
    └──requires──> [Connection Timeout Configuration]

[NVMe Multipath Support]
    ├──requires──> [Automatic NVMe Reconnection]
    └──conflicts──> [Single IP Architecture]

[Volume Attachment Fencing]
    └──requires──> [Controller tracks attachment state]

[Prometheus Metrics]
    └──enhances──> [Kubernetes Events for Storage Issues]
```

### Dependency Notes

- **Mount Point Validation requires Connection Status Checking:** Can't determine if mount is stale without knowing if NVMe connection is alive
- **Stale Mount Recovery requires both:** Must validate mounts AND check connection status before attempting recovery
- **Volume Health Monitoring enhances Metrics:** Health monitor data should feed into Prometheus metrics for unified observability
- **Multipath conflicts with Single IP:** Current RDS CSI uses single IP per volume. Multipath requires multiple controller IPs/paths.
- **Volume Condition requires Mount Validation:** Can't report accurate condition without validating mount point first

## MVP Definition

### Launch With (Reliability Milestone)

Minimum viable reliability features for brownfield bug fix.

- [x] **Basic Liveness Probe** — Already implemented (v2.12.0 sidecar)
- [x] **NodeGetVolumeStats** — Already implemented in node.go
- [ ] **NodeStageVolume Idempotency Enhancement** — Add explicit mount validation (check `/proc/mounts`)
- [ ] **Mount Point Validation** — Verify staging path is valid mount before NodePublishVolume
- [ ] **NVMe Connection Status Checking** — Verify connection alive before declaring success (enhance existing IsConnected())
- [ ] **Connection Timeout Configuration** — Make device discovery timeout configurable (currently hardcoded 30s)
- [ ] **Kubernetes Events for Mount Failures** — Post events to Pod when mount validation fails

### Add After Core Validation (v0.2)

Features to add once mount validation is working.

- [ ] **Volume Condition in NodeGetVolumeStats** — Report ABNORMAL when mount is stale
- [ ] **Automatic NVMe Reconnection Configuration** — Expose `ctrl-loss-tmo`, `reconnect-delay` as StorageClass params
- [ ] **Prometheus Metrics Endpoint** — Basic metrics: connection failures, mount failures, operation durations
- [ ] **Configurable Retry Parameters** — Make SSH retry count/backoff configurable via flags
- [ ] **Stale Mount Recovery** — Persist mount cache, reload on plugin restart (HIGH complexity - defer if not needed)

### Future Consideration (v1.0+)

Features to defer until production usage patterns are clear.

- [ ] **Volume Health Monitoring (external-health-monitor)** — Deploy sidecar controller after validating basic health works
- [ ] **NVMe Multipath Support** — Requires RDS dual-controller architecture (hardware limitation)
- [ ] **Volume Attachment Fencing** — Low priority (single-node workloads don't need this)
- [ ] **Advanced Metrics** — Detailed latency histograms, per-volume metrics (once basic metrics prove useful)

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority | Phase |
|---------|------------|---------------------|----------|-------|
| Mount Point Validation | HIGH | MEDIUM | P1 | MVP |
| NodeStageVolume Idempotency | HIGH | MEDIUM | P1 | MVP |
| Connection Status Checking | HIGH | LOW | P1 | MVP |
| Kubernetes Events | HIGH | LOW | P1 | MVP |
| Volume Condition in Stats | MEDIUM | MEDIUM | P2 | v0.2 |
| Prometheus Metrics | MEDIUM | MEDIUM | P2 | v0.2 |
| Reconnection Config | MEDIUM | LOW | P2 | v0.2 |
| Stale Mount Recovery | MEDIUM | HIGH | P2 | v0.2 (conditional) |
| external-health-monitor | LOW | MEDIUM | P3 | v1.0+ |
| NVMe Multipath | LOW | HIGH | P3 | v1.0+ (hardware dependent) |
| Volume Attachment Fencing | LOW | MEDIUM | P3 | v1.0+ |

**Priority key:**
- P1: Must have for bug fix (NVMe reconnection stale mount issue)
- P2: Should have for production readiness
- P3: Nice to have, future consideration

## Implementation Patterns from Production CSI Drivers

### AWS EBS CSI Driver Pattern
- NodeStageVolume checks if volume already staged by comparing device with source
- Returns success if already staged (idempotent)
- Uses device statistics for NodeGetVolumeStats
- Exposes Prometheus metrics at `/metrics` endpoint (v1.37.0+)

### SPDK CSI Driver Pattern
- Conforms to CSI Spec v1.7.0
- Provisions SPDK volumes via NVMe-oF or iSCSI
- Uses liveness probe sidecar for health checks
- Multi-node support with xPU connections

### iSCSI CSI Driver Pattern (csi-lib-iscsi)
- Multipath device management (flush/resize operations)
- Connection management with authentication handling
- Device discovery and error handling in connection lifecycle
- Handles stale paths via multipath layer

### Ceph CSI Driver Pattern
- Implements mount cache loading on CSI plugin restart
- Checks if volume IDs exist in cluster before remounting
- Verifies staging paths exist, remounts if needed
- Enables auto-recovery when pods exit and restart without deletion

### Common Patterns Across Drivers
1. **Idempotent Operations**: All CSI drivers validate operation already done before proceeding
2. **Liveness Probe Sidecar**: Standard pattern using port 9808 for `/healthz` endpoint
3. **Structured Logging**: klog with volume ID, operation, node ID fields
4. **Separate Health Monitor**: external-health-monitor as sidecar, not in main driver
5. **Mount Validation**: Driver responsibility (kubelet no longer validates)
6. **Graceful Cleanup**: Disconnect/cleanup on errors to prevent orphans

## Specific Configuration Values from Research

### NVMe Connection Parameters (from nvme-cli manpages)
```bash
nvme connect \
  --transport=tcp \
  --traddr=<IP> \
  --trsvcid=<PORT> \
  --nqn=<NQN> \
  --reconnect-delay=2      # Delay before reconnect attempt (seconds)
  --ctrl-loss-tmo=60       # Max time kernel will retry connection (seconds, 0=infinite)
  --keep-alive-tmo=5       # Keep-alive timeout (seconds)
  --nr-io-queues=6         # Number of I/O queues (optional)
```

### Kernel Parameters for Production (from community sources)
```bash
# /etc/default/grub additions for NVMe reliability:
nvme_core.io_timeout=255           # I/O timeout (seconds, max 4294967295 on kernel 4.15+)
nvme_core.max_retries=10           # I/O retry count (default 5, kernel 4.14+)
nvme_core.default_ps_max_latency_us=0  # Disable power state transitions
nvme_core.shutdown_timeout=10      # Shutdown timeout (seconds)

# Performance optimizations (may help prevent disconnects):
pcie_aspm.policy=performance       # PCIe power management
pcie_aspm=off                      # Disable ASPM
pcie_port_pm=off                   # Disable PCIe port power management
iommu=pt                           # Reduce DMA mapping latency
```

### Health Monitor Configuration (from Kubernetes CSI docs)
```yaml
# external-health-monitor-controller sidecar
args:
  - "--v=5"
  - "--csi-address=/csi/csi.sock"
  - "--leader-election"
  - "--http-endpoint=:8080"
  - "--enable-node-watcher=false"      # Optional: monitor node failures
  - "--monitor-interval=5m"             # ListVolumes check interval
# OR
  - "--monitor-interval=1m"             # ControllerGetVolume check interval
```

### Prometheus Metrics Configuration
```yaml
# Metrics endpoint in CSI driver (common pattern)
ports:
  - name: metrics
    containerPort: 8095
    protocol: TCP

# ServiceMonitor for Prometheus Operator
spec:
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

## Current RDS CSI Driver State

### Already Implemented (from codebase review)
- ✅ Liveness probe sidecar (v2.12.0) in both controller and node DaemonSet
- ✅ NodeGetVolumeStats (returns filesystem statistics in node.go)
- ✅ NVMe Connection Status Checking (IsConnected() in pkg/nvme/nvme.go)
- ✅ Device discovery timeout (30s hardcoded in NodeStageVolume)
- ✅ Graceful cleanup on errors (disconnects NVMe on format/mount failures)
- ✅ Structured logging (klog with volume IDs throughout)
- ✅ Orphan reconciler (detection and cleanup of orphaned volumes)

### Gaps to Address (from bug report)
- ❌ Mount point validation (doesn't check if staging path is stale after reconnection)
- ❌ NodeStageVolume idempotency (doesn't validate existing mounts thoroughly)
- ❌ Volume condition reporting (NodeGetVolumeStats doesn't return health condition)
- ❌ Kubernetes events for mount failures (no events posted to Pods on stale mounts)
- ❌ Configurable NVMe connection parameters (reconnect-delay, ctrl-loss-tmo hardcoded in kernel)
- ❌ Prometheus metrics endpoint (no observability beyond logs)

### Architecture Constraints
- Single NVMe IP per volume (10.42.68.1 data plane, 10.42.241.3 control plane)
- No native multipath support (RDS has single controller)
- NixOS worker nodes (diskless, network boot)
- Production cluster: 5 nodes, mix of x86 and ARM

## Sources

**Official Kubernetes CSI Documentation:**
- [Volume Health Monitoring - Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/volume-health-monitor.html)
- [Volume Health Monitoring | Kubernetes](https://kubernetes.io/docs/concepts/storage/volume-health-monitoring/)
- [external-health-monitor-controller - Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/external-health-monitor-controller.html)
- [Deploying a CSI Driver on Kubernetes - Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/deploying.html)

**CSI Driver Implementations:**
- [GitHub - kubernetes-csi/external-health-monitor](https://github.com/kubernetes-csi/external-health-monitor)
- [GitHub - spdk/spdk-csi: CSI driver for SPDK NVMe-oF](https://github.com/spdk/spdk-csi)
- [GitHub - kubernetes-csi/csi-driver-iscsi](https://github.com/kubernetes-csi/csi-driver-iscsi)
- [GitHub - kubernetes-sigs/aws-ebs-csi-driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver)
- [aws-ebs-csi-driver/pkg/driver/node.go](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/pkg/driver/node.go)

**NVMe-oF Configuration:**
- [Reconnecting Logical Volume - Simplyblock Documentation](https://docs.simplyblock.io/25.7.1/maintenance-operations/reconnect-nvme-device/)
- [nvme-connect(1) — nvme-cli — Debian Manpages](https://manpages.debian.org/testing/nvme-cli/nvme-connect.1.en.html)
- [Linux NVMe multipath — The Linux Kernel documentation](https://docs.kernel.org/admin-guide/nvme-multipath.html)
- [SPDK: NVMe Multipath](https://spdk.io/doc/nvme_multipath.html)

**Production Configuration:**
- [NVMe health monitoring | VMware vSphere](https://community.broadcom.com/vmware-cloud-foundation/discussion/nvme-health-monitoring)
- [Linux kernel optimizations for NVMe · GitHub](https://gist.github.com/v-fox/b7adbc2414da46e2c49e571929057429)
- [AWS I/O operation timeout setting on NVMe VMs · Issue #5694 · kubernetes/kops](https://github.com/kubernetes/kops/issues/5694)

**Monitoring and Metrics:**
- [Metrics - Secrets Store CSI Driver](https://secrets-store-csi-driver.sigs.k8s.io/topics/metrics)
- [HPE CSI Info Metrics Provider for Prometheus](https://scod.hpedev.io/csi_driver/metrics.html)
- [Announcing vSphere CSI driver v2.5 metrics for Prometheus monitoring](https://cormachogan.com/2022/03/10/announcing-vsphere-csi-driver-v2-5-metrics-for-prometheus-monitoring/)
- [Amazon EBS detailed performance statistics](https://docs.aws.amazon.com/ebs/latest/userguide/nvme-detailed-performance-stats.html)

**Stale Mount Recovery:**
- [remount old mount point when csi plugin unexpect exit · Pull Request #282 · ceph/ceph-csi](https://github.com/ceph/ceph-csi/pull/282)
- [fix: corrupted mount point in csi driver node stage/publish · Pull Request #88569 · kubernetes/kubernetes](https://github.com/kubernetes/kubernetes/pull/88569)
- [When mount dies, it is not remounted · Issue #164 · kubernetes-csi/csi-driver-smb](https://github.com/kubernetes-csi/csi-driver-smb/issues/164)

**Confidence Note:** MEDIUM confidence overall. Most findings verified across multiple official Kubernetes CSI documentation sources and production CSI driver implementations (AWS EBS, SPDK, Ceph). NVMe-oF specific configuration sourced from official nvme-cli documentation and kernel docs. Some implementation complexity estimates based on CSI spec requirements rather than hands-on implementation.

---
*Feature research for: RDS CSI Driver NVMe-oF Reliability Enhancement*
*Researched: 2026-01-30*
