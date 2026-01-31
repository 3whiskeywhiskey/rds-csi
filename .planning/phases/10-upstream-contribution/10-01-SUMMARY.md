---
phase: 10-upstream-contribution
plan: 01
subsystem: upstream-integration
tags: [kubevirt, git, dco, cherry-pick, fork-management]

# Dependency graph
requires:
  - phase: 09-implement-fix
    provides: Fix commits with implementation and tests
provides:
  - Clean upstream-pr branch with DCO-signed commits
  - Branch ready for PR to kubevirt/kubevirt
  - No CI workflow cruft in commit history
affects: [10-02-pr-submission]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - DCO sign-off workflow for upstream contributions
    - Clean branch creation via cherry-pick for separating fork-specific work

key-files:
  created: []
  modified:
    - /tmp/kubevirt-fork (git history in upstream-pr branch)

key-decisions:
  - "Cherry-pick commits from hotplug-fix-v1 to exclude CI workflow files"
  - "Use --signoff flag during cherry-pick to add DCO automatically"
  - "Base upstream-pr branch on upstream/main for clean PR"

patterns-established:
  - "Separate fork-specific changes (CI workflows) from upstream contribution commits"
  - "Always verify no focused test markers before upstream PR"

# Metrics
duration: 2min
completed: 2026-01-31
---

# Phase 10 Plan 01: Prepare Commits for Upstream Summary

**Clean upstream-pr branch with DCO-signed fix commits, ready for contribution to kubevirt/kubevirt**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-31T21:35:54Z
- **Completed:** 2026-01-31T21:38:08Z
- **Tasks:** 3
- **Files modified:** 1 (planning docs)

## Accomplishments
- Verified fork state with hotplug-fix-v1 branch containing fix commits
- Confirmed test file has no focused markers (FDescribe, FIt, FContext)
- Created upstream-pr branch with ONLY fix commits (no CI workflow files)
- Added DCO sign-off (Signed-off-by line) to both commits
- Pushed clean branch to whiskey-works/kubevirt fork

## Task Commits

Each task was committed atomically:

1. **Task 1: Verify fork state and add upstream remote** - (verification only, no commit)
2. **Task 2: Check for focused test markers** - (verification only, no commit)
3. **Task 3: Create clean branch with only fix commits** - `f261119` (feat)

**Plan metadata:** (pending final summary commit)

## Files Created/Modified
- `/tmp/kubevirt-fork/.git` - Created upstream-pr branch with 2 DCO-signed commits

## Decisions Made

**PREP-01: Cherry-pick commits to exclude CI workflow files**
- **Context:** hotplug-fix-v1 branch has 5 commits (2 fix, 3 CI workflows)
- **Decision:** Cherry-pick only cc1b700 (fix) and 6546421 (tests) to upstream-pr branch
- **Rationale:** Upstream doesn't need fork-specific CI configuration
- **Result:** Clean branch with only pkg/virt-controller/watch/vmi files

**PREP-02: Use --signoff flag during cherry-pick for DCO**
- **Context:** KubeVirt requires DCO sign-off on all commits
- **Decision:** Use `git cherry-pick --signoff` to add DCO automatically
- **Rationale:** Simpler than interactive rebase, less error-prone
- **Result:** Both commits have "Signed-off-by: whiskeywhiskey <whiskey@whiskey.works>"

**PREP-03: Base upstream-pr on upstream/main**
- **Context:** Fork's main branch may diverge from upstream
- **Decision:** Create upstream-pr from upstream/main, not origin/main
- **Rationale:** Ensures PR applies cleanly to current upstream codebase
- **Result:** Branch based on upstream commit 54046b51c9 (latest as of 2026-01-31)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

**1. Interactive rebase with sed failed**
- **Problem:** Attempted to use `GIT_SEQUENCE_EDITOR` with sed to add sign-off via rebase
- **Error:** Bash syntax error parsing the exec command
- **Solution:** Aborted rebase, reset to upstream/main, re-cherry-picked with --signoff flag
- **Outcome:** Simpler approach worked perfectly

## Next Phase Readiness

**Ready for Phase 10-02 (PR Submission):**
- ✓ upstream-pr branch exists at whiskey-works/kubevirt
- ✓ Branch has exactly 2 commits (fix + tests)
- ✓ All commits have DCO sign-off
- ✓ No CI workflow files in commit history
- ✓ No focused test markers in test file
- ✓ Branch based on latest upstream/main

**Commit details:**
- Fix: dde549140a "fix(hotplug): wait for new pod volumes ready before deleting old pod"
- Tests: d9622dd922 "test(hotplug): add unit tests for volume readiness check"

**GitHub branch URL:** https://github.com/whiskey-works/kubevirt/tree/upstream-pr

**Next steps (10-02):**
1. Open PR from whiskey-works/kubevirt:upstream-pr to kubevirt/kubevirt:main
2. Reference issues: #6564, #9708, #16520
3. Include validation results from Phase 09-03

---
*Phase: 10-upstream-contribution*
*Completed: 2026-01-31*
