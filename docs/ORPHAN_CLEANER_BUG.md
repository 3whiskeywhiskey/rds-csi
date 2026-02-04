# CRITICAL: Orphan Cleaner NQN Filtering Bug

**Status:** NOT CURRENTLY ACTIVE (code exists but unused)
**Severity:** CRITICAL if enabled
**Discovered:** 2026-02-03 during Phase 13 hardware validation

## Problem

The NVMe orphan cleaner in `pkg/nvme/orphan.go` lists **ALL** NVMe subsystems on the node without filtering for CSI-managed volumes. If enabled, it could disconnect the host's own NVMe-oF mounts (e.g., diskless NixOS nodes using NVMe/TCP for /var).

## Root Cause

### Current Implementation

```go
// pkg/nvme/sysfs.go
func (s *SysfsScanner) ListSubsystemNQNs() ([]string, error) {
    // Returns ALL NQNs from /sys/class/nvme-subsystem/*/subsysnqn
    // NO FILTERING - includes host system mounts
}
```

```go
// pkg/nvme/orphan.go
func (oc *OrphanCleaner) CleanupOrphanedConnections(ctx context.Context) error {
    nqns, err := oc.resolver.ListConnectedSubsystems() // Gets ALL subsystems
    for _, nqn := range nqns {
        orphaned, _ := oc.resolver.IsOrphanedSubsystem(nqn)
        if orphaned {
            oc.connector.Disconnect(nqn) // Could disconnect host mounts!
        }
    }
}
```

### Attack Vector

1. Orphan cleaner lists ALL NVMe subsystems (including host's /var mount with different NQN)
2. For each subsystem, checks if "orphaned" by looking for device in expected locations
3. If host's device isn't where scanner expects, marks as orphaned
4. **Disconnects it** → kills host /var mount → kubelet stops → node goes NotReady

### Example Scenario

**Host NVMe mount:**
- NQN: `nqn.2014-08.org.nvmexpress:uuid:12345678-1234-1234-1234-123456789abc` (host's /var)
- Device: `/dev/nvme0n1`
- Mount: `/var`

**CSI Volume:**
- NQN: `nqn.2000-02.com.mikrotik:pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890`
- Device: `/dev/nvme1n1`
- Mount: `/var/lib/kubelet/...`

**What happens if orphan cleaner runs:**
1. Lists both NQNs
2. Checks if host NQN is "orphaned"
3. Can't find device (looking in wrong place or wrong structure)
4. Marks as orphaned
5. **Disconnects host's /var** → node dies

## Current Status

**Code exists but is NOT instantiated or called in production:**

```bash
$ grep -r "NewOrphanCleaner\|CleanupOrphanedConnections" pkg/ --include="*.go" | grep -v "_test.go"
pkg/nvme/orphan.go:func NewOrphanCleaner(...)
pkg/nvme/orphan.go:func (oc *OrphanCleaner) CleanupOrphanedConnections(...)
# No production usage found
```

The orphan cleaner is defined but never used outside of unit tests.

## Required Fix

Add NQN filtering to only check CSI-managed volumes:

```go
// pkg/nvme/sysfs.go
func (s *SysfsScanner) ListSubsystemNQNs() ([]string, error) {
    // ... existing code to read all NQNs ...

    var filteredNQNs []string
    for _, nqn := range allNQNs {
        // Only include CSI-managed volumes
        if strings.HasPrefix(nqn, "nqn.2000-02.com.mikrotik:pvc-") {
            filteredNQNs = append(filteredNQNs, nqn)
        }
    }
    return filteredNQNs, nil
}
```

**OR** add filtering in the orphan cleaner itself:

```go
// pkg/nvme/orphan.go
func (oc *OrphanCleaner) CleanupOrphanedConnections(ctx context.Context) error {
    allNQNs, err := oc.resolver.ListConnectedSubsystems()
    // ... error handling ...

    for _, nqn := range allNQNs {
        // SAFETY: Only check CSI-managed volumes
        if !strings.HasPrefix(nqn, "nqn.2000-02.com.mikrotik:pvc-") {
            klog.V(4).Infof("Skipping non-CSI NQN: %s", nqn)
            continue
        }

        // ... existing orphan detection logic ...
    }
}
```

## Mitigation

**BLOCKER for enabling orphan cleanup:**
- Do NOT enable orphan reconciliation until this is fixed
- Do NOT instantiate `OrphanCleaner` in production code
- Do NOT add periodic orphan cleanup to node plugin

## Testing Requirements

Before enabling orphan cleanup:
1. Deploy to test cluster with host NVMe-oF mounts (like diskless NixOS nodes)
2. Verify orphan cleaner only lists CSI volume NQNs
3. Verify it never touches host system NQNs
4. Test with actual orphaned CSI volumes to ensure cleanup still works

## Related

- **File:** `pkg/nvme/orphan.go`
- **File:** `pkg/nvme/sysfs.go` (`ListSubsystemNQNs`)
- **File:** `pkg/nvme/resolver.go` (`ListConnectedSubsystems`)
- **Phase:** 13 - Hardware Validation
- **Date:** 2026-02-03
