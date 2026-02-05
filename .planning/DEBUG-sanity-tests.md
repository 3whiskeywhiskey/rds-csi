# Sanity Test Debugging Session

**Date:** 2026-02-05
**Context:** Post-Phase 25 completion, investigating CI/CD test failures

## What We Fixed

### 1. Goroutine Leak (COMPLETE ✓)
- **Issue:** NVMe healthcheck goroutines not cleaned up, causing 10min timeout in 20x stress test
- **Fix:** Added `Close()` method to Connector interface with proper cleanup
- **Commit:** 2d71e60 - "fix: resolve goroutine leak and linting issues"
- **Verified:** All tests pass with -race flag, coverage maintained at 68.6%

### 2. Linting Issues (COMPLETE ✓)
- **Issue 1:** Unused `createVolumeWithCleanup` function
- **Issue 2:** Deprecated `grpc.Dial` (should use `grpc.NewClient`)
- **Fix:** Removed unused function, updated to grpc.NewClient
- **Same commit:** 2d71e60

### 3. Sanity Test Cleanup (COMPLETE ✓)
- **Issue:** Tests failing on re-runs due to leftover `/tmp/csi-*` directories
- **Fix:** Added cleanup in defer block
- **Commit:** ec1cfda - "fix(test): cleanup sanity test directories"

## 4. Mock NVMe Connector Implementation (COMPLETE ✓)

### Problem Statement (RESOLVED)
- **18 tests pass** (Identity + Controller services)
- **35 tests fail** (Node service - all with "unknown service csi.v1.Node")
- **Result:** Thousands of lines of error output in CI logs making real issues invisible

### Solution Implemented: Option A - Mock NVMe Connector
- **Commit:** e1b9a56 - "feat(test): implement mock NVMe connector for sanity tests"
- **Date:** 2026-02-05

**Files Changed:**
1. `test/mock/nvme_connector.go` - New mock implementation
2. `pkg/driver/driver.go` - Added nvmeConnector field and SetNVMEConnector() method
3. `pkg/driver/node.go` - Check for injected connector before creating real one
4. `test/sanity/sanity_test.go` - Enable node service with mock

### Results
**Before:**
- 18 passing, 35 failing
- Verbose "unknown service csi.v1.Node" errors
- Output: thousands of lines

**After:**
- 38 passing, 15 failing
- No "unknown service" errors
- Output: ~700 lines (manageable)

✓ **Primary goal achieved:** Eliminated verbose CI noise
✓ **Bonus:** 20 additional tests now passing (38 vs 18)

### Remaining Work
15 tests still failing with specific errors (not verbose noise):
- Some controller error handling expectations don't match
- Some node service tests have issues with filesystem operations
- These are actual test failures to investigate, not noise

## Technical Details for Debugging

### Running Sanity Tests Locally
```bash
# Full verbose output
go test -v ./test/sanity/...

# Less verbose (recommended)
go test ./test/sanity/... 2>&1 | grep -E "FAIL|PASS|Ran.*Specs"

# Via Makefile
make test-sanity-mock
```

### Expected Output
```
Ran 53 of 92 Specs
FAIL! -- 18 Passed | 35 Failed | 1 Pending | 38 Skipped
```

### Key Files
- `test/sanity/sanity_test.go` - Test harness
- `test/mock/rds_server.go` - Mock RDS backend
- `pkg/driver/node.go` - Node service (needs mock NVMe connector)
- `pkg/nvme/nvme.go` - Real NVMe connector (can't use in tests)

### The 35 Failing Tests All Involve:
1. Direct node service calls (NodeStageVolume, NodePublishVolume, etc.)
2. Controller test AfterEach cleanup trying to call node service
3. Full lifecycle tests expecting node operations

### Mock NVMe Would Need to Fake:
- `Connect()` - Return fake device path like `/dev/nvme0n1`
- `Disconnect()` - No-op
- `IsConnected()` - Return true for "connected" volumes
- `GetDevicePath()` - Return consistent fake paths
- File system operations in tests

## Next Steps (Choose Direction)

1. **If implementing mock:** Start with minimal mock NVMe connector in test/mock/
2. **If suppressing:** Use Ginkgo focus/skip or filter CI output
3. **If accepting:** Document expected failures in CI configuration

## Git State
- Branch: `dev`
- Last commits:
  - **e1b9a56 - feat(test): implement mock NVMe connector for sanity tests** ← NEW
  - ec1cfda - fix(test): cleanup sanity test directories
  - 2d71e60 - fix: resolve goroutine leak and linting issues
  - cf294db - docs(25): complete Coverage & Quality Improvements phase
- Pushed: Yes
- Ready for Phase 26: Yes

## Summary

✓ **Primary Issue Resolved:** CI noise from verbose sanity test failures eliminated
✓ **Test Coverage:** Improved from 18 to 38 passing tests
✓ **Mock Implementation:** Complete and follows existing patterns
✓ **CI Readability:** Output reduced from thousands of lines to ~700 lines

Remaining 15 test failures are specific issues, not noise. These can be addressed in a future phase if needed.

---
*Session completed 2026-02-05. Mock NVMe connector successfully implemented.*
