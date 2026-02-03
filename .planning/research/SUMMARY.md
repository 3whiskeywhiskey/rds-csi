# Project Research Summary

**Project:** RDS CSI Driver - KubeVirt Live Migration Support
**Domain:** Kubernetes CSI Driver for Block Storage with VM Migration
**Researched:** 2026-02-03
**Confidence:** HIGH

## Executive Summary

KubeVirt live migration has a single hard requirement: **PVC access mode must be ReadWriteMany (RWX)**. The RDS CSI driver currently only advertises `SINGLE_NODE_WRITER` (RWO) and `SINGLE_NODE_READER_ONLY` (ROX), causing VMs to show `LiveMigratable: false`. The good news: **no new dependencies are required**. NVMe/TCP natively supports multi-initiator access, and the driver's existing infrastructure (AttachmentManager, VMIGrouper, grace period mechanism) provides most of the foundation needed.

The recommended approach is a "Trust QEMU" strategy: advertise `MULTI_NODE_MULTI_WRITER` capability for **block volumes only**, allow 2-node attachment during migration, and rely on QEMU/KubeVirt to coordinate I/O. This is the simplest path because QEMU already handles the hard problem of I/O coordination during VM migration - the CSI driver just needs to permit the dual-attachment window. RWX for filesystem-mode volumes should be explicitly rejected to prevent data corruption.

The primary risks are data corruption from dual filesystem mounts (critical but preventable by restricting RWX to block-only) and split-brain scenarios during network partitions (inherent to NVMe/TCP without storage-level fencing). Mitigation requires strict capability validation, separate handling for migration vs conflict detection, and clear documentation that RWX is safe only for KubeVirt live migration use cases.

## Key Findings

### Recommended Stack

**No new dependencies required.** The existing CSI driver stack is sufficient:

**Core technologies (unchanged):**
- Go 1.24: Driver implementation - well-established, no change needed
- github.com/container-storage-interface/spec v1.10.0: Already includes `MULTI_NODE_MULTI_WRITER` access mode enum
- NVMe/TCP: Data plane already supports multi-initiator connections to same target
- prometheus/client_golang: Existing metrics framework can be extended for migration tracking

**Existing infrastructure to leverage:**
- AttachmentManager (`pkg/attachment/manager.go`): Tracks volume-to-node attachments, has grace period support
- VMIGrouper (`pkg/driver/vmi_grouper.go`): Per-VMI operation serialization, PVC-to-VMI resolution
- AttachmentReconciler (`pkg/attachment/reconciler.go`): Stale attachment cleanup from deleted nodes
- EventPoster (`pkg/driver/events.go`): Kubernetes events for lifecycle visibility

### Expected Features

**Must have (table stakes):**
- ReadWriteMany access mode for block volumes - KubeVirt checks PVC access mode at VMI startup
- Simultaneous 2-node attachment during migration window - source and destination need concurrent access
- Idempotent ControllerUnpublishVolume - must succeed even if volume not attached (migration cleanup)

**Should have (competitive):**
- Migration-aware metrics - distinguish migration handoffs from RWO conflicts
- Migration events on PVC - visibility into migration lifecycle
- Automatic migration timeout handling - reconciler cleans up stuck migrations

**Defer (v2+):**
- Cluster filesystem support (GFS2/OCFS2) - adds complexity, only needed for true RWX filesystem use cases
- RDS-level namespace reservations/fencing - requires RouterOS investigation
- KubeVirt API client integration - can function without it using existing VMIGrouper

### Architecture Approach

The architecture is **primarily complete**. The core change is adding `MULTI_NODE_MULTI_WRITER` to the driver's capability list and modifying the AttachmentManager to allow 2-node attachment for RWX block volumes during migration. All other infrastructure exists: grace period mechanism can be repurposed for migration window, VMI serialization prevents concurrent operations on same VM, reconciler handles stale attachment cleanup.

**Major components and required changes:**

1. **Driver Capabilities** (`pkg/driver/driver.go`) - Add `MULTI_NODE_MULTI_WRITER` to vcaps array
2. **Attachment Manager** (`pkg/attachment/manager.go`) - Allow dual attachment when access mode is RWX and volume mode is block
3. **Controller Service** (`pkg/driver/controller.go`) - Pass access mode to attachment validation, differentiate migration from conflict
4. **Capability Validation** - Reject RWX for filesystem volumes (prevent corruption)

### Critical Pitfalls

1. **Filesystem mounted on two nodes simultaneously** - Fatal data corruption. Prevention: RWX only for `volumeMode: Block`, explicitly reject RWX filesystem volumes in capability validation.

2. **Advertising RWX without block-only restriction** - Enables silent corruption. Prevention: Validate that `MULTI_NODE_MULTI_WRITER` requests have `cap.GetBlock() != nil`, reject mount volumes.

3. **Single grace period for both migration and conflicts** - Conflicts should fail immediately, migrations need longer window. Prevention: Separate logic - migrations get configurable timeout (5 min default), non-migration dual-attach attempts fail with FAILED_PRECONDITION immediately.

4. **NVMe disconnect while device in use** - Kernel panic or I/O errors. Prevention: NodeUnstageVolume must verify no open file descriptors before `nvme disconnect`.

5. **Split-brain during network partition** - Source node appears down but still has NVMe connection. Prevention: Refuse migration if source node is NotReady; require manual verification for force-detach. Document the limitation clearly.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Core RWX Capability (MVP)
**Rationale:** Minimum change to enable live migration. Focus on the happy path first.
**Delivers:** KubeVirt VMs can live migrate with RWX block PVCs
**Addresses:** ReadWriteMany access mode, simultaneous attachment
**Avoids:** Filesystem corruption pitfall by block-only restriction

Implementation:
- Add `MULTI_NODE_MULTI_WRITER` to driver vcaps
- Modify ControllerPublishVolume to allow 2-node attachment for RWX block volumes
- Add access mode validation in capability check
- Reject RWX for filesystem volumes

**Estimated effort:** 2-3 days

### Phase 2: Migration Safety and Tracking
**Rationale:** MVP needs robustness - handle failures, track state, clean up stale migrations
**Delivers:** Production-ready migration with failure recovery
**Uses:** Existing AttachmentManager, reconciler infrastructure
**Implements:** Migration state machine, timeout handling

Implementation:
- Extend AttachmentState for secondary attachment and migration timestamp
- Separate migration timeout from conflict detection
- Add reconciler logic for stale migration cleanup
- Enhance device-in-use check in NodeUnstageVolume

**Estimated effort:** 2-3 days

### Phase 3: Observability and Documentation
**Rationale:** Operators need visibility; users need guidance on safe usage
**Delivers:** Migration-specific metrics, events, user documentation
**Avoids:** Poor observability pitfall, misuse of RWX outside KubeVirt

Implementation:
- Add Prometheus metrics: migrations_total, migration_duration_seconds, etc.
- Post events: MigrationStarted, MigrationCompleted, MigrationFailed
- Document: RWX is safe only for KubeVirt live migration
- Add troubleshooting guide for common issues

**Estimated effort:** 1-2 days

### Phase 4: Testing and Validation
**Rationale:** Migration has many edge cases; must test slow/failed/fault-injected scenarios
**Delivers:** Confidence that migration works in production conditions

Implementation:
- E2E tests: fast migration, slow migration (throttled network), failed migration
- Fault injection: node failure during migration, network partition
- Data integrity validation: checksum verification post-migration
- VMI serialization verification during migration

**Estimated effort:** 2-3 days

### Phase Ordering Rationale

- **Phase 1 first:** Unblocks the use case with minimal risk. Block-only restriction prevents the most critical pitfall.
- **Phase 2 before Phase 3:** Robustness before observability. A system that fails silently is worse than one that fails loudly.
- **Phase 3 before Phase 4:** Metrics help debug issues found during testing.
- **Phase 4 last:** Full test suite validates all previous work together.

This order also allows incremental delivery: Phase 1 is a working MVP that could be released with documentation caveats.

### Research Flags

**Phases needing deeper research during planning:**
- **Phase 2:** Split-brain protection strategy needs validation. Current recommendation is "refuse migration if source NotReady" but this may be too conservative. Consider testing RDS behavior with concurrent NVMe connections.
- **Phase 4:** VMI serialization during migration needs verification - does the VMIGrouper correctly handle two virt-launcher pods for the same VM?

**Phases with standard patterns (skip research-phase):**
- **Phase 1:** Capability declaration is straightforward CSI spec implementation. Well-documented pattern from Portworx, democratic-csi examples.
- **Phase 3:** Prometheus metrics and Kubernetes events follow established patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | No new dependencies; existing CSI spec supports required capability |
| Features | HIGH | KubeVirt documentation explicitly states RWX requirement |
| Architecture | HIGH | Components exist; changes are localized to capability and attachment logic |
| Pitfalls | HIGH | Well-documented failure modes; filesystem corruption risk is universally understood |

**Overall confidence:** HIGH

### Gaps to Address

- **RDS multi-initiator behavior:** Need to test `nvme connect` from two nodes to same NQN simultaneously on actual RDS hardware. Expected to work (NVMe/TCP spec supports it) but not verified.

- **Optimal migration timeout:** Default 5 minutes is estimated. May need tuning based on real-world memory sizes and network conditions. Recommend making it configurable.

- **KubeVirt virt-launcher lifecycle:** Exact timing of ControllerPublishVolume to target vs ControllerUnpublishVolume from source during migration is understood from docs but not verified with traces.

- **Non-KubeVirt RWX usage:** If users mount RWX block volumes outside KubeVirt context, data corruption is possible. Documentation must be explicit. Consider adding a StorageClass parameter like `restrictToKubevirt: true` to prevent misuse.

## Research Agreement and Disagreement

**Consensus across research files:**
- RWX is required for KubeVirt live migration (all agree)
- Block mode is the correct approach for NVMe/TCP (all agree)
- No new dependencies needed (STACK and ARCHITECTURE agree)
- Data corruption is the primary risk (FEATURES and PITFALLS agree)

**Minor disagreement:**
- ARCHITECTURE.md states "Live Migration NOT Supported" while FEATURES.md and STACK.md describe how to enable it. Resolution: ARCHITECTURE.md reflects current state; other files describe target state for v0.5.0.

**Key insight from synthesis:** PITFALLS.md correctly identifies that the CSI driver trusts QEMU/KubeVirt for I/O coordination - the driver's job is just to permit dual-attachment, not to coordinate writes. This simplifies the implementation significantly.

## Sources

### Primary (HIGH confidence)
- [KubeVirt Live Migration](https://kubevirt.io/user-guide/compute/live_migration/) - RWX requirement, migration workflow
- [CSI Specification](https://github.com/container-storage-interface/spec/blob/master/spec.md) - MULTI_NODE_MULTI_WRITER definition
- [Kubernetes CSI Raw Block Volume](https://kubernetes-csi.github.io/docs/raw-block.html) - Block mode multi-attach guidance
- Existing codebase: `pkg/driver/driver.go`, `pkg/attachment/manager.go`, `pkg/driver/vmi_grouper.go`

### Secondary (MEDIUM confidence)
- [Portworx Raw Block for Live Migration](https://docs.portworx.com/portworx-csi/operations/raw-block-for-live-migration) - 2-node limit pattern
- [democratic-csi RWX Issue](https://github.com/democratic-csi/democratic-csi/issues/285) - QEMU coordination insight
- [Red Hat Storage for OpenShift Virtualization](https://developers.redhat.com/articles/2025/07/10/storage-considerations-openshift-virtualization) - Filesystem corruption warning

### Tertiary (LOW confidence)
- RDS multi-initiator behavior - needs testing on actual hardware
- Optimal migration timeout values - needs E2E validation

---
*Research completed: 2026-02-03*
*Ready for roadmap: yes*
