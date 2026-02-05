---
phase: 24-automated-e2e-test-suite
plan: 01
status: complete
completed: 2026-02-05

subsystem: testing
tags: [ginkgo, e2e, test-infrastructure, mock-rds]

requires:
  - phase: 22
    plan: 01
    provides: "CSI sanity tests with in-process driver pattern"
  - phase: 23
    plan: 01
    provides: "Mock RDS with realistic timing and error injection"

provides:
  - capability: "E2E test suite infrastructure"
    details: "Ginkgo v2 suite with BeforeSuite/AfterSuite lifecycle"
  - capability: "Test isolation via unique run IDs"
    details: "e2e-{timestamp} prefix prevents conflicts"
  - capability: "Helper functions for common patterns"
    details: "createVolumeWithCleanup, waitForVolumeOnMockRDS, capability factories"

affects:
  - phase: 24
    plans: ["02", "03"]
    reason: "Provides foundation for controller and node E2E tests"

tech-stack:
  added:
    - "Ginkgo v2 BeforeSuite/AfterSuite pattern"
    - "DeferCleanup for automatic test cleanup"
    - "Eventually pattern for async waits"
  patterns:
    - "In-process driver testing (mock RDS + driver + gRPC)"
    - "Random port assignment (port 0) for parallel safety"
    - "Test run ID for isolation (e2e-{unix_timestamp})"

key-files:
  created:
    - path: "test/e2e/e2e_suite_test.go"
      purpose: "Ginkgo v2 suite with BeforeSuite/AfterSuite"
      lines: 187
    - path: "test/e2e/helpers.go"
      purpose: "Test helper functions"
      lines: 89
    - path: "test/e2e/fixtures.go"
      purpose: "Test fixtures and constants"
      lines: 54
  modified: []

decisions:
  - id: E2E-01
    choice: "Enable both controller and node services in E2E tests"
    rationale: "Validates full volume lifecycle including node operations"
    alternatives: ["Controller-only like sanity tests"]
  - id: E2E-02
    choice: "Use Eventually pattern for socket readiness (not sleep)"
    rationale: "More reliable and faster than fixed sleep delays"
    alternatives: ["time.Sleep with fixed duration"]
  - id: E2E-03
    choice: "Clean up volumes with testRunID prefix in AfterSuite"
    rationale: "Ensures cleanup even if individual tests fail"
    alternatives: ["Per-test cleanup only", "Manual cleanup"]

metrics:
  duration: 184s
  tasks_completed: 3
  files_created: 3
  commits: 3
  test_cases: 1
---

# Phase 24 Plan 01: E2E Suite Infrastructure Summary

**One-liner:** Ginkgo v2 E2E test foundation with in-process driver, mock RDS, and test isolation via unique run IDs

## What Was Done

Created the foundational E2E test suite infrastructure using Ginkgo v2, following the proven in-process driver pattern from sanity tests. The suite provides complete lifecycle management (BeforeSuite/AfterSuite), test isolation via unique run IDs, and helper functions for common test patterns.

### Task 1: E2E Suite Infrastructure (fd74d6a)

Created `test/e2e/e2e_suite_test.go` with Ginkgo v2 suite:

**BeforeSuite Setup:**
- Generate unique test run ID: `e2e-{unix_timestamp}`
- Start mock RDS on port 0 (random port assignment for parallel safety)
- Create driver with `EnableController=true`, `EnableNode=true`
- Start driver on Unix socket: `/tmp/csi-e2e-{testRunID}.sock`
- Wait for socket ready using `Eventually()` pattern (not sleep)
- Create gRPC connection and CSI clients (Identity, Controller, Node)
- Create context with 2 minute timeout

**AfterSuite Cleanup:**
- List all volumes and delete those with testRunID prefix
- Close gRPC connection
- Stop mock RDS server
- Remove socket file

**Key Pattern:** In-process driver testing - mock RDS, driver, and gRPC all in same process for fast startup and easy debugging.

### Task 2: Test Helper Functions (809a3d0)

Created `test/e2e/helpers.go` with common test utilities:

**Constants:**
- `GiB`, `MiB` - Size units
- `defaultTimeout`, `pollInterval` - Timing constants
- `testVolumeBasePath` - Volume storage path

**Helpers:**
- `testVolumeName(name)` - Generate unique volume name with testRunID prefix
- `mountVolumeCapability(fsType)` - Factory for mount volume capabilities
- `blockVolumeCapability()` - Factory for block volume capabilities
- `createVolumeWithCleanup(name, size, capability)` - Create volume with automatic cleanup via `DeferCleanup`
- `waitForVolumeOnMockRDS(volumeID)` - Wait for volume to exist on mock RDS
- `waitForVolumeDeletedFromMockRDS(volumeID)` - Wait for volume deletion

### Task 3: Test Fixtures (57f6830)

Created `test/e2e/fixtures.go` with test fixtures:

**Constants:**
- `testSSHPrivateKey` - Test RSA key (copied from sanity tests)
- `smallVolumeSize` - 1 GiB for quick tests
- `mediumVolumeSize` - 5 GiB for block volume tests
- `largeVolumeSize` - 10 GiB for expansion tests

**Path Generators:**
- `stagingPath(volumeID)` - Generate staging path for NodeStageVolume
- `publishPath(volumeID)` - Generate publish path for NodePublishVolume

**Verification:**
- Suite runs successfully with placeholder test
- Infrastructure validated: mock RDS starts, driver connects, cleanup works
- Test output confirms: "E2E suite infrastructure validated, testRunID=e2e-1770275585"

## Technical Decisions

### Decision E2E-01: Enable Both Controller and Node Services

**Choice:** Enable both controller and node services in E2E tests

**Rationale:**
- Validates full volume lifecycle including node operations
- Catches integration issues between controller and node services
- More realistic than controller-only testing
- Required for testing NodeStageVolume/NodePublishVolume

**Alternatives Considered:**
- Controller-only like sanity tests - doesn't validate full lifecycle
- Separate controller and node suites - more complex, less realistic

**Implementation:**
```go
driverConfig := driver.DriverConfig{
    EnableController: true,
    EnableNode:       true,
    ManagedNQNPrefix: "nqn.2000-02.com.mikrotik:",
    // ...
}
```

### Decision E2E-02: Eventually Pattern for Socket Readiness

**Choice:** Use Eventually pattern for socket readiness checks

**Rationale:**
- More reliable than fixed sleep delays
- Faster startup (polls every 100ms, max 10s)
- Ginkgo best practice for async operations
- Provides clear error message if socket doesn't become ready

**Alternatives Considered:**
- `time.Sleep(500 * time.Millisecond)` - used in sanity tests but less robust
- Retry loop with timeout - more verbose, less idiomatic

**Implementation:**
```go
Eventually(func() bool {
    if _, err := os.Stat(driverEndpoint); err != nil {
        return false
    }
    conn, err := net.Dial("unix", driverEndpoint)
    if err != nil {
        return false
    }
    _ = conn.Close()
    return true
}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
```

### Decision E2E-03: AfterSuite Volume Cleanup

**Choice:** Clean up volumes with testRunID prefix in AfterSuite

**Rationale:**
- Ensures cleanup even if individual tests fail
- Prevents volume leaks on mock RDS
- Uses testRunID prefix to only clean up current test run's volumes
- Complements per-test cleanup via `DeferCleanup`

**Alternatives Considered:**
- Per-test cleanup only - fails if test panics before cleanup
- Manual cleanup - error-prone, requires remembering to clean up
- No cleanup - causes mock RDS to accumulate volumes

**Implementation:**
```go
listResp, err := controllerClient.ListVolumes(ctx, &csi.ListVolumesRequest{})
if err == nil {
    for _, entry := range listResp.Entries {
        volumeID := entry.Volume.VolumeId
        if strings.HasPrefix(volumeID, testRunID) {
            _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
                VolumeId: volumeID,
            })
        }
    }
}
```

## Deviations from Plan

None - plan executed exactly as written.

## Testing Results

### Infrastructure Validation Test

**Test:** "E2E Suite Sanity - should have valid test infrastructure"

**Result:** ✅ PASSED

**Verification:**
- testRunID is set: `e2e-1770275585`
- mockRDS is initialized and listening on random port
- controllerClient is initialized
- gRPC connection established
- Driver responds to requests

**Output:**
```
I0205 02:13:05.961579 E2E suite infrastructure validated, testRunID=e2e-1770275585
Ran 1 of 1 Specs in 0.341 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 0 Skipped
```

## Next Phase Readiness

**Phase 24 Plan 02 - Controller E2E Tests:** ✅ READY

The E2E suite infrastructure is complete and validated. The following are ready for use:

1. **Test Isolation:** testRunID prefix prevents conflicts between test runs
2. **Helper Functions:** createVolumeWithCleanup, waitForVolumeOnMockRDS available
3. **CSI Clients:** controllerClient, nodeClient initialized and ready
4. **Mock RDS:** Started and connected, ready for volume operations
5. **Cleanup:** Automatic cleanup via AfterSuite and DeferCleanup

**Blockers:** None

**Next Steps:**
1. Plan 24-02: Implement controller E2E tests (CreateVolume, DeleteVolume, ListVolumes)
2. Plan 24-03: Implement node E2E tests (NodeStageVolume, NodePublishVolume)

## Lessons Learned

### What Worked Well

1. **In-Process Driver Pattern:** Following the proven pattern from sanity tests made implementation straightforward
2. **Eventually Pattern:** Much more reliable than fixed sleep delays
3. **Random Port Assignment:** Port 0 enables parallel test runs without conflicts
4. **testRunID Prefix:** Provides excellent test isolation and cleanup scope

### What Could Be Improved

1. **Node Service Requirement:** Driver requires ManagedNQNPrefix even for tests that don't use node operations - could make this optional for controller-only mode
2. **Error Messages:** Initial error about NQN format wasn't immediately clear - better validation/error messages would help

### Reusable Patterns

1. **Test Run ID Generation:** `fmt.Sprintf("e2e-%d", time.Now().Unix())` - simple and effective
2. **Socket Readiness Check:** Eventually pattern with connection test is more robust than file stat
3. **Cleanup Scope:** AfterSuite deletes volumes by prefix, not all volumes

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | fd74d6a | Create E2E suite infrastructure with Ginkgo v2 |
| 2 | 809a3d0 | Create test helper functions |
| 3 | 57f6830 | Create test fixtures and verify suite runs |

**Total:** 3 commits, 330 lines of code, 3 minutes execution time
