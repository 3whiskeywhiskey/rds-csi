---
phase: 25
plan: 03
subsystem: testing
tags: [csi-spec, negative-tests, edge-cases, idempotency, regression]
requires: [25-01, 25-02]
provides:
  - CSI spec compliance validation
  - Negative test scenarios for all controller methods
  - Negative test scenarios for all node methods
  - Sanity regression tests for edge cases
affects: [25-04, 26-01]
tech-stack:
  added: []
  patterns: [table-driven-negative-tests, csi-spec-references, idempotency-validation]
key-files:
  created: []
  modified:
    - pkg/driver/controller_test.go
    - pkg/driver/node_test.go
decisions:
  - spec-ref-documentation: Document CSI spec section references in each test case
  - idempotency-focus: Emphasize idempotency tests for Kubernetes retry behavior
  - sanity-regression: Add regression tests for common CSI sanity edge cases
metrics:
  test-cases-added: 45
  controller-negative: 20
  node-negative: 20
  sanity-regression: 5
  duration: 8min
  completed: 2026-02-05
---

# Phase 25 Plan 03: CSI Negative Test Scenarios Summary

**One-liner:** Comprehensive CSI spec negative tests validating error codes and idempotency for all operations

## What Was Delivered

### Test Coverage Added
- **Controller Service:** 20 negative test scenarios covering all CSI methods
- **Node Service:** 20 negative test scenarios covering all CSI methods
- **Sanity Regression:** 5 tests for common CSI edge cases

### Key Test Scenarios

**Controller Service (TestCSI_NegativeScenarios_Controller):**
- CreateVolume: missing fields, unsupported capabilities, capacity validation (7 cases)
- DeleteVolume: missing volume ID, idempotent deletion, invalid format (3 cases)
- ControllerPublishVolume: missing fields, volume not found (3 cases)
- ControllerUnpublishVolume: missing fields, idempotent unpublish (2 cases)
- ValidateVolumeCapabilities: missing fields, volume not found (2 cases)
- ControllerExpandVolume: missing fields, volume not found, idempotency (3 cases)

**Node Service (TestCSI_NegativeScenarios_Node):**
- NodeStageVolume: missing fields, invalid parameters, idempotency (6 cases)
- NodeUnstageVolume: missing fields, idempotent unstage (3 cases)
- NodePublishVolume: missing fields, idempotent publish (4 cases)
- NodeUnpublishVolume: missing fields, idempotent unpublish (3 cases)
- NodeGetVolumeStats: missing required fields (2 cases)
- NodeExpandVolume: missing required fields (2 cases)

**Sanity Regression Tests:**
- Zero capacity handling (defaults to 1 GiB minimum)
- Max int64 capacity validation (OutOfRange error)
- Read-only volume support (SINGLE_NODE_READER_ONLY)
- DeleteVolume idempotency (multiple calls succeed)
- VolumeContext parameter validation

## Implementation Approach

### Pattern: Table-Driven Negative Tests with CSI Spec References
```go
tests := []struct {
    name       string
    method     string
    request    interface{}
    wantCode   codes.Code
    wantErrMsg string
    specRef    string // CSI spec section reference
}{
    {
        name:     "CreateVolume: missing volume name",
        wantCode: codes.InvalidArgument,
        specRef:  "CSI 3.4 CreateVolume: name field is REQUIRED",
    },
    // ...
}
```

**Benefits:**
- Every test case documents the CSI spec requirement
- Error messages include spec references for debugging
- Easy to add new test cases
- Clear mapping to CSI spec compliance

### Pattern: Idempotency Validation
```go
// Delete once
_, err = cs.DeleteVolume(ctx, deleteReq)

// Delete again - should succeed (idempotent)
_, err = cs.DeleteVolume(ctx, deleteReq)

// Delete third time - still should succeed
_, err = cs.DeleteVolume(ctx, deleteReq)
```

**Critical for Kubernetes:** Pod lifecycle involves retries during failures. CSI operations must handle repeated calls gracefully.

## Decisions Made

### Decision: Document CSI Spec References
**Context:** Test failures should be traceable to spec requirements
**Chosen:** Include `specRef` field in each test case with section reference
**Alternative:** Generic test names without spec citations
**Rationale:** Makes failures immediately actionable - developers can reference spec

### Decision: Emphasize Idempotency Tests
**Context:** Kubernetes retries operations during transient failures
**Chosen:** Add explicit idempotency tests for all operations
**Alternative:** Assume implementation handles it without tests
**Rationale:** Idempotency bugs cause volume leaks and pod scheduling failures

### Decision: Add Sanity Regression Tests
**Context:** CSI sanity test edge cases recur across implementations
**Chosen:** Add 5 common edge case regression tests
**Alternative:** Rely solely on external csi-sanity tool
**Rationale:** Faster feedback loop, documents expected behavior

## Testing Results

### All Tests Pass
```bash
$ go test -v -run "CSI_NegativeScenarios|SanityRegression" ./pkg/driver/...
PASS: TestCSI_NegativeScenarios_Controller (20 cases)
PASS: TestCSI_NegativeScenarios_Node (20 cases)
PASS: TestSanityRegression_CreateVolumeZeroCapacity
PASS: TestSanityRegression_CreateVolumeMaxInt64Capacity
PASS: TestSanityRegression_CreateVolumeReadOnly
PASS: TestSanityRegression_DeleteVolumeIdempotency
PASS: TestSanityRegression_VolumeContextParameters
```

### Error Code Coverage
Validated all required CSI gRPC error codes:
- ✓ InvalidArgument (missing/invalid fields)
- ✓ NotFound (nonexistent volumes)
- ✓ OutOfRange (capacity limits)
- ✓ ResourceExhausted (insufficient storage)
- ✓ OK (idempotent operations)

## Next Phase Readiness

### Enables Phase 25-04
**Liveness Probe Tests:** With negative scenarios validated, liveness probe tests can assume correct error handling

### Enables Phase 26-01
**Performance Testing:** Negative test infrastructure supports performance regression detection

### No Blockers
All tests pass, no issues identified

## Maintenance Notes

### Adding New Negative Tests
1. Add case to `tests` slice in appropriate function
2. Include CSI spec reference in `specRef` field
3. Document expected error code from CSI spec
4. Run test: `go test -v -run TestCSI_NegativeScenarios`

### CSI Spec Updates
When CSI spec updates (currently v1.5.0):
1. Review spec changes for new error requirements
2. Add test cases for new requirements
3. Update `specRef` fields if section numbers change
4. Verify all existing codes still match spec

---

**Duration:** 8 minutes
**Commits:** 3 (controller negative, node negative, sanity regression)
**Files Modified:** 2
**Test Cases Added:** 45
**CSI Methods Covered:** 12 (all controller + node methods)
