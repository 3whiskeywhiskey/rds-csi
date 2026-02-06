# Phase 22: CSI Sanity Tests Integration - Context

**Gathered:** 2026-02-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Integrate csi-sanity test suite to validate CSI spec compliance for Identity and Controller services through automated testing in CI. Node service validation deferred to later phases when hardware available.

</domain>

<decisions>
## Implementation Decisions

### Test Execution Environment
- **Primary environment**: Mock RDS in CI for speed and reliability
- **Optional hardware testing**: Separate job for real RDS hardware validation
- **Local development**: Single `make sanity` target with auto-setup (mock RDS startup, test execution, cleanup)
- **CI platform**: GitHub Actions (only option until Gitea Actions available at gitea.whiskey.works)
- **CI optimization**: Minimal builds due to long execution times - be strategic with CI usage

### Test Scope and Coverage
- **Services validated**: Identity + Controller only (Node service deferred until NVMe/TCP mock ready)
- **Capabilities tested**: Core volume lifecycle (CreateVolume, DeleteVolume, ValidateVolumeCapabilities, GetCapacity) + ControllerExpandVolume
- **Capabilities deferred**: Snapshots (Phase 26), cloning, Node operations
- **Idempotency**: Critical requirement - CreateVolume/DeleteVolume must pass when called multiple times with same parameters
- **Documentation**: Capability matrix in TESTING.md showing implemented vs deferred capabilities with rationale

### Failure Handling and Debugging
- **CI behavior**: Fail build immediately on any sanity test failure - strict CSI spec compliance enforcement
- **No retry logic**: Fix flakiness properly rather than masking problems with retries
- **Debug artifacts captured**:
  - Full csi-sanity test logs with timing and request/response details
  - Mock RDS command history (all SSH commands and responses)
  - Driver logs with V(4) diagnostic verbosity

### Test Configuration and Parameters
- **Connection config**: Hardcoded for mock (localhost:2222), environment variables for hardware tests
- **Volume sizes**: Realistic sizes (10GB+) to catch size-related bugs despite slower execution
- **Cleanup strategy**: Automatic cleanup always (success and failure) to prevent orphans
- **Negative test cases**: Yes - validate proper CSI error codes (ALREADY_EXISTS, NOT_FOUND, INVALID_ARGUMENT, RESOURCE_EXHAUSTED)

### Claude's Discretion
- Driver deployment method for tests (in-process vs container-based) - choose based on csi-sanity best practices
- Debug artifact structure (separate files vs unified log) - optimize for troubleshooting workflow
- Specific test volume counts and concurrency limits within the 10GB+ guideline

</decisions>

<specifics>
## Specific Ideas

- **CI constraint**: GitHub Actions builds take very long - need to be strategic about when tests run
- **Hardware path**: gitea.whiskey.works runner coming soon for self-hosted CI with hardware access
- **Mock priority**: Mock RDS must be good enough to catch CSI spec violations without hardware dependency

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope

</deferred>

---

*Phase: 22-csi-sanity-tests-integration*
*Context gathered: 2026-02-04*
