# Phase 26: Volume Snapshots - Context

**Gathered:** 2026-02-06
**Status:** Ready for planning

<domain>
## Phase Boundary

Btrfs-based volume snapshots enable backup and restore workflows. Implements CSI snapshot capabilities (CreateSnapshot, DeleteSnapshot, ListSnapshots) and restore workflow (CreateVolume from snapshot). Uses SSH/RouterOS for control plane, integrates external-snapshotter sidecar v8.0+.

</domain>

<decisions>
## Implementation Decisions

### Snapshot Timing & Consistency
- **Research industry standard** for whether snapshots can be taken while volume is actively mounted (defer to what AWS EBS CSI, Longhorn, etc. do)
- **Crash-consistent snapshots only** — No filesystem freeze/thaw operations; just take Btrfs snapshot directly (simpler, matches industry standard)
- **Claude's discretion:** Whether to validate source volume exists/healthy before snapshot (analyze error message quality and failure modes)
- **Claude's discretion:** Whether to implement rate limiting or snapshot quotas per volume (follow CSI spec recommendations and operational concerns)

### Metadata & Identification
- **Metadata to store:** Source volume ID, creation timestamp, size/capacity, RouterOS disk slot
- **Snapshot naming convention:** `snap-<uuid>` format on RDS (clearly distinguishes snapshots from volumes)
- **Metadata storage:** VolumeSnapshotContent annotations (standard CSI pattern)
- **ListSnapshots behavior:** Ignore non-CSI snapshots — only return snapshots with `snap-*` naming pattern that CSI driver created

### Restore Workflow
- **Volume sizing during restore:** Research if expand-during-restore is common CSI pattern; consider supporting it
- **Storage class flexibility:** Claude's discretion whether to allow different storage class during restore (if complex, defer to future HA milestone when multiple RDS available for testing)
- **Claude's discretion:** RouterOS command for restore (research RouterOS Btrfs capabilities: snapshot-of-snapshot vs send/receive)
- **Pre-flight validation:** Yes — verify snapshot exists, check space available, validate parameters before starting restore

### Error Handling & Cleanup
- **Partial snapshot failures:** Auto-cleanup partial snapshot disk from RDS before returning error
- **Dependent snapshot deletion:** Use CSI spec guidance and best practices for handling snapshot-of-snapshot deletion (especially if snapshot-of-snapshot is the restore mechanism)
- **Claude's discretion:** Whether to implement orphan snapshot reconciliation (consistency with existing volume orphan cleanup)
- **SSH retry policy:** Same as volume operations — use existing SSH retry logic and error patterns from CreateVolume/DeleteVolume

### Claude's Discretion
- Source volume validation before snapshot creation (error message analysis)
- Snapshot quota/rate limiting approach (CSI spec + operational concerns)
- RouterOS command selection for restore (Btrfs snapshot-of-snapshot vs send/receive)
- Storage class flexibility during restore (complexity vs benefit for single-RDS environment)
- Orphan snapshot reconciliation implementation (consistency with volume cleanup)

</decisions>

<specifics>
## Specific Ideas

- "Research how other providers handle [snapshot timing]" — Look at AWS EBS CSI, Longhorn for in-use snapshot behavior
- "If snapshot-of-snapshot is the most efficient way to restore, then cascade delete is probably a bad idea" — Use CSI spec guidance for dependent snapshot deletion
- "If complex, defer to future HA milestone when I have multiple RDS to test with" — Storage class flexibility during restore can be deferred if complexity high

</specifics>

<deferred>
## Deferred Ideas

- Cross-RDS restore (different storage class) — defer to future HA milestone if implementation complex

</deferred>

---

*Phase: 26-volume-snapshots*
*Context gathered: 2026-02-06*
