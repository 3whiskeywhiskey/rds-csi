# Phase 7: Robustness and Observability - Context

**Gathered:** 2026-01-31
**Status:** Ready for planning

<domain>
## Phase Boundary

Production operators can monitor attachment conflicts and the driver handles stale state gracefully. This includes background reconciliation to clean stale attachments from deleted nodes, grace periods to prevent false conflicts during KubeVirt live migrations, Prometheus metrics for attachment operations, and Kubernetes events posted to PVCs.

</domain>

<decisions>
## Implementation Decisions

### Reconciliation Behavior
- Reconciliation interval is configurable via flag/env var
- Default interval: 5 minutes
- When stale attachment found (node deleted): clear after grace period, not immediately
- Reconcile both directions: check node existence AND check for orphaned PV annotations with no matching in-memory state

### Grace Period Logic
- Grace period duration is configurable via flag/env var
- Default grace period: 30 seconds
- Per-volume tracking: store last-detach timestamp per volume, not global
- If volume was detached less than grace period ago, don't reject new attachment (allows live migration handoff)

### Metrics Design
- Comprehensive metrics coverage:
  - `rds_csi_attachment_attach_total` — counter
  - `rds_csi_attachment_detach_total` — counter
  - `rds_csi_attachment_conflicts_total` — counter
  - `rds_csi_attachment_reconcile_total` — counter
  - `rds_csi_attachment_operation_duration_seconds` — histogram
  - `rds_csi_attachment_grace_period_used_total` — counter (when grace period prevented conflict)
  - `rds_csi_attachment_stale_cleared_total` — counter (reconciler cleanups)
- Metric prefix: `rds_csi_attachment_`

### Event Posting Strategy
- Post events for all attachment operations:
  - `AttachmentConflict` — Warning (already exists from Phase 6)
  - `VolumeAttached` — Normal
  - `VolumeDetached` — Normal
  - `StaleAttachmentCleared` — Normal (reconciler cleanup)
- Event types: Warning for conflicts, Normal for routine operations
- Verbose messages with timestamps, node names, durations, grace period info
- Events posted to PVCs only (not Nodes)

### Claude's Discretion
- Live migration detection: time-based grace vs checking node drain state via k8s API
- Metric labels: balance cardinality vs usefulness (volume_id, node_id decisions)
- Histogram bucket ranges for operation durations
- Exact message format for verbose events

</decisions>

<specifics>
## Specific Ideas

- Grace period specifically designed for KubeVirt live migration handoff timing
- Reconciler should catch drift in both directions (in-memory vs PV annotations)
- Comprehensive metrics give full visibility into attachment lifecycle

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 07-robustness-observability*
*Context gathered: 2026-01-31*
