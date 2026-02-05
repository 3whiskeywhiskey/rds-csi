# Roadmap: RDS CSI Driver

## Milestones

- ✅ **v0.1.0 Foundation** - Phases 1-3 (shipped 2024-03-15)
- ✅ **v0.2.0 Controller Service** - Phases 4-5 (shipped 2024-04-01)
- ✅ **v0.3.0 Node Service** - Phases 6-8 (shipped 2024-05-15)
- ✅ **v0.4.0 Production Hardening** - Phases 9-11 (shipped 2024-06-30)
- ✅ **v0.5.0 NVMe-oF Reconnection** - Phases 12-14 (shipped 2025-01-15)
- ✅ **v0.6.0 Block Volumes & KubeVirt** - Phases 15-16 (shipped 2025-11-15)
- ✅ **v0.7.0 Migration Tracking** - Phases 17-19 (shipped 2026-01-20)
- ✅ **v0.8.0 Code Quality & Logging** - Phases 20-21 (shipped 2026-02-04)
- 🚧 **v0.9.0 Production Readiness & Test Maturity** - Phases 22-27 (in progress)

## Phases

<details>
<summary>✅ v0.1.0 Foundation (Phases 1-3) - SHIPPED 2024-03-15</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.2.0 Controller Service (Phases 4-5) - SHIPPED 2024-04-01</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.3.0 Node Service (Phases 6-8) - SHIPPED 2024-05-15</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.4.0 Production Hardening (Phases 9-11) - SHIPPED 2024-06-30</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.5.0 NVMe-oF Reconnection (Phases 12-14) - SHIPPED 2025-01-15</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.6.0 Block Volumes & KubeVirt (Phases 15-16) - SHIPPED 2025-11-15</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.7.0 Migration Tracking (Phases 17-19) - SHIPPED 2026-01-20</summary>

[Historical phases collapsed for brevity]

</details>

<details>
<summary>✅ v0.8.0 Code Quality & Logging (Phases 20-21) - SHIPPED 2026-02-04</summary>

[Historical phases collapsed for brevity]

</details>

### 🚧 v0.9.0 Production Readiness & Test Maturity (In Progress)

**Milestone Goal:** Validate CSI spec compliance and build production-ready testing infrastructure enabling confident v1.0 release

#### Phase 22: CSI Sanity Tests Integration
**Goal**: CSI spec compliance validated through automated sanity testing in CI
**Depends on**: Nothing (first phase)
**Requirements**: COMP-01, COMP-02, COMP-03, COMP-04, COMP-05, COMP-06
**Success Criteria** (what must be TRUE):
  1. csi-sanity test suite runs in CI without failures for Identity and Controller services
  2. All controller service methods pass idempotency validation (CreateVolume/DeleteVolume called multiple times with same parameters return correct responses)
  3. Negative test cases validate proper CSI error codes (ALREADY_EXISTS, NOT_FOUND, INVALID_ARGUMENT, RESOURCE_EXHAUSTED)
  4. CSI capability matrix is documented showing which optional capabilities are implemented vs deferred
  5. Sanity test results are published in CI artifacts for traceability
**Plans**: 2 plans

Plans:
- [x] 22-01-PLAN.md — Go-based sanity test infrastructure with mock RDS
- [x] 22-02-PLAN.md — CI integration and capability documentation

#### Phase 23: Mock RDS Enhancements
**Goal**: Mock RDS server matches real hardware behavior enabling reliable CI testing
**Depends on**: Phase 22 (sanity tests identify mock gaps)
**Requirements**: MOCK-01, MOCK-02, MOCK-03, MOCK-04, MOCK-05, MOCK-06, MOCK-07
**Success Criteria** (what must be TRUE):
  1. Mock RDS server handles all volume lifecycle commands used by driver (/disk add, /disk remove, /file print detail)
  2. Mock server simulates realistic SSH latency (200ms average, 150-250ms range) exposing timeout bugs
  3. Mock server supports error injection for disk full scenarios, SSH timeout failures, and command parsing errors
  4. Mock server maintains stateful volume tracking so sequential operations (create, create same ID, delete, delete same ID) behave correctly
  5. Mock server returns RouterOS-formatted output matching production RDS (version 7.16) for parser validation
  6. Mock server handles concurrent SSH connections for stress testing without corrupting state
**Plans**: 2 plans

Plans:
- [x] 23-01-PLAN.md — Configuration system, timing simulation, and error injection infrastructure
- [x] 23-02-PLAN.md — Concurrent stress tests and documentation

#### Phase 24: Automated E2E Test Suite
**Goal**: Kubernetes integration validated through automated E2E tests running in CI
**Depends on**: Phase 23 (enhanced mock enables reliable E2E CI)
**Requirements**: E2E-01, E2E-02, E2E-03, E2E-04, E2E-05, E2E-06, E2E-07, E2E-08, E2E-09
**Success Criteria** (what must be TRUE):
  1. Automated volume lifecycle test completes full flow (create PVC, stage volume, publish to pod, write data, unpublish, unstage, delete PVC) in CI
  2. Block volume test with KubeVirt VirtualMachineInstance validates VM can boot and access RDS block storage
  3. Volume expansion test validates filesystem resizes correctly after PVC expansion request
  4. Multi-volume test validates driver handles 5+ concurrent volume operations without conflicts
  5. Cleanup test validates orphan detection finds and reconciles unused volumes on RDS
  6. Node failure simulation test validates volumes unstage cleanly when node is deleted from cluster
  7. Controller restart test validates driver rebuilds state from VolumeAttachment objects after pod restart
  8. E2E test cleanup prevents orphaned resources between runs (unique volume ID prefix per test run)
**Plans**: 4 plans

Plans:
- [x] 24-01-PLAN.md — E2E suite infrastructure with Ginkgo v2 and in-process driver
- [x] 24-02-PLAN.md — Core volume tests (lifecycle, block volume, expansion)
- [x] 24-03-PLAN.md — Advanced tests (concurrent operations, orphan detection)
- [x] 24-04-PLAN.md — State recovery tests and CI integration

#### Phase 25: Coverage & Quality Improvements
**Goal**: Test coverage increased to 70% with critical error paths validated
**Depends on**: Phase 24 (E2E tests reveal coverage gaps)
**Requirements**: COV-01, COV-02, COV-03, COV-04, COV-05, COV-06
**Success Criteria** (what must be TRUE):
  1. Error paths in controller service have test coverage for SSH failures, disk full scenarios, and invalid parameters
  2. Error paths in node service have test coverage for NVMe connection failures, mount failures, and device path changes
  3. Edge cases from sanity test failures have regression tests preventing reoccurrence
  4. Negative test scenarios validate error handling returns correct CSI error codes for invalid volume IDs, missing volumes, and duplicate operations
  5. Coverage enforcement in CI fails builds below 65% baseline preventing regression
  6. Flaky tests are identified and either fixed with proper synchronization or skipped with documented rationale in TESTING.md
**Plans**: 4 plans

Plans:
- [ ] 25-01-PLAN.md — Controller and RDS client error path tests
- [ ] 25-02-PLAN.md — Node service and mount error path tests
- [ ] 25-03-PLAN.md — CSI negative test scenarios and sanity regression tests
- [ ] 25-04-PLAN.md — CI threshold update, flaky test detection, TESTING.md documentation

#### Phase 26: Volume Snapshots
**Goal**: Btrfs-based volume snapshots enable backup and restore workflows
**Depends on**: Phase 25 (quality foundation for snapshot feature)
**Requirements**: SNAP-01, SNAP-02, SNAP-03, SNAP-04, SNAP-05, SNAP-06, SNAP-07, SNAP-08, SNAP-09, SNAP-10
**Success Criteria** (what must be TRUE):
  1. CreateSnapshot capability is advertised in controller service GetControllerCapabilities
  2. CreateSnapshot creates Btrfs snapshot via SSH RouterOS command and returns snapshot ID to Kubernetes
  3. Snapshot metadata (size, creation timestamp, source volume ID) is stored in VolumeSnapshotContent annotations
  4. DeleteSnapshot removes Btrfs snapshot from RDS and cleans up Kubernetes metadata
  5. ListSnapshots returns existing snapshots via RouterOS query for snapshot enumeration
  6. CreateVolume from snapshot (restore) creates new volume from Btrfs snapshot enabling backup restore workflow
  7. Snapshot operations pass CSI sanity snapshot tests validating spec compliance
  8. external-snapshotter sidecar v8.0+ is integrated in controller deployment manifest
  9. VolumeSnapshotClass supports StorageClass parameter for snapshot configuration
**Plans**: TBD

Plans:
- [ ] 26-01: TBD
- [ ] 26-02: TBD
- [ ] 26-03: TBD

#### Phase 27: Documentation & Hardware Validation
**Goal**: Manual validation scenarios documented and hardware testing plan established
**Depends on**: Phase 26 (all features complete)
**Requirements**: DOC-01, DOC-02, DOC-03, DOC-04, DOC-05, DOC-06, DOC-07
**Success Criteria** (what must be TRUE):
  1. Manual test scenarios are documented in HARDWARE_VALIDATION.md with step-by-step instructions for production RDS testing
  2. Testing guide for contributors (TESTING.md) documents how to run unit tests, integration tests, E2E tests, and sanity tests
  3. CSI capability gap analysis vs peer drivers (AWS EBS CSI, Longhorn) is documented in CAPABILITIES.md
  4. Known limitations are documented in README.md (RouterOS version compatibility, NVMe device timing assumptions, dual-IP architecture requirements)
  5. CI/CD integration guide documents how to add new test jobs and interpret test results
  6. Troubleshooting guide in TESTING.md covers common test failures with solutions (mock-reality divergence, device timing, cleanup issues)
  7. Snapshot usage guide with VolumeSnapshot/VolumeSnapshotContent examples is added to docs/snapshots.md
**Plans**: TBD

Plans:
- [ ] 27-01: TBD
- [ ] 27-02: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 22 → 23 → 24 → 25 → 26 → 27

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-21 | v0.1.0-v0.8.0 | Complete | Complete | 2026-02-04 |
| 22. CSI Sanity Tests Integration | v0.9.0 | 2/2 | ✅ Complete | 2026-02-04 |
| 23. Mock RDS Enhancements | v0.9.0 | 2/2 | ✅ Complete | 2026-02-04 |
| 24. Automated E2E Test Suite | v0.9.0 | 4/4 | ✅ Complete | 2026-02-05 |
| 25. Coverage & Quality Improvements | v0.9.0 | 0/4 | Not started | - |
| 26. Volume Snapshots | v0.9.0 | 0/TBD | Not started | - |
| 27. Documentation & Hardware Validation | v0.9.0 | 0/TBD | Not started | - |

---
*Last updated: 2026-02-05 after Phase 25 planning*
