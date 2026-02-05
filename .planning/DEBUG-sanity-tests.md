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

### 4. Mock NVMe Connector (COMPLETE ✓)
- **Issue:** 35 tests failing with "unknown service csi.v1.Node" verbose errors
- **Solution:** Implemented mock NVMe connector with device path simulation
- **Commit:** e1b9a56 - "feat(test): implement mock NVMe connector for sanity tests"
- **Result:** 18 → 38 passing tests, eliminated verbose CI noise

### 5. Controller Service Fixes (COMPLETE ✓)
- **Issues:** DeleteVolume idempotency, CreateVolume capacity, error codes
- **Fixes:** Sentinel error checks, validation, proper CSI error codes
- **Commit:** ec59ba8 - "fix(test): resolve 6 sanity test failures"
- **Result:** 38 → 44 passing tests

### 6. Mock Mounter (COMPLETE ✓)
- **Issue:** Node service tests need filesystem operations mocked
- **Solution:** Complete MockMounter with all 11 interface methods
- **Commit:** ea6a656 - "feat(test): add mock mounter and fix stale mount checker"
- **Result:** Build succeeds, infrastructure in place

### 7. Nil Resolver Crash (COMPLETE ✓)
- **Issue:** Nil pointer dereference in recovery/stale checking (line 991, 150)
- **Root Cause:** Mock GetResolver() returns nil, recovery calls resolver.ResolveDevicePath()
- **Solution:** Add nil checks in both recovery.Recover() and stale.GetStaleInfo()
- **Commit:** 459fe48 - "fix(test): handle nil resolver gracefully in recovery and stale checking"
- **Result:** Tests run to completion, no crashes (44/53 passing stable)

## Current Status: 44/53 Passing (9 Remaining Failures)

### Remaining Test Failures

1. **NodeGetInfo - should return appropriate values**
   - **Issue:** Test expects non-nil AccessibleTopology
   - **Current:** Returns nil topology
   - **Fix:** Add topology with zone/region labels to NodeGetInfo response

2. **NodeUnpublishVolume - should remove target path**
   - **Issue:** Unknown - needs investigation
   - **Likely:** Mock mounter not properly tracking/removing paths

3. **NodeGetVolumeStats - should fail when volume is not found**
   - **Issue:** Not returning proper error code
   - **Fix:** Already returns NotFound for missing paths, may need different check

4. **NodeGetVolumeStats - should fail when volume does not exist on specified path**
   - **Issue:** Similar to #3, different test condition
   - **Fix:** Verify error code matches CSI spec expectations

5. **NodeExpandVolume - should fail when volume is not found**
   - **Issue:** Error code or message mismatch
   - **Fix:** Return proper CSI NotFound error

6. **NodeExpandVolume - should work if node-expand is called after node-publish**
   - **Issue:** Expansion not working in mock environment
   - **Fix:** Mock mounter ResizeFilesystem implementation or expansion logic

7. **Node Service - should work** (full lifecycle test)
   - **Issue:** Stale mount detected, recovery fails in test mode
   - **Likely:** Mock mounter not properly simulating /proc/mountinfo
   - **Fix:** Inject mock getMountDev function or prevent stale checks in tests

8. **Node Service - should be idempotent** (lifecycle test)
   - **Issue:** Similar to #7
   - **Fix:** Same as #7

9. **ControllerPublishVolume - should fail when the node does not exist**
   - **Issue:** Not implemented (returns Unimplemented)
   - **Fix:** Implement ControllerPublishVolume or return proper error

### Root Causes

**Stale Mount False Positives:**
- Mock mounts not in /proc/mountinfo
- StaleChecker.getMountDev() uses real GetMountDevice by default
- Tests trigger stale detection → recovery attempt → fails gracefully but test expects success
- **Solution:** Inject mock getMountDev that returns mock device paths

**Missing Topology:**
- NodeGetInfo returns nil AccessibleTopology
- CSI sanity expects topology when driver supports it
- **Solution:** Return minimal topology or check if optional

**Mock Mount Tracking:**
- MockMounter tracks mounts but doesn't simulate /proc/mountinfo
- Some tests may check filesystem directly
- **Solution:** Ensure mock properly tracks all mount state

## Next Steps

**Priority 1: Fix Stale Mount False Positives** (affects 2+ tests)
1. Create mock getMountDev function in test/mock/
2. Inject into StaleMountChecker during test setup
3. Return mock device paths from MockMounter's tracked mounts

**Priority 2: Add Topology to NodeGetInfo** (affects 1 test)
1. Add AccessibleTopology to NodeGetInfoResponse
2. Use simple topology like `{"zone": "default"}`

**Priority 3: Fix Remaining Node Service Issues**
1. NodeUnpublishVolume path removal
2. NodeGetVolumeStats error codes
3. NodeExpandVolume implementation

**Priority 4: ControllerPublishVolume**
1. Check if actually needed or can be skipped
2. Implement minimal version if required

## Git State
- Branch: `dev`
- Last commits:
  - **459fe48 - fix(test): handle nil resolver gracefully** ← LATEST
  - 06f967c - docs: update DEBUG notes with mock mounter progress
  - ea6a656 - feat(test): add mock mounter and fix stale mount checker
  - ec59ba8 - fix(test): resolve 6 sanity test failures
- Status: 44/53 passing, infrastructure complete
- Ready: To fix remaining 9 test failures

---
*Session status: No crashes, 44/53 stable, 9 specific failures to fix*
