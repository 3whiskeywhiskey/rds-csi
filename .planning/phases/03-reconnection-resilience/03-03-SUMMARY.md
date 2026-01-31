---
phase: 03-reconnection-resilience
plan: 03
title: "Node Integration and Orphan Cleanup"
completed: 2026-01-30

# Execution Metadata
execution:
  duration: 3 min
  tasks_completed: 4/4
  commits:
    - hash: "4112792"
      type: feat
      scope: "03-03"
      description: "integrate node resilience and orphan cleanup"

# Traceability
subsystem: "nvme-tcp"
tags: ["node", "resilience", "orphan-cleanup", "sysfs"]

# Dependency Graph
dependencies:
  requires: ["03-01", "03-02"]
  provides: ["node-resilience-integration", "orphan-cleanup"]
  affects: ["03-04"]

# Technical Tracking
tech-stack:
  patterns:
    - "sysfs enumeration via nvme-subsystem"
    - "best-effort startup cleanup"
    - "connection parameter extraction from VolumeContext"

# File Changes
key-files:
  created:
    - pkg/nvme/orphan.go
  modified:
    - pkg/driver/node.go
    - pkg/nvme/resolver.go
    - pkg/nvme/sysfs.go
    - cmd/rds-csi-plugin/main.go

# Decisions
decisions:
  - key: orphan-cleanup-non-fatal
    choice: "Orphan cleanup failures logged as warnings, don't block startup"
    rationale: "Cleanup is best-effort; startup reliability more important than cleanup success"
  - key: separate-connector-for-cleanup
    choice: "Create fresh connector instance for orphan cleanup"
    rationale: "Driver creates node server internally; cleanup runs before gRPC server starts"
  - key: subsystem-enumeration-via-sysfs
    choice: "Use /sys/class/nvme-subsystem for NQN enumeration"
    rationale: "More reliable than parsing nvme list-subsys output; works without nvme-cli"
---

# Phase 03 Plan 03: Node Integration and Orphan Cleanup Summary

NodeStageVolume now extracts connection parameters from VolumeContext and uses ConnectWithRetry for resilient connections. Orphaned NVMe subsystems are detected and cleaned up on node startup.

## What Was Done

### Task 1: NodeStageVolume Connection Parameter Extraction
- Added strconv import to node.go
- Extract ctrlLossTmo, reconnectDelay, keepAliveTmo from VolumeContext
- Initialize ConnectionConfig with defaults, override with parsed values
- Log connection config at V(2) before connecting
- Changed from Connect() to ConnectWithRetry() for exponential backoff

### Task 2: ListConnectedSubsystems Method
- Added ListSubsystemNQNs to SysfsScanner in sysfs.go
  - Scans /sys/class/nvme-subsystem/*/subsysnqn
  - Returns nil slice if directory doesn't exist (no subsystems)
  - Skips entries with read errors gracefully
- Added ListConnectedSubsystems to DeviceResolver
  - Delegates to scanner.ListSubsystemNQNs()
  - Provides clean API for orphan detection

### Task 3: OrphanCleaner Implementation
- Created pkg/nvme/orphan.go with:
  - OrphanCleaner struct holding connector and resolver
  - NewOrphanCleaner constructor using connector.GetResolver()
  - CleanupOrphanedConnections method:
    - Scans all NQNs via ListConnectedSubsystems
    - Checks each with IsOrphanedSubsystem
    - Disconnects orphaned subsystems
    - Respects context cancellation
    - Best-effort: individual failures don't stop cleanup

### Task 4: Main.go Integration
- Added context and nvme imports to main.go
- Added orphan cleanup before driver creation (node mode only):
  - Creates fresh connector for cleanup
  - 2-minute timeout for cleanup operation
  - Logs warning on failure, doesn't block startup

## Verification Results

- `go build ./...` - All packages compile successfully
- `go test -short ./...` - All tests pass
- NodeStageVolume calls ConnectWithRetry with extracted connection config
- ListConnectedSubsystems method scans sysfs nvme-subsystem directory
- OrphanCleaner uses ListConnectedSubsystems + IsOrphanedSubsystem
- main.go calls CleanupOrphanedConnections on node startup

## Deviations from Plan

None - plan executed exactly as written.

## Key Code Patterns

### Connection Parameter Extraction (node.go)
```go
connConfig := nvme.DefaultConnectionConfig()
if val, ok := volumeContext["ctrlLossTmo"]; ok {
    if parsed, err := strconv.Atoi(val); err == nil {
        connConfig.CtrlLossTmo = parsed
    }
}
devicePath, err := ns.nvmeConn.ConnectWithRetry(ctx, target, connConfig)
```

### Subsystem Enumeration (sysfs.go)
```go
func (s *SysfsScanner) ListSubsystemNQNs() ([]string, error) {
    subsysDir := filepath.Join(s.Root, "class", "nvme-subsystem")
    entries, err := os.ReadDir(subsysDir)
    // Read subsysnqn from each subsystem directory
}
```

### Orphan Cleanup Flow (main.go)
```go
if *nodeMode {
    cleanupConnector := nvme.NewConnector()
    cleaner := nvme.NewOrphanCleaner(cleanupConnector)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    if err := cleaner.CleanupOrphanedConnections(ctx); err != nil {
        klog.Warningf("Orphan NVMe cleanup failed (non-fatal): %v", err)
    }
    cancel()
}
```

## Next Phase Readiness

Plan 03-03 completes the node-side resilience integration:
- Connection parameters flow from StorageClass -> VolumeContext -> nvme connect args
- Orphaned connections cleaned up on startup
- ConnectWithRetry provides exponential backoff

Ready for Plan 03-04: Health monitoring and liveness integration.

## Artifacts

| File | Purpose |
|------|---------|
| pkg/driver/node.go | NodeStageVolume with connection parameter extraction and ConnectWithRetry |
| pkg/nvme/sysfs.go | ListSubsystemNQNs for subsystem enumeration |
| pkg/nvme/resolver.go | ListConnectedSubsystems method |
| pkg/nvme/orphan.go | OrphanCleaner for startup cleanup |
| cmd/rds-csi-plugin/main.go | Orphan cleanup integration on node startup |
