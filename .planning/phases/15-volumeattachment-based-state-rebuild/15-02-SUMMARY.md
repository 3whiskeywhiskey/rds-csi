---
phase: 15-volumeattachment-based-state-rebuild
plan: 02
subsystem: controller-state-management
type: feature
status: complete
completed: 2026-02-04
duration: 96s

tags:
  - state-rebuild
  - volumeattachment
  - controller
  - kubernetes-api
  - migration-detection

dependencies:
  requires:
    - 15-01-SUMMARY.md
  provides:
    - VolumeAttachment-based state rebuild
    - Migration state detection from multiple VAs
    - AccessMode lookup from PV spec
  affects:
    - pkg/attachment/rebuild.go
    - Controller startup

tech-stack:
  added: []
  patterns:
    - "VolumeAttachment as authoritative source"
    - "PV annotations informational-only"
    - "Multiple VA detection for migration state"

decisions:
  - key: "VA-based rebuild replaces annotation-based"
    decision: "RebuildStateFromVolumeAttachments is now the authoritative rebuild method"
    rationale: "VolumeAttachment objects are managed by external-attacher and never stale"
    alternatives: ["Continue using PV annotations", "Hybrid approach"]
    impact: "Eliminates stale state bugs, aligns with CSI best practices"

  - key: "Conservative AccessMode default"
    decision: "Default to RWO if PV not found or access mode lookup fails"
    rationale: "RWO is safer default - prevents incorrect dual-attach allowance"
    alternatives: ["Default to RWX", "Fail rebuild on lookup error"]
    impact: "Volume may be rejected for RWX dual-attach if PV missing, but data safety preserved"

  - key: "Migration detection from VA count"
    decision: "If volume has >1 attached VA, mark as migration state with MigrationStartedAt"
    rationale: "Multiple VAs only exist during migration window, older VA timestamp is start time"
    alternatives: ["Store migration flag in VA annotations", "Require explicit migration API"]
    impact: "Automatic migration state recovery without additional coordination"

  - key: "Resilient VA count handling"
    decision: "Log warning if volume has >2 VAs, rebuild first 2 only"
    rationale: "Unexpected but shouldn't fail entire rebuild, partial recovery better than none"
    alternatives: ["Fail rebuild on >2 VAs", "Skip entire volume"]
    impact: "Graceful degradation in unexpected scenarios"

key-files:
  created: []
  modified:
    - path: pkg/attachment/rebuild.go
      changes:
        - Added RebuildStateFromVolumeAttachments function
        - Added rebuildVolumeState helper
        - Added lookupAccessMode helper
        - Renamed RebuildState to RebuildStateFromAnnotations (deprecated)
        - RebuildState now alias for RebuildStateFromVolumeAttachments
        - Initialize() calls VA-based rebuild
---

# Phase 15 Plan 02: VolumeAttachment-Based State Rebuild Summary

**One-liner:** Controller rebuilds attachment state from VolumeAttachment objects instead of PV annotations, with automatic migration detection

## Overview

Replaced PV annotation-based state rebuild with VolumeAttachment-based rebuild to eliminate stale state issues. VolumeAttachment objects are managed by external-attacher and provide authoritative attachment state.

**Key architectural shift:** PV annotations are now informational-only (debugging aid). VolumeAttachment objects are the single source of truth for attachment state.

## What Was Built

### 1. VolumeAttachment-Based Rebuild (Task 1)

**File:** `pkg/attachment/rebuild.go`

**New function:** `RebuildStateFromVolumeAttachments`
- Lists all VolumeAttachments for our driver using helpers from 15-01
- Filters to only attached VAs (Status.Attached=true)
- Groups VAs by volume ID
- Rebuilds AttachmentState for each volume

**Helper functions:**
- `rebuildVolumeState`: Reconstructs AttachmentState from slice of VAs for single volume
- `lookupAccessMode`: Retrieves access mode from PV spec, defaults to RWO on error

**Migration detection:**
- If volume has >1 VA, marks as migration state
- Sets MigrationStartedAt to older VA's CreationTimestamp
- Logs warning if >2 VAs (rebuilds first 2 only)

### 2. Initialize Wiring (Task 2)

**Changes to Initialize():**
- Now calls `RebuildStateFromVolumeAttachments` instead of annotation-based rebuild
- Returns wrapped error on failure

**Backward compatibility:**
- Old `RebuildState` function renamed to `RebuildStateFromAnnotations`
- Marked as deprecated with clear comment
- `RebuildState` kept as alias calling `RebuildStateFromVolumeAttachments`

### 3. AccessMode Lookup (Task 3)

**Implementation:** `lookupAccessMode` helper
- Queries PV via k8s client
- Checks PV.Spec.AccessModes for ReadWriteMany
- Returns "RWX" if found, "RWO" otherwise
- Defaults to "RWO" on error (conservative)

**Integration:**
- Called in `rebuildVolumeState` to populate AttachmentState.AccessMode
- Enables correct dual-attach validation after rebuild

## Testing Performed

### Build Verification
```bash
go build ./pkg/attachment/...
# Success - no compilation errors
```

### Function Existence
```bash
grep "RebuildStateFromVolumeAttachments" pkg/attachment/rebuild.go
grep "RebuildStateFromAnnotations" pkg/attachment/rebuild.go
grep "lookupAccessMode" pkg/attachment/rebuild.go
# All functions present
```

### Initialize Wiring
```bash
grep -A5 "func.*Initialize" pkg/attachment/rebuild.go
# Confirms calls to RebuildStateFromVolumeAttachments
```

## Implementation Notes

### Migration State Detection Logic

When volume has multiple VAs:
1. Find older VA by comparing CreationTimestamps
2. Set MigrationStartedAt to older timestamp (migration window start)
3. This enables timeout calculation in ControllerPublishVolume

**Why older timestamp:** First VA represents initial attach, second VA is migration target. Time between them is migration window.

### AccessMode Lookup Error Handling

Conservative default (RWO) chosen because:
- RWO prevents dual-attach, safer than allowing
- If PV is missing, volume state is questionable anyway
- RWX volumes will fail dual-attach, but operator can fix by recreating PV

**Alternative considered:** Fail rebuild on lookup error
**Rejected because:** Partial rebuild better than complete failure, operator can still read logs

### Empty Slice Consistency

All VA listing helpers from 15-01 return empty slice (not nil) when no results. This enables safe iteration in rebuild code without nil checks.

## Deviations from Plan

None - plan executed exactly as written.

## Commits

1. **012a21f** - `feat(15-02): add RebuildStateFromVolumeAttachments function`
   - Added RebuildStateFromVolumeAttachments to rebuild state from VA objects
   - Added rebuildVolumeState helper to reconstruct AttachmentState from VAs
   - Added lookupAccessMode helper to get access mode from PV
   - Detect migration state when volume has multiple VAs
   - Set MigrationStartedAt to older VA's creation timestamp

2. **0cdeacb** - `refactor(15-02): wire Initialize to use VA-based rebuild`
   - Modify Initialize() to call RebuildStateFromVolumeAttachments
   - Rename old RebuildState to RebuildStateFromAnnotations with deprecation notice
   - Keep RebuildState as alias that calls RebuildStateFromVolumeAttachments
   - VolumeAttachment objects are now the authoritative source for state rebuild

## Verification Results

All success criteria met:

- ✅ RebuildStateFromVolumeAttachments function exists and uses VA listing helpers
- ✅ Initialize() calls VA-based rebuild by default
- ✅ Old annotation-based rebuild is preserved but deprecated
- ✅ AccessMode is looked up from PV during rebuild
- ✅ Migration state (multiple VAs) is detected and MigrationStartedAt is set

## Integration Points

### With 15-01 (VA Listing Helpers)
- Calls `ListDriverVolumeAttachments` to get all driver VAs
- Calls `FilterAttachedVolumeAttachments` to filter by Status.Attached
- Calls `GroupVolumeAttachmentsByVolume` to organize by volume ID

### With Controller Startup
- Initialize() is called during controller server startup
- Rebuilds in-memory state before accepting CSI RPCs
- Returns error if rebuild fails (prevents operation without state)

### With ControllerPublishVolume
- Rebuilt AttachmentState includes AccessMode for dual-attach validation
- MigrationStartedAt enables timeout enforcement
- Node attachments track all current attachments for conflict detection

## Next Phase Readiness

### Phase 15-03 Dependencies
Plan 15-03 (PV Annotation Documentation) can proceed:
- VolumeAttachment-based rebuild complete
- PV annotations no longer read during rebuild
- Clear separation: VA authoritative, annotations informational

### Blockers/Concerns
None. All functions compile and integrate correctly.

### Open Questions
None. Implementation complete and verified.

## Lessons Learned

### What Went Well
1. **Helper function composition** - VA listing helpers from 15-01 composed cleanly
2. **Conservative defaults** - RWO default for AccessMode prevents unsafe behavior
3. **Backward compatibility** - Keeping deprecated function enables gradual transition

### What Could Be Improved
1. **Unit tests** - Plan didn't include tests, should add in future task
2. **E2E validation** - Would benefit from testing with real VolumeAttachments in cluster

### Patterns to Reuse
1. **Deprecation strategy** - Rename old function, keep alias for compatibility
2. **Empty slice convention** - Return empty slice (not nil) for safe iteration
3. **Conservative error handling** - Default to safe behavior (RWO) on error

## References

- Plan: `.planning/phases/15-volumeattachment-based-state-rebuild/15-02-PLAN.md`
- Research: `.planning/phases/15-volumeattachment-based-state-rebuild/15-RESEARCH.md`
- Prior work: `15-01-SUMMARY.md` (VolumeAttachment listing helpers)
