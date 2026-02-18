---
phase: 31-scheduled-snapshots
verified: 2026-02-18T05:59:13Z
status: passed
score: 4/4 must-haves verified
---

# Phase 31: Scheduled Snapshots Verification Report

**Phase Goal:** Users can configure automated periodic snapshots with retention-based cleanup deployed as part of the Helm chart
**Verified:** 2026-02-18T05:59:13Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A CronJob runs on a user-configured schedule and creates a VolumeSnapshot targeting a specified PVC | VERIFIED | `helm template --set scheduledSnapshots.enabled=true` renders a CronJob with `schedule: "0 2 * * *"` (default) or custom schedule; inline script calls `kubectl apply` with `kind: VolumeSnapshot` targeting the configured `pvcName` |
| 2 | Snapshots older than the configured retention age are deleted, always keeping at least N most recent | VERIFIED | Rendered CronJob script contains `MAX_COUNT`, `MAX_AGE`, temp-file retention loop with `REMAINING=$((TOTAL - DELETED))` guard and `kubectl delete volumesnapshot` when age exceeds threshold |
| 3 | `helm install` with `scheduledSnapshots.enabled=true` deploys the CronJob; `helm uninstall` removes it cleanly | VERIFIED | `helm template --set scheduledSnapshots.enabled=true` renders ServiceAccount, Role, RoleBinding, CronJob — all gated by single `{{- if .Values.scheduledSnapshots.enabled }}` block in one file, so uninstall removes them atomically |
| 4 | `helm install` with `scheduledSnapshots.enabled=false` (default) deploys nothing extra | VERIFIED | Default `helm template` renders 0 CronJob resources and only 2 ServiceAccounts (controller + node), no extra Role/RoleBinding |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `deploy/helm/rds-csi-driver/templates/scheduled-snapshots.yaml` | CronJob, ServiceAccount, Role, RoleBinding for scheduled snapshot creation and retention cleanup | VERIFIED | 172 lines; contains all four resource types; gated by `{{- if .Values.scheduledSnapshots.enabled }}`; uses `{{- range .Values.scheduledSnapshots.schedules }}` for multiple CronJobs |
| `deploy/helm/rds-csi-driver/values.yaml` | scheduledSnapshots configuration section with schedule, retention, PVC target | VERIFIED | Lines 324-359; contains `scheduledSnapshots:` with `enabled: false`, `schedules`, `image`, `resources` sub-keys; retention with `maxCount: 7` and `maxAge: "168h"` defaults |
| `deploy/helm/rds-csi-driver/values.schema.json` | JSON Schema validation for scheduledSnapshots values | VERIFIED | Lines 196-263; `scheduledSnapshots` object with `enabled`, `schedules` array (required: name, pvcName), `image`, `pullPolicy` enum validation |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `scheduled-snapshots.yaml` | `values.yaml` | Helm template values | WIRED | `.Values.scheduledSnapshots` referenced 8+ times in template; `dig "retention" "maxCount" 7 $schedule` used for nil-safe access |
| `scheduled-snapshots.yaml` | `snapshot.storage.k8s.io/v1` | kubectl create VolumeSnapshot in job script | WIRED | Rendered script contains `kind: VolumeSnapshot` and `apiVersion: snapshot.storage.k8s.io/v1` in the heredoc piped to `kubectl apply -f -` |

### Requirements Coverage

| Requirement | Status | Notes |
|-------------|--------|-------|
| SCHED-01: CronJob creates VolumeSnapshot on configurable schedule | SATISFIED | CronJob schedule field takes `$schedule.schedule \| default "0 2 * * *"`; VolumeSnapshot heredoc present in job script |
| SCHED-02: Retention cleanup deletes snapshots older than configurable age while keeping maxCount most recent | SATISFIED | Temp-file retention loop with `REMAINING` guard (never go below maxCount) and age check (`AGE_SECONDS -gt MAX_AGE_SECONDS`) both present in rendered output |
| SCHED-03: CronJob template included in Helm chart with clean enable/disable toggle | SATISFIED | Default render: 0 CronJobs; enabled render: 1 CronJob per schedule entry; 2 schedules = 2 CronJobs confirmed |

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| None | — | — | — |

`helm lint` reports 0 failures. No TODO/FIXME/placeholder comments found in any rendered output. No empty implementations. Retention script is fully implemented (not stubbed).

### Human Verification Required

#### 1. CronJob Execution Against Real Cluster

**Test:** Deploy with `scheduledSnapshots.enabled=true` targeting a real PVC, wait for the CronJob to fire, then inspect the created VolumeSnapshot and subsequent retention run.
**Expected:** VolumeSnapshot resource appears with labels `rds-csi.srvlab.io/schedule` and `rds-csi.srvlab.io/pvc`; subsequent runs delete old snapshots above maxCount once they exceed maxAge.
**Why human:** Requires a running Kubernetes cluster with VolumeSnapshot CRDs and a provisioned PVC. Cannot verify actual job execution or snapshot readiness programmatically.

#### 2. Helm Uninstall Removes All CronJob Resources

**Test:** `helm install` with `scheduledSnapshots.enabled=true`, then `helm uninstall` the release.
**Expected:** ServiceAccount, Role, RoleBinding, and CronJob are all removed; no orphaned resources remain.
**Why human:** Requires a live cluster; verifying resource deletion needs `kubectl get` against a real namespace.

### Additional Verified Behaviors

- **Multiple schedules:** Two schedule entries produce two CronJobs (confirmed via `helm template` with two schedule entries rendering `kind: CronJob` twice)
- **Custom retention values:** `--set scheduledSnapshots.schedules[0].retention.maxCount=24 --set scheduledSnapshots.schedules[0].retention.maxAge=48h` correctly renders `MAX_COUNT=24` and `MAX_AGE="48h"` in the script
- **Nil-safe retention access:** `dig "retention" "maxCount" 7 $schedule` used in place of `$schedule.retention.maxCount` to handle partial `--set` overrides without nil pointer panics
- **concurrencyPolicy=Forbid:** Present in rendered CronJob spec; prevents overlapping snapshot jobs
- **Namespaced RBAC:** Role (not ClusterRole) used — correct for namespace-scoped VolumeSnapshot resources
- **NOTES.txt:** Conditional section appears when `scheduledSnapshots.enabled=true` showing schedule details and monitoring commands
- **Helper template:** `rds-csi.snapshotScheduleServiceAccountName` defined in `_helpers.tpl` line 120-122

### Commits Verified

| Commit | Description |
|--------|-------------|
| `54f57a9` | feat(31-01): add scheduledSnapshots values, schema, and helper template |
| `900fc7a` | feat(31-01): create CronJob template for scheduled snapshots with retention cleanup |
| `e7d8b76` | feat(31-01): add scheduled snapshots section to NOTES.txt |

All three commits exist in git log and correspond to the files claimed in SUMMARY.md.

---

_Verified: 2026-02-18T05:59:13Z_
_Verifier: Claude (gsd-verifier)_
