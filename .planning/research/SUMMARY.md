# Research Summary: Volume Fencing (v0.3.0)

**Project:** RDS CSI Driver - Volume Fencing
**Domain:** Kubernetes CSI ControllerPublishVolume/ControllerUnpublishVolume implementation
**Researched:** 2026-01-30
**Confidence:** HIGH

## Executive Summary

Volume fencing through `ControllerPublishVolume` and `ControllerUnpublishVolume` solves the critical volume ping-pong problem causing KubeVirt VMs to pause every 7 minutes due to concurrent NVMe access. The recommended approach uses **in-memory attachment tracking backed by PV annotations** for persistence across controller restarts, with per-volume locking to prevent race conditions during concurrent publish requests. This is a well-established pattern used by production CSI drivers (AWS EBS, Azure Disk, vSphere).

The implementation requires **zero new dependencies** - everything needed exists in the current stack (client-go v0.28.0, sync.RWMutex, CSI spec v1.10.0). The primary risk is state loss on controller restart leading to silent data corruption, which is mitigated by persisting attachment state to PV annotations and rebuilding state on startup. Secondary risks include race conditions between concurrent publish operations (mitigated by per-volume locking) and dangling attachments after node deletion (mitigated by node existence validation).

This is a foundational capability that enables ReadWriteOnce enforcement for the RDS CSI driver. Without it, multi-attach errors and data corruption are inevitable in any multi-pod or migration scenario.

## Key Findings

### Recommended Stack

**No new dependencies required.** The existing codebase has everything needed for volume fencing:

**Core technologies:**
- **k8s.io/client-go v0.28.0**: PersistentVolumes().Update() for annotation CRUD, retry.RetryOnConflict for conflict handling — already in go.mod
- **sync.RWMutex**: Thread-safe in-memory attachment state tracking — standard library, matches existing codebase patterns (used in 7+ places)
- **CSI Spec v1.10.0**: PUBLISH_UNPUBLISH_VOLUME capability already defined — just needs declaration in ControllerGetCapabilities

**Key finding:** The codebase already uses client-go for the orphan reconciler (`pkg/reconciler/orphan_reconciler.go`), so the pattern for PV annotation updates is well-established. The AttachmentManager follows the same sync.RWMutex + map pattern used consistently throughout the codebase (`pkg/nvme/resolver.go`, `pkg/nvme/nvme.go`, `pkg/rds/pool.go`, `pkg/security/metrics.go`).

### Expected Features

**Must have (table stakes):**
- **PUBLISH_UNPUBLISH_VOLUME capability** - external-attacher requires this to call the RPCs
- **ControllerPublishVolume implementation** - makes volume available on target node before NodeStage
- **ControllerUnpublishVolume implementation** - revokes volume availability after NodeUnstage
- **Attachment state tracking** - in-memory + PV annotation persistence to detect conflicts
- **Idempotent Publish/Unpublish** - retries must succeed without errors
- **FAILED_PRECONDITION on multi-attach** - return gRPC code 9 when RWO volume already attached elsewhere
- **NOT_FOUND for missing volume/node** - return gRPC code 5 for non-existent resources
- **publish_context return** - pass NVMe connection metadata to NodeStageVolume

**Should have (differentiators):**
- **publish_context with NVMe parameters** - pass ctrl_loss_tmo, reconnect_delay for resilient connections (low effort, high value)
- **Kubernetes Events** - post events for attach/detach visibility (already have event infrastructure)
- **Prometheus metrics** - expose attachment counts, durations, failure rates (already have metrics infrastructure)

**Defer (v2+):**
- **Persistent attachment state in ConfigMap/CRD** - in-memory + PV annotations sufficient for single-replica controller
- **LIST_VOLUMES_PUBLISHED_NODES capability** - adds complexity for attachment reconciliation
- **Force-detach with timeout** - safety concerns, requires careful design
- **Node health integration** - consider node Ready status before allowing attachment

### Architecture Approach

The implementation adds a new **AttachmentManager** component that integrates into the existing ControllerServer. It maintains primary state in memory (fast lookups) with PV annotations as durable backup for controller restarts. The AttachmentManager uses per-volume locking to serialize concurrent ControllerPublishVolume calls, preventing race conditions where two nodes could simultaneously attach the same RWO volume.

**Major components:**
1. **AttachmentManager** (`pkg/attachment/manager.go`) - Thread-safe volume-to-node attachment tracking; enforces RWO constraints; persists to PV annotations
2. **ControllerPublishVolume** (modified `pkg/driver/controller.go`) - Validates RWO constraints; records attachment; returns publish_context with NVMe connection info
3. **ControllerUnpublishVolume** (modified `pkg/driver/controller.go`) - Removes attachment record defensively (always succeeds even if state inconsistent)
4. **Driver capability declaration** (modified `pkg/driver/driver.go`) - Adds PUBLISH_UNPUBLISH_VOLUME to ControllerGetCapabilities
5. **CSIDriver manifest** (`deploy/kubernetes/csi-driver.yaml`) - Sets attachRequired: true to enable external-attacher

**Key pattern:** In-memory state with annotation persistence. Primary state lives in a map[string]AttachmentInfo protected by sync.RWMutex. PV annotations serve as backup - updated asynchronously (best-effort) on attach/detach, and loaded synchronously on controller startup to rebuild state after restarts.

### Critical Pitfalls

1. **In-Memory State Loss on Controller Restart** - Pure in-memory tracking loses all attachment knowledge when controller restarts. Next ControllerPublishVolume for already-attached volume succeeds, allowing multi-attach. **Prevention:** Persist to PV annotations, rebuild state on startup from all PVs with driver name matching. This is the FIRST thing to implement.

2. **Race Condition Between Concurrent Publish Calls** - Two pods scheduled simultaneously on different nodes for same RWO volume. Both ControllerPublishVolume calls check attachments[vol] before either completes, both pass the check, both attach. **Prevention:** Per-volume locking using sync.Map of mutexes - serialize all publish operations for each volume. Must be present from day one.

3. **Dangling VolumeAttachments After Node Deletion** - Node forcefully removed but VolumeAttachment remains. Controller's attachment state shows volume → deleted_node. New pod scheduled on healthy node gets rejected with "already attached". **Prevention:** Validate node existence before rejecting publish request; background reconciliation loop to clean stale attachments every 5 minutes.

4. **ControllerUnpublish Before NodeUnstage Completes** - external-attacher doesn't wait for NodeUnstageVolume completion. Calls ControllerUnpublishVolume while NVMe still connected. Controller clears state, new pod attaches immediately while old node still accessing volume. **Prevention:** Reattachment grace period (30s) where volume marked as "unpublishing" but not immediately available for new attachments.

5. **Incorrect Error Codes** - Returning error when it should be success (idempotent same-node publish), or ABORTED when it should be FAILED_PRECONDITION. Wrong codes cause infinite retries or stuck pods. **Prevention:** Read CSI spec carefully - same-node publish returns success, different-node RWO publish returns FAILED_PRECONDITION (code 9), in-flight operation returns ABORTED (code 10).

## Implications for Roadmap

Based on research, this milestone naturally divides into 2-3 phases with clear boundaries and dependencies.

### Phase 1: Core Fencing (Foundation)
**Rationale:** Attachment tracking persistence and concurrency control are prerequisites for everything else. Without these, data corruption is inevitable. These must be correct from day one.

**Delivers:**
- AttachmentManager with in-memory state + PV annotation persistence
- State rebuild on controller startup
- Per-volume locking to prevent concurrent publish races
- Basic ControllerPublishVolume/ControllerUnpublishVolume stubs

**Addresses (from FEATURES.md):**
- Attachment state tracking (table stakes)
- Idempotent Publish/Unpublish (table stakes)
- PV annotation CRUD (persistence mechanism)

**Avoids (from PITFALLS.md):**
- Pitfall 1: In-memory state loss (CRITICAL)
- Pitfall 2: Concurrent publish race (CRITICAL)
- Pitfall 6: PV annotation update conflicts

**Implementation order:**
1. Create `pkg/attachment/types.go` - Attachment struct, AttachmentManager interface
2. Create `pkg/attachment/manager.go` - In-memory state with RWMutex, Attach/Detach/GetAttachment methods
3. Add PV annotation persistence - persistAttachment (async), LoadFromAnnotations (startup sync)
4. Wire AttachmentManager into ControllerServer - add field, initialize, call LoadFromAnnotations
5. Implement basic ControllerPublishVolume - request validation, RWO enforcement, idempotent behavior
6. Implement basic ControllerUnpublishVolume - defensive handling, always return success

### Phase 2: CSI Spec Compliance
**Rationale:** After core state management works, implement full CSI spec requirements. This ensures external-attacher integration works correctly and error handling is spec-compliant.

**Delivers:**
- PUBLISH_UNPUBLISH_VOLUME capability declaration
- Correct CSI error codes (FAILED_PRECONDITION, NOT_FOUND, ABORTED)
- publish_context with NVMe connection parameters
- Volume/node existence validation
- Unit tests for all CSI method behaviors

**Addresses (from FEATURES.md):**
- PUBLISH_UNPUBLISH_VOLUME capability (table stakes)
- FAILED_PRECONDITION on multi-attach (table stakes)
- NOT_FOUND for missing resources (table stakes)
- publish_context with NVMe parameters (differentiator)

**Uses (from STACK.md):**
- CSI Spec v1.10.0 capability definitions
- client-go for volume/node existence checks

**Avoids (from PITFALLS.md):**
- Pitfall 5: Incorrect error codes (HIGH severity)
- Pitfall 7: Missing publish_context
- Pitfall 11: Not handling volume not found in unpublish

**Implementation order:**
1. Update capability declarations - add PUBLISH_UNPUBLISH_VOLUME to cscaps
2. Update CSIDriver manifest - set attachRequired: true
3. Implement correct error code handling in Publish/Unpublish
4. Build publish_context map with NVMe connection info
5. Add volume/node existence validation
6. CSI sanity tests for spec compliance

### Phase 3: Production Robustness (Optional for v0.3.0, Required Before v1.0)
**Rationale:** After basic fencing works, add defensive handling for node failures and edge cases. These are critical for production use with node failures but not blocking for initial release.

**Delivers:**
- Node existence validation before rejecting publish
- Background reconciliation to clean stale attachments
- Reattachment grace period to prevent dual-access during migration
- Prometheus metrics for attachment operations
- Kubernetes Events for attach/detach lifecycle

**Addresses (from FEATURES.md):**
- Prometheus metrics (differentiator)
- Kubernetes Events (differentiator)
- Graceful handling of node failures

**Avoids (from PITFALLS.md):**
- Pitfall 3: Dangling attachments after node deletion (CRITICAL)
- Pitfall 4: Unpublish before unstage completes (HIGH)
- Pitfall 8: 6-minute force detach timeout (MEDIUM)
- Pitfall 9: Stuck VolumeAttachment finalizer (HIGH)

**Implementation order:**
1. Add node existence check in ControllerPublishVolume
2. Implement background reconciliation loop (5-minute interval)
3. Add reattachment grace period logic (30s delay)
4. Add Prometheus metrics for attachments
5. Post Kubernetes Events on attach/detach
6. E2E tests for node failure scenarios

### Phase Ordering Rationale

- **Phase 1 before Phase 2:** Must have working state management before declaring capability, otherwise external-attacher will call methods that crash
- **Phase 2 before Phase 3:** Spec compliance is higher priority than edge case handling; better to block multi-attach correctly than handle stale attachments gracefully
- **Phase 3 optional for initial release:** Can ship v0.3.0 with Phases 1-2, add Phase 3 in v0.3.1 or defer to v0.4.0 depending on testing results

**Dependency chain:**
```
Phase 1 (State Management)
    ↓
Phase 2 (CSI Compliance) ← Can release v0.3.0 here
    ↓
Phase 3 (Production Hardening) ← Add in v0.3.1 or v0.4.0
```

### Research Flags

**Phases with standard patterns (skip research-phase):**
- **Phase 1:** Attachment tracking is well-documented in AWS EBS CSI, Azure Disk CSI, vSphere CSI drivers. sync.RWMutex pattern already used throughout codebase.
- **Phase 2:** CSI spec is authoritative source. Error codes and capability declarations are explicit in spec.
- **Phase 3:** Node existence validation is standard Kubernetes API. Metrics and Events already implemented in codebase.

**No additional research needed.** This research is comprehensive and HIGH confidence. All patterns are established, all APIs are documented, all pitfalls are known from production driver issues.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | **HIGH** | Zero new dependencies. All APIs already in use in codebase. |
| Features | **HIGH** | CSI spec is explicit about requirements. Table stakes clearly defined. |
| Architecture | **HIGH** | Pattern verified in 3+ production CSI drivers. Matches existing codebase patterns. |
| Pitfalls | **HIGH** | Critical pitfalls derived from CSI spec requirements and production driver issues. 20+ GitHub issues analyzed. |

**Overall confidence:** **HIGH**

This is one of the most well-documented areas of CSI driver development. The CSI specification is explicit, production drivers provide reference implementations, and pitfalls are well-known from years of community experience.

### Gaps to Address

**No significant gaps.** The research is comprehensive for the v0.3.0 scope.

**Minor note:** Live migration with KubeVirt VMs is not supported with RWO volumes (Pitfall 12). This is a known limitation - live migration requires RWX access mode or storage-level migration. Document this clearly but defer RWX support to post-v0.3.0.

**Hardware limitation:** NVMe/TCP on MikroTik RDS does not support NVMe Reservations (storage-level fencing equivalent to SCSI-3 Persistent Reservations). Controller-level fencing is sufficient for standard Kubernetes workflows but won't prevent direct NVMe connections outside CSI. Document this limitation.

## Sources

### Primary (HIGH confidence)
- **CSI Specification v1.7.0-1.10.0** - ControllerPublishVolume/ControllerUnpublishVolume requirements, error code definitions, idempotency semantics
- **Kubernetes client-go v0.28.0** - PersistentVolume API, retry.RetryOnConflict patterns
- **Existing codebase** - `pkg/reconciler/orphan_reconciler.go` (PV access patterns), `pkg/nvme/resolver.go` (sync.RWMutex patterns), `pkg/driver/controller.go` (current stubs)

### Secondary (MEDIUM-HIGH confidence)
- **AWS EBS CSI Driver** - Production attachment tracking implementation, ControllerPublishVolume reference
- **Azure Disk CSI Driver** - State recovery after controller restart patterns
- **vSphere CSI Driver** - Multi-attach prevention and error handling
- **DigitalOcean CSI Driver** - ControllerPublishVolume example implementation
- **Kubernetes External-Attacher** - VolumeAttachment lifecycle and finalizer handling
- **Kubernetes Issues #67853, #77324, #106710** - VolumeAttachment dangling, force detach behavior
- **CSI Driver Issues** - 15+ issues analyzed across 5 production drivers for pitfall validation

### Tertiary (context only)
- **KubeVirt Documentation** - Live migration requirements (RWX for concurrent access)
- **Longhorn Volume Migration Enhancement** - Alternative to live migration for RWO volumes
- **AWS EBS NVMe Reservations** - Storage-level fencing (not applicable to RDS)

---
*Research completed: 2026-01-30*
*Ready for roadmap: **yes***
*Recommended phases: 2 core + 1 optional (Foundation → Compliance → Robustness)*
*Estimated effort: 25-30 hours for MVP (Phases 1-2)*
