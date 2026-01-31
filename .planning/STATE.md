# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-01-31)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.3.0 Volume Fencing — prevent multi-node attachment conflicts

## Current Position

Phase: 6 of 7 (CSI Publish/Unpublish)
Plan: 3 of 3 complete
Status: Phase complete
Last activity: 2026-01-31 — Completed 06-03-PLAN.md

Progress: [██████░░░░] 60%

## Milestone History

- **v1 Production Stability** — shipped 2026-01-31
  - Phases 1-4, 17 plans
  - NVMe-oF reconnection reliability

- **v0.3.0 Volume Fencing** — in progress
  - Phases 5-7, TBD plans
  - ControllerPublish/Unpublish implementation

## Accumulated Context

### Decisions

| ID        | Decision                                   | Phase | Context                      |
| --------- | ------------------------------------------ | ----- | ---------------------------- |
| ROADMAP-1 | Use ControllerPublish/Unpublish for fencing | 05    | Standard CSI approach        |
| ROADMAP-2 | Store state in-memory + PV annotations      | 05    | Survives controller restarts |
| ROADMAP-3 | Start from Phase 5 (continues from v1)      | 05    | v1 shipped Phase 4           |
| ATTACH-01 | In-memory map with RWMutex for tracking     | 05-01 | Simple, fast, single controller |
| ATTACH-02 | Per-volume locking with VolumeLockManager   | 05-01 | Prevents deadlocks, allows concurrency |
| ATTACH-03 | Lock order: release manager before per-volume | 05-01 | Critical deadlock prevention |
| ATTACH-04 | Rollback on persistence failure             | 05-02 | Ensures in-memory/PV consistency |
| ATTACH-05 | PV annotations for state persistence        | 05-02 | Survives controller restarts |
| ATTACH-06 | Initialize before orphan reconciler         | 05-02 | State ready before operations |
| CSI-01    | Warning event type for attachment conflicts | 06-01 | Blocks pod scheduling         |
| CSI-02    | Actionable message format with both nodes   | 06-01 | Operator visibility           |
| CSI-03    | Idempotent same-node publish returns success | 06-02 | CSI spec compliance           |
| CSI-04    | FAILED_PRECONDITION (code 9) for RWO conflicts | 06-02 | Standard CSI error code       |
| CSI-05    | snake_case keys in publish_context          | 06-02 | Matches volumeContext conventions |
| CSI-06    | Validate blocking node exists, auto-clear if deleted | 06-02 | Self-healing for stale state |
| CSI-07    | Fail-closed on K8s API errors              | 06-02 | Safety over availability      |
| TEST-01   | Test volume IDs use valid UUID format      | 06-03 | Required by validation        |
| TEST-02   | MockClient implements full RDSClient       | 06-03 | Test isolation                |

### Pending Todos

None

### Blockers/Concerns

Production issue motivating this milestone:
- Volume ping-pong between nodes every ~7 minutes
- `CONFLICT: PVC is in use by VMI` errors
- No ControllerPublish/Unpublish = no fencing

## Session Continuity

Last session: 2026-01-31
Stopped at: Completed 06-03-PLAN.md (Phase 6 complete)
Resume file: None

---
*State initialized: 2026-01-30*
*Last updated: 2026-01-31 — Completed 06-03-PLAN.md*
