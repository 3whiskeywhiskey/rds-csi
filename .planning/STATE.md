# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-03)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** Phase 12 - Compatibility and Quality (v0.6.0)

## Current Position

Phase: 13 of 14 (Hardware Validation)
Plan: 1 of 1 in current phase
Status: Ready to deploy - block volumes + mount storm protection fixes
Last activity: 2026-02-04 — Fixed block volumes and added mount storm pre-check (commits d33e09a, e2303ce)

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
- **Ready to deploy: Two critical fixes batched for single CI/CD run**
  - Fix 1 (d33e09a): Block volumes corrected to follow AWS EBS pattern
  - Fix 2 (e2303ce): Mount storm pre-detection activated (was implemented but never called!)
  - Both fixes address issues discovered during Phase 13 hardware validation
  - Single deployment minimizes CI/CD overhead as requested
- **r640 node recovery needed** - mount storm wedged node, requires reboot or manual cleanup
  - 502MB unreclaimable slab memory, soft lockup, OOM kill
  - New mount storm fix would have prevented this
  - Can recover after deploying fixes
- Helm chart needs update to expose CSI_MANAGED_NQN_PREFIX as configurable value (after Phase 14)

**Critical Discovery:**
- Diskless nodes mount /var from RDS via NVMe-oF (NQN pattern: nixos-*)
- Without NQN filtering, orphan cleaner or disconnect operations can brick nodes
- Fixed in commit 6d7cece with hardcoded filter, Phase 14 will make it configurable

## Session Continuity

Last session: 2026-02-04
Stopped at: Block volume implementation corrected, mount storm confirmed on r640
Resume file: None
Next action: Build and deploy block volume fix, then address mount storm issue

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
