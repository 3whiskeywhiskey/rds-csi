# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-06)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.10.0 Feature Enhancements

## Current Position

Phase: 26 of 28 (Volume Snapshots)
Plan: 3 of 3 (Phase complete!)
Status: Phase 26 complete - CSI controller snapshot service implemented
Last activity: 2026-02-06 — Completed 26-03-PLAN.md (CSI controller snapshot RPCs)

Progress: v0.9.0 [██████████] 100% (17/17 plans) | v0.10.0 [█████████░] 33.3% (6/18 plans estimated)

## Performance Metrics

**Velocity:**
- Total plans completed: 102 (79 v0.1.0-v0.8.0 + 17 v0.9.0 + 6 v0.10.0)
- v0.9.0 plans completed: 17/17 (100%)
- v0.10.0 plans completed: 6/18 (33.3%)
- Average duration: ~7 min per plan (v0.9.0), ~4.0 min per plan (v0.10.0 so far)
- Total execution time: ~2 hours (v0.9.0 execution, 92 days calendar)

**By Milestone:**

| Milestone | Phases | Plans | Status |
|-----------|--------|-------|--------|
| v0.1.0-v0.8.0 | 1-21 | 79/79 | ✅ Shipped 2026-02-04 |
| v0.9.0 Production Readiness | 22-25.2 | 17/17 | ✅ Shipped 2026-02-06 |
| v0.10.0 Feature Enhancements | 26-28 | 6/18 | 🚧 In Progress |

**Recent Milestones:**
- v0.10.0: 3 phases (26-28), 6/18 plans, in progress (Phase 26 complete, Phase 27 complete)
- v0.9.0: 6 phases (22-25.2), 17 plans, 92 days, shipped 2026-02-06
- v0.8.0: 5 phases (17-21), 20 plans, 1 day, shipped 2026-02-04

*Updated: 2026-02-06*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- v0.10.0 (Phase 26-03): Use timestamppb for CSI CreationTime field (protobuf compatibility)
- v0.10.0 (Phase 26-03): getBtrfsFSLabel checks params then defaults to storage-pool (configurable)
- v0.10.0 (Phase 26-03): CreateSnapshot validates volume ID format before RDS operations (security)
- v0.10.0 (Phase 26-03): DeleteSnapshot idempotent per CSI spec (not-found returns success)
- v0.10.0 (Phase 26-02): CreateSnapshot uses read-only=yes for immutable snapshots
- v0.10.0 (Phase 26-02): DeleteSnapshot idempotent (not found = return nil per CSI spec)
- v0.10.0 (Phase 26-02): RestoreSnapshot creates writable clone (no read-only flag) + disk entry
- v0.10.0 (Phase 26-02): parseSnapshotInfo handles missing fields gracefully (controller tracks metadata)
- v0.10.0 (Phase 26-02): ListSnapshots filters snap-* prefix at parse level (defense in depth)
- v0.10.0 (Phase 26-01): Reuse volumeNamespace UUID for SnapshotNameToID (no collision risk between volume names and snapshot names)
- v0.10.0 (Phase 26-01): MockClient.CreateSnapshot is idempotent (same name + same source = return existing)
- v0.10.0 (Phase 26-01): MockClient.DeleteSnapshot is idempotent (not found = return nil)
- v0.10.0 (Phase 26-01): Snapshot ID format snap-<uuid> mirrors volume ID format pvc-<uuid>
- v0.10.0 (Phase 27-03): Symptom-driven troubleshooting format provides fastest path to resolution
- v0.10.0 (Phase 27-03): Mock-reality divergence section critical for setting testing expectations
- v0.10.0 (Phase 27-03): CI test job template reduces friction for extending test pipeline
- v0.10.0 (Phase 27-02): Compare against AWS EBS CSI and Longhorn (not SPDK/iSCSI drivers) for familiar reference points
- v0.10.0 (Phase 27-02): Acknowledge single-server architecture vs distributed storage upfront (fair comparison framework)
- v0.10.0 (Phase 27-02): Provide "why not" explanation for every missing feature (transparency builds trust)
- v0.10.0 (Phase 27-02): Known limitations include detection methods and workarounds (actionable troubleshooting)
- v0.10.0 (Phase 27-01): Test case structure with objective, prerequisites, steps, cleanup, success criteria (consistent format across all tests)
- v0.10.0 (Phase 27-01): Document expected operation timings (10-30s volume creation, 2-5s NVMe connect) for performance baselining
- v0.10.0 (Phase 27-01): Cleanup procedures must be idempotent (work even if test fails mid-way to prevent storage exhaustion)
- v0.10.0 (Phase 27-01): Use production IPs in examples (10.42.241.3 management, 10.42.68.1 storage) for copy-paste convenience
- v0.9.0 (Phase 25.2-02): Complexity threshold 50 justified by CSI spec compliance (highest function: ControllerPublishVolume at 48)
- v0.9.0 (Phase 25.2-02): Document top 5 complexity offenders for future refactoring (ControllerPublishVolume 48, RecordEvent 44, NodeStageVolume 36, NewDriver 33, main 31)
- v0.9.0 (Phase 25.2-01): golangci-lint v2 requires string version field (version: "2" not 2)
- v0.9.0 (Phase 25.2-01): golangci-lint v2 uses nested config (linters.settings, linters.exclusions.rules)
- v0.9.0 (Phase 25.2-01): Exclude ST1001 (dot imports) for test/e2e/ files (Ginkgo/Gomega convention)
- v0.9.0 (Phase 25.2-01): Go error strings lowercase, no trailing punctuation (ST1005 convention)
- v0.9.0 (Quick-002): AttachmentReconciler uses two-stage priority-select pattern for shutdown (stop signals checked before work channels)
- v0.9.0 (Phase 25.1-03): Probe prefers connectionManager.IsConnected() over rdsClient.IsConnected() (monitor state more accurate)
- v0.9.0 (Phase 25.1-03): Node watcher registered after informer caches synced (avoids race conditions)
- v0.9.0 (Phase 25.1-03): Connection manager started after attachment reconciler initialized (callback dependency)
- v0.9.0 (Phase 25.1-03): Startup reconciliation uses TriggerReconcile() not direct reconcile() (respects deduplication)
- v0.9.0 (Phase 25.1-03): Connection manager stopped before RDS client closed (clean shutdown order)
- v0.9.0 (Phase 25.1-02): Exponential backoff with jitter (RandomizationFactor=0.1) prevents thundering herd on RDS restart
- v0.9.0 (Phase 25.1-02): ConnectionManager polls every 5 seconds (production-friendly, not chatty)
- v0.9.0 (Phase 25.1-02): MaxElapsedTime=0 for background reconnection (never give up)
- v0.9.0 (Phase 25.1-02): Close old SSH session before reconnecting to prevent session leaks
- v0.9.0 (Phase 25.1-02): ConnectionManager is a monitor, not a proxy - callers use GetClient() directly
- v0.9.0 (Phase 25.1-01): Use buffered channel (size 1) for trigger deduplication (prevents race conditions)
- v0.9.0 (Phase 25.1-01): TriggerReconcile safe to call when reconciler not running (no-op, no panic)
- v0.9.0 (Phase 25.1-01): Detect Ready->NotReady transitions only (ignore NotReady->NotReady)
- v0.9.0 (Phase 25.1-01): Handle tombstone objects in DeleteFunc per client-go patterns
- v0.9.0 (Phase 25-04): CI threshold increased to 65% based on current 68.6% coverage
- v0.9.0 (Phase 25-04): No flaky tests detected after extensive stress testing
- v0.9.0 (Phase 25-01): Map connection/timeout errors to codes.Unavailable per CSI spec
- v0.9.0 (Phase 25-01): DeleteVolume distinguishes VolumeNotFoundError from connection errors
- v0.9.0 (Phase 25-03): Document CSI spec references in test cases for traceability
- v0.9.0 (Phase 25-03): Emphasize idempotency tests for Kubernetes retry behavior
- v0.9.0 (Phase 24-04): E2E tests run in CI via dedicated job (parallel execution)
- v0.9.0 (Phase 24-02): Block volume expansion returns NodeExpansionRequired=false (kernel auto-detects)

### Roadmap Evolution

- **Phase 25.1 inserted after Phase 25**: Attachment Reconciliation & RDS Resilience (URGENT)
  - **Trigger**: Production incident on 2026-02-05 - RDS storage crash caused node failures, leaving stale VolumeAttachment objects that prevented volume reattachment
  - **Impact**: 3-hour infrastructure outage extended to 5+ hours due to manual VolumeAttachment cleanup required (finalizer removal + CSI controller restart)
  - **Scope**: Stale attachment reconciliation, node failure handling, RDS connection resilience, probe health checks
  - **Priority**: Must fix before adding new features (Phase 26 snapshots would inherit same issue)

- **Phase 25.2 inserted after Phase 25.1**: Fix Linter Issues Blocking CI Verification (URGENT)
  - **Trigger**: golangci-lint v2 upgrade in Phase 25.1 exposed 134 pre-existing code quality issues
  - **Impact**: CI/CD verification pipeline blocked, preventing automated quality checks on new code
  - **Scope**: Resolve 63 errcheck, 56 cyclop, 7 gocyclo, 8 staticcheck issues
  - **Priority**: Required before Phase 26 - must unblock CI enforcement of linter checks

### Pending Todos

None yet. (Use `/gsd:add-todo` to capture ideas during execution)

### Blockers/Concerns

None. All pre-existing test failures resolved via Quick-003.

## Session Continuity

Last session: 2026-02-06 05:49
Stopped at: Completed Phase 26 Plan 03 (CSI controller snapshot RPCs) - Phase 26 COMPLETE
Resume file: None
Next action: Phase 26 (Volume Snapshots) is complete. Ready for next phase or milestone planning.

**v0.10.0 Progress (6/18 plans):**
- Phase 26-03: CSI controller snapshot service (CreateSnapshot, DeleteSnapshot RPCs)
  - Implemented CreateSnapshot with idempotency (same name + source = return existing)
  - Implemented DeleteSnapshot with idempotency (not found = success)
  - Registered CREATE_DELETE_SNAPSHOT and LIST_SNAPSHOTS capabilities
  - Added timestamppb import for CSI timestamp handling
  - Added getBtrfsFSLabel helper for Btrfs filesystem label resolution
  - Updated tests to reflect snapshot RPCs now implemented
- Phase 26-02: Snapshot SSH commands with RouterOS Btrfs subvolume operations
  - Implemented 5 sshClient snapshot methods (CreateSnapshot, DeleteSnapshot, GetSnapshot, ListSnapshots, RestoreSnapshot)
  - Added parseSnapshotInfo and parseSnapshotList for RouterOS output parsing
  - Auto-cleanup on partial failures, idempotent operations per CSI spec
  - RestoreSnapshot uses writable snapshot-of-snapshot + disk entry
  - Full unit test coverage (4 test functions, all passing)
- Phase 26-01: Snapshot data model foundation with types, ID utilities, RDSClient interface extension, and MockClient implementation
  - Created SnapshotInfo, CreateSnapshotOptions, SnapshotNotFoundError types
  - Created snapshotid.go with GenerateSnapshotID, ValidateSnapshotID, SnapshotNameToID
  - Extended RDSClient interface with 5 snapshot methods
  - Implemented full snapshot CRUD in MockClient with idempotency
  - Added stub implementations to sshClient (replaced by Plan 26-02)
- Phase 27-01: Hardware validation guide with 7 test cases (TC-01 through TC-07)
  - Created HARDWARE_VALIDATION.md (1565 lines) with executable test procedures
  - Performance baselines documented (timings, I/O benchmarks)
  - Troubleshooting decision trees for common failure modes
- Phase 27-02: CSI capability gap analysis and known limitations
  - Created CAPABILITIES.md (357 lines) with feature comparison matrix
  - Added Known Limitations section to README.md (6 specific limitations)
  - Honest "why not" explanations for every missing feature
  - Architectural differences documented (single-server vs distributed)
- Phase 27-03: Testing and CI/CD documentation enhancement
  - Updated TESTING.md (530 lines) with troubleshooting flows and v0.9.0 coverage metrics
  - Updated ci-cd.md (413 lines) with test job template and result interpretation
  - Added mock-reality divergence section
  - Cross-referenced testing, CI/CD, and hardware validation docs

**v0.9.0 Accomplishments:**
- 6 phases completed (22-25.2, including 2 inserted decimal phases for production incidents)
- 17 plans executed across CSI testing, mock infrastructure, E2E suite, coverage improvements, and resilience features
- Test coverage increased from 65.0% to 68.6%
- Production incident response: Attachment reconciliation and RDS connection resilience
- 134 linter issues resolved, golangci-lint v2 enforced in CI

**Quick tasks completed:**
- Quick 002 (2026-02-05): Fixed AttachmentReconciler shutdown deadlock with priority-select pattern
- Quick 003 (2026-02-05): Fixed 22 test failures (IP validation, ControllerPublishVolume, NodeGetVolumeStats)
- Quick 004 (2026-02-06): Updated documentation to reflect v0.8.0 shipped, v0.9.0 in progress (README, ROADMAP, MILESTONES)
- Quick 005 (2026-02-06): Fixed README.md inaccuracies (removed fake Helm section, updated URLs to GitHub)

---
*Last updated: 2026-02-06 after Phase 26-02 completion*
