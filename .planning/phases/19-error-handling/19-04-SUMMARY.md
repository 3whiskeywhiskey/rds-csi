---
phase: 19-error-handling
plan: 04
subsystem: tooling
tags: [golangci-lint, errorlint, errcheck, linting, code-quality]

# Dependency graph
requires:
  - phase: 19-02
    provides: "Sentinel errors for type-safe classification"
provides:
  - "golangci-lint configuration with error handling linters"
  - "Automated enforcement of error wrapping patterns"
  - "CI-ready linting for future development"
affects: [19-05, 20-codebase-conventions, ci-pipeline]

# Tech tracking
tech-stack:
  added: [".golangci.yml configuration"]
  patterns: ["errorlint for %w enforcement", "errcheck for unchecked error detection"]

key-files:
  created: [".golangci.yml"]
  modified: []

key-decisions:
  - "errorlint and errcheck enabled to enforce Go 1.13+ error patterns"
  - "Test files excluded from strict error wrapping rules for ergonomic testing"
  - "Close() errors excluded from errcheck (common idiomatic pattern)"
  - "wrapcheck disabled as too strict for internal error handling"

patterns-established:
  - "Linter configuration enforces %w for error wrapping in fmt.Errorf"
  - "Automated detection of == comparisons that should use errors.Is"
  - "Automated detection of type assertions that should use errors.As"

# Metrics
duration: 1.5min
completed: 2026-02-04
---

# Phase 19 Plan 04: Error Handling Standardization Summary

**golangci-lint configuration with errorlint and errcheck enforces Go 1.13+ error patterns automatically**

## Performance

- **Duration:** 1.5 min
- **Started:** 2026-02-04T20:23:46Z
- **Completed:** 2026-02-04T20:25:18Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- Created .golangci.yml with error-focused linters (errorlint, errcheck)
- Configured 5-minute timeout matching existing Makefile
- Added test file exclusions for ergonomic testing
- Verified linter configuration catches 22 legitimate error handling issues
- Enabled automated enforcement for future development

## Task Commits

Each task was committed atomically:

1. **Task 1: Create golangci-lint configuration** - `f615c47` (feat)

Task 2 was verification-only (no code changes).

**Plan metadata:** (to be committed)

## Files Created/Modified
- `.golangci.yml` - golangci-lint configuration with errorlint, errcheck, and code quality linters

## Decisions Made

**Linter Configuration:**
- **errorlint enabled** - Enforces %w for error wrapping, errors.Is for comparisons, errors.As for type assertions
- **errcheck enabled** - Catches unchecked errors, excludes Close() methods (idiomatic pattern)
- **Test file exclusions** - Test files excluded from strict error wrapping rules for ergonomic testing
- **wrapcheck disabled** - Too strict for internal error handling patterns in this codebase

**Rationale:** Configuration strikes balance between strict error handling enforcement and practical development ergonomics.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

**Legitimate Error Handling Issues Found:**

During Task 2 verification, the linter correctly identified 22 legitimate error handling issues:

**errorlint violations (20 issues):**
- 6 instances in `pkg/utils/errors.go` - Type assertions on `*SanitizedError` should use `errors.As`
- 4 instances in `pkg/circuitbreaker/breaker.go` - Comparing errors with `==` should use `errors.Is`
- 4 instances in `pkg/rds/ssh_client.go` - Type assertions and comparisons should use `errors.As/Is`
- 2 instances in test files (`breaker_test.go`, `errors_test.go`, `pool_test.go`) - Error comparisons
- 4 instances in `pkg/rds/pool_test.go` - Sentinel error comparisons should use `errors.Is`

**gofmt violations (2 issues):**
- `pkg/attachment/va_lister_test.go:35` - Formatting issue
- `pkg/security/logger.go:222` - Formatting issue

**errcheck violations:** 0 (excellent!)

**Assessment:** These are legitimate issues, not false positives. The 96.1% compliance found in Phase 19-01 was for fmt.Errorf %w usage - these additional issues are error comparison and type assertion patterns. They should be addressed in a future plan (Phase 19-05 or dedicated cleanup phase).

**Status:** Configuration is working correctly. Issues documented for future work. Linter is now enforcing proper error handling patterns for all new code.

## Next Phase Readiness

- golangci-lint configuration complete and ready for CI integration
- 22 pre-existing error handling issues identified for cleanup
- Automated enforcement prevents new error handling issues
- Ready for Phase 19-05 (Document patterns in CONVENTIONS.md)
- Consider dedicated cleanup phase to address the 22 legacy issues

**Blockers:** None

**Recommendations:**
1. Add `make lint` to CI pipeline to enforce on all PRs
2. Fix the 22 error handling issues in a dedicated cleanup plan
3. Fix the 2 gofmt issues (trivial - run `go fmt`)

---
*Phase: 19-error-handling*
*Completed: 2026-02-04*
