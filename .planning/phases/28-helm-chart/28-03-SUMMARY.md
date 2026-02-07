---
phase: 28-helm-chart
plan: 03
subsystem: deployment
tags: [helm, storage, monitoring, documentation]
dependencies:
  requires: [28-01, 28-02]
  provides: ["storage-templates", "monitoring-templates", "chart-documentation"]
  affects: []
tech-stack:
  added: []
  patterns: ["helm-loops", "triple-conditional", "crd-detection"]
key-files:
  created:
    - deploy/helm/rds-csi-driver/templates/NOTES.txt
    - deploy/helm/rds-csi-driver/README.md
  modified:
    - deploy/helm/rds-csi-driver/templates/storageclass.yaml
    - deploy/helm/rds-csi-driver/templates/snapshotclass.yaml
    - deploy/helm/rds-csi-driver/templates/service.yaml
    - deploy/helm/rds-csi-driver/templates/servicemonitor.yaml
decisions:
  - id: HELM-STORAGE-LOOP
    what: StorageClass template uses range loop over storageClasses array
    why: Supports multiple StorageClasses with different configurations
    impact: Users can define RWO and RWX classes with different parameters
  - id: HELM-SERVICEMONITOR-TRIPLE
    what: ServiceMonitor has triple-conditional (monitoring + serviceMonitor + CRD detection)
    why: Prevents deployment failures on clusters without Prometheus Operator
    impact: Chart works in both Prometheus Operator and non-operator environments
  - id: HELM-COMPONENT-SELECTOR
    what: ServiceMonitor selector includes app.kubernetes.io/component=controller
    why: Matches Service selector to target only controller metrics endpoint
    impact: ServiceMonitor correctly discovers metrics Service
  - id: HELM-NOTES-SECRET-CHECK
    what: NOTES.txt includes Secret prerequisite validation with kubectl command
    why: Most common installation failure is missing or malformed Secret
    impact: Users get immediate feedback on Secret status after installation
  - id: HELM-DISTRIBUTION-GIT
    what: Chart distributed via git repository only (no Helm repo or OCI registry)
    why: Simpler deployment model for single-user/small-team projects
    impact: Users clone repo and reference local path in helm install
metrics:
  duration: 5m 11s
  completed: 2026-02-07
---

# Phase 28 Plan 03: Storage and Monitoring Templates Summary

**One-liner:** Helm chart storage classes with conditional loop, ServiceMonitor with CRD detection, and comprehensive installation documentation

## Objective

Create the feature templates (StorageClass, VolumeSnapshotClass, Service, ServiceMonitor), NOTES.txt post-install instructions, and chart README documentation to provide user-facing chart functionality.

## What Was Built

### Task 1: Storage and Monitoring Templates

**Note:** These templates were already created in Plan 28-02 (commit 87b9113). The files in this plan matched the existing implementation exactly, requiring no changes.

**Templates verified:**

1. **storageclass.yaml** - Loop-based StorageClass creation:
   - Iterates over `.Values.storageClasses` array with `range` loop
   - Creates resources only when `enabled: true`
   - Parameters default to `rds.*` values (storageIP, nvmePort, basePath)
   - Conditional `isDefault` annotation for default StorageClass
   - RWX class (rds-nvme-rwx) disabled by default

2. **snapshotclass.yaml** - Conditional VolumeSnapshotClass:
   - Created only when `snapshotClass.enabled: true`
   - Includes prerequisite comments (CRDs and snapshot-controller)
   - Driver name hardcoded to `rds.csi.srvlab.io`

3. **service.yaml** - Metrics Service for Prometheus:
   - Conditional on `monitoring.enabled`
   - Includes `app.kubernetes.io/component: controller` label
   - Selector targets controller pods only
   - Exposes port from `monitoring.port` (default 9809)

4. **servicemonitor.yaml** - Prometheus Operator integration:
   - Triple-conditional gate:
     1. `monitoring.enabled`
     2. `monitoring.serviceMonitor.enabled`
     3. CRD detection OR `forceEnable`
   - Selector includes both base selectorLabels AND `component: controller`
   - Prevents ServiceMonitor creation on non-Prometheus-Operator clusters
   - Avoids deployment failures from missing CRDs

### Task 2: Chart Documentation

Created two new files:

1. **templates/NOTES.txt** (184 lines):
   - Post-install instructions shown after `helm install`
   - Secret prerequisite validation with exact kubectl command
   - Required Secret keys documented (rds-private-key, rds-host-key, snmp-community)
   - Deployment verification commands (controller, node, CSIDriver)
   - Available StorageClasses list (conditional loop)
   - Conditional sections:
     - Snapshot prerequisites (if snapshotClass.enabled)
     - Monitoring access (if monitoring.enabled)
     - ServiceMonitor status (if serviceMonitor.enabled)
     - Upgrade notice (if Release.IsUpgrade)
   - Documentation links

2. **README.md** (625 lines):
   - Complete chart documentation
   - Prerequisites and distribution (git-only, no Helm repo)
   - Quick Start guide with Secret creation examples
   - Configuration reference table for ALL values.yaml parameters:
     - RDS connection settings
     - Controller settings (replicas, resources, reconcilers)
     - Node plugin settings
     - Sidecar settings (provisioner, attacher, resizer, snapshotter)
     - Monitoring settings (ServiceMonitor, RDS monitoring)
     - StorageClass settings
     - Snapshot settings
   - Secret structure with exact key names (rds-private-key, rds-host-key, snmp-community)
   - Storage Classes section:
     - rds-nvme (RWO) for general-purpose storage
     - rds-nvme-rwx (RWX) for KubeVirt live migration
     - Important note: RWX is NOT for general-purpose shared filesystems
   - Monitoring section:
     - All exposed metrics documented (CSI ops, disk, hardware)
     - Prometheus integration examples (manual and Operator)
   - Upgrading section:
     - Use `--reset-then-reuse-values` (not `--reuse-values`)
     - Chart vs app versioning explained
   - Uninstalling section with PV cleanup warning
   - Troubleshooting guide:
     - Controller pod not starting
     - Volume provisioning failures
     - Node plugin mount failures
     - Snapshot operation failures

## Technical Details

### StorageClass Loop Pattern

```yaml
{{- range $idx, $sc := .Values.storageClasses }}
{{- if $sc.enabled }}
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: {{ $sc.name }}
  {{- if $sc.isDefault }}
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
  {{- end }}
provisioner: rds.csi.srvlab.io
parameters:
  nvmeAddress: {{ $sc.nvmeAddress | default $.Values.rds.storageIP | quote }}
  nvmePort: {{ $sc.nvmePort | default (printf "%d" (int $.Values.rds.nvmePort)) | quote }}
  volumePath: {{ $sc.volumePath | default $.Values.rds.basePath | quote }}
{{- end }}
{{- end }}
```

**Key aspects:**
- Loop variable `$sc` for current StorageClass
- Root context `$` for accessing global values (rds.*)
- Default values use `| default` filter with fallback to rds.* settings
- Conditional annotation block for default class

### ServiceMonitor Triple-Conditional

```yaml
{{- if and .Values.monitoring.enabled .Values.monitoring.serviceMonitor.enabled }}
{{- if or .Values.monitoring.serviceMonitor.forceEnable (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1") }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
spec:
  selector:
    matchLabels:
      {{- include "rds-csi.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: controller
{{- end }}
{{- end }}
```

**Why three conditions:**
1. `monitoring.enabled` - User wants metrics
2. `serviceMonitor.enabled` - User wants Prometheus Operator integration
3. CRD detection OR forceEnable - Prevents failures on clusters without Prometheus Operator

**Component selector:** ServiceMonitor selector MUST include `component: controller` to match the Service selector. Without this, the ServiceMonitor would match any service with the base selectorLabels.

### NOTES.txt Secret Validation

```
1. Verify the Secret exists:

   kubectl get secret {{ .Values.rds.secretName }} -n {{ .Release.Namespace }}

   WARNING: The controller will fail to start if the Secret does not exist.

2. The Secret must contain these keys:
   - rds-private-key: SSH private key for RouterOS CLI authentication
   - rds-host-key: SSH host public key for host verification
   - snmp-community: (optional) SNMP community string for hardware monitoring
```

**Why this matters:** Missing or malformed Secret is the #1 installation failure mode. NOTES.txt provides immediate verification command using the actual configured Secret name and namespace.

## Verification

All verification checks passed:

1. `helm lint deploy/helm/rds-csi-driver/` - **PASS** (0 errors)
2. `helm template` renders 1 enabled StorageClass (rds-nvme) - **VERIFIED**
3. RWX StorageClass NOT rendered (disabled by default) - **VERIFIED**
4. VolumeSnapshotClass rendered with driver rds.csi.srvlab.io - **VERIFIED**
5. Metrics Service rendered on port 9809 - **VERIFIED**
6. ServiceMonitor NOT rendered (disabled by default) - **VERIFIED**
7. ServiceMonitor renders when enabled with `component: controller` selector - **VERIFIED**
8. isDefault annotation appears when `storageClasses[0].isDefault=true` - **VERIFIED**
9. NOTES.txt contains Secret prerequisite check with kubectl command - **VERIFIED**
10. README.md documents all configuration sections and Secret key names - **VERIFIED**

**Template rendering test:**

```bash
# Default rendering
helm template test-release deploy/helm/rds-csi-driver/ --namespace rds-csi
# Output: 1 StorageClass (rds-nvme), no RWX, Service created, no ServiceMonitor

# With ServiceMonitor enabled
helm template test-release deploy/helm/rds-csi-driver/ --namespace rds-csi \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.serviceMonitor.forceEnable=true
# Output: ServiceMonitor created with component=controller in selector

# With isDefault annotation
cat > /tmp/test-values.yaml << EOF
storageClasses:
  - name: rds-nvme
    enabled: true
    isDefault: true
EOF
helm template test-release deploy/helm/rds-csi-driver/ -f /tmp/test-values.yaml
# Output: StorageClass with storageclass.kubernetes.io/is-default-class: "true"
```

## Decisions Made

1. **StorageClass Loop (HELM-STORAGE-LOOP)**: Use `range` loop over `storageClasses` array instead of separate template files. Enables multiple StorageClasses with different configurations (e.g., different filesystem types or mount options) without template duplication.

2. **ServiceMonitor Triple-Conditional (HELM-SERVICEMONITOR-TRIPLE)**: Gate ServiceMonitor creation on three conditions (monitoring + serviceMonitor + CRD detection). Prevents deployment failures on clusters without Prometheus Operator while still allowing manual override via `forceEnable`.

3. **Component Selector in ServiceMonitor (HELM-COMPONENT-SELECTOR)**: ServiceMonitor selector MUST include `app.kubernetes.io/component: controller` to match the Service selector. Without this, ServiceMonitor would match any service with base selectorLabels.

4. **NOTES.txt Secret Validation (HELM-NOTES-SECRET-CHECK)**: Include kubectl command to verify Secret exists using `{{ .Values.rds.secretName }}`. Provides immediate feedback on most common installation failure.

5. **Git-Only Distribution (HELM-DISTRIBUTION-GIT)**: Chart distributed via git repository only (no Helm repository or OCI registry). Simpler for single-user/small-team projects. Users clone repo and reference local path: `helm install rds-csi ./deploy/helm/rds-csi-driver`.

## Files Modified

**Created:**
- `deploy/helm/rds-csi-driver/templates/NOTES.txt` (184 lines)
- `deploy/helm/rds-csi-driver/README.md` (625 lines)

**Templates verified (already created in 28-02):**
- `deploy/helm/rds-csi-driver/templates/storageclass.yaml` (28 lines)
- `deploy/helm/rds-csi-driver/templates/snapshotclass.yaml` (15 lines)
- `deploy/helm/rds-csi-driver/templates/service.yaml` (20 lines)
- `deploy/helm/rds-csi-driver/templates/servicemonitor.yaml` (23 lines)

**Total:** 2 files created, 4 templates verified

## Commits

- `add596a`: docs(28-03): add Helm chart NOTES.txt and README.md

## Testing

**Manual Testing:**

```bash
# Lint chart
helm lint deploy/helm/rds-csi-driver/
# PASS: 1 chart(s) linted, 0 chart(s) failed

# Template with defaults
helm template test-release deploy/helm/rds-csi-driver/ --namespace rds-csi
# Verified: 1 StorageClass, VolumeSnapshotClass, Service, no ServiceMonitor

# Template with ServiceMonitor enabled
helm template test-release deploy/helm/rds-csi-driver/ --namespace rds-csi \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.serviceMonitor.forceEnable=true
# Verified: ServiceMonitor created with component=controller selector

# Template with isDefault
helm template test-release deploy/helm/rds-csi-driver/ --namespace rds-csi \
  -f /tmp/test-values.yaml
# Verified: StorageClass has is-default-class annotation
```

## Next Phase Readiness

**Blocked:** None

**Concerns:** None

**Prerequisites for Phase 28-04:** Wave 2 complete. Ready for Wave 3 (Deployment templates - controller and node).

## Deviations from Plan

**None.** Plan executed exactly as written. Task 1 templates were already created in Plan 28-02, requiring no changes (files matched exactly).

## What's Next

Phase 28-04 will create the Deployment and DaemonSet templates for controller and node plugins, completing the Helm chart's core infrastructure templates.

---

**Phase:** 28-helm-chart (Helm Chart for Production Deployment)
**Wave:** 2 (Storage and Monitoring Templates)
**Duration:** 5m 11s
**Completion:** 2026-02-07
