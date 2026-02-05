# Requirements: RDS CSI Driver v0.9.0

**Defined:** 2026-02-04
**Core Value:** Volumes remain accessible after NVMe-oF reconnections

## v0.9.0 Requirements

### CSI Compliance (COMP)

- [ ] **COMP-01**: CSI sanity test suite runs without failures in CI
- [ ] **COMP-02**: All controller service methods pass idempotency validation
- [ ] **COMP-03**: All node service methods pass idempotency validation
- [ ] **COMP-04**: CreateVolume/DeleteVolume idempotency with volume ID collisions tested
- [ ] **COMP-05**: Negative test cases validate proper error codes (ALREADY_EXISTS, NOT_FOUND, etc.)
- [ ] **COMP-06**: CSI spec compliance matrix documented showing implemented vs optional capabilities

### Mock Infrastructure (MOCK)

- [ ] **MOCK-01**: Mock RDS server supports all volume lifecycle commands (/disk add, /disk remove)
- [ ] **MOCK-02**: Mock RDS server supports capacity queries (/file print detail)
- [ ] **MOCK-03**: Mock server simulates realistic SSH latency (200ms average)
- [ ] **MOCK-04**: Mock server supports error injection (disk full, SSH timeout, command failures)
- [ ] **MOCK-05**: Mock server maintains stateful volume tracking across operations
- [ ] **MOCK-06**: Mock server returns RouterOS-formatted output matching production format
- [ ] **MOCK-07**: Mock server supports concurrent SSH connections for stress testing

### E2E Testing (E2E)

- [ ] **E2E-01**: Automated volume lifecycle test (create → stage → publish → unpublish → unstage → delete)
- [ ] **E2E-02**: Automated block volume lifecycle test with KubeVirt VirtualMachineInstance
- [ ] **E2E-03**: Volume expansion test validates filesystem resize
- [ ] **E2E-04**: Multi-volume test validates concurrent operations
- [ ] **E2E-05**: Cleanup test validates orphan detection and reconciliation
- [ ] **E2E-06**: Node failure simulation test validates volume unstaging on node deletion
- [ ] **E2E-07**: Controller restart test validates state rebuild from VolumeAttachment objects
- [ ] **E2E-08**: E2E tests run in CI against mock RDS without real hardware
- [ ] **E2E-09**: E2E test cleanup prevents orphaned resources between test runs

### Coverage & Quality (COV)

- [ ] **COV-01**: Error paths in controller service have test coverage
- [ ] **COV-02**: Error paths in node service have test coverage
- [ ] **COV-03**: Edge cases from sanity test failures have regression tests
- [ ] **COV-04**: Negative test scenarios validate error handling (invalid parameters, missing volumes)
- [ ] **COV-05**: Coverage enforcement in CI prevents regression below 65% baseline
- [ ] **COV-06**: Flaky tests identified and fixed or skipped with documented rationale

### Volume Snapshots (SNAP)

- [ ] **SNAP-01**: CreateSnapshot capability advertised in controller service
- [ ] **SNAP-02**: CreateSnapshot creates Btrfs snapshot via SSH/RouterOS
- [ ] **SNAP-03**: CreateSnapshot stores snapshot metadata in VolumeSnapshotContent
- [ ] **SNAP-04**: DeleteSnapshot removes Btrfs snapshot and cleans up metadata
- [ ] **SNAP-05**: ListSnapshots returns existing snapshots via RouterOS query
- [ ] **SNAP-06**: CreateVolume from snapshot (restore) creates new volume from Btrfs snapshot
- [ ] **SNAP-07**: Snapshot operations pass CSI sanity snapshot tests
- [ ] **SNAP-08**: external-snapshotter sidecar integrated in controller deployment
- [ ] **SNAP-09**: VolumeSnapshotClass StorageClass parameter support
- [ ] **SNAP-10**: Snapshot size and creation timestamp tracked in metadata

### Documentation & Validation (DOC)

- [ ] **DOC-01**: Manual test scenarios documented for hardware validation
- [ ] **DOC-02**: Testing guide for contributors (unit, integration, E2E, sanity)
- [ ] **DOC-03**: CSI capability gap analysis vs peer drivers documented
- [ ] **DOC-04**: Known limitations documented (RouterOS version compatibility, timing assumptions)
- [ ] **DOC-05**: CI/CD integration guide for test automation
- [ ] **DOC-06**: Troubleshooting guide for common test failures
- [ ] **DOC-07**: Snapshot usage guide with examples and best practices

## Future Requirements (v0.10.0+)

### Advanced Testing

- **CHAOS-01**: Chaos testing framework for network partitions and node failures
- **PERF-01**: Performance benchmarking suite for throughput and latency
- **STRESS-01**: Stress testing with 100+ concurrent volumes
- **LOAD-01**: Load testing for sustained I/O operations

### Security Hardening

- **SEC-01**: Remove SSH InsecureSkipVerify flag from production builds
- **SEC-02**: Command injection defense with fuzzing tests
- **SEC-03**: SSH key file permission runtime checks
- **SEC-04**: Mount option validation with malicious input tests

### Advanced Features

- **CLONE-01**: Volume cloning using Btrfs reflinks
- **TOPO-01**: Topology-aware scheduling for multi-site deployments
- **HA-01**: Controller high availability with leader election
- **ENC-01**: Volume encryption at rest

## Out of Scope

| Feature | Reason |
|---------|--------|
| NVMe multipath | Single RDS controller, not applicable |
| Real-time volume migration | Complex, low value for homelab use case |
| Automated stress testing in CI | Would hammer production lab hardware |
| Volume replication | Requires multiple RDS instances |
| Quota management | RDS/Btrfs level concern, not CSI driver |

## Traceability

**Coverage:**
- v0.9.0 requirements: 45 total
- Mapped to phases: 45/45 (100%)
- Unmapped: 0

| Requirement | Phase | Status |
|-------------|-------|--------|
| COMP-01 | Phase 22 | Pending |
| COMP-02 | Phase 22 | Pending |
| COMP-03 | Phase 22 | Pending |
| COMP-04 | Phase 22 | Pending |
| COMP-05 | Phase 22 | Pending |
| COMP-06 | Phase 22 | Pending |
| MOCK-01 | Phase 23 | Pending |
| MOCK-02 | Phase 23 | Pending |
| MOCK-03 | Phase 23 | Pending |
| MOCK-04 | Phase 23 | Pending |
| MOCK-05 | Phase 23 | Pending |
| MOCK-06 | Phase 23 | Pending |
| MOCK-07 | Phase 23 | Pending |
| E2E-01 | Phase 24 | Pending |
| E2E-02 | Phase 24 | Pending |
| E2E-03 | Phase 24 | Pending |
| E2E-04 | Phase 24 | Pending |
| E2E-05 | Phase 24 | Pending |
| E2E-06 | Phase 24 | Pending |
| E2E-07 | Phase 24 | Pending |
| E2E-08 | Phase 24 | Pending |
| E2E-09 | Phase 24 | Pending |
| COV-01 | Phase 25 | Pending |
| COV-02 | Phase 25 | Pending |
| COV-03 | Phase 25 | Pending |
| COV-04 | Phase 25 | Pending |
| COV-05 | Phase 25 | Pending |
| COV-06 | Phase 25 | Pending |
| SNAP-01 | Phase 26 | Pending |
| SNAP-02 | Phase 26 | Pending |
| SNAP-03 | Phase 26 | Pending |
| SNAP-04 | Phase 26 | Pending |
| SNAP-05 | Phase 26 | Pending |
| SNAP-06 | Phase 26 | Pending |
| SNAP-07 | Phase 26 | Pending |
| SNAP-08 | Phase 26 | Pending |
| SNAP-09 | Phase 26 | Pending |
| SNAP-10 | Phase 26 | Pending |
| DOC-01 | Phase 27 | Pending |
| DOC-02 | Phase 27 | Pending |
| DOC-03 | Phase 27 | Pending |
| DOC-04 | Phase 27 | Pending |
| DOC-05 | Phase 27 | Pending |
| DOC-06 | Phase 27 | Pending |
| DOC-07 | Phase 27 | Pending |

---
*Requirements defined: 2026-02-04*
*Last updated: 2026-02-04 after roadmap creation*
