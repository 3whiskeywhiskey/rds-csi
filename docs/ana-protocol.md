# NVMe ANA (Asymmetric Namespace Access) Protocol - Deep Dive

## Table of Contents

- [Overview](#overview)
- [What Problem Does ANA Solve?](#what-problem-does-ana-solve)
- [How ANA Works](#how-ana-works)
- [ANA States](#ana-states)
- [Architecture](#architecture)
- [Technical Implementation](#technical-implementation)
- [ANA vs ALUA (SCSI)](#ana-vs-alua-scsi)
- [Multipath Path Selection](#multipath-path-selection)
- [Failover Behavior](#failover-behavior)
- [Active-Active vs Active-Passive](#active-active-vs-active-passive)
- [Linux Implementation](#linux-implementation)
- [Why MikroTik RDS Doesn't Support ANA](#why-mikrotik-rds-doesnt-support-ana)
- [How to Get ANA for RDS CSI Driver](#how-to-get-ana-for-rds-csi-driver)

---

## Overview

**Asymmetric Namespace Access (ANA)** is a standard defined in the NVMe specification (ratified March 2018) that enables **multipathing** and **intelligent failover** for NVMe namespaces accessed through multiple controllers.

**Simple Analogy**: Imagine a library book (namespace) that can be accessed from multiple service desks (controllers). ANA is the system that tells you which desk currently has the fastest access to that book, and automatically redirects you if your desk closes.

**Key Capabilities**:
- Multiple paths to the same NVMe namespace
- Host knows which path is "optimal" (lowest latency)
- Automatic failover on path/controller failure
- Sub-second recovery times (<1s typical)

---

## What Problem Does ANA Solve?

### The Problem

In enterprise storage, you need:
- **Redundancy**: Multiple paths to the same data to survive controller/network failures
- **Performance**: Route I/O through the optimal path (lowest latency)
- **Failover**: Automatic recovery when a path fails

### Before ANA

- **SCSI world**: Had ALUA (Asymmetric Logical Unit Access) for this purpose
- **Early NVMe**: No standardized way to communicate path state between storage and host
- **Result**: Hosts didn't know which controller path was "best" for a given namespace

### After ANA (NVMe 1.3+)

- Storage controllers report ANA state for each namespace
- Hosts read ANA state and route I/O accordingly
- State changes trigger automatic path selection
- Standardized across all NVMe implementations

---

## How ANA Works

### Basic Concept

Each NVMe controller reports one of **four possible states** for each namespace it can access. The host uses these states to decide which controller to send I/O through.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│ NVMe Host (Worker Node)                                 │
│                                                         │
│  ┌──────────────────────────────────────────────────┐  │
│  │ NVMe Multipath Driver                            │  │
│  │  - Reads ANA state from each controller          │  │
│  │  - Routes I/O based on ANA state                 │  │
│  │  - Handles ANA state changes                     │  │
│  └───┬────────────────────────────┬─────────────────┘  │
│      │                            │                     │
│      │ Path 1                     │ Path 2              │
│      │ (optimal)                  │ (non-optimal)       │
└──────┼────────────────────────────┼─────────────────────┘
       │                            │
       │ NVMe/TCP                   │ NVMe/TCP
       │                            │
┌──────▼─────────────┐      ┌───────▼──────────────┐
│ Controller A       │      │ Controller B         │
│ NVMe Subsystem     │◄────►│ NVMe Subsystem       │
│                    │ sync │                      │
│ Namespace 1        │      │ Namespace 1          │
│ ANA State:         │      │ ANA State:           │
│   OPTIMIZED        │      │   NON_OPTIMIZED      │
└────────────────────┘      └──────────────────────┘
         │                           │
         │    Shared Storage         │
         └──────────┬────────────────┘
                    │
            ┌───────▼────────┐
            │ Physical Disk  │
            │ /dev/nvme0n1   │
            └────────────────┘
```

---

## ANA States

Each controller reports one of **four ANA states** for each namespace:

### State Definitions

| ANA State | Meaning | I/O Behavior | Use Case |
|-----------|---------|--------------|----------|
| **Optimized** | Best path - controller has direct/fast access to namespace | ✅ Send all I/O here (preferred) | Primary controller with local access |
| **Non-Optimized** | Accessible but slower (e.g., remote controller, cross-node access) | ⚠️ Use only if optimized path unavailable | Secondary controller via interconnect |
| **Inaccessible** | Temporarily unavailable (failover in progress, maintenance) | ❌ Don't send I/O (queue requests) | During failover transition |
| **Change** | State is transitioning (e.g., optimized → non-optimized) | ⏸️ Queue I/O until state stabilizes | Intermediate state during updates |

### State Transitions

```
Normal Operation:
  Controller A: Optimized  ──┐
  Controller B: Non-Optimized │ Host sends all I/O to Controller A
                              │
Controller A Fails:
  Controller A: (disconnected)
  Controller B: Non-Optimized ──> Change ──> Optimized
                                   │
                                   └── Queue I/O during transition

Controller A Returns:
  Controller A: Inaccessible ──> Change ──> Optimized
  Controller B: Optimized ────> Non-Optimized
```

### ANA Groups

- Namespaces are organized into **ANA groups** (identified by `ANAGRPID`)
- All namespaces in a group share the same ANA state on a given controller
- Reduces overhead (set state for group instead of each namespace individually)

**Example**:
```
ANA Group 1 (ANAGRPID=1):
  - Namespaces: 1, 2, 3
  - State on Controller A: Optimized
  - State on Controller B: Non-Optimized

ANA Group 2 (ANAGRPID=2):
  - Namespaces: 4, 5, 6
  - State on Controller A: Non-Optimized
  - State on Controller B: Optimized
```

This allows **load balancing** across controllers (Group 1 → Controller A, Group 2 → Controller B).

---

## ANA Transition Time (ANATT)

**ANATT** = Maximum time (in seconds) for a state change to complete.

- Reported by the NVMe subsystem in the `Identify Controller` response
- Host queues I/O during transitions up to ANATT timeout
- If ANATT expires, host may retry on alternate path or return error

**Example**: If ANATT=10s, when state goes to "Inaccessible", it should become accessible again within 10 seconds.

---

## Technical Implementation

### How Hosts Discover ANA State

#### 1. Initial Discovery

Host sends `Identify Controller` admin command:

```bash
nvme id-ctrl /dev/nvme0
```

**Response includes**:
```
anacap    : 0x7      # ANA capabilities (which states are supported)
                     # 0x7 = supports Optimized, Non-Optimized, Inaccessible
anatt     : 10       # ANA transition time (10 seconds)
anagrpmax : 256      # Max number of ANA groups
nanagrpid : 32       # Number of ANA group IDs currently in use
```

#### 2. Query ANA State

Host sends `Get Log Page - ANA` command:

```bash
nvme ana-log /dev/nvme0
```

**Response contains**:
```
Asymmetric Namespace Access Log
Number of ANA Group Descriptors: 2

Group ID (ANAGRPID): 1
  ANA State: Optimized
  Number of NSIDs: 3
  NSIDs: 1, 2, 3

Group ID (ANAGRPID): 2
  ANA State: Non-Optimized
  Number of NSIDs: 3
  NSIDs: 4, 5, 6
```

#### 3. Monitor Changes

Host can monitor ANA state changes via:

**a) Polling**: Periodically query the ANA log page (e.g., every 5 seconds)

**b) Asynchronous Events**: Subscribe to ANA Change Notices
```bash
# Controller sends Asynchronous Event when ANA state changes
# Host receives event and queries ANA log page for new state
```

### ANA Capability Bitmask

The `anacap` field is a bitmask:

```
Bit 0: Supports Optimized state
Bit 1: Supports Non-Optimized state
Bit 2: Supports Inaccessible state
Bit 3: Supports Persistent Loss state
Bit 4: Supports Change state
Bit 6: Supports ANA Group ID doesn't change while namespace attached
Bit 7: Supports non-zero ANA Group ID
```

**Example**: `anacap = 0x7` (binary: 0000 0111)
- Bit 0 set: Supports Optimized ✅
- Bit 1 set: Supports Non-Optimized ✅
- Bit 2 set: Supports Inaccessible ✅

---

## ANA vs ALUA (SCSI)

ANA is the NVMe equivalent of SCSI's ALUA:

| Feature | SCSI ALUA | NVMe ANA |
|---------|-----------|----------|
| **Purpose** | Multipathing for SCSI LUNs | Multipathing for NVMe namespaces |
| **States** | Active/Optimized, Active/Non-Optimized, Standby, Unavailable | Optimized, Non-Optimized, Inaccessible, Change |
| **Protocol** | SCSI commands (MODE SELECT, etc.) | NVMe Admin commands (Identify, Get Log Page) |
| **Granularity** | Per-LUN | Per-namespace or per-ANA group |
| **Performance** | Higher command overhead | Lower latency (simpler commands) |
| **Standardized** | SCSI-3 (T10 standard) | NVMe 1.3+ (NVM Express standard) |

**Both serve the same purpose**: Tell the host which path is best for accessing storage.

---

## Multipath Path Selection

The Linux NVMe driver uses ANA state for intelligent I/O routing:

### Path Selection Algorithm (Simplified)

```c
// Simplified Linux kernel logic
int nvme_path_select(struct nvme_ns *ns, struct nvme_ctrl *ctrl) {
    // Priority 1: Use optimized paths
    if (ctrl->ana_state[ns->ana_grpid] == NVME_ANA_OPTIMIZED)
        return USE_THIS_PATH;

    // Priority 2: Use non-optimized if no optimized available
    if (ctrl->ana_state[ns->ana_grpid] == NVME_ANA_NON_OPTIMIZED)
        return USE_IF_NO_OPTIMIZED;

    // Priority 3: Queue I/O if inaccessible/changing
    if (ctrl->ana_state[ns->ana_grpid] == NVME_ANA_INACCESSIBLE ||
        ctrl->ana_state[ns->ana_grpid] == NVME_ANA_CHANGE) {
        queue_io_until_accessible();
        return WAIT;
    }
}
```

### Path Selection Policies

Linux NVMe multipath supports two policies:

#### 1. **Numa (Default)**: Prioritize local paths
```
- Prefer optimized paths on local NUMA node
- Fall back to non-optimized paths if needed
- Best for multi-controller systems with NUMA topology
```

#### 2. **Round-Robin**: Load balance across optimized paths
```
- Distribute I/O evenly across all optimized paths
- Best for active-active configurations
- Maximizes bandwidth utilization
```

**Configuration**:
```bash
# Set multipath policy
echo "round-robin" > /sys/class/nvme-subsystem/nvme-subsys0/iopolicy
```

---

## Failover Behavior

### Scenario: Controller A (Optimized) Fails

```
Time  Event                          ANA State         I/O Behavior
──────────────────────────────────────────────────────────────────────
t0    Normal operation               A: Optimized      All I/O → A
                                     B: Non-Optimized

t1    Controller A fails             A: (dead)         TCP timeout (~5s)
      (NVMe/TCP connection lost)     B: Non-Optimized  I/O errors start

t2    Host detects failure           A: (disconnected) Mark path dead
      (nvme-tcp keepalive timeout)   B: Non-Optimized  Failover to B

t3    Controller B detects failure   A: (dead)         B promotes to
      Promotes itself to optimized   B: Change         Optimized

t4    Host queries ANA log           A: (dead)         All I/O → B
      Sees B is now Optimized        B: Optimized      Normal operation

Total Failover Time: ~1-5 seconds (depends on keepalive settings)
```

### Failover Times

| Transport | Detection Time | Total Failover |
|-----------|----------------|----------------|
| **NVMe/TCP** | 5-10s (TCP keepalive) | ~5-10s |
| **NVMe/TCP (tuned)** | 1-2s (custom keepalive) | ~1-2s |
| **NVMe/RDMA** | <1s (RDMA timeout) | <1s |
| **NVMe/FC** | <1s (FC link loss) | <1s |

**Tuning NVMe/TCP for faster failover**:
```bash
# Set shorter keepalive interval (default: 5s)
nvme connect -t tcp -a 10.42.68.1 -n <nqn> --keep-alive-tmo=2

# Set ctrl_loss_tmo (time to wait before giving up)
echo 5 > /sys/class/nvme-fabrics/ctl/nvme0/ctrl_loss_tmo
```

---

## Active-Active vs Active-Passive

### Active-Active Configuration

**Both controllers report Optimized**:
```
Controller A: Namespace 1 = Optimized
Controller B: Namespace 1 = Optimized
```

**Characteristics**:
- ✅ Both controllers can serve I/O simultaneously
- ✅ Load balancing across controllers (round-robin)
- ✅ Maximum throughput (aggregate bandwidth)
- ❌ Requires shared storage backend with **cache coherency**
- ❌ Complex to implement (need synchronized write caches)

**Use Case**: Dual-controller enterprise arrays (NetApp, Pure Storage, Dell EMC)

### Active-Passive Configuration

**Primary Optimized, Secondary Non-Optimized**:
```
Controller A: Namespace 1 = Optimized
Controller B: Namespace 1 = Non-Optimized (or Inaccessible)
```

**Characteristics**:
- ✅ Primary controller serves all I/O
- ✅ Secondary only used during failover
- ✅ Simpler to implement (no cache coherency needed)
- ❌ No load balancing (waste of secondary controller)
- ❌ Failover latency (wait for state transition)

**Use Case**: Most common for simpler HA setups

---

## Linux Implementation

### Enable Native NVMe Multipath

```bash
# Check if multipath is enabled
cat /sys/module/nvme_core/parameters/multipath
# Output: Y (enabled) or N (disabled)

# Enable multipath (persistent across reboots)
echo "options nvme_core multipath=Y" > /etc/modprobe.d/nvme.conf

# OR enable at runtime (temporary)
echo 1 > /sys/module/nvme_core/parameters/multipath
```

### Connect to Multiple Controllers

```bash
# Connect to Controller A
nvme connect -t tcp -a 10.42.68.1 -s 4420 \
  -n nqn.2000-02.com.mikrotik:volume-1

# Connect to Controller B (same NQN!)
nvme connect -t tcp -a 10.42.68.2 -s 4420 \
  -n nqn.2000-02.com.mikrotik:volume-1

# Kernel creates multipath device automatically
# /dev/nvme0n1 and /dev/nvme1n1 → /dev/nvme0n1 (multipath)
```

### View Multipath Status

```bash
nvme list-subsys
```

**Output**:
```
nvme-subsys0 - NQN=nqn.2000-02.com.mikrotik:volume-1
\
 +- nvme0 tcp traddr=10.42.68.1 trsvcid=4420 live optimized
 +- nvme1 tcp traddr=10.42.68.2 trsvcid=4420 live non-optimized
```

### Check ANA State

```bash
nvme ana-log /dev/nvme0
```

**Output**:
```
Asymmetric Namespace Access Log for NVME device: nvme0 namespace-id:1
Number of Asymmetric Namespace Access Group Descriptors: 1

ANA Group ID: 1
  Number of NSID Values: 1
  NSID: 1
  ANA State: Optimized

ANA Group ID: 1 (from nvme1)
  Number of NSID Values: 1
  NSID: 1
  ANA State: Non-Optimized
```

### Device Mapper vs Native NVMe Multipath

Linux offers two multipath solutions:

| Feature | Device Mapper (dm-multipath) | Native NVMe Multipath |
|---------|------------------------------|------------------------|
| **Layer** | Block layer (generic) | NVMe driver (native) |
| **ANA Support** | No (manual configuration) | Yes (automatic) |
| **Failover Time** | 5-10s | <1s |
| **Performance** | Extra layer overhead | Direct NVMe I/O path |
| **Use Case** | Legacy/mixed storage | NVMe-only (recommended) |

**Recommendation**: Use **native NVMe multipath** for ANA support and best performance.

---

## Why MikroTik RDS Doesn't Support ANA

Based on research, MikroTik RDS likely doesn't implement ANA because:

### 1. Single Controller Architecture

- RDS appears to be a **single-node storage appliance** (one RouterOS instance per RDS unit)
- ANA requires **multiple controllers** accessing the **same namespace**
- Each RDS unit is independent (no inter-RDS coordination)

### 2. NVMe Target Implementation

ANA support requires the NVMe target to:
- Report ANA capability in `Identify Controller` (set `anacap` field)
- Maintain ANA state for each namespace
- Support `Get Log Page - ANA` command
- Send ANA Change Notices on failover

**RouterOS NVMe/TCP target**:
- Based on Linux kernel `nvmet` or custom implementation
- May not implement ANA extensions (NVMe 1.3+ feature)
- Focus on basic NVMe/TCP functionality

### 3. Target Market

- **RDS use case**: SMB/homelab, single-unit deployments
- **Enterprise arrays**: Dual-controller HA (Pure, NetApp, Dell) implement ANA
- **MikroTik focus**: Simplicity and cost, not enterprise HA features

### 4. Shared Storage Requirement

ANA assumes **shared backend storage** between controllers:
```
Controller A ──┐
               ├──> Shared SAS/NVMe Pool
Controller B ──┘
```

RDS units have **independent storage** (each has its own NVMe drives), not shared.

---

## How to Get ANA for RDS CSI Driver

### Option 1: Wait for MikroTik to Add ANA

**Steps**:
1. File feature request with MikroTik support
2. Explain use case (Kubernetes HA storage)
3. Request ANA support in RouterOS NVMe/TCP target

**Likelihood**: Low in short term (not their target market)

### Option 2: Build Custom NVMe/TCP Gateway

Create a Linux-based NVMe/TCP target using **SPDK** or **kernel nvmet** that:
- Exports the same namespace from multiple gateway VMs
- Each gateway connects to a different RDS backend
- Gateways coordinate ANA state (active-passive or active-active)

#### Architecture

```
┌──────────────────────────────────────────────────┐
│ Worker Node                                      │
│  nvme connect → Gateway-1 (Optimized)            │
│  nvme connect → Gateway-2 (Non-Optimized)        │
└───────┬──────────────────────┬───────────────────┘
        │ NVMe/TCP             │ NVMe/TCP
┌───────▼─────────┐    ┌───────▼─────────┐
│ Gateway VM-1    │    │ Gateway VM-2    │
│ (SPDK/nvmet)    │◄──►│ (SPDK/nvmet)    │
│ ANA: Optimized  │ HA │ ANA: Non-Opt    │
└───────┬─────────┘    └───────┬─────────┘
        │ NVMe/TCP             │ NVMe/TCP
        │                      │
    ┌───▼──────┐          ┌────▼─────┐
    │ RDS-1    │          │ RDS-2    │
    │ Primary  │◄────────►│ Replica  │
    └──────────┘  rsync   └──────────┘
```

#### Implementation Using SPDK

**Gateway VM Setup** (Ubuntu/NixOS):
```bash
# Install SPDK
git clone https://github.com/spdk/spdk
cd spdk
./scripts/pkgdep.sh
./configure
make

# Create NVMe/TCP target
./scripts/rpc.py nvmf_create_transport -t TCP
./scripts/rpc.py nvmf_create_subsystem \
  nqn.2000-02.com.mikrotik:pvc-12345 -a -s SPDK00000000000001

# Add namespace (backed by RDS NVMe device)
./scripts/rpc.py nvmf_subsystem_add_ns \
  nqn.2000-02.com.mikrotik:pvc-12345 /dev/nvme0n1

# Add listener
./scripts/rpc.py nvmf_subsystem_add_listener \
  nqn.2000-02.com.mikrotik:pvc-12345 -t TCP -a 0.0.0.0 -s 4420

# Enable ANA (active-passive)
./scripts/rpc.py nvmf_subsystem_set_ana_state \
  nqn.2000-02.com.mikrotik:pvc-12345 -g 1 -s optimized
```

**Gateway HA Coordination**:
- Use **etcd** or **Consul** for leader election
- Active gateway sets ANA state to "Optimized"
- Passive gateway sets ANA state to "Non-Optimized"
- On active failure, passive promotes itself

**Complexity**: Medium-High (need to implement gateway HA, state coordination)

### Option 3: Use Device Mapper Multipath (dm-multipath)

Linux dm-multipath can provide failover **without ANA**:

```bash
# Install multipath tools
apt-get install multipath-tools

# Configure dm-multipath for NVMe devices
cat >> /etc/multipath.conf <<EOF
devices {
    device {
        vendor "NVME"
        product "MikroTik RDS"
        path_grouping_policy "failover"  # Active-passive
        failback "immediate"
        no_path_retry 5
    }
}
EOF

# Restart multipathd
systemctl restart multipathd
```

**Characteristics**:
- ✅ Works without ANA support from RDS
- ✅ Standard Linux tooling
- ❌ Slower failover than native NVMe multipath (~5-10s vs <1s)
- ❌ No ANA state awareness (manual path priority)
- ❌ Extra block layer overhead

---

## Relevant to RDS CSI Driver

### Current State

**Single RDS** → No multipath, no ANA needed
- Simple, direct NVMe/TCP connection
- RTO on RDS failure: ∞ (manual intervention)

### With Multiple RDS

#### Without ANA
- Must use **active-passive at CSI driver level** (manual failover)
- See [high-availability.md - Approach 2](high-availability.md#approach-2-active-passive-failover-with-volume-migration)
- RTO: 2-5 minutes (pod restart + volume remount)

#### With ANA
- **Native NVMe multipath** handles failover (transparent, <1s)
- See [high-availability.md - Approach 1](high-availability.md#approach-1-nvmetcp-multipathing-with-ana-active-active)
- RTO: <1 second (automatic path switch)

### Bottom Line

ANA would be the **"gold standard"** for HA, but requires either:
1. MikroTik adding ANA to RDS firmware, or
2. Building a gateway layer with ANA support (SPDK/nvmet)

For a **practical homelab HA solution**, **active-passive with rsync** (no ANA) is more realistic.

---

## Further Reading

### NVMe Specifications
- [NVMe 1.3 Specification](https://nvmexpress.org/specifications/) - ANA introduced in section 8.20
- [NVMe/TCP Transport Specification](https://nvmexpress.org/specifications/)

### Linux Kernel Documentation
- [NVMe Multipath Documentation](https://www.kernel.org/doc/html/latest/block/nvme-multipath.html)
- [nvme-cli GitHub](https://github.com/linux-nvme/nvme-cli)

### SPDK Documentation
- [SPDK NVMe-oF Target](https://spdk.io/doc/nvmf.html)
- [SPDK ANA Support](https://spdk.io/doc/nvme_multipath.html)

### Enterprise Implementations
- [NetApp ANA Implementation](https://www.netapp.com/media/10681-tr4684.pdf)
- [Pure Storage ActiveCluster](https://www.purestorage.com/products/storage-software/purity/active-cluster.html)

---

## See Also

- [High Availability Architecture](high-availability.md) - HA approaches for RDS CSI
- [Architecture](architecture.md) - Current single-RDS architecture
- [RDS Commands](rds-commands.md) - RouterOS CLI reference
