# High Availability Architecture for RDS CSI Driver

## Executive Summary

The current RDS CSI architecture has a **single point of failure** at the storage layer (one RDS server at 10.42.68.1). This document explores five viable high availability approaches for deployments with multiple RDS servers, analyzing trade-offs in complexity, performance, consistency guarantees, and operational overhead.

## Table of Contents

- [Current Architecture Limitations](#current-architecture-limitations)
- [Approach 1: NVMe/TCP Multipathing with ANA](#approach-1-nvmetcp-multipathing-with-ana-active-active)
- [Approach 2: Active-Passive Failover with Volume Migration](#approach-2-active-passive-failover-with-volume-migration)
- [Approach 3: Longhorn-Style Distributed Replication](#approach-3-longhorn-style-distributed-replication)
- [Approach 4: CSI Volume Snapshots for DR](#approach-4-csi-volume-snapshots-for-disaster-recovery)
- [Approach 5: Storage Class Topology with Zone-Aware Scheduling](#approach-5-storage-class-topology-with-zone-aware-scheduling)
- [Comparison Matrix](#comparison-matrix)
- [Recommended Implementation Path](#recommended-implementation-path)
- [Open Questions](#open-questions--considerations)

## Current Architecture Limitations

**Single RDS Server (10.42.68.1)**:
- ✅ Simple architecture, easy to reason about
- ✅ Low latency (direct NVMe/TCP connection)
- ❌ **Single point of failure** - RDS down = all PVs unavailable
- ❌ No redundancy for hardware failures
- ❌ Maintenance requires workload downtime
- ❌ Limited to single RDS capacity and bandwidth

---

## Approach 1: NVMe/TCP Multipathing with ANA (Active-Active)

### Overview

Use **native NVMe multipathing** with **Asymmetric Namespace Access (ANA)** protocol to connect each volume to **multiple RDS controllers simultaneously**. The Linux kernel's NVMe driver handles failover automatically with sub-second recovery times.

### Architecture Diagram

```
┌─────────────────────────────────────────────────┐
│ Worker Node                                     │
│  ┌──────────────────────────────────────────┐  │
│  │ NVMe Multipath Device (/dev/nvme0n1)     │  │
│  │                                           │  │
│  │  Path 1 (ANA Optimized)  ──────┐         │  │
│  │  Path 2 (ANA Non-optimized) ───┼────┐    │  │
│  └─────────────────────────────────┼────┼────┘  │
└────────────────────────────────────┼────┼────────┘
                                     │    │
                          NVMe/TCP   │    │  NVMe/TCP
                                     │    │
              ┌──────────────────────┘    └────────────────┐
              │                                             │
   ┌──────────▼──────────┐                    ┌────────────▼────────┐
   │ RDS-1 (Primary)     │                    │ RDS-2 (Replica)     │
   │ 10.42.68.1          │ ◄── Rsync sync ──► │ 10.42.68.2          │
   │ ANA State: Optimized│                    │ ANA State: Non-opt  │
   └─────────────────────┘                    └─────────────────────┘
```

### Implementation Details

**Volume Creation** (Controller):
1. Create volume on RDS-1 (primary) via SSH
2. Create volume on RDS-2 (replica) via SSH or rsync sync
3. Both RDS servers export the **same NQN** but different subsystem NSIDs
4. Configure RDS-1 as ANA "Optimized", RDS-2 as ANA "Non-Optimized"

**Node Stage** (Node Plugin):
```bash
# Connect to both targets
nvme connect -t tcp -a 10.42.68.1 -s 4420 -n nqn.2000-02.com.mikrotik:pvc-<uuid>
nvme connect -t tcp -a 10.42.68.2 -s 4420 -n nqn.2000-02.com.mikrotik:pvc-<uuid>

# Enable native NVMe multipath
echo 1 > /sys/module/nvme_core/parameters/multipath

# Kernel automatically creates /dev/nvme0n1 (multipath device)
# ANA state determines which path gets I/O
```

**Failover Behavior**:
- RDS-1 fails → Kernel detects path loss → I/O switches to RDS-2 within **<1 second**
- ANA transition triggers automatic path selection
- No volume remount required (transparent to application)

**Data Consistency**:
- Requires **synchronous replication** between RDS-1 and RDS-2 (rsync continuous sync or DRBD-like mechanism)
- OR accept eventual consistency (writes go to RDS-1, async replicate to RDS-2)
- **Split-brain risk** if replication breaks and both RDS servers diverge

### Pros

- ✅ **Sub-second failover** (0.5-1s typical)
- ✅ **Transparent to applications** (no volume remount)
- ✅ Active-Active for reads (can read from either RDS)
- ✅ Uses standard NVMe multipath (well-tested, kernel-native)
- ✅ No CSI driver changes for failover logic

### Cons

- ❌ **Requires synchronous replication** between RDS servers (complex)
- ❌ MikroTik RouterOS **does not natively support ANA** (would need custom NVMe target)
- ❌ Split-brain protection needed (fencing, quorum)
- ❌ Write performance penalty from replication overhead
- ❌ Complex to implement RDS-to-RDS replication (not a RouterOS feature)

### Feasibility

⚠️ **Medium** - Requires extending MikroTik RDS with ANA support or custom NVMe target. See [ana-protocol.md](ana-protocol.md) for detailed ANA implementation requirements.

---

## Approach 2: Active-Passive Failover with Volume Migration

### Overview

One RDS is **active** (serves volumes), others are **passive** (standby). On failure, the CSI driver **migrates volumes** to a standby RDS by detecting active RDS failure, promoting a passive RDS to active, and reconnecting pods to the new RDS.

### Architecture Diagram

```
┌──────────────────────────────────────────────────────────┐
│ CSI Controller (Deployment)                              │
│  - Health checks RDS-1 (active)                          │
│  - On failure: promote RDS-2, update volume metadata     │
└────────┬─────────────────────────────────┬───────────────┘
         │ SSH (control)                   │ SSH (control)
         │                                 │
         │ (write if active)               │ (write if passive)
         │                                 │
┌────────▼─────────────┐       ┌───────────▼──────────────┐
│ RDS-1 (Active)       │       │ RDS-2 (Passive)          │
│ 10.42.68.1           │ ─────►│ 10.42.68.2               │
│ Serves NVMe/TCP      │ rsync │ Replica only             │
└──────────────────────┘       └──────────────────────────┘
```

### Implementation Details

**Volume Metadata** (stored in PV annotations):
```yaml
csi.storage.k8s.io/pv/active-rds: "10.42.68.1"
csi.storage.k8s.io/pv/standby-rds: "10.42.68.2,10.42.68.3"
csi.storage.k8s.io/pv/replication-status: "synced"
```

**Replication Strategy**:
- Use MikroTik RouterOS **rsync** (already supported!) to replicate volumes
- Continuous sync from active RDS to passive RDS servers
- Configure via RouterOS CLI or automated sync jobs

**Failover Process**:
1. CSI Controller detects RDS-1 unreachable (SSH health check fails)
2. Promote RDS-2 to active:
   - Update PV annotations (`active-rds: 10.42.68.2`)
   - Stop rsync replication to RDS-2
   - Start rsync replication from RDS-2 to RDS-3 (if available)
3. Trigger pod restart to reconnect volumes:
   - Delete VolumeAttachment objects (kubelet retries mount)
   - Node plugin connects to new RDS-2 NVMe/TCP target

**Node Stage Changes**:
```go
// Read active RDS from PV annotation
activeRDS := pv.Annotations["csi.storage.k8s.io/pv/active-rds"]

// Connect to active RDS
cmd := exec.Command("nvme", "connect", "-t", "tcp", "-a", activeRDS,
                    "-s", "4420", "-n", nqn)
```

### RDS Rsync Configuration

MikroTik RouterOS supports rsync for file synchronization:

```bash
# On RDS-1 (active), configure rsync to RDS-2 (passive)
/system scheduler add \
  name=sync-volumes \
  interval=30s \
  on-event="/tool rsync \
    src=/storage-pool/kubernetes-volumes/ \
    dst=admin@10.42.68.2:/storage-pool/kubernetes-volumes/ \
    key=/rsync-key"
```

### Pros

- ✅ **Leverages existing RDS rsync** (no custom replication code)
- ✅ Relatively simple CSI controller logic
- ✅ Works with current MikroTik RDS hardware/software
- ✅ Clear active/passive roles (no split-brain)
- ✅ Can have multiple standby RDS servers

### Cons

- ❌ **Minutes of downtime** during failover (pod restart required)
- ❌ Manual intervention may be needed (promoting passive RDS)
- ❌ **Data loss window** if rsync not fully synced (async replication)
- ❌ Rsync overhead on active RDS (CPU, network bandwidth)
- ❌ Requires pod rescheduling (disruptive to workloads)

### Feasibility

✅ **High** - Can implement with existing RDS features (rsync is built-in to RouterOS)

---

## Approach 3: Longhorn-Style Distributed Replication

### Overview

The **CSI driver manages replication** at the block level, similar to Longhorn. Each volume has **N replicas** distributed across multiple RDS servers. The CSI node plugin acts as a **RAID-1 controller**, writing to all replicas and reading from the optimal replica.

### Architecture Diagram

```
┌─────────────────────────────────────────────────┐
│ Worker Node                                     │
│  ┌──────────────────────────────────────────┐  │
│  │ RDS CSI Replication Engine (new)        │  │
│  │  - Intercepts I/O                        │  │
│  │  - Writes to all replicas (RAID-1)       │  │
│  │  - Reads from optimal replica            │  │
│  │  - Handles replica failure               │  │
│  └────┬────────┬────────┬──────────────────┘  │
│       │        │        │                      │
└───────┼────────┼────────┼──────────────────────┘
        │        │        │
        │ NVMe   │ NVMe   │ NVMe
        │        │        │
   ┌────▼────┐ ┌▼────────▼┐ ┌────────▼───┐
   │ RDS-1   │ │ RDS-2    │ │ RDS-3      │
   │ Replica │ │ Replica  │ │ Replica    │
   └─────────┘ └──────────┘ └────────────┘
```

### Implementation Details

**Volume Creation**:
```bash
# Controller creates volume on 3 RDS servers
ssh rds-1 "/disk add slot=pvc-<uuid>-replica-1 ..."
ssh rds-2 "/disk add slot=pvc-<uuid>-replica-2 ..."
ssh rds-3 "/disk add slot=pvc-<uuid>-replica-3 ..."
```

**Node Stage** (new replication engine):
```go
// Connect to all replicas
replicas := []Replica{
    {Host: "10.42.68.1", NQN: "nqn.2000-02.com.mikrotik:pvc-<uuid>-r1"},
    {Host: "10.42.68.2", NQN: "nqn.2000-02.com.mikrotik:pvc-<uuid>-r2"},
    {Host: "10.42.68.3", NQN: "nqn.2000-02.com.mikrotik:pvc-<uuid>-r3"},
}

// Create virtual block device (loopback + dm-linear)
engine := NewReplicationEngine(replicas)
virtualDev := engine.CreateVirtualBlockDevice() // /dev/rds-csi/<volume-id>

// Write I/O: replicate to all replicas
func (e *ReplicationEngine) Write(offset, data) {
    var wg sync.WaitGroup
    for _, replica := range e.replicas {
        wg.Add(1)
        go func(r Replica) {
            r.Write(offset, data) // parallel writes
            wg.Done()
        }(replica)
    }
    wg.Wait() // wait for quorum (2 out of 3)
}

// Read I/O: read from optimal replica (lowest latency)
func (e *ReplicationEngine) Read(offset, size) {
    return e.optimalReplica.Read(offset, size)
}
```

**Replica Failure Handling**:
- Detect replica failure (NVMe disconnect event)
- Mark replica as degraded
- Continue I/O with remaining replicas (2 out of 3)
- CSI controller triggers rebuild: create new replica on healthy RDS, resync data

### Pros

- ✅ **True active-active HA** (any replica can fail)
- ✅ **Transparent failover** (no pod restart)
- ✅ **Synchronous replication** (no data loss)
- ✅ Can tolerate N-1 RDS failures (for N replicas)
- ✅ Similar to proven Longhorn/Ceph architecture

### Cons

- ❌ **Significant development effort** (custom replication engine)
- ❌ **Performance overhead** (3x write amplification for 3 replicas)
- ❌ **Network bandwidth** (3x for writes)
- ❌ Requires userspace I/O interception (FUSE or LD_PRELOAD) or kernel module
- ❌ Complex failure scenarios (split-brain, quorum loss)
- ❌ Rebuild process is complex (background sync while serving I/O)

### Feasibility

⚠️ **Low** - Requires building a distributed storage system (6-12 months development effort)

---

## Approach 4: CSI Volume Snapshots for Disaster Recovery

### Overview

Use **CSI snapshot capabilities** to create point-in-time snapshots on secondary RDS servers. On primary RDS failure, restore from snapshot.

### Architecture Diagram

```
┌──────────────────────────────────────────────────────────┐
│ CSI Controller                                           │
│  - Periodic snapshots (every 15 min) via CSI API        │
│  - Store snapshots on RDS-2, RDS-3                      │
└────────┬─────────────────────────────────┬───────────────┘
         │ SSH                             │ SSH
         │                                 │
┌────────▼─────────────┐       ┌───────────▼──────────────┐
│ RDS-1 (Primary)      │       │ RDS-2 (DR Site)          │
│ - Live volumes       │       │ - Snapshots              │
│                      │       │ - Can restore to volume  │
└──────────────────────┘       └──────────────────────────┘
```

### Implementation Details

**Snapshot Creation**:
```bash
# CSI controller creates snapshot (e.g., every 15 minutes via CronJob)
# Using Btrfs snapshot on RDS
ssh rds-1 "btrfs subvolume snapshot \
  /storage-pool/kubernetes-volumes/pvc-<uuid>.img \
  /storage-pool/snapshots/pvc-<uuid>-$(date +%s)"

# Transfer snapshot to RDS-2 via rsync
ssh rds-1 "rsync -a /storage-pool/snapshots/pvc-<uuid>-* \
           admin@10.42.68.2:/storage-pool/snapshots/"
```

**CSI Snapshot RPCs**:
- Implement `CreateSnapshot()` - create Btrfs snapshot on RDS
- Implement `DeleteSnapshot()` - clean up old snapshots
- Implement `CreateVolumeFromSnapshot()` - restore snapshot to new volume

**Disaster Recovery Process**:
1. Detect RDS-1 failure
2. Admin triggers restore: `kubectl annotate pvc <name> restore-from-snapshot=<snap-id>`
3. CSI controller restores snapshot on RDS-2:
   ```bash
   ssh rds-2 "/disk add type=file \
              file-path=/storage-pool/snapshots/pvc-<uuid>-<timestamp>.img \
              slot=pvc-<uuid> nvme-tcp-export=yes ..."
   ```
4. Update PV to point to RDS-2
5. Restart pods

### Pros

- ✅ **Simple implementation** (uses CSI snapshot spec)
- ✅ **Point-in-time recovery** (can restore to any snapshot)
- ✅ Offsite DR capability (snapshots on different RDS)
- ✅ No performance impact on live volumes
- ✅ Useful for non-HA scenarios (user error, corruption)

### Cons

- ❌ **Not true HA** (RPO = snapshot interval, e.g., 15 min data loss)
- ❌ **Manual intervention** required for failover
- ❌ **Long RTO** (minutes to restore snapshot and restart pods)
- ❌ Snapshots consume storage (need retention policy)

### Feasibility

✅ **High** - Straightforward CSI snapshot implementation, Btrfs snapshots are efficient

---

## Approach 5: Storage Class Topology with Zone-Aware Scheduling

### Overview

Treat each RDS as a **separate availability zone**. Use Kubernetes **topology-aware scheduling** to ensure pods and their volumes are co-located. On RDS failure, reschedule pods to a different zone with volumes on a different RDS.

### Architecture Diagram

```
┌──────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                       │
│                                                          │
│  Zone: rds-1         Zone: rds-2         Zone: rds-3    │
│  ┌────────────┐      ┌────────────┐      ┌────────────┐ │
│  │ Node-1     │      │ Node-2     │      │ Node-3     │ │
│  │ Pod-A      │      │ Pod-B      │      │ Pod-C      │ │
│  │  └─ PVC-A  │      │  └─ PVC-B  │      │  └─ PVC-C  │ │
│  └─────┬──────┘      └──────┬─────┘      └──────┬─────┘ │
│        │ NVMe               │ NVMe              │ NVMe   │
└────────┼────────────────────┼───────────────────┼────────┘
         │                    │                   │
   ┌─────▼──────┐      ┌──────▼───────┐   ┌──────▼───────┐
   │ RDS-1      │      │ RDS-2        │   │ RDS-3        │
   │ PVC-A      │      │ PVC-B        │   │ PVC-C        │
   └────────────┘      └──────────────┘   └──────────────┘
```

### Implementation Details

**StorageClass with Topology**:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rds-csi-topology
provisioner: rds.csi.mikrotik.com
volumeBindingMode: WaitForFirstConsumer
allowedTopologies:
- matchLabelExpressions:
  - key: topology.kubernetes.io/zone
    values:
    - rds-zone-1
    - rds-zone-2
    - rds-zone-3
parameters:
  rds-zone-1: "10.42.68.1"
  rds-zone-2: "10.42.68.2"
  rds-zone-3: "10.42.68.3"
```

**Node Labels**:
```bash
kubectl label node worker-1 topology.kubernetes.io/zone=rds-zone-1
kubectl label node worker-2 topology.kubernetes.io/zone=rds-zone-2
kubectl label node worker-3 topology.kubernetes.io/zone=rds-zone-3
```

**CSI Controller Logic**:
- When creating volume, read pod's scheduled node from `CreateVolumeRequest.AccessibilityRequirements`
- Determine which zone (RDS) to use based on node topology
- Create volume on the corresponding RDS

**Failover**:
- Pod on Node-1 with volume on RDS-1
- RDS-1 fails → Pod goes to CrashLoopBackOff (can't mount volume)
- Admin or automation:
  1. Snapshot volume on RDS-1 (if possible) or accept data loss
  2. Delete PVC (deletes volume on RDS-1)
  3. Reschedule pod to Node-2 (zone rds-2)
  4. New PVC provisioned on RDS-2

### Pros

- ✅ **Load distribution** across multiple RDS servers
- ✅ Simple to implement (standard CSI topology support)
- ✅ **No replication overhead** (each volume on one RDS)
- ✅ Scales horizontally (add more RDS servers = more zones)
- ✅ Maximizes capacity utilization

### Cons

- ❌ **No true HA** (volume tied to one RDS)
- ❌ **Data loss** on RDS failure (unless replicated separately)
- ❌ Requires **pod rescheduling** and **PVC recreation** on failure
- ❌ Complex automation needed for failover

### Feasibility

✅ **High** - Standard CSI feature, minimal code changes

---

## Comparison Matrix

| Approach | Failover Time | Data Loss | Dev Effort | RDS Feature Req | Perf Impact | Split-Brain Risk |
|----------|---------------|-----------|------------|-----------------|-------------|------------------|
| **1. NVMe Multipath + ANA** | <1s | None (sync) | High | ANA support | High (repl) | Yes |
| **2. Active-Passive + Rsync** | 2-5 min | RPO=sync lag | Medium | rsync (✅) | Medium | Low |
| **3. Longhorn-Style Repl** | <1s | None (sync) | Very High | None | Very High | Yes |
| **4. CSI Snapshots** | 5-15 min | RPO=snap interval | Low | Btrfs snaps | Low | No |
| **5. Topology Zones** | N/A (resched) | Full volume | Low | None | None | No |

### Legend

- **Failover Time**: How long until workload is operational again
- **Data Loss**: Potential data loss (RPO - Recovery Point Objective)
- **Dev Effort**: Engineering time to implement
- **RDS Feature Req**: What MikroTik RDS features are required
- **Perf Impact**: Performance overhead on normal operations
- **Split-Brain Risk**: Risk of data divergence between replicas

---

## Recommended Implementation Path

### Phase 1: Active-Passive with Rsync (Milestone 5)
**Timeline**: 3-4 weeks
**Why**: Balances HA benefits with implementation complexity. Leverages existing RDS rsync feature.

**Implementation Tasks**:
1. Add multi-RDS configuration to CSI driver (RDS pool with active/passive roles)
2. Controller creates volumes on active RDS, triggers rsync to passive RDS(es)
3. Add health checking to controller (SSH liveness probes every 10s)
4. Implement failover logic:
   - Detect active RDS failure
   - Promote passive → active
   - Update PV annotations with new active RDS
5. Update node plugin to read active RDS from PV metadata
6. Add VolumeAttachment cleanup on failover (trigger pod remount)

**Expected Metrics**:
- **RTO (Recovery Time Objective)**: 2-5 minutes
- **RPO (Recovery Point Objective)**: <30 seconds (rsync interval)

### Phase 2: CSI Snapshots for DR (Milestone 6)
**Timeline**: 2-3 weeks
**Why**: Complements active-passive with point-in-time recovery and offsite backup.

**Implementation Tasks**:
1. Implement CSI `CreateSnapshot` RPC (Btrfs snapshot on RDS)
2. Implement CSI `DeleteSnapshot` RPC (cleanup old snapshots)
3. Implement CSI `CreateVolumeFromSnapshot` RPC
4. Deploy external-snapshotter sidecar
5. Create VolumeSnapshotClass
6. Schedule periodic snapshots via CronJob or Velero

**Expected Metrics**:
- **RPO**: Configurable (snapshot interval: 15min, 1hr, 6hr)
- **Retention**: Configurable (keep last N snapshots)

### Phase 3 (Optional): NVMe Multipathing (Milestone 7+)
**Timeline**: 8-12 weeks
**Why**: If MikroTik adds ANA support to RDS or you implement custom NVMe target, this provides sub-second failover.

**Requirements**:
- MikroTik RDS firmware update with ANA support, OR
- Custom NVMe/TCP target on Linux VM fronting RDS storage (using SPDK), OR
- Contribution to SPDK to support multi-target namespace sharing

**See**: [ana-protocol.md](ana-protocol.md) for detailed ANA implementation guide

---

## Open Questions & Considerations

### 1. MikroTik RDS Replication Capabilities
**Question**: Does RouterOS support real-time block-level replication, or only rsync (file-level)?
**Impact**: Block-level replication would enable lower RPO (<1s) for active-passive.
**Current Knowledge**: RDS supports rsync (file-level), RAID (local redundancy), but no documented block-level replication.

### 2. Network Topology
**Question**: Are RDS servers on the same L2 network or separate subnets?
**Impact**: Affects multipathing feasibility (needs direct NVMe/TCP connectivity from workers to all RDS).
**Recommendation**: Deploy all RDS on same L2 segment for maximum flexibility.

### 3. Consistency Requirements
**Question**: What RPO/RTO targets are acceptable for your workloads (KubeVirt VMs)?
**Impact**:
- **Mission-critical VMs** (RPO=0, RTO<1s) → Need synchronous replication (Longhorn-style or NVMe multipath)
- **Standard workloads** (RPO=30s, RTO=2-5min) → Active-passive with rsync is sufficient
- **Dev/test** (RPO=15min, RTO=10min) → CSI snapshots are sufficient

### 4. Budget for Development
**Question**: How much engineering time is available?
**Impact**:
- **4 weeks** → Approach 2 (Active-passive) + Approach 4 (Snapshots)
- **12 weeks** → Add Approach 1 (NVMe multipath) if RDS supports ANA
- **24 weeks** → Approach 3 (Full Longhorn-style distributed storage)

### 5. Split-Brain Protection
**Question**: For active-active approaches, how to prevent split-brain (both RDS think they're primary)?
**Options**:
- **Witness/Quorum**: Third node (lightweight VM) votes on which RDS is primary
- **STONITH** (Shoot The Other Node In The Head): Fencing mechanism to power off failed RDS
- **Application-level**: Use Kubernetes lease objects for leader election

---

## Conclusion

For a **homelab/SMB production environment** with MikroTik RDS:

**Short-term (next 6 weeks)**:
- Implement **Approach 2 (Active-Passive)** + **Approach 4 (Snapshots)**
- Achieves production-grade HA with reasonable RTO/RPO
- Uses existing RDS features (rsync, Btrfs snapshots)
- Proven pattern (similar to traditional storage array failover)

**Long-term (6+ months)**:
- Evaluate **Approach 1 (NVMe Multipath)** if MikroTik adds ANA support to RDS
- Or consider implementing SPDK-based NVMe/TCP gateway that fronts multiple RDS servers

**Avoid**:
- Approach 3 (Longhorn-style) unless building a commercial product - the engineering effort (6-12 months) is substantial for a homelab driver

---

## See Also

- [ANA Protocol Deep Dive](ana-protocol.md) - Detailed explanation of NVMe Asymmetric Namespace Access
- [Architecture](architecture.md) - Current single-RDS architecture
- [RDS Commands](rds-commands.md) - RouterOS CLI reference
