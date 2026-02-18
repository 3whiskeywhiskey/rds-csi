---
phase: 31-scheduled-snapshots
plan: 01
subsystem: infra
tags: [helm, kubernetes, cronjob, volumesnapshot, snapshot-retention]

# Dependency graph
requires:
  - phase: 29-snapshot-implementation-fix
    provides: VolumeSnapshot CSI implementation via /disk add copy-from
  - phase: 30-snapshot-validation
    provides: CSI sanity tests validating snapshot lifecycle (70/70 pass)
provides:
  - Helm CronJob template for automated periodic VolumeSnapshot creation
  - Retention-based cleanup that keeps at least N most recent snapshots
  - Namespaced RBAC (ServiceAccount, Role, RoleBinding) for snapshot-scheduler
  - JSON schema validation for scheduledSnapshots values
  - NOTES.txt section showing scheduled snapshot config at install time
affects:
  - future helm chart users wanting automated data protection

# Tech tracking
tech-stack:
  added: [bitnami/kubectl image for CronJob pods]
  patterns:
    - "Helm conditional resource gating with {{- if .Values.feature.enabled }}"
    - "dig helper for nil-safe nested value access in Helm templates"
    - "Temp file approach for shell retention loop to avoid subshell scope issues"
    - "One CronJob per schedule entry via range over .Values.scheduledSnapshots.schedules"

key-files:
  created:
    - deploy/helm/rds-csi-driver/templates/scheduled-snapshots.yaml
  modified:
    - deploy/helm/rds-csi-driver/values.yaml
    - deploy/helm/rds-csi-driver/values.schema.json
    - deploy/helm/rds-csi-driver/templates/_helpers.tpl
    - deploy/helm/rds-csi-driver/templates/NOTES.txt

key-decisions:
  - "Used dig helper (not .retention.maxCount) for nil-safe access when retention not set via --set"
  - "Temp file at /tmp/snapshots.txt for retention loop to avoid pipe-subshell DELETED counter bug"
  - "Namespaced Role (not ClusterRole) since VolumeSnapshots are namespace-scoped resources"
  - "concurrencyPolicy=Forbid prevents overlapping snapshot jobs for same schedule"
  - "bitnami/kubectl:1.28 as the CronJob image (Debian-based, GNU date compatible)"

patterns-established:
  - "Snapshot labels rds-csi.srvlab.io/schedule and rds-csi.srvlab.io/pvc enable targeted retention cleanup"
  - "Retention respects both maxCount (floor) and maxAge (age-based deletion above floor)"

# Metrics
duration: 3min
completed: 2026-02-18
---

# Phase 31 Plan 01: Scheduled Snapshots Helm Template Summary

**Helm CronJob template for automated VolumeSnapshot creation with configurable retention using bitnami/kubectl and kubectl apply inline YAML**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-18T05:52:29Z
- **Completed:** 2026-02-18T05:55:32Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- Added `scheduledSnapshots` section to values.yaml with enabled=false default (opt-in)
- Created CronJob template that generates ServiceAccount, Role, RoleBinding, and one CronJob per schedule entry
- Retention cleanup script deletes snapshots older than maxAge while always keeping at least maxCount most recent
- JSON schema validates all scheduledSnapshots configuration fields
- NOTES.txt shows schedule details and monitoring commands when feature is enabled

## Task Commits

Each task was committed atomically:

1. **Task 1: Add scheduled snapshot values, schema, and helper template** - `54f57a9` (feat)
2. **Task 2: Create CronJob template with snapshot creation and retention cleanup** - `900fc7a` (feat)
3. **Task 3: Update NOTES.txt with scheduled snapshot information** - `e7d8b76` (feat)

**Plan metadata:** TBD (docs: complete plan)

## Files Created/Modified
- `deploy/helm/rds-csi-driver/templates/scheduled-snapshots.yaml` - ServiceAccount, Role, RoleBinding, CronJob resources (gated by scheduledSnapshots.enabled)
- `deploy/helm/rds-csi-driver/values.yaml` - scheduledSnapshots section with enabled, schedules, image, resources
- `deploy/helm/rds-csi-driver/values.schema.json` - JSON schema for scheduledSnapshots validation
- `deploy/helm/rds-csi-driver/templates/_helpers.tpl` - rds-csi.snapshotScheduleServiceAccountName helper
- `deploy/helm/rds-csi-driver/templates/NOTES.txt` - Conditional scheduled snapshot section

## Decisions Made
- **dig helper for nil-safe access:** When using `--set scheduledSnapshots.schedules[0].name=x` without specifying retention, `.retention` is nil. Used `dig "retention" "maxCount" 7 $schedule` instead of `$schedule.retention.maxCount` to avoid nil pointer errors.
- **Temp file for retention loop:** Used `/tmp/snapshots.txt` + `while read < file` instead of piped `while read` to prevent DELETED counter from being lost in subshell scope.
- **Namespaced Role not ClusterRole:** VolumeSnapshots are namespace-scoped resources so a namespaced Role is both sufficient and safer (principle of least privilege).
- **concurrencyPolicy=Forbid:** Prevents overlapping snapshot creation/retention jobs for the same PVC schedule.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed nil pointer for retention values when using --set**
- **Found during:** Task 2 (CronJob template verification)
- **Issue:** `$schedule.retention.maxCount` panicked with "nil pointer evaluating interface {}.maxCount" when `--set` created the schedule entry without a retention sub-object
- **Fix:** Replaced `$schedule.retention.maxCount | default 7` with `dig "retention" "maxCount" 7 $schedule` which safely traverses nil maps
- **Files modified:** deploy/helm/rds-csi-driver/templates/scheduled-snapshots.yaml (also applied same fix in NOTES.txt)
- **Verification:** `helm template ... --set scheduledSnapshots.schedules[0].name=daily` rendered successfully
- **Committed in:** 900fc7a (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - Bug)
**Impact on plan:** Nil-safe fix essential for correct operation with minimal --set overrides. No scope creep.

## Issues Encountered
- `helm template --show-only templates/NOTES.txt` returns an error (Helm does not expose NOTES.txt via show-only). Used `helm lint` to verify NOTES.txt syntax instead — 0 failures confirmed.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Scheduled snapshots Helm feature is complete and ready for use
- Users can enable with `scheduledSnapshots.enabled=true` and configure PVC targets per schedule
- Hardware validation against real RDS (Phase 31 main focus) remains the primary open item

## Self-Check: PASSED

All files present, all commits verified, all key content confirmed.

---
*Phase: 31-scheduled-snapshots*
*Completed: 2026-02-18*
