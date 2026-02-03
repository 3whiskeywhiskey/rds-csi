# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-03)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.5.0 KubeVirt Live Migration - enable VM live migration with RDS volumes

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining v0.6.0 requirements
Last activity: 2026-02-03 - Started v0.6.0 Block Volume Support milestone

Progress: Milestone initialization

## Milestone History

- **v1 Production Stability** - shipped 2026-01-31
  - Phases 1-4, 17 plans
  - NVMe-oF reconnection reliability

- **v0.3.0 Volume Fencing** - shipped 2026-02-03
  - Phases 5-7, 12 plans
  - ControllerPublish/Unpublish implementation

- **v0.5.0 KubeVirt Live Migration** - VALIDATING 2026-02-03
  - Phases 8-10, 11 plans (implementation complete)
  - Phase 8: Core RWX Capability (RWX-01, RWX-02, RWX-03) ✅
  - Phase 9: Migration Safety (SAFETY-01-04) ✅
  - Phase 10: Observability (OBS-01-05) ✅
  - Hardware validation: Fixing deployment issues on metal cluster

## Accumulated Context

### Decisions

| ID        | Decision                                   | Phase | Context                      |
| --------- | ------------------------------------------ | ----- | ---------------------------- |
| ROADMAP-1 | Use ControllerPublish/Unpublish for fencing | 05    | Standard CSI approach        |
| ROADMAP-2 | Store state in-memory + PV annotations      | 05    | Survives controller restarts |
| ROADMAP-3 | Start from Phase 5 (continues from v1)      | 05    | v1 shipped Phase 4           |
| ROADMAP-4 | RWX block-only, reject RWX filesystem       | 08    | Prevent data corruption      |
| ROADMAP-5 | 2-node limit during migration               | 08    | Sufficient for KubeVirt, prevents misuse |
| ROADMAP-6 | Trust QEMU for I/O coordination             | 08    | Driver permits dual-attach, doesn't coordinate |
| 08-02-01  | Keep deprecated NodeID for backward compat  | 08-02 | Existing PV annotations use NodeID field |
| 08-02-02  | TrackAttachment unchanged (delegates)       | 08-02 | Preserve API compatibility for callers |
| 08-02-03  | Inline access mode detection                | 08-02 | VolumeCapability available in request |
| 08-02-04  | RemoveNodeAttachment returns bool           | 08-02 | Distinguish full vs partial detach |
| 08-03-01  | Table-driven tests for capability coverage  | 08-03 | Easy to add scenarios, clear test names |
| 08-03-02  | Test error messages for user guidance       | 08-03 | Verify actionable help in errors |
| 08-03-03  | Comprehensive dual-attach scenario tests    | 08-03 | Success, idempotent, limits, edge cases |
| 09-01-01  | Migration timeout stored in AttachmentState | 09-01 | Per-volume timeout based on StorageClass |
| 09-01-02  | Default 5 minute timeout with 30s-1h range  | 09-01 | Balance realistic time vs indefinite dual-attach |
| 09-01-03  | Pass timeout via VolumeContext              | 09-01 | Parameters flow CreateVolume to ControllerPublishVolume |
| 09-02-01  | Check timeout before allowing secondary attachment | 09-02 | Prevents indefinite dual-attach from stuck migrations |
| 09-02-02  | RWO grace period documented as reattachment-only | 09-02 | Clarifies it's for sequential handoff, not concurrent access |
| 09-02-03  | Detailed error message with elapsed time and remediation | 09-02 | Operators need actionable guidance when timeout exceeded |
| 09-03-01  | 5-second timeout for lsof device check | 09-03 | Balance responsiveness vs false positives, proceed on timeout |
| 09-03-02  | Skip device check if GetDevicePath returns error | 09-03 | Device not connected (idempotent unstage), no point checking |
| 09-03-03  | Block unstage with FAILED_PRECONDITION if device busy | 09-03 | Prevent data corruption, include process list in error |
| 09-03-04  | Proceed on check failure or timeout | 09-03 | Prevent blocking cleanup in recovery scenarios |
| 09-04-01  | Test migration helper methods in isolation | 09-04 | Pure logic on AttachmentState, test separately from manager |
| 09-04-02  | Limited device check testing without mocking | 09-04 | Real lsof behavior without complex infrastructure |
| 09-04-03  | Table-driven tests for ParseMigrationTimeout | 09-04 | Clear coverage of all cases: valid, invalid, clamped, boundary |
| 10-01-01  | Use subsystem "migration" for metric naming | 10-01 | Consistent with existing patterns, groups related metrics |
| 10-01-02  | Histogram buckets tailored for migration durations | 10-01 | Buckets [15,30,60,90,120,180,300,600]s for migration-specific times |
| 10-01-03  | Result label values match migration outcomes | 10-01 | success/failed/timeout align with migration enforcement |
| 10-01-04  | RecordMigrationResult always decrements gauge | 10-01 | Prevents gauge drift for any result type |
| 10-02-01  | PostMigrationFailed uses EventTypeWarning | 10-02 | Failed migrations are abnormal conditions requiring operator attention |
| 10-02-02  | Duration/timeout rounded to seconds in event messages | 10-02 | Millisecond precision unnecessary for migration timescales (minutes), improves readability |
| 10-02-03  | Event reason constants follow existing naming pattern | 10-02 | Consistency with existing events (VolumeAttached, VolumeDetached, etc.) |
| 10-03-01  | EventPoster created inline in ControllerPublishVolume | 10-03 | Controller doesn't store EventPoster, create when needed for best-effort event posting |
| 10-03-02  | Capture source node before AddSecondaryAttachment | 10-03 | Need source node for event message, but implicit in existing.Nodes[0] before secondary attach |
| 10-03-03  | Capture migration start time before clearing state | 10-03 | RemoveNodeAttachment clears MigrationStartedAt, but need it for duration calculation |
| 10-04-01  | Prominent safety warnings using ✅/❌ symbols for visual clarity | 10-04 | Users must immediately understand RWX is safe ONLY for KubeVirt, not general workloads |
| 10-04-02  | Include complete code examples of both safe and unsafe usage | 10-04 | Show exact YAML to avoid ambiguity about what NOT to do |
| 10-04-03  | Document data corruption as unrecoverable - restore from backup | 10-04 | Set realistic expectations, prevent futile recovery attempts |
| 10-05-01  | Query PV to get PVC namespace/name in ControllerUnpublishVolume | 10-05 | Best-effort event posting, avoids storing PVC info in state |
| 10-05-02  | Capture migration state before RemoveNodeAttachment | 10-05 | Preserve source/target nodes and start time before state cleared |
| 10-05-03  | Post event only on partial detach | 10-05 | Partial detach = migration completion, full detach = normal unpublish |

### Pending Todos

None

### Blockers/Concerns

**Active (Hardware Validation):**
- r740xd node: OOM-killed node plugin due to insufficient memory limits
  - Fixed: Increased limits 256Mi→512Mi, requests 64Mi→128Mi
  - Status: Pending reboot to apply DaemonSet changes
  - Impact: Test VM stuck in Scheduling, blocking migration validation

**Hardware Testing Progress:**
1. ✅ Missing csi-attacher sidecar (commit 0bccf4d)
2. ✅ CSIDriver attachRequired=false → true (commit e7291be)
3. ✅ Missing RBAC permission for PV updates (commit e7291be)
4. ✅ Node plugin memory limits too low (deploy/kubernetes/node.yaml modified)
5. ⏳ Waiting for r740xd reboot to test VM creation and live migration

**Previously Identified:**
- RDS multi-initiator behavior needs testing on actual hardware (IN PROGRESS on metal cluster)
- Optimal migration timeout (5 min default) may need tuning (pending validation results)
- Non-KubeVirt RWX usage risk requires clear documentation (✅ ADDRESSED: comprehensive docs/kubevirt-migration.md with prominent warnings)

## Validation Session

**Test Environment:** Metal cluster (Kubernetes, 7 nodes, r740xd with KubeVirt)
**Test Resources:**
- PVC: `kubevirt-migration-test` - Bound, RWX, Block, 10Gi ✅
- VolumeAttachment: Created and attached to r740xd ✅
- VM: `migration-test-vm` - Stuck in Scheduling (OOM-killed node plugin)

**Commits Made:**
- `38f56cc` - docs: add metal cluster deployment guide for csi-attacher fix
- `0bccf4d` - fix: add csi-attacher sidecar to controller deployment
- `e7291be` - fix: enable ControllerPublishVolume for RWX block volumes

**Files Modified:**
- `deploy/kubernetes/controller.yaml` - Added csi-attacher v4.5.0 sidecar
- `deploy/kubernetes/csidriver.yaml` - Set attachRequired=true (immutable field)
- `deploy/kubernetes/rbac.yaml` - Added PV update permission
- `deploy/kubernetes/node.yaml` - Increased memory limits to 512Mi

**Current State:**
- Controller v0.5.1: Running with all 5 containers (including csi-attacher) ✅
- Node plugins: 6/7 running, r740xd pending reboot
- VolumeAttachment workflow: Working correctly ✅
- ControllerPublishVolume: Being called correctly ✅

**Next Steps After r740xd Reboot:**
1. Verify node plugin starts without OOM
2. Check if VM starts successfully
3. Test live migration with `virtctl migrate migration-test-vm`
4. Verify migration metrics and events
5. Push commits to remote (currently blocked: permission denied)

## Session Continuity

Last session: 2026-02-03T16:45:12Z
Stopped at: Awaiting r740xd reboot - 4 deployment fixes committed
Resume file: None
Next: Complete hardware validation after r740xd recovers from OOM issue

---
*State initialized: 2026-01-30*
*Last updated: 2026-02-03 - Hardware validation in progress - Fixed 4 deployment issues, awaiting r740xd reboot*
