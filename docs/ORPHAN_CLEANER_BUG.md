# Historical: orphan cleaner NQN filtering bug

**Discovered:** 2026-02-03 during Phase 13 hardware validation
**Current status:** The specific missing-filter issue described below was fixed in the current code. This document records the original finding; it is not a current warning that all orphan cleanup is disabled.

The original cleaner enumerated all NVMe subsystems on a node. On a diskless node, a non-CSI subsystem could have been mistaken for an orphan and disconnected, potentially affecting a host mount such as `/var`.

The current `pkg/nvme/orphan.go` checks each NQN with `NQNMatchesPrefix` before calling `IsOrphanedSubsystem` or disconnecting it. The node plugin invokes this cleaner at startup, and both the Helm chart and raw node manifest set `CSI_MANAGED_NQN_PREFIX`. `pkg/driver/driver.go` also refuses to start a node plugin without a managed NQN prefix.

This resolves the missing-filter failure mode identified in 2026. It does not replace hardware validation of orphan detection and disconnect behavior on the target node and RDS configuration. Keep the prefix scoped to CSI-managed volumes.
