---
phase: 08
plan: 01
subsystem: csi-driver
tags:
  - csi
  - rwx
  - kubevirt
  - validation
requires:
  - "07-02" # Volume fencing infrastructure
provides:
  - "RWX capability declaration"
  - "RWX block-only validation"
affects:
  - "08-02" # Next plan will use this capability
  - "08-03" # Multi-attach count enforcement
tech-stack:
  added: []
  patterns:
    - "CSI capability declaration"
    - "Volume access mode validation"
key-files:
  created: []
  modified:
    - pkg/driver/driver.go
    - pkg/driver/controller.go
decisions:
  - id: ROADMAP-4
    decision: "RWX block-only, reject RWX filesystem"
    rationale: "Prevent data corruption from unsynchronized filesystem access"
    alternatives: "Trust users to handle filesystem locking (rejected)"
metrics:
  duration: "92 seconds"
  completed: "2026-02-03"
---

# Phase 08 Plan 01: Add MULTI_NODE_MULTI_WRITER Capability Summary

**One-liner:** Driver now declares RWX support and enforces block-only access mode for safe KubeVirt live migration.

## Objective

Add MULTI_NODE_MULTI_WRITER capability declaration and implement RWX block-only validation to enable KubeVirt live migration while preventing unsafe RWX filesystem volumes.

## What Was Built

### 1. MULTI_NODE_MULTI_WRITER Capability Declaration
**File:** `pkg/driver/driver.go`

Added MULTI_NODE_MULTI_WRITER to the volume capabilities array in `addVolumeCapabilities()`:

```go
d.vcaps = []*csi.VolumeCapability_AccessMode{
    {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
    {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
    {Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}, // NEW
}
```

This advertises to Kubernetes that the driver supports RWX access mode.

### 2. RWX Block-Only Validation
**File:** `pkg/driver/controller.go`

Enhanced the existing `validateVolumeCapabilities()` function to enforce RWX + block-only:

```go
// RWX block-only validation (ROADMAP-4)
if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
    if cap.GetMount() != nil {
        return fmt.Errorf("RWX access mode requires volumeMode: Block. " +
            "Filesystem volumes risk data corruption with multi-node access. " +
            "For KubeVirt VM live migration, use volumeMode: Block in your PVC")
    }
    klog.V(2).Info("RWX block volume capability validated (KubeVirt live migration use case)")
}
```

**Validation behavior:**
- **Accepts:** RWX + volumeMode: Block (for KubeVirt VMs)
- **Rejects:** RWX + filesystem with actionable error message
- **Logs:** Valid RWX block usage for audit trail

This validation runs in two CSI methods:
1. **CreateVolume** - Fails fast at PVC creation time (line 66)
2. **ValidateVolumeCapabilities** - Returns unconfirmed for unsupported combos (line 290)

## Decisions Made

### ROADMAP-4: RWX Block-Only
**Decision:** Reject RWX + filesystem, accept RWX + block only

**Rationale:**
- KubeVirt live migration requires RWX for dual-attach during migration
- KubeVirt VMs use raw block devices (QEMU handles coordination)
- Filesystem volumes with RWX would cause corruption (no cluster-aware filesystem)
- Error message guides users to correct PVC configuration

**Alternatives Considered:**
- Trust users to handle filesystem locking → Rejected (high risk of data corruption)
- Implement cluster filesystem support → Out of scope for v0.5.0

## Implementation Details

### CSI Spec Compliance
- Driver declares MULTI_NODE_MULTI_WRITER in GetPluginCapabilities
- CreateVolume validates capabilities before creating volume
- ValidateVolumeCapabilities returns unconfirmed (not error) for unsupported combos per CSI spec

### Error Messages
Actionable error message for users:
```
RWX access mode requires volumeMode: Block. Filesystem volumes risk data
corruption with multi-node access. For KubeVirt VM live migration, use
volumeMode: Block in your PVC
```

### Testing Strategy
- Existing tests continue to pass
- Manual testing required for RWX PVC scenarios (covered in later plans)

## Verification

All success criteria met:

1. ✅ **MULTI_NODE_MULTI_WRITER in vcaps** - Added to `addVolumeCapabilities()`
2. ✅ **RWX+block accepted** - No error from validateVolumeCapabilities
3. ✅ **RWX+filesystem rejected** - Returns error with "volumeMode: Block" message
4. ✅ **Code compiles** - `go build ./...` succeeds
5. ✅ **Tests pass** - Existing test suite passes

```bash
# Verification commands executed
go build ./...                    # No errors
make test                         # All tests pass
grep MULTI_NODE_MULTI_WRITER      # Found in driver.go and controller.go
grep "RWX access mode requires"   # Found in controller.go
```

## Deviations from Plan

None - plan executed exactly as written.

## Known Limitations

1. **No 2-node enforcement yet** - Next plan (08-02) adds attachment count tracking
2. **No dual-attach validation** - Will be added in 08-02 for ControllerPublishVolume
3. **Manual testing required** - E2E tests for RWX scenarios in later plans

## Next Phase Readiness

### Unblocks
- **08-02 Multi-Attach Count Tracking** - Capability declared, ready for count enforcement
- **08-03 Dual-Attach Restriction** - Validation in place, ready for runtime checks

### Dependencies
None - this plan is self-contained.

### Integration Points
- **ControllerPublishVolume** - Will add RWX-specific attachment logic (08-02)
- **StorageClass** - Users can now create RWX PVCs (block mode only)

## Key Takeaways

1. **Fast-fail validation** - PVC creation fails immediately with actionable error
2. **Safe by default** - Prevents data corruption from filesystem RWX
3. **Clear user guidance** - Error message explains how to fix
4. **Audit trail** - Valid RWX usage logged for monitoring

## Commits

| Commit  | Type | Description                                      |
|---------|------|--------------------------------------------------|
| c2c1523 | feat | Add MULTI_NODE_MULTI_WRITER volume capability   |
| f684360 | feat | Add RWX block-only validation                    |

**Total commits:** 2 (atomic per-task commits)

## Files Modified

- `pkg/driver/driver.go` - Added MULTI_NODE_MULTI_WRITER to vcaps
- `pkg/driver/controller.go` - Added RWX block-only validation

## Metrics

- **Duration:** 92 seconds (~1.5 minutes)
- **Tasks completed:** 2/2
- **Tests:** All existing tests passing
- **Lines of code:** +15 insertions

---

**Status:** ✅ Complete
**Next Plan:** 08-02 Multi-Attach Count Tracking
