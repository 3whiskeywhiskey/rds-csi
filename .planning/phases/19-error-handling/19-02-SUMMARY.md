---
phase: 19-error-handling
plan: 02
subsystem: error-handling
tags: [errors, sentinel-errors, type-safety, error-classification]
requires:
  - "19-01: Error wrapping audit"
provides:
  - "Sentinel errors for 10 common conditions"
  - "Type-safe error.Is() classification"
  - "Helper functions for wrapping errors with context"
affects:
  - "19-03: Replace string matching with sentinel checks"
  - "19-04: Adopt WrapError helpers in controller/node"
tech-stack:
  added: []
  patterns: ["sentinel errors", "error wrapping helpers", "errors.Is() classification"]
key-files:
  created: []
  modified:
    - pkg/utils/errors.go
    - pkg/utils/errors_test.go
decisions:
  - id: ERR-02-sentinel-errors
    title: "Define 10 sentinel errors for CSI driver domain"
    rationale: "Enable type-safe error classification with errors.Is() instead of fragile string matching"
    alternatives: ["Custom error types with methods", "Error codes enum"]
    decision: "Simple sentinel errors align with Go stdlib pattern and pkg/rds/pool.go existing sentinels"
    impact: "Enables robust error classification in caller code"
  - id: ERR-02-helper-functions
    title: "Add wrapper helpers for volume/node/device/mount errors"
    rationale: "Consistent formatting and error chain preservation when adding context"
    decision: "Four helpers with optional details parameter for flexible usage"
    impact: "Simplifies adding context while maintaining errors.Is() compatibility"
metrics:
  duration: 82s
  completed: 2026-02-04
---

# Phase 19 Plan 02: Add Sentinel Errors Summary

**One-liner:** Define 10 sentinel errors enabling type-safe error classification with errors.Is() instead of string matching

## What Was Built

Added sentinel error infrastructure to `pkg/utils/errors.go`:

**Sentinel errors defined:**
1. `ErrVolumeNotFound` - volume does not exist
2. `ErrVolumeExists` - volume already exists
3. `ErrNodeNotFound` - node does not exist
4. `ErrInvalidParameter` - invalid parameter provided
5. `ErrResourceExhausted` - insufficient storage capacity
6. `ErrOperationTimeout` - operation timed out
7. `ErrDeviceNotFound` - NVMe device not found
8. `ErrDeviceInUse` - device currently in use
9. `ErrMountFailed` - mount operation failed
10. `ErrUnmountFailed` - unmount operation failed

**Helper functions for wrapping:**
- `WrapVolumeError(sentinel, volumeID, details)` - volume context
- `WrapNodeError(sentinel, nodeID, details)` - node context
- `WrapDeviceError(sentinel, devicePath, details)` - device context
- `WrapMountError(sentinel, target, details)` - mount context

All helpers preserve error chains for `errors.Is()` compatibility.

## Implementation Journey

### Task 1: Define Sentinel Errors
Added 10 sentinel errors at top of `pkg/utils/errors.go` after imports. Pattern follows `pkg/rds/pool.go` which already has `ErrPoolClosed`, `ErrPoolExhausted`, `ErrCircuitOpen`. Each error has clear documentation of its meaning.

**Commit:** `2f7fb27` - feat(19-02): define sentinel errors for common conditions

### Task 2: Add Comprehensive Tests
Created three test functions:
- `TestSentinelErrors` - verifies each sentinel has correct error message
- `TestSentinelErrorsWithWrapping` - tests single and double wrapping with `errors.Is()`
- `TestSentinelErrorsAreDistinct` - ensures sentinels don't match each other

All 10 sentinels tested, all pass.

**Commit:** `1565a92` - test(19-02): add comprehensive sentinel error tests

### Task 3: Add Helper Functions
Added four wrapper helpers at end of `pkg/utils/errors.go`. Each has optional `details` parameter for flexible usage:
- With details: `"volume pvc-123: disk full: resource exhausted"`
- Without details: `"volume pvc-123: resource exhausted"`

All preserve `%w` wrapping for error chain compatibility.

**Commit:** `27c2288` - feat(19-02): add helper functions for wrapping sentinel errors

## Verification Results

```bash
# Package compiles
$ go build ./pkg/utils/...
✓ No errors

# All sentinel tests pass
$ go test ./pkg/utils/... -run TestSentinel -v
✓ TestSentinelErrors - all 10 sentinels have correct messages
✓ TestSentinelErrorsWithWrapping - errors.Is() works with wrapping
✓ TestSentinelErrorsAreDistinct - sentinels are unique

# Full test suite passes (no regressions)
$ make test
✓ 148/148 tests pass across all packages
```

## Deviations from Plan

None - plan executed exactly as written.

## Decisions Made

**ERR-02-sentinel-errors:** Define 10 sentinel errors for CSI driver domain
- **Rationale:** Enable type-safe error classification with errors.Is() instead of fragile string matching
- **Pattern alignment:** Follows existing pattern from pkg/rds/pool.go (ErrPoolClosed, ErrPoolExhausted, ErrCircuitOpen)
- **Simplicity:** Simple sentinel errors align with Go stdlib pattern (io.EOF, fs.ErrNotExist)
- **Impact:** Callers can now use `if errors.Is(err, ErrVolumeNotFound)` instead of `strings.Contains(err.Error(), "not found")`

**ERR-02-helper-functions:** Add wrapper helpers for volume/node/device/mount errors
- **Rationale:** Consistent formatting and error chain preservation when adding context
- **Design:** Four helpers (volume, node, device, mount) with optional details parameter
- **Flexibility:** Optional details enables both concise and detailed error messages
- **Chain preservation:** All use `%w` format verb to maintain errors.Is() compatibility
- **Impact:** Simplifies adding context in driver code: `return WrapVolumeError(ErrVolumeNotFound, volID, "")`

## Metrics

**Test Coverage:**
- 3 new test functions covering 10 sentinel errors
- Single wrapping, double wrapping, and distinctness verified
- 100% of sentinel errors tested

**Code Changes:**
- pkg/utils/errors.go: +68 lines (35 sentinels + 33 helpers)
- pkg/utils/errors_test.go: +101 lines (comprehensive tests)
- Total: +169 lines

**Performance:**
- Build time: <1s
- Test execution: 1.4s for full pkg/utils suite
- No performance impact (sentinels are static errors)

## What's Next

### Immediate Next Steps (Phase 19)
1. **Plan 19-03:** Replace string matching with sentinel error checks
   - Use `errors.Is(err, ErrVolumeNotFound)` instead of string contains
   - Update error classification in pkg/driver/controller.go and pkg/driver/node.go
   - Verify CSI error code mapping still works correctly

2. **Plan 19-04:** Adopt WrapError helpers in controller/node packages
   - Replace bare `fmt.Errorf()` with `WrapVolumeError()` etc.
   - Add consistent context to volume/node/device operations
   - Maintain error chain for debugging

### Integration Points
- **Controller CreateVolume/DeleteVolume:** Use ErrVolumeExists, ErrResourceExhausted sentinels
- **Node NodeStageVolume:** Use ErrDeviceNotFound, ErrMountFailed sentinels
- **Error classification:** Replace string matching in ClassifyError() function
- **Logging:** Sentinel errors provide consistent error messages for log parsing

### Testing Strategy
- Unit tests verify errors.Is() works with wrapped sentinels
- Integration tests ensure CSI error codes still map correctly
- E2E tests validate user-facing error messages remain clear

## Next Phase Readiness

**Phase 20+ (Future Error Handling Work):**
- ✅ Sentinel errors enable reliable error classification
- ✅ Helper functions provide consistent wrapping pattern
- ✅ Error chain preservation enables debugging with errors.Unwrap()
- ✅ Pattern can extend to RDS package if needed (ssh, RouterOS commands)

**Outstanding Concerns:**
None. Implementation is complete and well-tested. Ready to integrate in controller/node packages.

## Artifacts

**Git Commits:**
- 2f7fb27: feat(19-02): define sentinel errors for common conditions
- 1565a92: test(19-02): add comprehensive sentinel error tests
- 27c2288: feat(19-02): add helper functions for wrapping sentinel errors

**Modified Files:**
- pkg/utils/errors.go (+68 lines)
- pkg/utils/errors_test.go (+101 lines)

**Test Coverage:**
- TestSentinelErrors - message verification
- TestSentinelErrorsWithWrapping - errors.Is() compatibility
- TestSentinelErrorsAreDistinct - uniqueness verification
