# Phase 29: Snapshot Implementation Fix - Context

**Gathered:** 2026-02-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Rewrite SSH snapshot commands in pkg/rds/commands.go to use `/disk add copy-from` CoW instead of broken Btrfs subvolume operations. Update CSI controller RPCs (CreateSnapshot, DeleteSnapshot, ListSnapshots, CreateVolume from snapshot) to use the new SSH backend. The v0.10.0 snapshot code is a full rewrite, not a patch.

</domain>

<decisions>
## Implementation Decisions

### Snapshot Identity & Naming
- Snapshot slot name format: `snap-<source-pvc-uuid>-at-<unix-timestamp>`
- `pvc-` and `snap-` are distinct namespaces — no ambiguity between volumes and snapshots
- Snapshot file path: `<basePath>/snap-<source-uuid>-at-<timestamp>.img` — same base directory as volumes, using existing basePath config
- CSI snapshot ID = RDS slot name (direct mapping, no indirection layer)
- Multiple snapshots of the same volume are distinguishable by timestamp suffix

### Copy-from Behavior & Flags
- Omit NVMe flags entirely on snapshot disks (no nvme-tcp-export, no port, no NQN) — snapshots are not network-exported
- CreateSnapshot is synchronous — blocks until copy-from completes, then returns
- Source reference method and file-size handling: Claude's discretion (determine based on RouterOS CLI semantics)

### Restore Workflow
- Restore uses `/disk add copy-from=<snapshot-slot>` with full NVMe export flags — produces a new independent writable volume
- Restored volume gets fresh `pvc-<new-uuid>` identity (CSI provisioner generates UUID) — standard CSI CreateVolume flow with snapshot dataSource
- Allow live snapshots — snapshot while volume is active (crash-consistent), no quiesce requirement
- Restored volume size policy: Claude's discretion (determine based on CSI spec and practical needs)

### Cleanup & Deletion
- Delete snapshot = remove disk entry + verify/delete underlying .img file (belt and suspenders, no orphaned files)
- Always allow snapshot deletion, even if restored volumes exist (CoW copies are independent)
- Snapshots survive source volume deletion — independent CoW copies, user deletes snapshots explicitly
- ListSnapshots filtering: Claude's discretion (determine based on CSI spec requirements and external-snapshotter behavior)

### Claude's Discretion
- copy-from source reference: by slot name vs by file path (whichever is more reliable in RouterOS CLI)
- Snapshot file-size: omit and let copy-from determine from source, or pass explicitly
- Restored volume sizing: whether to allow larger-than-snapshot restores
- ListSnapshots filtering behavior

</decisions>

<specifics>
## Specific Ideas

- The basePath for file storage is configurable (e.g., `metal-csi` or `homelab-csi` for nested clusters) — already templated in the codebase via `VolumeIDToFilePath()` in `pkg/utils/volumeid.go`
- Snapshot naming embeds source lineage (`snap-<source-uuid>-at-<ts>`) so `ListSnapshots` can filter by source without extra metadata
- CSI handles restore vs clone distinction at the user level (dataSource on PVC) — driver always does copy-from, user intent is expressed through kubectl

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 29-snapshot-implementation-fix*
*Context gathered: 2026-02-17*
