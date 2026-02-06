# Phase 28: Helm Chart - Context

**Gathered:** 2026-02-06
**Status:** Ready for planning

<domain>
## Phase Boundary

Helm chart for easy deployment and configuration of the RDS CSI driver (controller + node plugin) to Kubernetes clusters. Enables users to install via `helm install` with customizable values for RDS connection, storage classes, monitoring, and component configuration.

</domain>

<decisions>
## Implementation Decisions

### Values structure & customization
- Flat structure at top level: `controller.*`, `node.*`, `rds.*`, `storageClass.*` (simpler --set overrides vs nested)
- Controller customizable settings: image, resources, replica count, tolerations, affinity, logLevel/verbosity
- Node plugin customizable settings: image, resources, node selector, tolerations, kubelet directory path, NVMe connection timeouts/retries
- Monitoring configuration: Full metrics config with `monitoring.port`, `monitoring.serviceMonitor.enabled`, and `rdsMonitoring.enabled` (toggle RDS health metrics separately from CSI metrics)

### Secret management & RDS credentials
- SSH private key via existing secret reference (user creates Secret first, chart references via `rds.secretName`)
- Chart does NOT manage SSH key material - user responsibility
- RDS connection parameters (managementIP, storageIP, sshPort, nvmePort) in values.yaml (non-sensitive config)
- SNMP community string stored in same secret as SSH key (`rds-csi-secret` contains both `ssh-key` and `snmp-community` keys)
- No secret validation in chart - let controller fail at runtime with clear error if secret missing (simpler chart)

### Storage class configuration
- Two StorageClass resources: `rds-nvme` (RWO) and `rds-nvme-rwx` (RWX)
- RWX class disabled by default (`storageClasses.rwx.enabled: false`) - users explicitly enable for KubeVirt live migration
- Both classes use **same base path** on RDS (no segregation by access mode or workload type)
- Default StorageClass configurable via `storageClasses.default.enabled` and `storageClasses.default.name` (don't assume cluster has no default)
- Customizable StorageClass parameters: basePath, fsLabel, volumeBindingMode, reclaimPolicy, allowVolumeExpansion
- RWX documentation must clarify: "Enables KubeVirt live migration, NOT general-purpose shared filesystems"

### Chart repository & versioning
- Chart published in git repository only (no GitHub Pages or OCI registry) - users install via local clone or `--repo`
- Independent chart versioning (chart 1.0.0 can bundle driver 0.10.0)
- `appVersion` in Chart.yaml tracks driver version for clarity
- Helm upgrade uses standard rolling update (controller restarts, DaemonSet rolling restart)
- Chart handles breaking changes via automatic migration (templates accept old and new value formats, log deprecation warnings)

</decisions>

<specifics>
## Specific Ideas

**RWX for KubeVirt:**
- RWX is fully implemented and validated for VM live migration
- RWX enables multi-attach during migration windows (not for concurrent multi-writer filesystems like NFS)
- StorageClass comment should state: "Required for KubeVirt live migration. Not intended for general-purpose RWX filesystems."

**Same storage path rationale:**
- Cannot convert PVC from RWO → RWX (Kubernetes limitation) - requires new PVC + data copy
- CSI driver enforces access mode behavior at runtime (technical isolation)
- Same path keeps all Kubernetes volumes in one location for operational simplicity
- Doesn't artificially restrict RWX to "VMs only" use case

**Secret structure:**
- Single secret (`rds-csi-secret`) contains:
  - `ssh-key`: SSH private key for RouterOS CLI
  - `snmp-community`: SNMP community string for RDS hardware monitoring
- User creates secret before helm install, chart only references it

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 28-helm-chart*
*Context gathered: 2026-02-06*
