# Roadmap: RDS CSI Driver

## Overview

This roadmap tracks the journey from initial driver implementation through production stability, block volume support, and systematic code quality improvements. Phases 1-16 delivered core features and reliability. Phases 17-21 focus on technical debt cleanup and maintainability.

## Milestones

- ✅ **v1 Production Stability** - Phases 1-4 (shipped 2026-01-31)
- ✅ **v0.3.0 Volume Fencing** - Phases 5-7 (shipped 2026-02-03)
- ✅ **v0.5.0 KubeVirt Live Migration** - Phases 8-10 (shipped 2026-02-03)
- ✅ **v0.6.0 Block Volume Support** - Phases 11-14 (shipped 2026-02-04)
- ✅ **v0.7.0 State Management & Observability** - Phases 15-16 (shipped 2026-02-04)
- 🚧 **v0.7.1 Code Quality and Logging Cleanup** - Phases 17-21 (in progress)

## Phases

<details>
<summary>✅ v1 Production Stability (Phases 1-4) - SHIPPED 2026-01-31</summary>

### Phase 1: Device Path Resolution
**Goal**: NVMe device paths resolved by NQN, not hardcoded paths
**Plans**: 5 plans
Plans:
- [x] 01-01: NQN-based device resolution
- [x] 01-02: Sysfs scanning implementation
- [x] 01-03: Device path caching
- [x] 01-04: Unit tests
- [x] 01-05: Integration tests

### Phase 2: Stale Mount Detection
**Goal**: Detect and recover from stale mounts automatically
**Plans**: 4 plans
Plans:
- [x] 02-01: Mount table parsing
- [x] 02-02: Stale mount detection
- [x] 02-03: Automatic recovery
- [x] 02-04: Unit tests

### Phase 3: Kernel Reconnection Parameters
**Goal**: Configure NVMe-oF reconnection behavior
**Plans**: 4 plans
Plans:
- [x] 03-01: StorageClass parameter parsing
- [x] 03-02: Connection parameter setting
- [x] 03-03: Default values
- [x] 03-04: Unit tests

### Phase 4: Observability
**Goal**: Prometheus metrics and health reporting
**Plans**: 4 plans
Plans:
- [x] 04-01: Prometheus metrics endpoint
- [x] 04-02: CSI operation metrics
- [x] 04-03: VolumeCondition health reporting
- [x] 04-04: Kubernetes events

</details>

<details>
<summary>✅ v0.3.0 Volume Fencing (Phases 5-7) - SHIPPED 2026-02-03</summary>

### Phase 5: Attachment Manager
**Goal**: In-memory attachment tracking with persistence
**Plans**: 5 plans
Plans:
- [x] 05-01: AttachmentManager structure
- [x] 05-02: PV annotation persistence
- [x] 05-03: State rebuild on startup
- [x] 05-04: Unit tests
- [x] 05-05: Integration tests

### Phase 6: Controller Publish/Unpublish
**Goal**: Enforce ReadWriteOnce semantics
**Plans**: 4 plans
Plans:
- [x] 06-01: ControllerPublishVolume implementation
- [x] 06-02: ControllerUnpublishVolume implementation
- [x] 06-03: Conflict detection
- [x] 06-04: Unit tests

### Phase 7: Stale Attachment Cleanup
**Goal**: Reconcile stale attachments from deleted nodes
**Plans**: 3 plans
Plans:
- [x] 07-01: Background reconciler
- [x] 07-02: Grace period logic
- [x] 07-03: Unit tests

</details>

<details>
<summary>✅ v0.5.0 KubeVirt Live Migration (Phases 8-10) - SHIPPED 2026-02-03</summary>

### Phase 8: ReadWriteMany Support
**Goal**: RWX access mode for block volumes
**Plans**: 4 plans
Plans:
- [x] 08-01: RWX capability declaration
- [x] 08-02: Access mode validation
- [x] 08-03: Block-only enforcement
- [x] 08-04: Unit tests

### Phase 9: Migration Window Handling
**Goal**: 2-node attachment during migration
**Plans**: 4 plans
Plans:
- [x] 09-01: Dual-attach detection
- [x] 09-02: Migration timeout
- [x] 09-03: Kubernetes events
- [x] 09-04: Unit tests

### Phase 10: Migration Metrics
**Goal**: Observability for migration operations
**Plans**: 4 plans
Plans:
- [x] 10-01: Migration counter metrics
- [x] 10-02: Duration histogram
- [x] 10-03: Active migration gauge
- [x] 10-04: Unit tests

</details>

<details>
<summary>✅ v0.6.0 Block Volume Support (Phases 11-14) - SHIPPED 2026-02-04</summary>

### Phase 11: Block Volume Lifecycle
**Goal**: CSI block volume support without formatting
**Plans**: 3 plans
Plans:
- [x] 11-01: NodeStageVolume block handling
- [x] 11-02: NodePublishVolume mknod implementation
- [x] 11-03: NodeUnstageVolume block cleanup

### Phase 12: KubeVirt Validation
**Goal**: VM boot and migration on metal cluster
**Plans**: 2 plans
Plans:
- [x] 12-01: VM boot validation
- [x] 12-02: Live migration validation

### Phase 13: Critical Bug Fixes
**Goal**: Mount storm and stale state fixes
**Plans**: 2 plans
Plans:
- [x] 13-01: Mknod vs bind mount fix
- [x] 13-02: Clear annotations on detach

### Phase 14: Mount Storm Prevention
**Goal**: Detect and prevent mount storms
**Plans**: 4 plans
Plans:
- [x] 14-01: NQN prefix filtering
- [x] 14-02: Duplicate mount detection
- [x] 14-03: Circuit breaker and health checks
- [x] 14-04: Graceful shutdown

</details>

<details>
<summary>✅ v0.7.0 State Management & Observability (Phases 15-16) - SHIPPED 2026-02-04</summary>

### Phase 15: VolumeAttachment-Based State Rebuild
**Goal**: Use VolumeAttachment objects as source of truth
**Plans**: 4 plans
Plans:
- [x] 15-01: VolumeAttachment listing helpers
- [x] 15-02: Rebuild from VolumeAttachments
- [x] 15-03: PV annotations informational-only
- [x] 15-04: Comprehensive restart tests

### Phase 16: Migration Metrics Emission
**Goal**: Wire migration metrics for observability
**Plans**: 1 plan
Plans:
- [x] 16-01: Wire AttachmentManager.SetMetrics()

</details>

## 🚧 v0.7.1 Code Quality and Logging Cleanup (In Progress)

**Milestone Goal:** Systematic codebase cleanup to improve maintainability, reduce log noise, and eliminate technical debt

**Status:** Phase 17 starting

---

### Phase 17: Test Infrastructure Fix

**Goal**: Fix failing block volume tests to establish stable test baseline

**Depends on**: Phase 16 (previous milestone)

**Requirements**: TEST-01

**Success Criteria** (what must be TRUE):
1. Block volume test suite runs without nil pointer dereferences
2. All existing tests pass consistently in CI
3. Test infrastructure supports adding new block volume tests
4. Root cause of nil pointer issue documented

**Plans**: TBD

Plans:
- [ ] 17-01: TBD during planning

---

### Phase 18: Logging Cleanup

**Goal**: Reduce production log noise through systematic verbosity rationalization

**Depends on**: Phase 17

**Requirements**: LOG-01, LOG-02, LOG-03, LOG-04

**Success Criteria** (what must be TRUE):
1. Security logger consolidated from 300+ lines to <50 lines with configurable helper
2. DeleteVolume operation produces maximum 2 log statements per operation (down from 4-6)
3. All CSI operations audited with info=actionable, debug=diagnostic separation documented
4. Severity mapping uses table-driven approach instead of switch statements
5. Production logs contain only actionable information at info level

**Plans**: TBD

Plans:
- [ ] 18-01: TBD during planning

---

### Phase 19: Error Handling Standardization

**Goal**: Consistent error patterns with proper context propagation across all packages

**Depends on**: Phase 18

**Requirements**: ERR-01, ERR-02, ERR-03, ERR-04

**Success Criteria** (what must be TRUE):
1. All 160+ error returns using %v converted to %w for proper error wrapping
2. Every error includes contextual information (operation, volume ID, node, reason)
3. Error handling patterns documented in CONVENTIONS.md
4. Error paths audited with no silent failures or missing context

**Plans**: TBD

Plans:
- [ ] 19-01: TBD during planning

---

### Phase 20: Test Coverage Expansion

**Goal**: Increase test coverage to >80% on all critical packages

**Depends on**: Phase 19

**Requirements**: TEST-02, TEST-03, TEST-04, TEST-05, TEST-06

**Success Criteria** (what must be TRUE):
1. SSH client test coverage increased from 0% to >80%
2. RDS package test coverage increased from 44.5% to >80%
3. Mount package test coverage increased from 55.9% to >80%
4. NVMe package test coverage increased from 43.3% to >80%
5. Zero-coverage files now have comprehensive tests (ssh_client.go, server.go, persist.go, client.go)
6. Critical error paths have explicit test coverage

**Plans**: TBD

Plans:
- [ ] 20-01: TBD during planning

---

### Phase 21: Code Quality Improvements

**Goal**: Extract common patterns and resolve documented code smells

**Depends on**: Phase 20

**Requirements**: QUAL-01, QUAL-02, QUAL-03, QUAL-04

**Success Criteria** (what must be TRUE):
1. Common error handling patterns extracted into pkg/utils/errors.go
2. Duplicated severity mapping switch statements replaced with shared table
3. Large packages refactored for better separation of concerns
4. All code smells from CONCERNS.md either resolved or explicitly documented as deferred
5. Codebase maintainability score improved (measured by golangci-lint complexity metrics)

**Plans**: TBD

Plans:
- [ ] 21-01: TBD during planning

---

## Progress

**Execution Order:**
Phases execute in numeric order: 17 → 18 → 19 → 20 → 21

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Device Path Resolution | v1 | 5/5 | Complete | 2026-01-31 |
| 2. Stale Mount Detection | v1 | 4/4 | Complete | 2026-01-31 |
| 3. Kernel Reconnection Parameters | v1 | 4/4 | Complete | 2026-01-31 |
| 4. Observability | v1 | 4/4 | Complete | 2026-01-31 |
| 5. Attachment Manager | v0.3.0 | 5/5 | Complete | 2026-02-03 |
| 6. Controller Publish/Unpublish | v0.3.0 | 4/4 | Complete | 2026-02-03 |
| 7. Stale Attachment Cleanup | v0.3.0 | 3/3 | Complete | 2026-02-03 |
| 8. ReadWriteMany Support | v0.5.0 | 4/4 | Complete | 2026-02-03 |
| 9. Migration Window Handling | v0.5.0 | 4/4 | Complete | 2026-02-03 |
| 10. Migration Metrics | v0.5.0 | 4/4 | Complete | 2026-02-03 |
| 11. Block Volume Lifecycle | v0.6.0 | 3/3 | Complete | 2026-02-04 |
| 12. KubeVirt Validation | v0.6.0 | 2/2 | Complete | 2026-02-04 |
| 13. Critical Bug Fixes | v0.6.0 | 2/2 | Complete | 2026-02-04 |
| 14. Mount Storm Prevention | v0.6.0 | 4/4 | Complete | 2026-02-04 |
| 15. VolumeAttachment-Based State Rebuild | v0.7.0 | 4/4 | Complete | 2026-02-04 |
| 16. Migration Metrics Emission | v0.7.0 | 1/1 | Complete | 2026-02-04 |
| 17. Test Infrastructure Fix | v0.7.1 | 0/? | Not started | - |
| 18. Logging Cleanup | v0.7.1 | 0/? | Not started | - |
| 19. Error Handling Standardization | v0.7.1 | 0/? | Not started | - |
| 20. Test Coverage Expansion | v0.7.1 | 0/? | Not started | - |
| 21. Code Quality Improvements | v0.7.1 | 0/? | Not started | - |

---

_Last updated: 2026-02-04 (v0.7.1 roadmap created)_
