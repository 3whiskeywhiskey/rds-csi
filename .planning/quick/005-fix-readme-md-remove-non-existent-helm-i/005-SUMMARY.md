---
phase: quick-005
plan: 01
subsystem: documentation
tags: [readme, github, urls]

# Dependency graph
requires: []
provides:
  - Accurate README.md with correct GitHub URLs and repository name
  - Removed non-existent Helm installation instructions
affects: [documentation]

# Tech tracking
tech-stack:
  added: []
  patterns: []

key-files:
  created: []
  modified:
    - README.md

key-decisions:
  - "Removed non-existent Helm installation section to avoid user confusion"
  - "Updated all URLs from private git.srvlab.io to public github.com/3whiskeywhiskey/rds-csi"
  - "Standardized repository name to rds-csi (from rds-csi-driver)"

patterns-established: []

# Metrics
duration: <1min
completed: 2026-02-06
---

# Quick Task 005: Fix README.md Inaccuracies Summary

**Removed non-existent Helm installation section and updated all URLs to github.com/3whiskeywhiskey/rds-csi**

## Performance

- **Duration:** <1 min (40 seconds)
- **Started:** 2026-02-06T02:34:43Z
- **Completed:** 2026-02-06T02:35:23Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Removed fake "Via Helm (Recommended)" installation section with non-existent helm repo URLs
- Updated all git.srvlab.io URLs to github.com/3whiskeywhiskey/rds-csi
- Changed all rds-csi-driver references to rds-csi throughout
- Marked Helm chart status as unchecked/planned (not complete)

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix all README.md inaccuracies** - `774736f` (docs)

## Files Created/Modified
- `README.md` - Fixed all URLs, removed Helm section, updated repo name

## Changes Made

### 1. Removed Non-existent Helm Installation Section
Deleted lines 126-143 containing:
- `helm repo add rds-csi https://git.srvlab.io/...` (URL doesn't exist)
- `helm install rds-csi rds-csi/rds-csi-driver ...` (helm chart is just empty directory)
- Simplified installation to show only the working kubectl method

### 2. Updated Status Checklist
- Changed `- [x] Helm chart` to `- [ ] Helm chart (planned)` (line 93)
- Reflects actual state: deploy/helm/ contains empty templates/ directory with no Chart.yaml or values.yaml

### 3. Fixed All URLs
- **Go Report Card badge** (line 4): `github.com/whiskey/rds-csi-driver` → `github.com/3whiskeywhiskey/rds-csi`
- **CI badges** (lines 5-6): `3whiskeywhiskey/rds-csi-driver` → `3whiskeywhiskey/rds-csi`
- **Clone URL** (line 310): `ssh://git@git.srvlab.io:2222/whiskey/rds-csi-driver.git` → `https://github.com/3whiskeywhiskey/rds-csi.git`
- **CD command** (line 311): `cd rds-csi-driver` → `cd rds-csi`
- **Issues URL** (line 353): `git.srvlab.io/whiskey/rds-csi-driver/issues` → `github.com/3whiskeywhiskey/rds-csi/issues`

## Decisions Made
- **Remove Helm section entirely:** The helm chart directory exists but is empty (no Chart.yaml, no values.yaml, just empty templates/ folder). Having fake installation instructions creates confusion and wastes users' time.
- **Use public GitHub URLs:** Project is now public on GitHub, so all references should point there instead of private git.srvlab.io
- **Standardize on rds-csi name:** Repository was renamed from rds-csi-driver to rds-csi; documentation should match

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None - all changes were straightforward find-and-replace operations.

## Verification
All checks passed:
- `grep -c "git.srvlab.io" README.md` → 0 (no stale URLs)
- `grep -c "rds-csi-driver" README.md` → 0 (no old repo name)
- `grep -c "Via Helm" README.md` → 0 (Helm section removed)
- `grep -c "helm repo add" README.md` → 0 (no helm commands)
- `grep -c "helm install" README.md` → 0 (no helm commands)
- `grep "3whiskeywhiskey/rds-csi" README.md` → Shows badges, clone URL, issues URL
- `grep "- \[ \] Helm chart" README.md` → Shows unchecked status

## Next Phase Readiness
README.md is now accurate and reflects actual project state. No blockers.

---
*Phase: quick-005*
*Completed: 2026-02-06*
