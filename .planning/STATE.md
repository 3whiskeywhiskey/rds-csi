# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-06)

**Core value:** Volumes remain accessible after NVMe-oF reconnections
**Current focus:** v0.10.0 Feature Enhancements

## Current Position

Phase: 28.1 of 28 (Fix rds_csi_nvme_connections_active Metric Accuracy)
Plan: 1 of 1 (Phase complete!)
Status: Phase 28.1 complete - GaugeFunc-based metric querying AttachmentManager state
Last activity: 2026-02-06 — Completed 28.1-01-PLAN.md (Replace counter-derived gauge with GaugeFunc)

Progress: v0.9.0 [██████████] 100% (17/17 plans) | v0.10.0 [███████████] 52.6% (10/19 plans)

## Performance Metrics

**Velocity:**
- Total plans completed: 106 (79 v0.1.0-v0.8.0 + 17 v0.9.0 + 10 v0.10.0)
- v0.9.0 plans completed: 17/17 (100%)
- v0.10.0 plans completed: 10/19 (52.6%)
- Average duration: ~7 min per plan (v0.9.0), ~7 min per plan (v0.10.0 so far)
- Total execution time: ~2 hours (v0.9.0 execution, 92 days calendar)

**By Milestone:**

| Milestone | Phases | Plans | Status |
|-----------|--------|-------|--------|
| v0.1.0-v0.8.0 | 1-21 | 79/79 | ✅ Shipped 2026-02-04 |
| v0.9.0 Production Readiness | 22-25.2 | 17/17 | ✅ Shipped 2026-02-06 |
| v0.10.0 Feature Enhancements | 26-28 | 9/18 | 🚧 In Progress |

**Recent Milestones:**
- v0.10.0: 4 phases (26-28.1), 10/19 plans, in progress (Phase 26 complete, Phase 27 complete, Phase 28.1 complete)
- v0.9.0: 6 phases (22-25.2), 17 plans, 92 days, shipped 2026-02-06
- v0.8.0: 5 phases (17-21), 20 plans, 1 day, shipped 2026-02-04

*Updated: 2026-02-06*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- v0.10.0 (Phase 28.1-01): Use func() int callback instead of AttachmentCounter interface (simpler, avoids import cycle)
- v0.10.0 (Phase 28.1-01): SetAttachmentManager registers GaugeFunc dynamically (metric only appears in controller)
- v0.10.0 (Phase 28.1-01): Metric counts volumes not per-node attachments (dual-attach during migration = 1 not 2)
- v0.10.0 (Phase 28.1-01): RecordNVMeDisconnect retained with empty body (API compatibility)
- v0.10.0 (Phase 26-06): Mock RDS server outputs source-volume field for testing (real RouterOS doesn't)
- v0.10.0 (Phase 26-06): parseSnapshotInfo extracts source-volume opportunistically (testing compatibility)
- v0.10.0 (Phase 26-06): CreateSnapshot populates SourceVolume from opts if backend doesn't provide it
- v0.10.0 (Phase 26-06): Mock list output includes entry numbers for RouterOS format parsing
- v0.10.0 (Phase 26-05): csi-snapshotter v8.2.0 selected (not v8.4.0) - avoids unnecessary VolumeGroupSnapshot v1beta2 features
- v0.10.0 (Phase 26-05): VolumeSnapshotClass uses deletionPolicy: Delete (matches StorageClass deletion policy pattern)
- v0.10.0 (Phase 26-05): Installation prerequisites documented in VolumeSnapshotClass comments (CRD and snapshot-controller required)
- v0.10.0 (Phase 26-04): ListSnapshots uses integer-based pagination tokens (CSI spec pattern, matching hostpath driver)
- v0.10.0 (Phase 26-04): ListSnapshots returns empty response (not error) for invalid/missing snapshot ID (CSI spec)
- v0.10.0 (Phase 26-04): CreateVolume from snapshot enforces minimum size >= snapshot size (CSI spec requirement)
- v0.10.0 (Phase 26-04): ContentSource included in CreateVolume response for Kubernetes tracking
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

- **Phase 28.1 inserted after Phase 27**: Fix rds_csi_nvme_connections_active Metric Accuracy (URGENT)
  - **Trigger**: GitHub Issue #19 - Production observability bug discovered during v0.9.0 monitoring
  - **Impact**: Metric reports 0 instead of actual connection count (16 volumes attached), making monitoring dashboards, alerting, and debugging unreliable
  - **Root Cause**: Metric derived from attach/detach counters instead of querying attachment manager state; counters reset on restart while attachments persist
  - **Scope**: Fix gauge to query current attachment manager state, add unit/integration tests, validate metric accuracy
  - **Priority**: Must fix before Helm chart release - users deploying via Helm will rely on accurate metrics for production monitoring

### Pending Todos

None yet. (Use `/gsd:add-todo` to capture ideas during execution)

### Blockers/Concerns

None. All pre-existing test failures resolved via Quick-003.

## Session Continuity

Last session: 2026-02-06
Stopped at: Phase 28.1 complete (metric accuracy fix)
Resume file: None
Next action: Continue with Phase 28 (Helm chart) - metric fix unblocks Helm release.

**v0.10.0 Progress (10/19 plans):**
- Phase 28.1-01: Fix rds_csi_nvme_connections_active metric accuracy (GitHub Issue #19)
  - Replaced counter-derived gauge (Inc/Dec pattern) with GaugeFunc querying AttachmentManager state
  - Metric now survives controller restarts (derived from persistent attachment state)
  - Added SetAttachmentManager wiring in driver.go with func() int callback (no import cycle)
  - Updated 4 existing tests, added 3 new tests (QueriesAttachmentManager, SurvivesRestart, DynamicUpdates)
  - All 7 NVMe tests pass, metric correctly reports 16 connections after restart
  - Production observability bug resolved, ready for Helm chart release
- Phase 26-06: Snapshot testing (CSI sanity tests + controller unit tests)
  - Configured CSI sanity tests with TestSnapshotParameters (snapshot test suite enabled)
  - Extended mock RDS server with Btrfs snapshot command handlers (create/delete/list/get)
  - Added MockSnapshot struct tracking source volume, parent, fs-label, read-only, size
  - Updated parseSnapshotInfo to extract source-volume field opportunistically
  - Added 32 unit test cases: CreateSnapshot(6), DeleteSnapshot(3), ListSnapshots(9), CreateVolumeFromSnapshot(3)
  - All snapshot sanity tests passing (65/70 total, 5 pre-existing Node Service failures)
- Phase 26-05: RBAC and deployment manifests for snapshot support
  - Added snapshot.storage.k8s.io RBAC rules to controller ClusterRole
  - Added csi-snapshotter v8.2.0 sidecar container to controller deployment
  - Created VolumeSnapshotClass with driver=rds.csi.srvlab.io and deletionPolicy=Delete
  - Installation prerequisites documented (VolumeSnapshot CRDs and snapshot-controller)
  - All YAML manifests validated with kubectl dry-run
- Phase 26-04: ListSnapshots with pagination and CreateVolume snapshot restore
  - Implemented ListSnapshots with CSI-compliant integer-based pagination
  - Single snapshot lookup, source volume filtering, deterministic sorting
  - CreateVolume detects VolumeContentSource and routes to snapshot restore
  - createVolumeFromSnapshot validates snapshot exists, enforces minimum size
  - ContentSource included in response for Kubernetes tracking
  - Reject volume cloning with actionable error (not yet supported)
  - Updated tests to reflect ListSnapshots now implemented
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
