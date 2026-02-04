# Requirements: RDS CSI Driver v0.7.1

**Defined:** 2026-02-04
**Core Value:** Volumes remain accessible after NVMe-oF reconnections
**Milestone Goal:** Systematic codebase cleanup to improve maintainability and reduce technical debt

## v1 Requirements

### Logging Cleanup

- [x] **LOG-01**: Security logger consolidated from 11 duplicate methods (300+ lines) to <50 lines with configurable helper
- [x] **LOG-02**: DeleteVolume operation logging reduced from 4-6 V(3) statements to maximum 2 per operation
- [x] **LOG-03**: All CSI operations audited and verbosity rationalized (info = actionable, debug = diagnostic)
- [x] **LOG-04**: Severity mapping uses table-driven approach instead of switch statements

### Error Handling

- [x] **ERR-01**: All 160+ error returns using %v converted to %w for proper error wrapping
- [x] **ERR-02**: Every error includes contextual information (operation, volume ID, node, reason)
- [x] **ERR-03**: Error handling patterns documented and consistently applied across all packages
- [x] **ERR-04**: Error paths audited for missing context or silent failures

### Test Coverage

- [x] **TEST-01**: Failing block volume tests fixed (nil pointer dereference resolved)
- [ ] **TEST-02**: SSH client test coverage increased from 0% to >80% (pkg/rds/ssh_client.go)
- [ ] **TEST-03**: RDS package test coverage increased from 44.5% to >80%
- [ ] **TEST-04**: Mount package test coverage increased from 55.9% to >80%
- [ ] **TEST-05**: NVMe package test coverage increased from 43.3% to >80%
- [ ] **TEST-06**: Files with 0% coverage now have comprehensive tests:
  - pkg/rds/ssh_client.go (341 lines)
  - pkg/driver/server.go (145 lines)
  - pkg/attachment/persist.go (147 lines)
  - pkg/rds/client.go (69 lines)

### Code Quality

- [ ] **QUAL-01**: Common error handling patterns extracted into reusable utilities
- [ ] **QUAL-02**: Duplicated switch statements for severity mapping replaced with shared table
- [ ] **QUAL-03**: Large packages refactored for better separation of concerns
- [ ] **QUAL-04**: Code smells from CONCERNS.md analysis resolved or explicitly documented as deferred

## Future Requirements

Deferred to later milestones:

### Observability
- Enhanced Prometheus metrics for fine-grained operation tracking
- Distributed tracing integration
- Structured logging with JSON output

### Resilience
- End-to-end lifecycle tests in CI pipeline
- Chaos testing (network partitions, RDS unavailability)
- Orphan reconciler edge case testing

### Security
- SSH host key verification enforcement (remove bypass flag)
- Audit logging for all control plane operations

## Out of Scope

| Item | Reason |
|------|--------|
| Volume snapshots | Separate feature milestone |
| Controller HA | Requires leader election, separate architectural work |
| Volume encryption | Different concern, separate security milestone |
| Performance optimization | Focus on correctness first, optimization later |
| E2E tests in CI | Infrastructure not ready, defer to CI improvement milestone |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| TEST-01 | Phase 17 | Complete |
| LOG-01 | Phase 18 | Complete |
| LOG-02 | Phase 18 | Complete |
| LOG-03 | Phase 18 | Complete |
| LOG-04 | Phase 18 | Complete |
| ERR-01 | Phase 19 | Complete |
| ERR-02 | Phase 19 | Complete |
| ERR-03 | Phase 19 | Complete |
| ERR-04 | Phase 19 | Complete |
| TEST-02 | Phase 20 | Pending |
| TEST-03 | Phase 20 | Pending |
| TEST-04 | Phase 20 | Pending |
| TEST-05 | Phase 20 | Pending |
| TEST-06 | Phase 20 | Pending |
| QUAL-01 | Phase 21 | Pending |
| QUAL-02 | Phase 21 | Pending |
| QUAL-03 | Phase 21 | Pending |
| QUAL-04 | Phase 21 | Pending |

**Coverage:**
- v1 requirements: 18 total
- Mapped to phases: 18/18 ✓
- Unmapped: 0

---
*Requirements defined: 2026-02-04*
*Last updated: 2026-02-04 (roadmap created, 100% coverage validated)*
