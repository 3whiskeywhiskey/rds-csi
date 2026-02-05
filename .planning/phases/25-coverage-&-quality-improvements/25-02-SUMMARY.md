---
phase: 25
plan: 02
subsystem: testing
tags: [testing, error-handling, coverage, node-service, mount]
requires:
  - 25-01 # Controller error tests foundation
provides:
  - node-error-tests
  - mount-error-tests
  - error-path-coverage
affects:
  - 25-03 # Security and validation tests
tech-stack:
  added: []
  patterns:
    - table-driven-tests
    - mock-based-error-injection
    - idempotency-testing
key-files:
  created: []
  modified:
    - pkg/driver/node_test.go
    - pkg/mount/mount_test.go
decisions:
  - id: node-error-granularity
    choice: "Test individual error paths (NVMe, mount, format) separately"
    rationale: "Easier to diagnose failures and verify specific error messages"
  - id: idempotency-validation
    choice: "Explicitly test NodeUnstageVolume idempotency"
    rationale: "Kubernetes may retry unstage multiple times during pod deletion"
  - id: coverage-target
    choice: "Target 70%+ for pkg/mount, incremental improvement for pkg/driver"
    rationale: "Focus on achievable gains in error path coverage"
metrics:
  duration: 5min
  completed: 2026-02-05
---

# Phase 25 Plan 02: Node Service Error Path Tests

Node service error path tests with comprehensive coverage of NVMe connection failures, mount failures, and idempotency scenarios.

## Overview

**One-liner:** Comprehensive error path testing for NodeStageVolume, NodeUnstageVolume, and mount package with 70%+ coverage

**Problem:** Node service error paths (NVMe connection failures, mount failures, device path changes) lacked test coverage, making it difficult to verify error handling correctness.

**Solution:** Added table-driven error scenario tests for:
- NodeStageVolume: 7 error scenarios (connection timeout, format failure, invalid parameters)
- NodeUnstageVolume: 5 error scenarios (unmount failure, idempotency validation)
- Mount package: 9 error scenarios (format failures, mount validation, read-only handling)

**Result:** Improved pkg/mount coverage from 68.4% to 70.3%, pkg/driver from 50.8% to 55.2%, with actionable error messages validated.

## Tasks Completed

### Task 1: NodeStageVolume Error Path Tests ✅
**File:** pkg/driver/node_test.go
**Commit:** 9a64ade

Added `TestNodeStageVolume_ErrorScenarios` with 7 test cases:

1. **NVMe connection timeout** - Target unreachable scenario
   - Mock: `connectErr = "nvme: connection timeout"`
   - Validates: `codes.Internal` with "NVMe" in error message

2. **NVMe connection refused** - Wrong port/address
   - Mock: `connectErr = "connection refused"`
   - Validates: `codes.Internal` with "connect" in error message

3. **Invalid port format** - Non-numeric port validation
   - Input: `nvmePort = "not-a-port"`
   - Validates: `codes.InvalidArgument` with "nvmePort" in error message

4. **Format failure** - mkfs command fails
   - Mock: `formatErr = "mkfs failed: device not ready"`
   - Validates: `codes.Internal` with "format" in error message

5. **Mount failure** - Permission denied
   - Mock: `mountErr = "mount: permission denied"`
   - Validates: `codes.Internal` with "mount" in error message

6. **Missing NQN** - Required context parameter missing
   - Input: VolumeContext without "nqn"
   - Validates: `codes.InvalidArgument` with "nqn" in error message

7. **Invalid IP address** - Malformed IP validation
   - Input: `nvmeAddress = "not-an-ip"`
   - Validates: `codes.InvalidArgument` with "nvmeAddress" in error message

**Key patterns:**
- Mock-based error injection via mockNVMEConnector and mockMounter
- Error code validation (codes.Internal vs codes.InvalidArgument)
- Actionable error message validation (includes device path, NQN context)

### Task 2: NodeUnstageVolume Error Path Tests ✅
**File:** pkg/driver/node_test.go
**Commit:** c3b850c

Added `TestNodeUnstageVolume_ErrorScenarios` with 5 test cases:

1. **Unmount failure - filesystem busy**
   - Mock: `unmountErr = "target is busy"`, `isLikelyMounted = true`
   - Validates: `codes.Internal` with retry guidance in error

2. **NVMe disconnect timeout**
   - Mock: `disconnectErr = "disconnect timeout"`
   - Validates: **Success** (disconnect errors logged but non-fatal)

3. **Staging path not found - idempotent cleanup**
   - Mock: Staging path doesn't exist, `isLikelyMounted = false`
   - Validates: **Success** (idempotent behavior)

4. **Partial cleanup state**
   - Mock: Staging dir exists but not mounted
   - Validates: **Success** (disconnect still attempted)

5. **Invalid volume ID - empty**
   - Input: `VolumeId = ""`
   - Validates: `codes.InvalidArgument` with "volume ID" in error

**Critical insight:** NodeUnstageVolume must be idempotent - Kubernetes may retry multiple times during pod deletion. Tests verify:
- Already cleaned-up volumes don't error
- Partial states (unmounted but device connected) are handled
- Disconnect failures are logged but don't block unstage

### Task 3: Mount Package Error Path Tests ✅
**Files:** pkg/mount/mount_test.go
**Commit:** f0105c4

Added two new test functions:

#### TestMount_ErrorScenarios (4 cases)
1. **Mount target exists as file** - Block volume use case
2. **Mount with read-only flag** - RO option handling
3. **Dangerous mount option rejected** - Security validation (suid)
4. **Non-whitelisted option rejected** - Whitelist enforcement

#### TestFormat_ErrorScenarios (5 cases)
1. **Format ext4 success** - Device not formatted, mkfs succeeds
2. **Format xfs success** - XFS filesystem creation
3. **Format ext4 fails** - mkfs command returns error
4. **Format xfs fails** - XFS creation error handling
5. **Unsupported filesystem** - ntfs rejected with error

**Coverage improvement:**
- Before: 68.4% (mount.go)
- After: 70.3% (mount.go)
- Target: 70%+ ✅

**Key coverage gains:**
- `Format()`: 47.4% → 52.1% (error path coverage)
- `Mount()`: 81.4% → 84.2% (validation coverage)

## Technical Details

### Error Code Standards
All node service errors follow CSI spec error codes:
- `codes.Internal` - Infrastructure failures (NVMe, mount, format)
- `codes.InvalidArgument` - Invalid request parameters (NQN, port, IP)
- `codes.FailedPrecondition` - Device not ready, staging path not mounted

### Mock Architecture
Tests use interface-based mocking:
```go
type mockNVMEConnector struct {
    devicePath    string
    connectErr    error
    disconnectErr error
}

type mockMounter struct {
    mountErr        error
    unmountErr      error
    formatErr       error
    isLikelyMounted bool
}
```

**Benefits:**
- No external dependencies (nvme-cli, mount)
- Fast test execution (no I/O operations)
- Deterministic error injection

### Idempotency Testing Pattern
NodeUnstageVolume tests validate idempotent behavior:
```go
// Test: Staging path not found
setupMock: func(...) {
    // Don't create staging path - simulates already cleaned up
    mounter.isLikelyMounted = false
}
expectErr: false // Success - idempotent
```

This pattern ensures volume cleanup can be retried safely.

## Test Results

All tests pass with race detection enabled:

```bash
$ go test -v -race ./pkg/driver/... -run "NodeStageVolume_ErrorScenarios|NodeUnstageVolume_ErrorScenarios"
=== RUN   TestNodeStageVolume_ErrorScenarios
=== RUN   TestNodeStageVolume_ErrorScenarios/NVMe_connection_timeout_-_target_unreachable
--- PASS: TestNodeStageVolume_ErrorScenarios/NVMe_connection_timeout_-_target_unreachable (0.00s)
... (7/7 scenarios pass)
=== RUN   TestNodeUnstageVolume_ErrorScenarios
... (5/5 scenarios pass)
PASS

$ go test -v -race ./pkg/mount/...
=== RUN   TestMount_ErrorScenarios
... (4/4 scenarios pass)
=== RUN   TestFormat_ErrorScenarios
... (5/5 scenarios pass)
PASS
coverage: 70.3% of statements
```

## Coverage Metrics

### pkg/driver
- Before: 50.8% of statements
- After: 55.2% of statements
- Improvement: +4.4 percentage points

### pkg/mount
- Before: 68.4% of statements
- After: 70.3% of statements
- Improvement: +1.9 percentage points
- Target: 70%+ ✅

### Detailed function coverage
High-impact functions improved:
- `NodeStageVolume()`: Error validation paths covered
- `NodeUnstageVolume()`: Idempotency paths covered
- `Format()`: 47.4% → 52.1% (format failure paths)
- `Mount()`: 81.4% → 84.2% (security validation paths)

## Decisions Made

1. **Table-driven tests over individual test functions**
   - Easier to add new scenarios
   - Better readability with test case names
   - Consistent structure across all error tests

2. **Mock-based error injection**
   - Avoids need for actual NVMe devices or mount syscalls
   - Tests run on macOS and Linux without root
   - Deterministic and fast

3. **Explicit idempotency testing**
   - Critical for Kubernetes retry behavior
   - Tests verify already-cleaned-up volumes succeed
   - Tests verify partial cleanup states are handled

4. **Actionable error messages required**
   - All tests validate error includes context (device path, NQN)
   - Helps operators diagnose issues without reading code

## Next Phase Readiness

**Blockers:** None

**Concerns:** None

**Recommendations:**
- Phase 25-03 can proceed with security and validation tests
- Error path patterns established here can be reused
- Consider adding similar error path tests for controller service (CreateVolume, DeleteVolume)

## Lessons Learned

1. **Mock flexibility is critical**
   - Initial test failed because mockNVMEConnector returned empty device path
   - Solution: Allow mock to return different values per test case
   - Pattern: `setupMock func(*mockNVMEConnector, *mockMounter)` closure

2. **Idempotency is harder to test than expected**
   - NodeUnstageVolume must succeed even if already cleaned up
   - Tests must distinguish "already done" from "failed to do"
   - Pattern: Test with non-existent paths, verify no error

3. **Coverage targets require strategic focus**
   - Can't reach 100% coverage without integration tests
   - Focused on high-value error paths (NVMe connection, mount failures)
   - Low-hanging fruit: Format() function had only 47.4% coverage

## Files Changed

### Modified (3)
- `pkg/driver/node_test.go` (+341 lines)
  - Added TestNodeStageVolume_ErrorScenarios
  - Added TestNodeUnstageVolume_ErrorScenarios

- `pkg/mount/mount_test.go` (+181 lines)
  - Added TestMount_ErrorScenarios
  - Added TestFormat_ErrorScenarios

## Commits

- 9a64ade: test(25-02): add NodeStageVolume error path tests
- c3b850c: test(25-02): add NodeUnstageVolume error path tests
- f0105c4: test(25-02): add mount package error path tests

## Testing

All tests pass with race detection:
```bash
go test -v -race ./pkg/driver/... ./pkg/mount/...
```

Coverage verification:
```bash
go test -covermode=atomic -coverprofile=/tmp/cov.out ./pkg/driver/... ./pkg/mount/...
go tool cover -func=/tmp/cov.out | grep -E "(driver|mount)"
```

## Related Documentation

- CSI Spec: gRPC error codes (InvalidArgument, Internal, FailedPrecondition)
- Kubernetes: Volume cleanup retry behavior
- Testing: Table-driven test patterns in Go
