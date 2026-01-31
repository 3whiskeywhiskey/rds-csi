# Phase 6: CSI Publish/Unpublish Implementation - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement ControllerPublishVolume and ControllerUnpublishVolume to enforce ReadWriteOnce semantics. The driver will track volume-to-node attachments (using Phase 5's AttachmentManager), reject conflicting attachment requests, and return NVMe connection info to NodeStage. This is the core fencing logic that prevents multi-node attachment conflicts.

</domain>

<decisions>
## Implementation Decisions

### Error Handling Strategy
- Return FAILED_PRECONDITION (code 9) when RWO volume is attached to different node — CSI spec standard
- Error message includes which node currently has the volume: "Volume attached to node-A, cannot attach to node-B"
- Post Kubernetes events to PVC when attachment conflicts occur — operators see via `kubectl describe pvc`
- Retry strategy for RDS SSH failures: Claude's discretion based on existing patterns

### Node Validation Behavior
- Before rejecting a publish due to conflict, verify the blocking node still exists in Kubernetes
- If blocking node no longer exists: auto-clear stale attachment and allow the new publish (self-healing)
- Target node validation: Claude's discretion based on defensive coding practices
- Kubernetes API failure handling during node checks: Claude's discretion (safety vs availability tradeoff)

### Publish Context Format
- Return separate fields: `nvme_address`, `nvme_port`, `nvme_nqn` — everything NodeStage needs
- Include `fs_type` in publish_context for NodeStage convenience
- Key naming convention: snake_case (matches existing patterns)
- NodeStage behavior: Claude decides on publish_context vs volume_context usage — backwards compatibility not a concern, use the correct approach

### Idempotency Behavior
- ControllerPublishVolume for volume already attached to SAME node: return success with publish_context
- ControllerUnpublishVolume for volume not currently attached: return success (already detached = success)
- Concurrent requests for same volume: serialize with VolumeLockManager from Phase 5
- Unpublish requested during in-progress publish: queue behind the lock, process sequentially

### Claude's Discretion
- RDS SSH retry strategy (internal retries vs return error to CO)
- Target node existence validation before publish
- Kubernetes API failure handling (fail-closed vs fail-open)
- publish_context vs volume_context priority in NodeStage

</decisions>

<specifics>
## Specific Ideas

- Error messages should help operators debug: include node names, volume IDs, and suggested actions
- Events pattern should match existing EventPoster usage from v1 milestone
- Self-healing for stale attachments: if the blocking node doesn't exist, there's no conflict

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-csi-publish-unpublish*
*Context gathered: 2026-01-30*
