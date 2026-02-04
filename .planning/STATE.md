# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-03)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 12 - Compatibility and Quality (v0.6.0)

## Current Position

Phase: 15 of 15 (VolumeAttachment-Based State Rebuild)
Plan: 3 of 4 in current phase
Status: In progress
Last activity: 2026-02-04 — Completed 15-03-PLAN.md (PV annotation documentation)

Progress: [███████████████████████████████████] 100% (53/53 plans completed across all phases)

## Performance Metrics

**Velocity:**
- Total plans completed: 53
- Phases completed: 13
- Average phase completion: 4.08 plans/phase

**By Milestone:**

| Milestone | Phases | Plans | Status |
|-----------|--------|-------|--------|
| v1 Production Stability | 1-4 | 17/17 | Complete |
| v0.3.0 Volume Fencing | 5-7 | 12/12 | Complete |
| v0.5.0 KubeVirt Live Migration | 8-10 | 12/12 | Complete |
| v0.6.0 Block Volume Support | 11-14 | 9/9 | Complete |

**Recent Trend:**
- Last milestone (v0.5.0): 12 plans, 3 phases
- Trend: Stable execution pattern

*Updated: 2026-02-04*

## Accumulated Context

### Roadmap Evolution

- Phase 15 added: VolumeAttachment-Based State Rebuild (v0.7.0 milestone - architectural improvement to use VolumeAttachment objects as source of truth instead of PV annotations)
- Phase 14 added: Error Resilience and Mount Storm Prevention (discovered mount storm issue during Phase 13 execution - corrupted filesystem caused thousands of duplicate mounts)

### Decisions

Recent decisions from PROJECT.md affecting v0.7.0 work:

- Phase 15-03 (2026-02-04): **PV annotations are informational-only**
  - Annotations written during ControllerPublishVolume for debugging/observability
  - Never read during state rebuild - VolumeAttachment objects are authoritative
  - Package-level documentation added explaining write-only nature
  - Prevents future confusion about annotation vs VolumeAttachment roles
- Phase 15-02 (2026-02-04): **VA-based rebuild replaces annotation-based**
  - RebuildStateFromVolumeAttachments is now the authoritative rebuild method
  - VolumeAttachment objects are managed by external-attacher and never stale
  - Old annotation-based rebuild renamed to RebuildStateFromAnnotations (deprecated)
  - Eliminates stale state bugs, aligns with CSI best practices
- Phase 15-02 (2026-02-04): **Conservative AccessMode default**
  - Default to RWO if PV not found or access mode lookup fails
  - RWO is safer default - prevents incorrect dual-attach allowance
  - Volume may be rejected for RWX dual-attach if PV missing, but data safety preserved
- Phase 15-02 (2026-02-04): **Migration detection from VA count**
  - If volume has >1 attached VA, mark as migration state with MigrationStartedAt
  - Multiple VAs only exist during migration window, older VA timestamp is start time
  - Automatic migration state recovery without additional coordination
- Phase 15-01 (2026-02-04): **Empty slice return convention**
  - VolumeAttachment listing functions return empty slice (not nil) when no results found
  - Allows safe iteration without nil checks, consistent Go idiom
  - Applied to: ListDriverVolumeAttachments, FilterAttachedVolumeAttachments, GroupVolumeAttachmentsByVolume
- Phase 15-01 (2026-02-04): **Skip invalid VAs instead of failing**
  - GroupVolumeAttachmentsByVolume skips VAs with nil PersistentVolumeName
  - Logs warning but continues processing other VAs
  - Rationale: Partial data better than complete failure in state rebuild
- Phase 15-01 (2026-02-04): **Client-side VA filtering**
  - List all VolumeAttachments, filter client-side by Spec.Attacher
  - Could use field selectors but not all fields indexed
  - Rationale: Simpler, works across all Kubernetes versions, acceptable for small VA counts

Recent decisions from v0.6.0 work:

- Phase 15 planning (2026-02-04): **VolumeAttachment-based rebuild deferred to v0.7.0**
  - Bug report suggested using VolumeAttachment objects as source of truth instead of PV annotations
  - Architecturally superior approach (external-attacher is authoritative, no stale state risk)
  - Current fix (62197ce - clear annotations on detach) is sufficient for v0.6.0
  - Deferring to v0.7.0 because: 1) Current fix solves the production bug, 2) Grace period logic needs careful design, 3) Project is 92% through milestone with critical fixes ready to deploy
  - Phase 15 will implement VolumeAttachment watcher, rebuild from VolumeAttachment objects, keep PV annotations informational only
- Phase 13 (2026-02-04): **Clear PV annotations on full detachment** (commit 62197ce - FIX)
  - Root cause: RemoveNodeAttachment only cleared in-memory state, not PV annotations
  - On controller restart, RebuildState read stale annotations and resurrected detached volumes
  - Caused "RWO volume already attached to node X" errors blocking legitimate reattach to node Y
  - Solution: Call clearAttachment() when last node detaches to remove annotations from PV
  - Prevents stale state across controller restarts, fixes legacy PVC attachment issues
- Phase 13 (2026-02-04): **Block volumes use mknod instead of bind mount** (commit 0ea6bee - CRITICAL FIX)
  - Root cause of mount storm: bind mounting from `/dev/nvmeXnY` (in devtmpfs) triggers mount namespace propagation
  - Mount propagation cascades devtmpfs through containers → 2048 devtmpfs mounts → kernel memory exhaustion
  - Solution: Use syscall.Mknod to create device node with same major:minor as source device
  - Creates device node directly at target path without touching devtmpfs (no bind mount = no propagation)
  - NodeUnpublishVolume detects block devices (syscall.S_IFBLK) and removes via os.Remove instead of unmount
  - Avoids K3s 1.34.1 mount storm entirely while maintaining CSI spec compliance
  - CSI spec allows either bind mount OR mknod - we chose mknod for stability
- Phase 13 (2026-02-04): **Block volumes follow AWS EBS pattern - no staging directory** (root cause analysis)
  - NodeStageVolume for block volumes connects NVMe device but creates NOTHING at staging_target_path
  - NodePublishVolume finds device by NQN lookup (not from staging path) and bind mounts to target file
  - Previous approach (symlink at staging_path/device) was wrong - kubelet never reads from staging path
  - Kubelet calls AttachFileDevice on globalMapPath, which runs losetup if file isn't already a block device
  - AWS EBS NodeStageVolume for block volumes just returns success immediately - no staging operations
  - Our implementation stages NVMe connection (device appears on node) but mirrors EBS pattern for paths
  - Losetup error was caused by trying to use directory/symlink instead of letting kubelet handle device mapping
- Phase 13 (2026-02-04): **Pre-mount storm detection activated** (commit e2303ce)
  - Mount() now calls DetectDuplicateMounts BEFORE attempting mount syscall
  - Refuses to mount if device already has >= 100 mounts (fail-fast instead of wedging node)
  - DetectDuplicateMounts existed since Phase 14-02 but was never called in production (only tests!)
  - r640 mount storm validated need: 502MB slab memory, soft lockup, OOM kill
  - With fix, Mount() fails fast with clear error instead of getting stuck in kernel mount propagation
  - Complements circuit breaker and health check (prevents initial mount during storm, not just retries)
- Phase 13 (2026-02-04): **Smart orphaned mount cleanup** (commit dc4140f)
  - NodeUnstageVolume now cleans up orphaned bind mounts before device-in-use check
  - Prevents node wedging from self-detecting own bind mounts as "device in use"
  - Forces cleanup during graceful shutdown (ctx.Done()) to prevent mount namespace wedging
  - Eliminates need for node reboots when cleanup fails

- Phase 14-03: Circuit breaker opens after 3 consecutive failures with 5-minute timeout
- Phase 14-03: Per-volume isolation - one volume failure doesn't affect others
- Phase 14-03: Annotation-based reset: rds.csi.srvlab.io/reset-circuit-breaker=true
- Phase 14-03: Health check only runs on existing filesystems (skip new volumes)
- Phase 14-03: Skip health check if fsck tool not available (test compatibility)
- Phase 14-03: Block volumes bypass health check (no filesystem to check)
- Phase 14-04: 30 second shutdown timeout balances operation completion with restart speed
- Phase 14-04: 60 second terminationGracePeriodSeconds gives 2x buffer for graceful shutdown
- Phase 14-04: ConfigMap-based NQN prefix configuration enables cluster-specific filtering
- Phase 14-04: Driver waits for signal with goroutine-based error handling
- Phase 14-01: Driver refuses to start if CSI_MANAGED_NQN_PREFIX not set or invalid (fail-fast safety)
- Phase 14-01: NQN prefix validation checks NVMe spec compliance (nqn. prefix, colon, 223 byte limit)
- Phase 14-01: OrphanCleaner requires prefix at construction (no default, explicit configuration)
- Phase 14-01: Environment variable over flag for NQN prefix configuration
- Phase 14-02: Use moby/sys/mountinfo for production-ready mount parsing (Docker/containerd standard)
- Phase 14-02: 10 second timeout for procmounts parsing prevents hangs
- Phase 14-02: 100 mount threshold for duplicate detection catches mount storms
- Phase 14-02: Deprecate GetMounts in favor of GetMountsWithTimeout
- Phase 13: Critical bug fix in Mount() - skip MkdirAll when target is file (block volumes)
- Phase 13: Orphan cleaner NQN filtering bug documented (not active, but blocker for future use)
- Phase 13: All worker nodes recovered, CSI driver deployed with fix (commit 3807645)
- Phase 12-01: Use invalid volume ID format in tests to skip stale mount checker complexity
- Phase 12-01: Error messages validated for WHAT + HOW structure (problem + solution)
- Phase 11-03: Block volume detection via staging metadata file in NodeUnstageVolume
- Phase 11-03: Skip unmount for block volumes, clean up metadata file and staging directory
- Phase 11-02: Bind mount NVMe device to target file (not mknod - simpler, safer)
- Phase 11-02: Unified cleanup with os.RemoveAll for both file and directory targets
- Phase 11-01: Block staging metadata in plain text device file (simple, debuggable)
- Phase 11-01: staging_target_path always directory per CSI spec (publish target is file for block)
- Phase 10: ctrl_loss_tmo=-1 default prevents filesystem read-only mount
- Phase 10: Custom prometheus.Registry avoids restart panics
- v0.5.0: RWX block-only, reject RWX filesystem (prevents data corruption)
- v0.5.0: 2-node limit during migration (sufficient for KubeVirt)

### Pending Todos

None yet. (Use `/gsd:add-todo` to capture ideas during execution)

### Blockers/Concerns

**Resolved:**
- ✓ Critical Mount() bug fixed (commit 3807645) - block volumes now work
- ✓ Worker nodes recovered and healthy
- ✓ Fixed driver deployed to all nodes
- ✓ NQN filtering bug fixed (commit 6d7cece) - prevents system volume disconnect
- ✓ CI test failure fixed (commit 7728bd4) - health check now skips when device doesn't exist

**Active:**
- **CRITICAL: Two fixes batched for single CI/CD deployment**
  - Fix 1 (0ea6bee): mknod for block volumes - prevents devtmpfs mount storm
  - Fix 2 (62197ce): Clear PV annotations on detach - fixes stale attachment state
  - Both fixes address production issues with legacy PVCs and block volumes
  - CI/CD build in progress (15 minutes)
  - Will test both fixes on r740xd immediately after deployment
- **r640/r740xd node recovery needed** - both experienced mount storms during testing
  - r740xd: 2048 devtmpfs mounts, caught early and recovered after pod deletion
  - r640: 502MB unreclaimable slab memory, soft lockup, OOM kill (needs reboot)
  - mknod fix eliminates root cause, preventing future storms
- Helm chart needs update to expose CSI_MANAGED_NQN_PREFIX as configurable value (after Phase 14)

**Critical Discovery:**
- Diskless nodes mount /var from RDS via NVMe-oF (NQN pattern: nixos-*)
- Without NQN filtering, orphan cleaner or disconnect operations can brick nodes
- Fixed in commit 6d7cece with hardcoded filter, Phase 14 will make it configurable

## Session Continuity

Last session: 2026-02-04
Stopped at: Phase 15-03 complete (PV annotation documentation)
Resume file: None
Next action: Continue with Phase 15-04 (final integration and testing)

**Phase 13 Hardware Validation Progress:**
1. ✓ Created comprehensive validation runbook (test/e2e/PROGRESSIVE_VALIDATION.md)
2. ✓ Fixed block volume losetup error (removed dev/ directory creation)
3. ✓ Implemented smart orphaned mount cleanup (prevents node wedging)
4. ✓ Deployed fixes to cluster (commits dae1c4f, dc4140f, 48630c2)
5. ✓ Root cause analysis - symlink approach was wrong, not K3s bug
   - Researched AWS EBS CSI driver implementation (correct pattern)
   - NodeStageVolume for block volumes does NOT create staging directory
   - NodePublishVolume finds device by NQN, not from staging path
   - Kubelet's AttachFileDevice expects nothing at staging path for CSI block volumes
6. ✓ **Fixed block volume implementation** - follows AWS EBS pattern
   - NodeStageVolume: connect NVMe, return success (no staging directory)
   - NodePublishVolume: find device by NQN, bind mount to target file
   - NodeUnstageVolume: detect block volumes by absence of mount, not symlink
7. 🔍 **Mount storm confirmed on r640:**
   - Kernel logs show 502MB unreclaimable slab memory, soft lockup, OOM
   - Validates Phase 14 mount storm prevention work is critical
   - Node wedged and needs recovery
