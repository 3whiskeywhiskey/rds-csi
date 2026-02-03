# Phase 8: Core RWX Capability - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Enable ReadWriteMany block volumes so KubeVirt VMs can live migrate. The driver accepts dual-node attachment (2-node limit) for RWX block volumes during migration. RWX filesystem volumes are rejected to prevent data corruption. This phase adds capability declaration and attachment logic — migration safety and observability are separate phases.

</domain>

<decisions>
## Implementation Decisions

### Validation Behavior
- Error messages are technical + actionable: "RWX access mode requires volumeMode: Block. Filesystem volumes risk data corruption with multi-node access."
- Claude's discretion: validation point (CreateVolume vs ControllerPublishVolume), warning logs on RWX usage, opt-in requirements

### Dual-Attach Rules
- 2-node limit is fixed (not configurable via StorageClass)
- Error message is migration-aware: "Volume already attached to 2 nodes (migration limit). Wait for migration to complete."
- Track attachment order — know which node attached first (primary) vs second (secondary)
- Claude's discretion: 3rd attach attempt handling (immediate rejection vs brief wait)

### RWO Coexistence
- RWO conflict error should hint about RWX: "Volume attached elsewhere. For multi-node access, use RWX with block volumes."
- Distinct log entries for RWX dual-attach vs RWO conflict — easier debugging
- Claude's discretion: whether RWO behavior changes, detection method (access mode only vs access mode + block check)

### Capability Declaration
- RWX volumes use same topology constraints as RWO — consistent behavior
- Claude's discretion: whether to always advertise MULTI_NODE_MULTI_WRITER, StorageClass approach, capability detail level

### Claude's Discretion
- Validation point selection (CreateVolume recommended)
- Whether to log warnings on valid RWX block usage
- Whether to require StorageClass opt-in for RWX
- 3rd attach attempt handling approach
- RWO behavior change scope
- Access mode + block check combination for detection
- MULTI_NODE_MULTI_WRITER advertisement strategy
- StorageClass recommendations for RWX workloads
- CSI capability documentation approach

</decisions>

<specifics>
## Specific Ideas

- Error messages should guide users toward correct usage, not just state what's wrong
- Migration-aware messaging helps operators understand the 2-node limit context
- Logging should clearly distinguish RWX dual-attach (expected) from RWO conflicts (problems)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 08-core-rwx-capability*
*Context gathered: 2026-02-03*
