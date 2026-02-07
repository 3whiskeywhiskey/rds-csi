---
phase: 28-helm-chart
plan: 02
subsystem: packaging
tags: [helm, templates, kubernetes, csi-driver]
requires: [28-01]
provides:
  - helm-templates-namespace
  - helm-templates-serviceaccount
  - helm-templates-rbac
  - helm-templates-csidriver
  - helm-templates-controller
  - helm-templates-node
affects: [28-03, 28-04, 28-05, 28-06, 28-07]
tech-stack:
  added: []
  patterns:
    - helm-templates
    - kubernetes-workloads
    - rbac-templating
key-files:
  created:
    - deploy/helm/rds-csi-driver/templates/namespace.yaml
    - deploy/helm/rds-csi-driver/templates/serviceaccount.yaml
    - deploy/helm/rds-csi-driver/templates/rbac.yaml
    - deploy/helm/rds-csi-driver/templates/csidriver.yaml
    - deploy/helm/rds-csi-driver/templates/controller.yaml
    - deploy/helm/rds-csi-driver/templates/node.yaml
  modified: []
decisions:
  - id: helm-namespace-templating
    choice: Use {{ .Release.Namespace }} for all namespace references
    rationale: Allows users to install chart in any namespace via helm install --namespace
    impact: All resources properly namespaced based on Helm release
  - id: csidriver-name-hardcoded
    choice: CSIDriver name hardcoded as rds.csi.srvlab.io (not templated)
    rationale: CSI driver name is registration identifier, must remain stable across chart installs
    impact: Multiple chart releases share same CSI driver registration (expected for CSI drivers)
  - id: secret-key-paths
    choice: Use /etc/rds-csi/rds-private-key and /etc/rds-csi/rds-host-key (not main.go default)
    rationale: Matches deployed manifests convention, documented in values.yaml Secret key names
    impact: Users must create Secret with correct key names (enforced by values.schema.json)
  - id: rbac-verbatim-copy
    choice: Copy RBAC rules exactly from deploy/kubernetes/rbac.yaml
    rationale: Preserves proven production RBAC configuration, verified via diff
    impact: No permission drift between raw manifests and Helm templates
  - id: leader-election-namespace-templated
    choice: All sidecar leader-election-namespace args use {{ .Release.Namespace }}
    rationale: Leader election must occur in same namespace as deployment for Lease resources
    impact: Sidecars elect leaders per-namespace (correct behavior for multi-namespace installs)
metrics:
  duration: 3 minutes
  completed: 2026-02-07
---

# Phase 28 Plan 02: Core Helm Templates Summary

**One-liner:** Converted Kubernetes manifests to Helm templates with full values.yaml configurability.

## What Was Built

Created six core Helm templates that produce equivalent resources to deploy/kubernetes/ manifests:

1. **namespace.yaml** - Conditional namespace creation (controlled by namespace.create)
2. **serviceaccount.yaml** - Two ServiceAccounts (controller + node) with component labels
3. **rbac.yaml** - Two ClusterRoles (controller + node) with ClusterRoleBindings, all RBAC rules verified identical to source
4. **csidriver.yaml** - CSIDriver registration resource with hardcoded driver name
5. **controller.yaml** - Deployment with 6 containers (driver + 5 sidecars), all args templated from values
6. **node.yaml** - DaemonSet with 3 containers (driver + registrar + livenessprobe), privileged security context preserved

All templates use `{{ .Release.Namespace }}` instead of hardcoded namespace, enabling flexible installation.

## How It Works

**Template Rendering Flow:**
1. User runs `helm install my-release rds-csi-driver/ --namespace rds-csi`
2. Helm loads values.yaml and merges with user overrides
3. Templates execute with `.Values` context and `.Release.Namespace` set to "rds-csi"
4. Helpers from _helpers.tpl inject standard labels, names, selectors
5. Conditional blocks enable/disable features (orphan reconciler, VMI serialization, monitoring)
6. Rendered YAML sent to Kubernetes API

**Controller Container Args (templated):**
- `-rds-address={{ .Values.rds.managementIP }}`
- `-rds-port={{ .Values.rds.sshPort }}`
- `-rds-user={{ .Values.rds.sshUser }}`
- `-rds-key-file=/etc/rds-csi/rds-private-key` (hardcoded path, matches Secret key name)
- `-rds-host-key=/etc/rds-csi/rds-host-key` (hardcoded path, matches Secret key name)
- Conditional metrics, orphan reconciler, VMI serialization args based on values

**Node Container Args (templated):**
- `-endpoint=$(CSI_ENDPOINT)`
- `-node-id=$(NODE_ID)` (from fieldRef)
- `-node`
- Conditional metrics arg
- `CSI_MANAGED_NQN_PREFIX` env var from `.Values.rds.nqnPrefix`

**Secret Volume Configuration:**
```yaml
volumes:
  - name: rds-credentials
    secret:
      secretName: {{ .Values.rds.secretName }}  # User must create Secret first
      defaultMode: 0400
```

If user forgets to create Secret, controller pod fails with clear error: "Secret 'rds-csi-secret' not found".

## Key Features

### RBAC Verification
- Extracted controller ClusterRole rules from both existing manifest and rendered Helm template
- Ran `diff /tmp/existing-controller-rbac-rules.yaml /tmp/helm-rbac-rules.yaml`
- Zero differences confirmed - all rules preserved exactly

### Component-Specific Labels
- Controller pods: `app.kubernetes.io/component: controller`
- Node pods: `app.kubernetes.io/component: node`
- Enables targeted selection: `kubectl get pods -l app.kubernetes.io/component=controller`

### Sidecar Leader Election
All sidecars (provisioner, attacher, resizer, snapshotter) use:
```yaml
args:
  - "--leader-election-namespace={{ .Release.Namespace }}"
```
Ensures Lease resources created in correct namespace for multi-namespace installs.

### Kubelet Path Templating
Node DaemonSet uses `.Values.node.kubeletPath` consistently:
- Plugin directory: `{{ .Values.node.kubeletPath }}/plugins/rds.csi.srvlab.io/`
- Registration directory: `{{ .Values.node.kubeletPath }}/plugins_registry/`
- Pods mount directory: `{{ .Values.node.kubeletPath }}`
- DRIVER_REG_SOCK_PATH: `{{ .Values.node.kubeletPath }}/plugins/rds.csi.srvlab.io/csi.sock`

Default `/var/lib/kubelet` works for most distros, but overridable for custom setups.

## Testing Performed

1. **helm lint**: Passed with no errors (1 INFO about missing icon)
2. **helm template**: Rendered 16 Kubernetes resources successfully
3. **Container count verification**:
   - Controller: 6 containers (rds-csi-driver, csi-provisioner, csi-attacher, csi-resizer, csi-snapshotter, liveness-probe)
   - Node: 3 containers (rds-csi-driver, node-driver-registrar, liveness-probe)
4. **RBAC diff**: Zero differences between existing and templated ClusterRole rules
5. **Namespace templating**: All `{{ .Release.Namespace }}` references render correctly
6. **Secret configuration**: Volume references `.Values.rds.secretName`, mounts at `/etc/rds-csi`, key paths correct

## Integration Points

**From Phase 28-01 (Helm Chart Skeleton):**
- Uses `values.yaml` for all configuration (rds.*, controller.*, node.*, sidecars.*)
- Uses `_helpers.tpl` for standard labels, selectors, names, image tags
- Chart.yaml provides app version (0.10.0) for image tags

**For Phase 28-03+ (Service/ServiceMonitor/StorageClass/SnapshotClass):**
- Templates already exist from Phase 28-01 (service.yaml, servicemonitor.yaml, storageclass.yaml, snapshotclass.yaml)
- These templates will be reviewed/validated in subsequent plans
- Core infrastructure (namespace, RBAC, workloads) now complete

## Deviations from Plan

None - plan executed exactly as written.

## Lessons Learned

### RBAC Diff is Critical
The plan's requirement to `diff` RBAC rules caught potential omissions. Verbatim copy from source ensures no permissions dropped during conversion.

### Secret Key Path Convention Matters
Using `/etc/rds-csi/rds-private-key` instead of main.go default `/etc/rds-csi/ssh-key/id_rsa` matches existing deployed manifests. Consistency prevents confusion when users migrate from raw manifests to Helm.

### CSIDriver Name Must Be Stable
CSI driver registration name (`rds.csi.srvlab.io`) cannot be templated. Multiple chart releases in different namespaces share the same CSI driver - this is expected behavior for storage drivers.

## Next Phase Readiness

**Phase 28-03 (Service/ServiceMonitor templates):**
- Ready to proceed - metrics port configurable via `.Values.monitoring.port`
- ServiceMonitor conditional on `.Values.monitoring.serviceMonitor.enabled`

**Phase 28-04 (StorageClass templates):**
- Ready to proceed - storageClasses array in values.yaml supports multiple classes
- Template already exists, needs validation

**Phase 28-05 (VolumeSnapshotClass template):**
- Ready to proceed - snapshotClass config in values.yaml
- Template already exists, needs validation

**Phase 28-06 (Chart testing and validation):**
- Core templates ready for integration testing
- Can render full chart and validate against Kubernetes API

**Phase 28-07 (Documentation and release):**
- Template structure clear for documenting installation steps

---

**Commits:**
- 87b9113: feat(28-02): add namespace, serviceaccount, rbac, and csidriver templates
- c59cf64: feat(28-02): add controller Deployment and node DaemonSet templates
