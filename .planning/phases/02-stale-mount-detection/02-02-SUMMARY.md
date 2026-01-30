# Phase 2 Plan 2: Kubernetes Event Posting Summary

**One-liner:** EventPoster implementation for posting mount failures and recovery events to PVCs using client-go EventRecorder

---

## Metadata

```yaml
phase: 02-stale-mount-detection
plan: 02
subsystem: observability
tags: [kubernetes, events, client-go, observability, troubleshooting]
completed: 2026-01-30
duration: 3 min

# Dependency graph
requires:
  - "02-01: Mount infrastructure (DeviceResolver, MountInfo)"
provides:
  - "EventPoster for Kubernetes event posting"
  - "EventRecorder integration via client-go broadcaster"
affects:
  - "02-04: Stale mount recovery (will use EventPoster)"
  - "Future troubleshooting workflows (events visible in kubectl)"

# Tech tracking
tech-stack:
  added:
    - k8s.io/client-go/tools/record (EventRecorder)
    - k8s.io/client-go/kubernetes/typed/core/v1 (EventInterface)
  patterns:
    - Event broadcasting with klog integration
    - EventSink adapter pattern for context API mismatch
    - Graceful degradation when PVC unavailable

# Files
key-files:
  created:
    - pkg/driver/events.go
  modified:
    - pkg/driver/node.go
    - pkg/driver/driver.go
    - go.mod
    - go.sum

# Decisions
decisions:
  - id: event-reasons
    decision: "Use consistent event reasons for filtering"
    rationale: "EventReasonMountFailure, EventReasonRecoveryFailed, EventReasonStaleMountDetected allow kubectl get events filtering"

  - id: event-types
    decision: "Warning for failures, Normal for informational"
    rationale: "Warning events highlight actionable issues; Normal events provide context without alarming users"

  - id: graceful-pvc-handling
    decision: "Don't fail operations if PVC lookup fails"
    rationale: "PVC might be deleted/terminating during cleanup; event posting is best-effort"

  - id: eventsink-adapter
    decision: "Create adapter for EventInterface → EventSink"
    rationale: "client-go v0.28 EventInterface requires context.Context, but record.EventSink doesn't; adapter bridges the gap"

  - id: integration-placeholder
    decision: "Add postEvent helper but don't use it yet"
    rationale: "Plan 04 will call EventPoster methods; this plan just establishes integration points"
```

---

## What Was Built

### EventPoster Implementation (pkg/driver/events.go)

Created EventPoster that posts Kubernetes events to PVCs using client-go EventRecorder pattern.

**Key features:**
- EventRecorder with broadcaster for event distribution
- Broadcaster logs to klog (visibility in driver logs)
- Broadcaster records to Kubernetes EventSink (visible in kubectl get events)
- Three event posting methods: PostMountFailure, PostRecoveryFailed, PostStaleMountDetected

**Event posting methods:**
1. **PostMountFailure**: Warning event when mount fails
   - Message format: `[volumeID] on [nodeName]: [message]`
   - Reason: `MountFailure`

2. **PostRecoveryFailed**: Warning event when recovery exhausted
   - Includes attempt count and final error
   - Reason: `RecoveryFailed`

3. **PostStaleMountDetected**: Normal event for stale mount detection
   - Includes old/new device paths
   - Reason: `StaleMountDetected`

**Error handling:**
- If PVC lookup fails (deleted, terminating), logs warning and returns nil
- Doesn't fail the operation just because event couldn't be posted
- Best-effort delivery

### NodeServer Integration (pkg/driver/node.go)

Added EventPoster integration point to NodeServer:
- `eventPoster *EventPoster` field in NodeServer struct
- NewNodeServer accepts optional `kubernetes.Interface`
- If k8sClient provided, creates EventPoster; otherwise nil (events disabled)
- `postEvent` helper method (placeholder for Plan 04)

### Driver Updates (pkg/driver/driver.go)

Store k8sClient in Driver and pass to NodeServer:
- Added `k8sClient kubernetes.Interface` field to Driver
- Store from DriverConfig.K8sClient
- Pass to NewNodeServer for event posting

### EventSink Adapter Pattern

Created `eventSinkAdapter` to bridge API mismatch:
- record.EventSink methods don't accept context
- typedcorev1.EventInterface methods require context
- Adapter wraps EventInterface, provides context.Background()

**Adapter methods:**
- `Create(event) → eventInterface.Create(ctx, event, opts)`
- `Update(event) → eventInterface.Update(ctx, event, opts)`
- `Patch(event, data) → eventInterface.Patch(ctx, name, type, data, opts)`

---

## Testing Performed

**Build verification:**
```bash
go build ./pkg/driver/...  # Success
go vet ./pkg/driver/...    # Success
```

**Dependency resolution:**
- Added k8s.io/client-go/tools/record@v0.28.0
- Added github.com/golang/groupcache (transitive dependency)

**No runtime testing yet** - integration happens in Plan 04

---

## Deviations from Plan

None - plan executed exactly as written.

---

## Commits

| Commit | Type | Description |
|--------|------|-------------|
| 69e95cb | feat | Create EventPoster for Kubernetes events |
| 599a01a | feat | Add EventPoster integration point to NodeServer |

**Total commits:** 2

---

## Next Phase Readiness

### Ready for Plan 04 (Stale Mount Recovery)

Plan 04 will:
- Detect stale mounts using DeviceResolver + MountInfo (from Plan 01)
- Attempt recovery with retry logic
- Post events via EventPoster (from this plan)
- Use PostMountFailure when recovery exhausted
- Use PostStaleMountDetected when stale mount detected

### Integration Points Established

1. **EventPoster available in NodeServer** via `ns.eventPoster`
2. **Methods ready to call:**
   - `PostMountFailure(ctx, namespace, name, volumeID, nodeName, message)`
   - `PostRecoveryFailed(ctx, namespace, name, volumeID, nodeName, attemptCount, err)`
   - `PostStaleMountDetected(ctx, namespace, name, volumeID, nodeName, oldPath, newPath)`
3. **Graceful when disabled** - nil check prevents errors

### PVC Namespace/Name Extraction

Plan 04 will need to extract PVC namespace/name from CSI request.

Options:
1. Parse from volume context (if controller sets it)
2. Use PV lookup → ClaimRef (requires k8s API call)
3. Cache PVC info on NodeStageVolume (store in metadata file)

**Recommendation:** Cache PVC info during NodeStageVolume to avoid lookups on every operation.

---

## Lessons Learned

### client-go Event API Evolution

client-go v0.28 changed EventInterface to require `context.Context`, but `record.EventSink` interface didn't update. This creates adapter requirement.

**Pattern:** When EventSink and EventInterface APIs diverge, create thin adapter with `context.Background()`.

### Event Posting Best Practices

- **Attach to user-facing resources** (PVCs, not PVs) for discoverability
- **Use consistent Reasons** for filterability (`kubectl get events --field-selector reason=MountFailure`)
- **Verbose messages** with context (volume ID, node, device paths)
- **Best-effort delivery** - don't fail operations if event posting fails

### Broadcaster Pattern Benefits

EventBroadcaster enables:
1. **Multiple sinks** - klog AND Kubernetes API
2. **Fan-out** - single Event() call reaches all sinks
3. **Structured logging** - klog integration with consistent format

---

## Documentation Impact

### Files to Update

- **docs/troubleshooting.md** (create in future): Event filtering examples
- **README.md**: Mention event posting for mount failures

### Event Filtering Examples

```bash
# Show mount failures for specific PVC
kubectl get events --field-selector reason=MountFailure,involvedObject.name=my-pvc

# Show all stale mount detections
kubectl get events --field-selector reason=StaleMountDetected

# Show recovery failures
kubectl get events --field-selector reason=RecoveryFailed
```

---

**Plan complete. Events ready for integration in Plan 04.**
