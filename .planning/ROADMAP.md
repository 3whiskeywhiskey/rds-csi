# Roadmap: RDS CSI Driver

## Milestones

- ✅ **v0.1.0 Foundation** - Phases 1-3 (shipped 2024-03-15)
- ✅ **v0.2.0 Controller Service** - Phases 4-5 (shipped 2024-04-01)
- ✅ **v0.3.0 Volume Fencing** - Phases 6-8 (shipped 2026-02-03)
- ✅ **v0.4.0 Production Hardening** - Phases 9-11 (shipped 2024-06-30)
- ✅ **v0.5.0 NVMe-oF Reconnection** - Phases 12-14 (shipped 2025-01-15)
- ✅ **v0.6.0 Block Volumes & KubeVirt** - Phases 15-16 (shipped 2025-11-15)
- ✅ **v0.7.0 Migration Tracking** - Phases 17-19 (shipped 2026-01-20)
- ✅ **v0.8.0 Code Quality & Logging** - Phases 20-21 (shipped 2026-02-04)
- ✅ **v0.9.0 Production Readiness & Test Maturity** - Phases 22-25.2 (shipped 2026-02-06)
- 🚧 **v0.10.0 Feature Enhancements** - Phases 26-28 (planned)

## Phases

<details>
<summary>✅ v0.1.0-v0.9.0 (Phases 1-25.2) - SHIPPED 2026-02-06</summary>

See milestone archives in `.planning/milestones/` for complete phase details:
- `.planning/milestones/v0.1.0-ROADMAP.md`
- `.planning/milestones/v0.2.0-ROADMAP.md`
- `.planning/milestones/v0.3.0-ROADMAP.md`
- `.planning/milestones/v0.4.0-ROADMAP.md`
- `.planning/milestones/v0.5.0-ROADMAP.md`
- `.planning/milestones/v0.6.0-ROADMAP.md`
- `.planning/milestones/v0.7.0-ROADMAP.md`
- `.planning/milestones/v0.8.0-ROADMAP.md`
- `.planning/milestones/v0.9.0-ROADMAP.md`

</details>

### 🚧 v0.10.0 Feature Enhancements (Planned)

**Milestone Goal:** Add volume snapshots, comprehensive documentation, and Helm chart for easier deployment

#### Phase 26: Volume Snapshots
**Goal**: Btrfs-based volume snapshots enable backup and restore workflows
**Depends on**: Phase 25.2 (all v0.9.0 work complete)
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
**Plans**: 6 plans

Plans:
- [x] 26-01-PLAN.md -- Snapshot foundation (types, ID utils, interface extension, mock support)
- [x] 26-02-PLAN.md -- RDS SSH snapshot commands (Btrfs subvolume operations + unit tests)
- [x] 26-03-PLAN.md -- Controller RPCs: CreateSnapshot, DeleteSnapshot + capability registration
- [x] 26-04-PLAN.md -- Controller RPCs: ListSnapshots pagination + CreateVolume from snapshot restore
- [x] 26-05-PLAN.md -- Deployment manifests (RBAC, csi-snapshotter sidecar, VolumeSnapshotClass)
- [x] 26-06-PLAN.md -- Testing (CSI sanity snapshot config + controller unit tests)

#### Phase 27: Documentation & Hardware Validation
**Goal**: Comprehensive documentation for hardware validation, testing, capabilities, and troubleshooting -- enabling operators to validate v0.9.0 against production hardware
**Depends on**: Phase 25.2 (v0.9.0 complete; Phase 26 NOT required -- DOC-07 deferred until snapshots ship)
**Requirements**: DOC-01, DOC-02, DOC-03, DOC-04, DOC-05, DOC-06 (DOC-07 deferred to post-Phase 26)
**Success Criteria** (what must be TRUE):
  1. Manual test scenarios are documented in HARDWARE_VALIDATION.md with step-by-step instructions for production RDS testing
  2. Testing guide for contributors (TESTING.md) documents how to run unit tests, integration tests, E2E tests, and sanity tests
  3. CSI capability gap analysis vs peer drivers (AWS EBS CSI, Longhorn) is documented in CAPABILITIES.md
  4. Known limitations are documented in README.md (RouterOS version compatibility, NVMe device timing assumptions, dual-IP architecture requirements)
  5. CI/CD integration guide documents how to add new test jobs and interpret test results
  6. Troubleshooting guide in TESTING.md covers common test failures with solutions (mock-reality divergence, device timing, cleanup issues)
  7. ~~Snapshot usage guide~~ DEFERRED: Blocked on Phase 26 (snapshots not yet implemented). Will be added as part of Phase 26 delivery or as a follow-up quick task.
**Plans**: 3 plans

Plans:
- [x] 27-01-PLAN.md -- Hardware validation guide (HARDWARE_VALIDATION.md) with 7 test cases
- [x] 27-02-PLAN.md -- Capabilities gap analysis (CAPABILITIES.md) + README.md known limitations
- [x] 27-03-PLAN.md -- Testing guide updates (TESTING.md) + CI/CD integration guide (ci-cd.md)

#### Phase 28.1: Fix rds_csi_nvme_connections_active Metric Accuracy (INSERTED)
**Goal**: Fix production observability bug where rds_csi_nvme_connections_active metric incorrectly reports 0 instead of actual connection count
**Depends on**: Phase 27 (documentation complete)
**GitHub Issue**: #19
**Requirements**: METRIC-01
**Success Criteria** (what must be TRUE):
  1. rds_csi_nvme_connections_active gauge reports actual count of active volumes with NVMe/TCP connections from attachment manager state (counts volumes, not per-node attachments during migration dual-attach)
  2. Metric value persists correctly across controller restarts (not derived from ephemeral counters)
  3. Metric accurately reflects VolumeAttachment count in cluster (validates via kubectl comparison)
  4. Unit tests verify metric updates on attach/detach operations
  5. Unit test validates metric accuracy after controller restart (TestNVMeConnectionsActive_SurvivesRestart simulates restart scenario)
**Plans**: 1 plan

Plans:
- [x] 28.1-01-PLAN.md -- Replace counter-derived gauge with GaugeFunc querying AttachmentManager + unit tests

**Root Cause**: Metric derived from attach/detach counters (attach_total - detach_total) instead of querying attachment manager current state. Counters reset on restart while attachments persist.

**Impact**: Unreliable monitoring dashboards, alerting, and debugging. Production cluster shows 16 active volumes but metric reports 0.

#### Phase 28.2: RDS Health & Performance Monitoring (INSERTED)
**Goal**: Implement RDS storage health monitoring via SSH polling of /disk monitor-traffic, exposing IOPS, throughput, latency, and queue depth as Prometheus metrics
**Depends on**: Phase 28.1 (metric accuracy fixed, GaugeFunc pattern established)
**Requirements**: MON-01, MON-02, MON-03
**Success Criteria** (what must be TRUE):
  1. RouterOS /disk monitor-traffic command capabilities documented (IOPS, throughput, latency, queue depth metrics available)
  2. SNMP monitoring capabilities researched (OIDs for disk health, available MIBs documented)
  3. RouterOS API REST/socket capabilities investigated (alternative to SSH polling for real-time metrics)
  4. Polling approach recommendations documented (frequency, performance impact, metric selection)
  5. Metric naming conventions defined (rds_disk_* namespace, labels for slot/operation type)
  6. Implementation complexity assessed (simple SSH polling vs API integration vs SNMP agent)
  7. Production impact analysis (CPU/memory overhead of polling, SSH connection limits)
**Plans**: 2 plans

Plans:
- [x] 28.2-01-PLAN.md -- RDS disk metrics SSH command, parsing, types, mock, and unit tests
- [x] 28.2-02-PLAN.md -- Prometheus GaugeFunc metric registration, driver wiring, and monitoring design doc

**Discovery**: User found `/disk monitor-traffic <slot>` command exposes:
- read-ops-per-second, write-ops-per-second (IOPS)
- read-rate, write-rate (throughput in bps)
- read-time, write-time, wait-time (latency indicators)
- in-flight-ops (queue depth)
- active-time (disk utilization)

**Approach**: SSH polling with GaugeFunc collectors (research concluded SSH > SNMP > API for this use case).

#### Phase 28: Helm Chart
**Goal**: Helm chart enables one-command deployment of the RDS CSI driver with configurable values for RDS connection, storage classes, monitoring, and all component settings
**Depends on**: Phase 28.2 (monitoring research complete for decision: include in Helm or defer)
**Requirements**: HELM-01, HELM-02, HELM-03, HELM-04, HELM-05
**Success Criteria** (what must be TRUE):
  1. Helm chart deploys controller and node plugin with configurable values
  2. Chart supports customization of RDS connection parameters (address, SSH port, NVMe port)
  3. Chart includes RBAC, ServiceAccount, and Secret reference management (user creates Secret, chart references it)
  4. Chart supports multiple storage classes with different configurations
  5. Chart documentation includes installation instructions and configuration examples
  6. Chart distributed via git repository (users install from local clone or deploy/helm/ directory)
**Plans**: 3 plans

Plans:
- [ ] 28-01-PLAN.md -- Chart skeleton (Chart.yaml, values.yaml, values.schema.json, _helpers.tpl)
- [ ] 28-02-PLAN.md -- Core templates (controller Deployment, node DaemonSet, RBAC, CSIDriver, ServiceAccount)
- [ ] 28-03-PLAN.md -- Feature templates (StorageClass, VolumeSnapshotClass, ServiceMonitor, NOTES.txt, README)

## Progress

**Execution Order:**
Phases execute in numeric order: 26 → 27 → 28.1 → 28.2 → 28

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-25.2 | v0.1.0-v0.9.0 | Complete | ✅ Complete | 2026-02-06 |
| 26. Volume Snapshots | v0.10.0 | 6/6 | ✅ Complete | 2026-02-06 |
| 27. Documentation & Hardware Validation | v0.10.0 | 3/3 | ✅ Complete | 2026-02-05 |
| 28.1. Fix Metric Accuracy | v0.10.0 | 1/1 | ✅ Complete | 2026-02-06 |
| 28.2. RDS Health & Performance Monitoring | v0.10.0 | 2/2 | ✅ Complete | 2026-02-06 |
| 28. Helm Chart | v0.10.0 | 0/3 | Not started | - |

---
*Last updated: 2026-02-06 after Phase 28 plan revision*
