# Phase 2: Stale Mount Detection and Recovery - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Automatic detection and recovery from stale mounts caused by NVMe-oF controller renumbering after reconnections. Driver detects when mount device path no longer matches the NQN-resolved device, recovers transparently when possible, and reports failures via Kubernetes events. This is internal driver behavior — invisible to pods unless recovery fails.

</domain>

<decisions>
## Implementation Decisions

### Detection strategy
- Check for staleness on every CSI node operation (NodePublishVolume, NodeUnpublishVolume, NodeGetVolumeStats)
- Staleness defined by EITHER condition:
  - Device path mismatch: mount points to /dev/nvme0n1 but NQN now resolves to /dev/nvme1n1
  - Device disappeared: mount points to device that no longer exists in /dev
- Both conditions trigger recovery attempt

### Recovery behavior
- Auto-recover silently — transparent to pods unless recovery fails
- 3 recovery attempts before giving up
- Exponential backoff between attempts (1s, 2s, 4s)
- On success: no Kubernetes event, just proceed with original operation
- On failure: post event, return appropriate gRPC error

### Force unmount policy
- Try normal unmount first
- Escalate to lazy unmount (umount -l) only if normal unmount fails
- Wait 10 seconds for normal unmount before escalating
- Refuse to force unmount if mount is actively in use (open file handles)
- If in use: fail the operation, post event explaining why

### Kubernetes events
- Post events on failures only (no noise on successful recovery)
- Normal events for successful recovery (when event is warranted)
- Warning events for unrecoverable failures
- Attach events to PVC (users watch their PVCs)
- Verbose event content: volume ID, node name, device paths, mount points, timestamps, retry count, what failed

### Claude's Discretion
- Method for getting current mount device (parse /proc/mounts vs mount-utils package)
- Symlink resolution strategy when comparing device paths
- How to detect if mount is in use (fuser vs /proc/*/fd scan)
- Appropriate gRPC error codes for different failure scenarios

</decisions>

<specifics>
## Specific Ideas

- Recovery should be invisible to running pods — they shouldn't notice reconnection happened
- Verbose events help operators diagnose issues without digging through logs
- Refusing force unmount when in use prevents data loss at cost of recovery failure

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-stale-mount-detection*
*Context gathered: 2026-01-30*
