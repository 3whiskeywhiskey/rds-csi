---
phase: 21-code-quality-improvements
plan: 02
subsystem: code-quality
tags: [golangci-lint, gocyclo, cyclop, cyclomatic-complexity, linting]

# Dependency graph
requires:
  - phase: 21-01
    provides: golangci-lint configuration with error handling linters
provides:
  - Complexity metrics enforcement via golangci-lint
  - Baseline complexity tracking (max: 44, threshold: 50)
  - Ratcheting plan for gradual improvement
affects: [21-03, 21-04, code-quality, refactoring]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Complexity threshold ratcheting strategy"
    - "Baseline-aware linter configuration"

key-files:
  created: []
  modified:
    - .golangci.yml

key-decisions:
  - "Threshold set to 50 (above current max of 44) to prevent new violations without breaking existing code"
  - "Both gocyclo and cyclop enabled for comprehensive complexity tracking"
  - "Ratcheting plan: 50 -> 30 (v0.8) -> 20 (v1.0)"

patterns-established:
  - "Baseline measurement before enforcement: Always measure current state before setting thresholds"
  - "Conservative initial thresholds: Set above worst case to enable enforcement without breaking builds"
  - "Gradual ratcheting: Plan progressive improvement rather than demanding perfection immediately"

# Metrics
duration: 3.5min
completed: 2026-02-04
---

# Phase 21 Plan 02: Complexity Linter Configuration Summary

**gocyclo and cyclop linters enabled with threshold 50, establishing complexity baseline and preventing future regression**

## Performance

- **Duration:** 3.5 min
- **Started:** 2026-02-04T21:09:00Z
- **Completed:** 2026-02-04T21:12:28Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments

- Enabled gocyclo and cyclop complexity linters in golangci-lint configuration
- Established complexity baseline: max 44 (RecordEvent), threshold 50
- Verified entire codebase passes with zero complexity violations
- Documented ratcheting plan for gradual improvement to industry standards

## Task Commits

Each task was committed atomically:

1. **Task 1: Add gocyclo and cyclop linters** - `fc995a2` (chore)
   - Enabled both linters in .golangci.yml
   - Configured thresholds at 50 (above current max of 44)
   - Added inline documentation of ratcheting plan

2. **Task 2: Verify linter passes** - (verification only, no code changes)
   - Confirmed zero complexity violations with threshold 50
   - Validated linters are enabled and checking properly
   - Documented complexity baseline for future reference

## Files Created/Modified

- `.golangci.yml` - Added gocyclo and cyclop linter configuration with threshold 50

## Complexity Baseline

**Current maximum complexity values:**
- RecordEvent (pkg/security/metrics.go): 44
- ControllerPublishVolume (pkg/driver/controller.go): 43
- NodeStageVolume (pkg/driver/node.go): 36

**Configuration:**
- gocyclo: min-complexity: 50
- cyclop: max-complexity: 50

**Ratcheting plan:**
- v0.8.0: Threshold 50 (baseline, prevents new violations)
- v0.9.0: Reduce to 40 (forces refactor of top 2 functions)
- v1.0.0: Reduce to 30 (industry standard)
- v2.0.0: Reduce to 20 (excellent quality)

## Decisions Made

1. **Set threshold above current maximum (50 vs 44)**
   - Rationale: Prevent new high-complexity code without breaking existing builds
   - Enables enforcement from day 1 without disruptive refactoring
   - Establishes baseline for gradual improvement

2. **Enable both gocyclo and cyclop**
   - gocyclo: Function-level complexity (industry standard tool)
   - cyclop: Package-level complexity (additional perspective)
   - Both use same threshold for consistency

3. **Don't skip tests (skip-tests: false)**
   - Test code should also have reasonable complexity
   - Complex test helpers are hard to maintain
   - Helps prevent "the test is harder to understand than the code" scenarios

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - straightforward linter configuration with clear baseline.

## Verification

All verification criteria met:

1. ✓ `grep -A2 "gocyclo\|cyclop" .golangci.yml` shows linter configuration
2. ✓ `golangci-lint run ./pkg/...` passes without complexity errors (exit code 0 with complexity-only config)
3. ✓ `golangci-lint linters | grep -E "gocyclo|cyclop"` shows both linters enabled

## Next Phase Readiness

**Ready for:** Plan 21-03 (Test coverage metrics) and 21-04 (Static analysis enhancement)

**Foundation established:**
- Complexity metrics now tracked automatically on every lint run
- Clear baseline documented for future refactoring work
- Gradual improvement path defined without disrupting current development

**No blockers or concerns.**

---
*Phase: 21-code-quality-improvements*
*Completed: 2026-02-04*
