# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-03)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 12 - Compatibility and Quality (v0.6.0)

## Current Position

Phase: 13 of 14 (Hardware Validation)
Plan: 1 of 1 in current phase
Status: Blocked - Kubernetes 1.34 / K3s bug prevents block volume testing
Last activity: 2026-02-04 — Phase 13 validation blocked by kubelet losetup bug

Progress: [█████████████████████████████████] 92% (49/53 plans completed across all phases)

## Performance Metrics

**Velocity:**
- Total plans completed: 49
- Phases completed: 13
- Average phase completion: 3.77 plans/phase

**By Milestone:**

| Milestone | Phases | Plans | Status |
|-----------|--------|-------|--------|
| v1 Production Stability | 1-4 | 17/17 | Complete |
| v0.3.0 Volume Fencing | 5-7 | 12/12 | Complete |
| v0.5.0 KubeVirt Live Migration | 8-10 | 12/12 | Complete |
| v0.6.0 Block Volume Support | 11-14 | 8/9 | In progress (1 plan remaining) |

**Recent Trend:**
- Last milestone (v0.5.0): 12 plans, 3 phases
- Trend: Stable execution pattern

*Updated: 2026-02-03*

## Accumulated Context

### Roadmap Evolution

- Phase 14 added: Error Resilience and Mount Storm Prevention (discovered mount storm issue during Phase 13 execution - corrupted filesystem caused thousands of duplicate mounts)

### Decisions

Recent decisions from PROJECT.md affecting v0.6.0 work:

- Phase 13 (2026-02-04): **Removed dev/ directory and symlink from NodeStageVolume** (commit dae1c4f)
  - Previous code incorrectly created dev/ directory with symlink in staging path
  - Kubelet interpreted symlink as file needing losetup, causing "Permission denied"
  - Now follows AWS EBS CSI pattern: NodeStageVolume only connects NVMe and stores metadata
  - NodePublishVolume handles bind mount to target path
- Phase 13 (2026-02-04): **Smart orphaned mount cleanup** (commit dc4140f)
  - NodeUnstageVolume now cleans up orphaned bind mounts before device-in-use check
  - Prevents node wedging from self-detecting own bind mounts as "device in use"
  - Forces cleanup during graceful shutdown (ctx.Done()) to prevent mount namespace wedging
  - Eliminates need for node reboots when cleanup fails
- Phase 13 (2026-02-04): **Discovered Kubernetes 1.34 / K3s bug**
  - Kubelet incorrectly calls MapBlockVolume (local volume path) for CSI block volumes
  - Should use CSI NodePublishVolume bind mount, not create loop devices
  - Bug appears specific to K3s 1.34.1 (Kubernetes 1.34 released August 2025)
  - CSI driver implementation verified correct per spec

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
- **CRITICAL: Kubernetes 1.34 / K3s 1.34.1 bug** - kubelet incorrectly calls MapBlockVolume for CSI block volumes, causing losetup failure
  - CSI driver implementation is correct (verified by logs)
  - Kubelet tries to run losetup on already-created block device file
  - Error: "makeLoopDevice failed: losetup -f ... failed: exit status 1"
  - Affects ALL CSI block volumes on K3s 1.34.1
  - Options: downgrade K3s, wait for 1.34.2, or report bug upstream
- Helm chart needs update to expose CSI_MANAGED_NQN_PREFIX as configurable value (after Phase 14)

**Critical Discovery:**
- Diskless nodes mount /var from RDS via NVMe-oF (NQN pattern: nixos-*)
- Without NQN filtering, orphan cleaner or disconnect operations can brick nodes
- Fixed in commit 6d7cece with hardcoded filter, Phase 14 will make it configurable

## Session Continuity

Last session: 2026-02-04
Stopped at: Phase 13 blocked by K3s 1.34.1 kubelet bug with block volumes
Resume file: None
Next action: Debug K3s losetup issue or consider downgrade/workaround

**Phase 13 Hardware Validation Progress:**
1. ✓ Created comprehensive validation runbook (test/e2e/PROGRESSIVE_VALIDATION.md)
2. ✓ Fixed block volume losetup error (removed dev/ directory creation)
3. ✓ Implemented smart orphaned mount cleanup (prevents node wedging)
4. ✓ Deployed fixes to cluster (commits dae1c4f, dc4140f, 48630c2)
5. ✓ Verified CSI driver implementation correct (logs show all operations succeeding)
6. ❌ **BLOCKED:** K3s 1.34.1 kubelet bug - MapBlockVolume called for CSI volumes
   - CSI driver creates bind mount correctly at publish path
   - Kubelet creates block device file at globalMapPath (correct)
   - Kubelet then tries losetup on already-existing block device (incorrect)
   - Error: "makeLoopDevice failed: losetup -f ... failed: exit status 1"
7. 🔍 **Investigation showed:**
   - Same inode at publish path and globalMapPath (kubelet linked correctly)
   - Device type 259,5 (/dev/nvme1n1) - already a block device
   - kubelet.AttachFileDevice doesn't check if path is already block device
   - This may be Kubernetes 1.34 regression (released Aug 2025, very recent)
