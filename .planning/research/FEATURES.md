# Feature Research: CSI Driver Gap Analysis for v0.9.0

**Domain:** Kubernetes Container Storage Interface (CSI) Driver
**Researched:** 2026-02-04
**Confidence:** HIGH

## Executive Summary

Based on comparison with production CSI drivers (AWS EBS CSI, Longhorn, democratic-csi, Ceph RBD) and the CSI specification, the RDS CSI driver has achieved **strong core functionality** but is **missing several optional features** that production drivers typically implement. The driver currently supports:

✅ **Core volume lifecycle** (create, delete, attach, detach, mount, unmount)
✅ **Block and filesystem volumes** (ReadWriteOnce, ReadWriteMany)
✅ **Volume expansion** (online resizing)
✅ **Production stability features** (reconnection resilience, stale mount recovery, orphan cleanup)
✅ **Observability** (Prometheus metrics, Kubernetes events)

**Key gaps for v0.9.0:**
- ❌ Volume snapshots (CREATE_DELETE_SNAPSHOT capability)
- ❌ Volume cloning (CLONE_VOLUME capability)
- ❌ Topology awareness (VOLUME_ACCESSIBILITY_CONSTRAINTS)
- ⚠️ Testing maturity (CSI sanity tests, E2E test suite, chaos testing)
- ⚠️ Multi-tenancy features (capacity tracking, quotas)

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = driver feels incomplete.

| Feature | Why Expected | Status | Complexity | Notes |
|---------|--------------|--------|------------|-------|
| Dynamic provisioning | Core CSI capability, all drivers support | ✅ Implemented | LOW | Via CreateVolume/DeleteVolume |
| Volume mounting | Required for pods to use storage | ✅ Implemented | MEDIUM | Two-phase: stage → publish |
| Volume lifecycle management | Create, attach, mount, unmount, detach, delete | ✅ Implemented | MEDIUM | Full CSI controller + node services |
| Access mode validation | ReadWriteOnce, ReadWriteMany, ReadOnlyMany | ✅ Implemented | LOW | ValidateVolumeCapabilities |
| Volume persistence | Data survives pod restarts | ✅ Implemented | LOW | NVMe/TCP block storage |
| Error reporting | Clear error messages via events | ✅ Implemented | LOW | EventPoster + Kubernetes events |
| Basic metrics | Volume count, operation latency | ✅ Implemented | LOW | Prometheus endpoint on :9809 |
| Volume expansion | Resize volumes post-creation | ✅ Implemented | MEDIUM | ControllerExpandVolume + NodeExpandVolume |
| Health checking | Liveness probe for CSI plugin | ✅ Implemented | LOW | CSI livenessprobe sidecar |
| CSI spec compliance | Implements Identity, Controller, Node services | ✅ Implemented | MEDIUM | CSI v1.5.0+ |

**Assessment:** All table stakes features implemented. RDS CSI driver meets baseline expectations for production use.

### Differentiators (Competitive Advantage)

Features that set production drivers apart. Not required, but highly valued.

| Feature | Value Proposition | Status | Complexity | Notes |
|---------|-------------------|--------|------------|-------|
| **Volume snapshots** | Point-in-time backups, fast recovery | ❌ Missing | HIGH | Requires CREATE_DELETE_SNAPSHOT capability + Btrfs snapshot integration |
| **Volume cloning** | Fast VM template duplication | ❌ Missing | HIGH | Requires CLONE_VOLUME capability + Btrfs clone/reflink |
| **Topology awareness** | Intelligent zone/rack placement | ❌ Missing | MEDIUM | Requires VOLUME_ACCESSIBILITY_CONSTRAINTS + node labels |
| **NVMe/TCP protocol** | Lower latency than iSCSI (~1ms vs ~3ms) | ✅ Implemented | N/A | Unique differentiator vs iSCSI drivers |
| **Reconnection resilience** | Volumes survive network hiccups | ✅ Implemented | HIGH | Device path resolution, stale mount recovery, kernel reconnect params |
| **Orphan cleanup** | Automatic leak detection and cleanup | ✅ Implemented | MEDIUM | Reconciler scans RDS vs Kubernetes PVs |
| **Multi-path support** | High availability with redundant paths | ⚠️ Partial | MEDIUM | NVMe supports multipath, not explicitly configured |
| **ReadWriteMany for block** | Multiple pods access same block device | ✅ Implemented | LOW | Enabled for KubeVirt VMs with migration |
| **Raw block volumes** | Direct block device access | ✅ Implemented | LOW | VolumeMode=Block support |
| **Volume fencing** | Prevent split-brain during migrations | ✅ Implemented | HIGH | SSH-based force disconnect |

**Assessment:** Strong operational resilience features (reconnection, orphan cleanup, volume fencing) differentiate from basic drivers. Missing advanced data management features (snapshots, clones) common in enterprise drivers.

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems in CSI driver context.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Volume shrinking** | "Save space on oversized volumes" | Filesystem corruption risk, unsupported by most filesystems (ext4, xfs) | Size volumes appropriately, use monitoring to right-size future volumes |
| **Automatic snapshot scheduling** | "Set and forget backups" | CSI drivers should be thin, not backup orchestrators; conflicts with existing backup tools | Use Velero, Kasten K10, or Kubernetes CronJob + VolumeSnapshot CRDs |
| **In-driver replication** | "Built-in HA" | Adds complexity, conflicts with storage backend replication; difficult to test | Use RDS-level replication or external replication tools |
| **Volume encryption in driver** | "Security compliance" | Key management complexity, performance overhead; backend encryption is simpler | Use RDS Btrfs encryption or dm-crypt at node level |
| **Multi-backend support** | "Use multiple RDS servers" | Single driver = single backend paradigm in CSI; topology handles multi-zone | Deploy multiple CSI driver instances with different StorageClasses |
| **ReadWriteMany for filesystem** | "Shared filesystem across pods" | Requires NFS layer or cluster filesystem; corruption risk without proper locking | Use ReadWriteOnce + application-level sharding, or deploy NFS provisioner on top |

**Assessment:** Current design appropriately avoids these anti-features. Volume expansion (not shrinking) is correctly implemented. No plans to add backup scheduling, in-driver replication, or encryption.

## Feature Dependencies

```
[Volume Snapshots]
    └──requires──> [Btrfs snapshot support]
    └──requires──> [external-snapshotter sidecar]
    └──requires──> [VolumeSnapshot CRDs]

[Volume Cloning]
    └──requires──> [Volume Snapshots] (or independent Btrfs clone)
    └──requires──> [CreateVolume with VolumeContentSource]

[Topology Awareness]
    └──requires──> [Node labels (topology.kubernetes.io/zone)]
    └──requires──> [CSIDriver topology keys]
    └──requires──> [WaitForFirstConsumer binding mode] (already implemented)

[Multi-path NVMe/TCP]
    └──requires──> [Multiple RDS IP addresses in StorageClass]
    └──requires──> [nvme-cli multipath support]
    └──enhances──> [Reconnection Resilience] (already implemented)

[CSI Sanity Tests]
    └──validates──> [All CSI operations]
    └──blocks──> [v1.0 release confidence]

[E2E Test Suite]
    └──requires──> [CSI Sanity Tests] (baseline)
    └──validates──> [Real cluster workflows]
    └──blocks──> [v1.0 release confidence]

[Chaos Testing]
    └──requires──> [E2E Test Suite] (baseline)
    └──validates──> [Reconnection Resilience]
```

### Dependency Notes

- **Volume Snapshots require Volume Cloning foundation:** Most CSI drivers implement cloning via temporary snapshots (snapshot → clone from snapshot → delete snapshot). Independent implementation paths exist but snapshots are typically the foundation.
- **Topology Awareness enhances placement:** While not blocking core functionality, topology enables multi-zone clusters and reduces cross-zone traffic. Requires StorageClass `volumeBindingMode: WaitForFirstConsumer` (already implemented).
- **Testing features are sequential:** CSI sanity tests validate spec compliance before E2E tests validate real-world workflows before chaos tests validate resilience. Cannot skip levels.
- **Multi-path enhances resilience:** Current reconnection resilience works with single path. Multi-path would reduce reconnection delays by failing over to secondary paths instantly.

## Gap Analysis by Peer Comparison

### AWS EBS CSI Driver (Baseline Enterprise Driver)

**Features we match:**
- ✅ Dynamic provisioning
- ✅ Volume expansion (online resizing)
- ✅ Raw block volumes
- ✅ Topology awareness via availability zones (we're missing)
- ✅ Volume snapshots (we're missing)
- ✅ Prometheus metrics
- ✅ Multi-attach for specialized use cases

**Features they have that we're missing:**
- ❌ **Volume snapshots** (CREATE_DELETE_SNAPSHOT) — High priority gap
- ❌ **Volume cloning** (CLONE_VOLUME) — Medium priority gap
- ❌ **Topology awareness** (VOLUME_ACCESSIBILITY_CONSTRAINTS) — Low priority (single-site homelab)
- ❌ **Comprehensive E2E test suite** — High priority for v1.0
- ❌ **VolumeAttributesClass** (modify IOPS/throughput post-creation) — Not applicable to NVMe/TCP

**Confidence:** HIGH (official AWS documentation, GitHub repository analysis)

### Longhorn CSI Driver (Cloud-Native Storage)

**Features we match:**
- ✅ Dynamic provisioning
- ✅ Volume expansion
- ✅ Volume snapshots (we're missing)
- ✅ Volume cloning (we're missing)
- ✅ Backup/restore via snapshots (we're missing)

**Features they have that we're missing:**
- ❌ **Volume snapshots** — Longhorn uses filesystem-level snapshots
- ❌ **Volume cloning** — Fast duplication for stateful workloads
- ❌ **Cross-volume replication** — Enterprise feature, not applicable
- ❌ **ReadWriteMany native support** — We support RWX for block, not filesystem

**Unique to Longhorn (not gaps for us):**
- Distributed storage across multiple nodes (RDS is centralized)
- Built-in backup to S3/NFS (use external backup tools)

**Confidence:** MEDIUM (official documentation, community blog posts)

### democratic-csi (TrueNAS/ZFS Driver)

**Features we match:**
- ✅ NFS + iSCSI + NVMe-oF protocol support (we have NVMe-oF)
- ✅ Volume resizing
- ✅ Volume snapshots (we're missing)
- ✅ Volume cloning (we're missing)
- ✅ Multi-protocol support (we're NVMe/TCP only by design)

**Features they have that we're missing:**
- ❌ **Volume snapshots** — ZFS snapshots are instant, Btrfs snapshots should be similar
- ❌ **Volume cloning** — ZFS clones are instant, Btrfs reflinks similar
- ❌ **NFS provisioner mode** — Not applicable (we're block-only)

**Confidence:** HIGH (GitHub repository, user deployment guides)

### Ceph RBD CSI Driver (Enterprise Storage)

**Features we match:**
- ✅ Dynamic provisioning
- ✅ Volume expansion
- ✅ Volume snapshots (we're missing)
- ✅ Volume cloning (we're missing)
- ✅ Raw block volumes
- ✅ ReadWriteMany for block

**Features they have that we're missing:**
- ❌ **Volume snapshots** — RBD snapshots with configurable depth limits
- ❌ **Volume cloning** — RBD clones with automatic flattening
- ❌ **Snapshot flattening** — Prevents deep snapshot chains (Btrfs may need similar)

**Confidence:** HIGH (official Ceph documentation, GitHub repository)

## Testing Requirements

### Table Stakes Testing (Must Have for v1.0)

| Test Type | Purpose | Status | Priority | Effort |
|-----------|---------|--------|----------|--------|
| **CSI sanity tests** | Validate CSI spec compliance | ❌ Missing | P0 | LOW |
| **Unit tests** | Code coverage for logic paths | ✅ 65% coverage | P0 | LOW |
| **Integration tests (manual)** | Real cluster E2E workflows | ✅ Partial | P0 | LOW |
| **Smoke tests** | Basic create → mount → delete | ✅ Manual | P1 | LOW |
| **Regression tests** | Prevent breaking changes | ⚠️ Manual | P1 | MEDIUM |
| **Volume lifecycle tests** | Create/delete/expand workflows | ✅ Manual | P0 | LOW |

**Gap:** CSI sanity tests are **mandatory** for spec compliance validation. Currently deferred since Milestone 4. High confidence in implementation, but sanity tests provide automated proof.

**Recommendation:** Add CSI sanity tests in v0.9.0 before v1.0 release.

### Differentiators (Production-Grade Testing)

| Test Type | Purpose | Status | Priority | Effort |
|-----------|---------|--------|----------|--------|
| **Automated E2E test suite** | Continuous validation | ❌ Missing | P1 | HIGH |
| **Chaos testing** | Network partition, node failure | ❌ Missing | P2 | HIGH |
| **Performance benchmarks** | Latency, throughput, IOPS | ⚠️ Ad-hoc | P2 | MEDIUM |
| **Load testing** | Concurrent volume operations | ❌ Missing | P2 | MEDIUM |
| **Soak testing** | 24-hour stability runs | ❌ Missing | P2 | MEDIUM |
| **Upgrade testing** | Driver version upgrades | ❌ Missing | P2 | LOW |
| **Multi-node scenarios** | Volume migration, multi-attach | ⚠️ Manual | P1 | MEDIUM |

**Gap:** Testing maturity is below enterprise CSI driver standards. Manual testing has validated core functionality, but automated continuous testing is missing.

**Recommendation:**
- **v0.9.0:** Add CSI sanity tests + basic automated E2E suite
- **v1.0:** Add chaos testing (using Chaos Mesh or similar)
- **v1.1+:** Add performance benchmarking and soak testing

### Anti-Patterns (Testing Practices to Avoid)

| Anti-Pattern | Why Problematic | Better Approach |
|--------------|-----------------|-----------------|
| **Testing only happy paths** | Leaves error handling unvalidated | Test error scenarios explicitly (out of space, connection failures, invalid parameters) |
| **Manual-only E2E testing** | Doesn't catch regressions | Automate E2E tests in CI, run on every commit |
| **Testing in production** | Risky, slow feedback | Use ephemeral test clusters (kind, k3s, Talos) |
| **Skipping CSI sanity tests** | Violates spec, breaks kubelet compatibility | Run csi-sanity as part of CI gate |
| **Testing with fake storage** | Doesn't catch real-world issues | Test against real RDS hardware (or VM) |
| **Ignoring cleanup failures** | Causes orphaned volumes | Test failure scenarios explicitly, verify cleanup |

**Assessment:** Current testing approach correctly tests against real RDS hardware. Adding CSI sanity + automated E2E will address the gaps.

## CSI Specification Compliance

### Required Capabilities (✅ Implemented)

| Capability | Description | Evidence |
|------------|-------------|----------|
| **Identity Service** | GetPluginInfo, Probe, GetPluginCapabilities | pkg/driver/identity.go |
| **Controller Service** | CreateVolume, DeleteVolume, ValidateVolumeCapabilities | pkg/driver/controller.go |
| **Node Service** | NodeStageVolume, NodePublishVolume, NodeGetInfo | pkg/driver/node.go |
| **STAGE_UNSTAGE_VOLUME** | Two-phase mounting (device → staging → publish) | NodeStageVolume + NodeUnstageVolume |
| **PUBLISH_UNPUBLISH_VOLUME** | Attach/detach operations | NodePublishVolume + NodeUnpublishVolume |
| **CREATE_DELETE_VOLUME** | Dynamic provisioning | CreateVolume + DeleteVolume |
| **GET_CAPACITY** | Query available storage | GetCapacity (queries RDS free space) |
| **VOLUME_ACCESSIBILITY_CONSTRAINTS** | Topology-aware scheduling | ❌ Not implemented |

**Compliance Level:** **Core CSI spec compliant** for required features. Missing optional topology awareness.

### Optional Capabilities (Partially Implemented)

| Capability | Description | Status | Priority |
|------------|-------------|--------|----------|
| **EXPAND_VOLUME (Controller)** | Resize volume at storage backend | ✅ Implemented | N/A |
| **EXPAND_VOLUME (Node)** | Resize filesystem after backend resize | ✅ Implemented | N/A |
| **CREATE_DELETE_SNAPSHOT** | Volume snapshots | ❌ Missing | HIGH |
| **CLONE_VOLUME** | Volume cloning | ❌ Missing | MEDIUM |
| **VOLUME_ACCESSIBILITY_CONSTRAINTS** | Topology awareness | ❌ Missing | LOW |
| **GET_VOLUME_STATS** | Filesystem usage reporting | ✅ Implemented | N/A |
| **VOLUME_CONDITION** | Volume health status | ✅ Implemented | N/A |

**Snapshot Gap:** CREATE_DELETE_SNAPSHOT is the highest priority missing capability. Widely used for backup/restore workflows.

**Cloning Gap:** CLONE_VOLUME enables fast duplication (VM templates, database cloning). Often built on top of snapshots.

**Topology Gap:** VOLUME_ACCESSIBILITY_CONSTRAINTS enables multi-zone deployments. Lower priority for single-site homelab.

### Validation Gaps (❌ Not Validated)

| Requirement | Description | Gap | Remediation |
|-------------|-------------|-----|-------------|
| **CSI sanity tests pass** | Spec compliance validation | No automated run | Add `make sanity` target, run in CI |
| **Idempotency verified** | Repeated calls with same parameters return success | Manual testing only | CSI sanity validates this |
| **Error code compliance** | gRPC status codes match spec | Manual inspection | CSI sanity validates this |
| **Concurrent operations** | Multiple volume ops don't conflict | Limited testing | Add load testing |
| **Volume handle uniqueness** | Volume IDs are globally unique | UUID-based (compliant) | No gap |

**Confidence:** HIGH (implementation follows CSI patterns), but **lacks automated proof** via sanity tests.

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Test Cost | Priority | Target Milestone |
|---------|------------|---------------------|-----------|----------|------------------|
| CSI sanity tests | HIGH | LOW | LOW | **P0** | v0.9.0 |
| Automated E2E suite | HIGH | MEDIUM | MEDIUM | **P0** | v0.9.0 |
| Volume snapshots | HIGH | HIGH | MEDIUM | **P1** | v0.10.0 |
| Volume cloning | MEDIUM | HIGH | MEDIUM | **P2** | v0.11.0 |
| Topology awareness | LOW | MEDIUM | LOW | **P3** | v1.1+ |
| Chaos testing | MEDIUM | HIGH | HIGH | **P2** | v1.0 |
| Performance benchmarks | MEDIUM | MEDIUM | MEDIUM | **P2** | v1.0 |
| Multi-path NVMe/TCP | MEDIUM | MEDIUM | MEDIUM | **P2** | v1.1+ |
| Capacity tracking API | LOW | MEDIUM | LOW | **P3** | v1.2+ |

**Priority key:**
- **P0:** Must have for v1.0 release confidence (testing validation)
- **P1:** Should have for feature parity with peers (snapshots)
- **P2:** Nice to have for production maturity (cloning, chaos testing)
- **P3:** Future consideration (topology, advanced features)

## Recommendations for v0.9.0

### Must Implement (Blocking v1.0)

1. **CSI Sanity Tests Integration**
   - Effort: LOW (1-2 days)
   - Value: HIGH (spec compliance proof)
   - Action: Add `make sanity` target, integrate csi-test framework, run in CI

2. **Basic Automated E2E Test Suite**
   - Effort: MEDIUM (1 week)
   - Value: HIGH (regression prevention)
   - Action: Create test/e2e/ with basic workflows (create → mount → write → delete)

### Should Implement (High Value)

3. **Volume Snapshots Support**
   - Effort: HIGH (2-3 weeks)
   - Value: HIGH (backup/restore workflows)
   - Action: Implement CreateSnapshot/DeleteSnapshot, integrate external-snapshotter sidecar, use Btrfs snapshots
   - Dependencies: Requires VolumeSnapshot CRDs in cluster (external-snapshotter)

### Could Defer (Lower Priority)

4. **Volume Cloning Support**
   - Effort: HIGH (2 weeks)
   - Value: MEDIUM (fast VM template duplication)
   - Action: Implement CreateVolume with VolumeContentSource, use Btrfs reflinks or snapshot → clone pattern
   - Dependencies: Easier to implement after snapshots

5. **Topology Awareness**
   - Effort: MEDIUM (1 week)
   - Value: LOW (single-site homelab doesn't need multi-zone)
   - Action: Add VOLUME_ACCESSIBILITY_CONSTRAINTS capability, node topology labels, StorageClass topology keys

## Sources

### CSI Specification and Development
- [Container Storage Interface Specification](https://github.com/container-storage-interface/spec)
- [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/)
- [Developing a CSI Driver for Kubernetes](https://kubernetes-csi.github.io/docs/developing.html)
- [CSI Drivers List](https://kubernetes-csi.github.io/docs/drivers.html)

### Testing Frameworks
- [CSI Test Frameworks (csi-test)](https://github.com/kubernetes-csi/csi-test)
- [CSI Sanity Tests](https://github.com/kubernetes-csi/csi-test/blob/master/pkg/sanity/README.md)
- [Functional Testing Documentation](https://kubernetes-csi.github.io/docs/functional-testing.html)
- [Testing of CSI Drivers (Kubernetes Blog)](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/)

### Production CSI Drivers
- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver)
- [Longhorn CSI Driver](https://github.com/longhorn/longhorn)
- [democratic-csi (TrueNAS/ZFS)](https://github.com/democratic-csi/democratic-csi)
- [Ceph RBD CSI Driver](https://github.com/ceph/ceph-csi)

### CSI Features
- [Volume Snapshots](https://kubernetes-csi.github.io/docs/snapshot-restore-feature.html)
- [Volume Expansion](https://kubernetes-csi.github.io/docs/volume-expansion.html)
- [Topology Awareness](https://kubernetes-csi.github.io/docs/topology.html)
- [Raw Block Volumes](https://kubernetes-csi.github.io/docs/raw-block.html)

### Chaos Testing
- [Chaos Mesh Platform](https://chaos-mesh.org/)
- [AWS Fault Injection Simulator](https://aws.amazon.com/blogs/devops/chaos-engineering-on-amazon-eks-using-aws-fault-injection-simulator/)

---

*Feature research for: RDS CSI Driver v0.9.0 Gap Analysis*
*Researched: 2026-02-04*
*Confidence: HIGH (verified with official CSI documentation, production driver comparison, and existing codebase audit)*
