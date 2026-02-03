# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-03)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.5.0 KubeVirt Live Migration - enable VM live migration with RDS volumes

## Current Position

Phase: 10 of 10 (Observability)
Plan: 1/3 complete (10-01-PLAN.md)
Status: In progress
Last activity: 2026-02-03 - Completed 10-01 (Prometheus Migration Metrics)

Progress: [████████--] 80% (8/10 plans estimated)

## Milestone History

- **v1 Production Stability** - shipped 2026-01-31
  - Phases 1-4, 17 plans
  - NVMe-oF reconnection reliability

- **v0.3.0 Volume Fencing** - shipped 2026-02-03
  - Phases 5-7, 12 plans
  - ControllerPublish/Unpublish implementation

- **v0.5.0 KubeVirt Live Migration** - in progress
  - Phases 8-10
  - Phase 8: Core RWX Capability (RWX-01, RWX-02, RWX-03)
  - Phase 9: Migration Safety (SAFETY-01-04)
  - Phase 10: Observability (OBS-01-03)

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

### Pending Todos

None

### Blockers/Concerns

Research identified concerns to address during implementation:
- RDS multi-initiator behavior needs testing on actual hardware
- Optimal migration timeout (5 min default) may need tuning
- Non-KubeVirt RWX usage risk requires clear documentation

## Session Continuity

Last session: 2026-02-03T16:05:07Z
Stopped at: Completed 10-01-PLAN.md (Prometheus Migration Metrics)
Resume file: None
Next: Plan 10-02 - Event Posting Methods

---
*State initialized: 2026-01-30*
*Last updated: 2026-02-03 - Completed 10-01 (Prometheus Migration Metrics)*
