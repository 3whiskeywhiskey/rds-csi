# Phase 28.1: Fix rds_csi_nvme_connections_active Metric Accuracy

**Status**: INSERTED (Urgent)  
**GitHub Issue**: [#19](https://github.com/3whiskeywhiskey/rds-csi/issues/19)  
**Depends on**: Phase 27 (Documentation complete)

## Problem

The `rds_csi_nvme_connections_active` metric incorrectly reports `0` despite having 16 active VolumeAttachments in production.

### Current Behavior
- **Metric value**: `rds_csi_nvme_connections_active 0`
- **Actual VolumeAttachments**: 16 volumes attached (all showing `ATTACHED true`)
- **Counter metrics**: 
  - `rds_csi_attachment_attach_total{status="success"} 14`
  - `rds_csi_attachment_detach_total{status="success"} 14`

### Root Cause
The metric is derived from the difference between attach and detach counters (`attach_total - detach_total`) rather than directly querying the attachment manager's current state. This breaks when:
1. The controller restarts (counters reset to 0, but attachments persist)
2. Volumes are attached before metrics are initialized
3. Counter drift occurs over time

## Impact

Makes the `rds_csi_nvme_connections_active` metric unreliable for:
- Monitoring dashboards (Grafana)
- Alerting on connection health
- Capacity planning
- Debugging connection issues

Users cannot trust this metric to reflect actual system state.

## Solution

The `rds_csi_nvme_connections_active` gauge should directly query the attachment manager's current state to count active connections, not derive the value from counters.

## Success Criteria

1. ✅ Gauge reports actual count of active NVMe/TCP connections from attachment manager state
2. ✅ Metric value persists correctly across controller restarts
3. ✅ Metric accurately reflects VolumeAttachment count (validated via kubectl)
4. ✅ Unit tests verify metric updates on attach/detach operations
5. ✅ Integration test validates metric accuracy after controller restart

## Next Steps

Run `/gsd:plan-phase 28.1` to create detailed execution plan.
