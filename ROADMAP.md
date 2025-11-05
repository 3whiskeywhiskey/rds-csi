# RDS CSI Driver - Roadmap

This document outlines the development phases and timeline for the RDS CSI Driver project.

## Overview

**Goal**: Build a production-ready CSI driver for MikroTik ROSE Data Server (RDS) NVMe/TCP storage

**Total Timeline**: 8-11 weeks to v0.1.0 (alpha release)

## Milestones

### Milestone 1: Foundation (Weeks 1-3) 🚧 **Current Phase**

**Objective**: Establish project structure, documentation, and basic infrastructure

**Issues**:
- [#1] Project scaffolding and Go module setup
- [#2] Document RDS NVMe/TCP commands and workflows
- [#3] Implement SSH client for RouterOS CLI
- [#4] Implement CSI Identity service
- [#5] Create basic Dockerfile and Makefile

**Deliverables**:
- ✅ Project repository created with standard structure
- ✅ Comprehensive documentation (README, architecture, RDS commands)
- [ ] SSH client wrapper for RouterOS CLI commands
- [ ] CSI Identity service implementation (GetPluginInfo, Probe, GetPluginCapabilities)
- [ ] Build system (Makefile, Dockerfile)
- [ ] Unit tests for SSH client and Identity service

**Success Criteria**:
- Can build binary and container image
- SSH client can connect to RDS and execute commands
- Identity service responds to gRPC calls

---

### Milestone 2: Controller Service (Weeks 4-6)

**Objective**: Implement volume lifecycle management

**Issues**:
- [#6] Implement CreateVolume (file-backed disk creation)
- [#7] Implement DeleteVolume (cleanup)
- [#8] Implement ValidateVolumeCapabilities
- [#9] Implement GetCapacity (query RDS free space)
- [#10] Add CSI sanity tests for controller

**Deliverables**:
- Controller service implementation with core methods:
  - `CreateVolume`: Creates file-backed NVMe/TCP export on RDS
  - `DeleteVolume`: Removes volume and cleans up
  - `ValidateVolumeCapabilities`: Validates requested access modes
  - `GetCapacity`: Queries available storage on RDS
  - `ControllerGetCapabilities`: Declares driver capabilities
- Volume ID generation and tracking
- Error handling and retry logic
- CSI sanity tests passing for controller

**Success Criteria**:
- Can create/delete volumes via direct gRPC calls
- Volumes appear on RDS as file-backed disks with NVMe/TCP export
- Cleanup is reliable (no orphaned volumes)
- CSI sanity tests pass (controller subset)

---

### Milestone 3: Node Service (Weeks 7-8)

**Objective**: Implement volume attachment and mounting on worker nodes

**Issues**:
- [#11] Implement NodeStageVolume (nvme connect)
- [#12] Implement NodeUnstageVolume (nvme disconnect)
- [#13] Implement NodePublishVolume (mount filesystem)
- [#14] Implement NodeUnpublishVolume (unmount)
- [#15] Add CSI sanity tests for node

**Deliverables**:
- Node service implementation:
  - `NodeStageVolume`: Connects to NVMe/TCP target, waits for device
  - `NodeUnstageVolume`: Disconnects from NVMe/TCP target
  - `NodePublishVolume`: Formats and mounts filesystem to pod path
  - `NodeUnpublishVolume`: Unmounts from pod path
  - `NodeGetCapabilities`: Declares staging support
  - `NodeGetInfo`: Returns node ID
- NVMe device discovery logic
- Filesystem operations (format, mount, unmount)
- CSI sanity tests passing for node

**Success Criteria**:
- Can stage/unstage volumes (NVMe connect/disconnect)
- Can publish/unpublish volumes (mount/unmount)
- Volumes are accessible from pods with correct permissions
- CSI sanity tests pass (full suite)

---

### Milestone 4: Kubernetes Integration (Weeks 9-10)

**Objective**: Deploy driver in Kubernetes cluster and validate E2E workflows

**Issues**:
- [#16] Create Kubernetes manifests (controller + node)
- [#17] Create RBAC configuration
- [#18] Create example StorageClass
- [#19] Deploy and test in metal cluster
- [#20] Document installation and usage

**Deliverables**:
- Kubernetes deployment manifests:
  - Controller Deployment (StatefulSet with 1 replica)
  - Node DaemonSet (runs on all worker nodes)
  - ServiceAccount, ClusterRole, ClusterRoleBinding
  - CSIDriver object registration
  - Example StorageClass
- Installation documentation
- E2E testing in real cluster:
  - PVC creation → volume appears on RDS
  - Pod creation → volume mounts successfully
  - Pod deletion → volume unmounts cleanly
  - PVC deletion → volume removed from RDS

**Success Criteria**:
- Driver deploys successfully to Kubernetes
- Can create PVC and use in pod
- Volume lifecycle (create → mount → unmount → delete) works end-to-end
- No resource leaks (orphaned volumes, NVMe connections)

---

### Milestone 5: Production Readiness (Weeks 11-12)

**Objective**: Harden for production use and create release artifacts

**Issues**:
- [#21] Create Helm chart
- [#22] Add monitoring/metrics
- [#23] Implement volume expansion support
- [#24] Add comprehensive error handling
- [#25] Create CI/CD workflows (build, test, release)

**Deliverables**:
- Helm chart with configurable values
- Prometheus metrics endpoint
- Volume expansion support (`ControllerExpandVolume`, `NodeExpandVolume`)
- Improved error handling:
  - Retries with exponential backoff
  - Better error messages
  - Graceful degradation
- CI/CD workflows:
  - Lint and test on PRs
  - Build multi-arch images on tag
  - Automated releases
- Security review and hardening

**Success Criteria**:
- Can install via Helm
- Metrics visible in Prometheus
- Can expand volumes dynamically
- CI/CD pipeline functional
- Ready for alpha release (v0.1.0)

---

## Future Enhancements (Post v0.1.0)

### Phase 2: Advanced Features

**Issues**:
- [#26] Snapshot support (Btrfs snapshots)
- [#27] Volume cloning
- [#28] Topology awareness
- [#29] Performance optimization

**Features**:
- **Snapshots**: `CreateSnapshot` / `DeleteSnapshot` using Btrfs snapshots
- **Cloning**: Fast volume duplication for VM templates
- **Topology**: Node affinity based on storage network connectivity
- **Performance**: Connection pooling, parallel operations

**Timeline**: 4-6 weeks

---

### Phase 3: Enterprise Features

**Features**:
- **Volume Replication**: Cross-RDS replication
- **Multi-tenancy**: Namespace isolation, quotas
- **Advanced Monitoring**: Detailed metrics, alerting rules
- **High Availability**: Multiple controller replicas with leader election
- **Encryption**: Volume encryption at rest (LUKS) and in transit

**Timeline**: 8-12 weeks

---

## Release Strategy

### v0.1.0 (Alpha) - Target: Week 12
- Core functionality working
- Tested in homelab environment
- Documentation complete
- Known limitations documented

**Status**: **Not recommended for production**

### v0.2.0 (Beta) - Target: +6 weeks
- Advanced features (snapshots, cloning)
- Additional testing and hardening
- Community feedback incorporated

**Status**: **Suitable for testing environments**

### v1.0.0 (GA) - Target: +12 weeks
- Production-grade reliability
- Comprehensive test coverage
- Security audit completed
- Performance benchmarked

**Status**: **Production ready**

---

## Dependencies and Risks

### External Dependencies
- **MikroTik RouterOS**: Requires specific CLI commands to work correctly
  - **Risk**: CLI format changes in RouterOS updates
  - **Mitigation**: Version-specific command parsing, integration tests

- **NVMe/TCP Kernel Support**: Requires Linux 5.0+ with nvme-tcp module
  - **Risk**: Older nodes without NVMe/TCP support
  - **Mitigation**: Document prerequisites, add node selector

- **Kubernetes CSI Spec**: CSI v1.5.0+ required
  - **Risk**: Breaking changes in CSI spec
  - **Mitigation**: Pin CSI library versions, follow spec updates

### Technical Risks
1. **RDS Stability**: Unknown behavior under high concurrency
   - **Mitigation**: Rate limiting, connection pooling, extensive testing

2. **NVMe Connection Limits**: Maximum concurrent NVMe connections per node
   - **Mitigation**: Document limits, implement connection management

3. **Network Partitions**: Handling connectivity loss to RDS
   - **Mitigation**: Timeouts, retries, clear error messages

4. **Cleanup Failures**: Orphaned volumes if cleanup fails
   - **Mitigation**: Idempotent operations, manual cleanup tools

---

## Success Metrics

### v0.1.0 Targets
- [ ] 100% of core CSI operations implemented
- [ ] 90%+ code coverage for unit tests
- [ ] CSI sanity tests passing
- [ ] E2E tests passing in homelab cluster
- [ ] < 5 known critical bugs
- [ ] Documentation coverage: 100%

### Performance Targets (v1.0.0)
- Volume creation: < 30 seconds
- Volume deletion: < 15 seconds
- Mount/unmount: < 5 seconds
- Sustained throughput: > 1 GB/s per volume
- Latency: < 5ms p99 for I/O operations

---

## Community and Contributions

### Getting Involved
- **Report Bugs**: Use GitHub issues
- **Feature Requests**: Open issue with [FEATURE] prefix
- **Code Contributions**: Follow CONTRIBUTING.md guidelines
- **Documentation**: PRs for docs always welcome

### Communication Channels
- **Issues**: Primary discussion forum
- **Pull Requests**: Code review and technical discussion
- **Wiki**: User guides and tutorials (post-v0.1.0)

---

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2025-11-05 | 1.0 | Initial roadmap created |

---

**Last Updated**: 2025-11-05
**Current Milestone**: Milestone 1 (Foundation)
**Next Milestone**: Milestone 2 (Controller Service) - ETA Week 4
