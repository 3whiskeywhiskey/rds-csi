---
phase: 14-error-resilience-mount-storm-prevention
plan: 01
subsystem: nvme-storage
tags: [nvme, nqn, validation, safety, orphan-cleanup, go]

# Dependency graph
requires:
  - phase: 13-hardware-validation
    provides: Critical discovery that diskless nodes use NVMe-oF for system volumes
provides:
  - Configurable NQN prefix validation preventing system volume disconnection
  - Fail-fast driver startup enforcing safety configuration
  - NQN prefix filtering in orphan cleaner
affects: [helm-deployment, 14-02-mount-storm-recovery, future-orphan-operations]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "NQN prefix validation at startup (fail-fast pattern)"
    - "Environment variable validation in NewDriver constructor"
    - "Configurable filtering via managedNQNPrefix field"

key-files:
  created:
    - pkg/nvme/nqn.go
    - pkg/nvme/nqn_test.go
  modified:
    - pkg/nvme/orphan.go
    - pkg/nvme/orphan_test.go
    - pkg/driver/driver.go
    - cmd/rds-csi-plugin/main.go

key-decisions:
  - "Driver refuses to start if CSI_MANAGED_NQN_PREFIX not set or invalid (fail-fast safety)"
  - "NQN prefix validation checks NVMe spec compliance (nqn. prefix, colon, 223 byte limit)"
  - "OrphanCleaner requires prefix at construction time (no default, explicit configuration)"

patterns-established:
  - "Safety-critical configuration validated at driver startup before any operations"
  - "NQN prefix filtering pattern for all NVMe operations affecting external volumes"

# Metrics
duration: 4min
completed: 2026-02-03
---

# Phase 14 Plan 01: NQN Prefix Validation and Configurable Filtering Summary

**Driver validates NQN prefix at startup and filters orphan cleaner operations to prevent system volume disconnection on diskless nodes**

## Performance

- **Duration:** 4 minutes
- **Started:** 2026-02-03T23:48:25Z
- **Completed:** 2026-02-03T23:52:30Z
- **Tasks:** 3
- **Files modified:** 6
- **Commits:** 3

## Accomplishments

- NQN validation module with comprehensive test coverage (ValidateNQNPrefix, NQNMatchesPrefix, GetManagedNQNPrefix)
- Driver fails to start if CSI_MANAGED_NQN_PREFIX env var is missing or invalid format
- OrphanCleaner uses configurable NQN prefix instead of hardcoded "pvc-" pattern
- System volumes (nixos-*) protected from accidental disconnection

## Task Commits

Each task was committed atomically:

1. **Task 1: Create NQN validation module** - `759767f` (feat)
   - Created pkg/nvme/nqn.go with ValidateNQNPrefix, NQNMatchesPrefix, GetManagedNQNPrefix
   - Created pkg/nvme/nqn_test.go with 10 test cases covering all validation scenarios
   - 245 lines of code

2. **Task 2: Update orphan cleaner to use configurable prefix** - `f059b2a` (refactor)
   - Added managedNQNPrefix field to OrphanCleaner struct
   - Updated NewOrphanCleaner to accept prefix parameter
   - Replaced hardcoded strings.HasPrefix with NQNMatchesPrefix function call
   - Updated tests to pass prefix to constructor

3. **Task 3: Wire NQN validation into driver startup** - `8afe347` (feat)
   - Added ManagedNQNPrefix to DriverConfig and Driver structs
   - Validation in NewDriver fails fast if node mode enabled without valid prefix
   - Read CSI_MANAGED_NQN_PREFIX env var in main.go startup
   - Pass prefix to OrphanCleaner instantiation
   - Import nvme package in driver for validation

## Files Created/Modified

### Created
- `pkg/nvme/nqn.go` - NQN prefix validation and matching functions (56 lines)
- `pkg/nvme/nqn_test.go` - Comprehensive validation tests (189 lines)

### Modified
- `pkg/nvme/orphan.go` - Configurable prefix field and NQNMatchesPrefix usage
- `pkg/nvme/orphan_test.go` - Updated constructor calls with prefix parameter
- `pkg/driver/driver.go` - NQN validation at startup, managedNQNPrefix field
- `cmd/rds-csi-plugin/main.go` - Read env var, pass to driver config and orphan cleaner

## Decisions Made

1. **Fail-fast validation:** Driver refuses to start if CSI_MANAGED_NQN_PREFIX is not set or invalid. This ensures safety-critical configuration is explicit and correct before any NVMe operations occur.

2. **NVMe spec compliance:** ValidateNQNPrefix checks all NVMe spec requirements (nqn. prefix, colon separator, 223 byte limit) to prevent malformed NQNs from causing kernel driver issues.

3. **No default prefix:** OrphanCleaner and Driver require explicit prefix configuration. This prevents the dangerous pattern of assuming a default that might not match deployment reality (e.g., assuming "pvc-" when deployment uses different pattern).

4. **Environment variable over flag:** Using CSI_MANAGED_NQN_PREFIX env var instead of command-line flag allows configuration via Helm values and Kubernetes ConfigMap, which is more idiomatic for containerized deployments.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation proceeded smoothly. All tests passed on first run, validation logic straightforward, integration points clear.

## Next Phase Readiness

**Critical safety feature complete.** Driver will now refuse to start without proper NQN prefix configuration, preventing the catastrophic failure mode discovered in Phase 13 where orphan cleaner could disconnect system volumes on diskless nodes.

**Next steps:**
1. Update Helm chart to expose CSI_MANAGED_NQN_PREFIX as configurable value (default: "nqn.2000-02.com.mikrotik:pvc-")
2. Redeploy driver with NQN filtering to all worker nodes
3. Resume Phase 13 hardware validation with safety features in place
4. Phase 14-02 can proceed with mount storm recovery (also needs NQN filtering)

**No blockers.** This was a pure safety enhancement with no dependencies on external systems.

---
*Phase: 14-error-resilience-mount-storm-prevention*
*Completed: 2026-02-03*
