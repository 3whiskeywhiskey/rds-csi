# Codebase Concerns

**Analysis Date:** 2026-02-04

## Code Smells & Quality Issues (v0.7.1 Priority)

### Excessive Logging in Helper Methods

**Issue:** `pkg/security/logger.go` contains 11 nearly-identical helper methods (LogVolumeCreate, LogVolumeDelete, LogVolumeStage, LogVolumeUnstage, LogVolumePublish, LogVolumeUnpublish, LogNVMEConnect, LogNVMEDisconnect, etc.) that follow identical patterns.

**Files:** `pkg/security/logger.go` (lines 221-506)

**Code Pattern (repeated 6+ times):**
```go
// LogVolumeCreate, LogVolumeDelete, LogVolumeStage, LogVolumeUnstage, LogVolumePublish, LogVolumeUnpublish all follow:
func (l *Logger) LogVolume{Op}(...outcome..., ...err..., ...duration...) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolume{Op}Success
		severity = SeverityInfo
		message = "Volume {op} successfully"
	case OutcomeFailure:
		eventType = EventVolume{Op}Failure
		severity = SeverityError
		message = "Volume {op} failed"
	default:
		eventType = EventVolume{Op}Request
		severity = SeverityInfo
		message = "Volume {op} requested"
	}

	event := NewSecurityEvent(...)
		.WithVolume(volumeID, ...)
		.WithOutcome(outcome)
		.WithOperation("{Op}", duration)
	if err != nil {
		event.WithError(err)
	}
	l.LogEvent(event)
}
```

**Impact:** 300+ lines of duplicated logic; difficult to maintain consistency; changes to event format require updates in 6+ places

**Fix for v0.7.1:**
- Extract generic `LogVolumeOperation(op string, volumeID string, outcome EventOutcome, err error, duration time.Duration)` helper
- Reduce 6 methods to 1 configurable method
- Apply same pattern to `LogNVMEConnect` and `LogNVMEDisconnect` (similar duplication)
- Target: Remove 150+ lines of duplication

---

### Inconsistent Error Message Format

**Issue:** Error wrapping inconsistent between `status.Errorf()` and direct string format.

**Files:**
- `pkg/driver/controller.go` (61 error returns)
- `pkg/driver/node.go` (58 error returns)
- `pkg/rds/commands.go` (40+ error returns)

**Examples:**
```go
// controller.go - using %v (verbose)
return nil, status.Errorf(codes.Internal, "failed to create volume on RDS: %v", err)

// node.go - using %v
return nil, status.Errorf(codes.Internal, "failed to stage volume: %v", err)

// rds/commands.go - using %w (wrapped, loses original format)
return fmt.Errorf("failed to create volume: %w", err)
```

**Impact:** Inconsistent error messages in logs make parsing/alerting difficult; loses context in some paths

**Fix for v0.7.1:**
- Establish standard: Use `status.Errorf(codes.X, "operation failed: %w", err)` consistently
- Standardize all internal errors to use `fmt.Errorf()` with `%w`
- Create lint rule or script to enforce this pattern

---

### Logging Verbosity Inconsistency

**Issue:** V() level usage inconsistent across modules; V(3) logs intermediate steps on every error path, creating noise.

**Files:**
- `pkg/driver/controller.go` - DeleteVolume logs 4 separate V(3) statements per operation
- `pkg/rds/commands.go` - DeleteVolume logs volume existence check, then file cleanup steps
- `pkg/mount/mount.go` - Mount recovery logs multiple V(3) statements per path

**Examples:**
```go
// controller.go DeleteVolume
klog.V(3).Infof("Deleting volume %s (path=%s, size=%d bytes, nvme_export=%v)", volumeID, filePath, size, export)
klog.V(3).Infof("Successfully removed disk slot for volume %s", slot)
klog.V(3).Infof("Successfully deleted backing file for volume %s", filePath)
// Plus 2+ more per operation

// Result: Single operation produces 4-6 log lines at V(3) - excessive even for debug mode
```

**Impact:** With cluster verbosity at V(3), operational logs become 5-10x larger; makes troubleshooting harder, not easier

**Fix for v0.7.1:**
- Audit all V(3) logs - keep only those essential for troubleshooting
- Move "intermediate step" logs to V(4) (detailed)
- Keep V(3) for: errors, major phase transitions, idempotency checks only
- DeleteVolume: Reduce from 6 V(3) statements to 2 (start and any error)
- Target: 30-40% reduction in V(3) output

---

### Duplication in Attachment Manager Logger

**Issue:** `pkg/security/logger.go` also duplicates the switch pattern within `LogEvent()` itself (lines 49-69).

**Files:** `pkg/security/logger.go` (lines 49-69)

```go
switch event.Severity {
case SeverityInfo:
	verbosity = 2
	logFunc = func(args ...interface{}) {
		klog.V(verbosity).Info(args...)
	}
case SeverityWarning:
	verbosity = 1
	logFunc = klog.Warning
case SeverityError:
	verbosity = 0
	logFunc = klog.Error
case SeverityCritical:
	verbosity = 0
	logFunc = klog.Error
default:
	verbosity = 2
	logFunc = func(args ...interface{}) {
		klog.V(verbosity).Info(args...)
	}
}
```

**Impact:** Severity-to-klog mapping should be table-driven; if mapping changes, must update in two places

**Fix for v0.7.1:**
- Extract severity mapping to package-level map:
  ```go
  var severityToVerbosity = map[EventSeverity]klog.Level{
      SeverityInfo: 2,
      SeverityWarning: 1,
      SeverityError: 0,
      SeverityCritical: 0,
  }
  ```
- Replace switch with table lookup

---

## Tech Debt

**SSH Host Key Verification Bypass:**
- Issue: Default configuration allows skipping SSH host key verification via `--rds-insecure-skip-verify` flag, enabled in test deployments
- Files: `cmd/rds-csi-plugin/main.go` (line 31, 87-100), `pkg/rds/ssh_client.go` (line 26, 95-100)
- Impact: Man-in-the-middle attacks possible in production if flag is inadvertently used
- Fix approach: Enforce host key verification requirement in production builds; consider removing InsecureSkipVerify from release builds

**Orphaned NVMe Subsystems Detection:**
- Issue: Controller may encounter orphaned NVMe subsystems that appear connected but have no device paths
- Files: `pkg/nvme/nvme.go` (line 495-504 fallback logic, line 307-323 legacy implementation)
- Impact: Connection attempts may appear successful but subsequent device operations fail with delayed error detection
- Fix approach: Improve `GetDevicePath()` robustness; add metrics for orphan detection frequency

**Hard-coded Device Lookup Delays:**
- Issue: Fixed 100ms sleep in device polling and 500ms ticker
- Files: `pkg/nvme/nvme.go` (lines 150-200)
- Impact: Tuning relies on empirical observation; may fail on slower hardware or high load
- Fix approach: Make polling intervals configurable via Config struct; add jitter

**Legacy NVMe Connection Check:**
- Issue: Deprecated `isConnectedLegacy()` method uses string matching instead of JSON parsing
- Files: `pkg/nvme/nvme.go` (line 307-323)
- Impact: Code duplication; if accidentally used, robustness suffers
- Fix approach: Remove deprecated method; ensure new implementation used consistently

---

## Known Bugs

**Potential Goroutine Leak in gRPC Server:**
- Symptoms: Server goroutine may not cleanly exit if listener fails after goroutine creation
- Files: `pkg/driver/server.go` (line 85-89)
- Trigger: Race condition between listener creation and Serve() call
- Workaround: Server gracefully shuts down via Stop(), but lingering goroutine could prevent clean exit

**NVMe-oF Device Naming Race Condition:**
- Symptoms: Device path lookup may return incorrect subsystem-based vs controller-based paths inconsistently
- Files: `pkg/nvme/nvme.go` (line 343-399)
- Trigger: Multiple NVMe connections to same target across different controllers
- Workaround: Current implementation prefers simple paths but doesn't validate they're functional

**Potential Resource Leak in Orphan Reconciler:**
- Symptoms: If Stop() called mid-cleanup, pending goroutine may continue
- Files: `pkg/reconciler/orphan_reconciler.go` (line 103-116, 119-125)
- Trigger: Rapid start/stop cycles during deployment
- Workaround: Orphan reconciler disabled by default

---

## Security Considerations

**Command Injection in SSH Operations:**
- Risk: SSH commands constructed with volume IDs and file paths could be vulnerable
- Files: `pkg/rds/commands.go`, `pkg/rds/ssh_client.go`, `pkg/utils/volumeid.go`
- Current mitigation: Strict validation of volume IDs (`^pvc-[a-f0-9-]+$`), file paths normalized, slot names validated
- Recommendations: Add integration tests with injection patterns; consider pre-compiled command templates

**NQN Validation Coverage:**
- Risk: NQN format validation may not catch all malformed input
- Files: `pkg/utils/validation.go`, `pkg/nvme/nvme.go` (line 456-458, 461-465)
- Current mitigation: NQN validated before use in nvme-cli commands
- Recommendations: Add fuzzing tests; verify all code paths validate NQN

**Mount Options Validation:**
- Risk: Mount options validation uses whitelist but regex in `pkg/utils/regex.go` could have edge cases
- Files: `pkg/mount/mount.go` (line 13-56), `pkg/utils/regex.go`
- Current mitigation: Dangerous options explicitly blocked (suid, dev, exec); whitelist enforced
- Recommendations: Add unit tests with malicious combinations; consider explicit option parser instead of regex

**SSH Key File Permissions:**
- Risk: SSH private key could be readable by unauthorized users if Kubernetes Secret permissions misconfigured
- Files: `cmd/rds-csi-plugin/main.go` (line 80)
- Current mitigation: Key path `/etc/rds-csi/ssh-key/id_rsa` with restricted permissions via Kubernetes Secret
- Recommendations: Add runtime check that key file has mode 0600; log warning if readable

---

## Performance Bottlenecks

**Synchronous NVMe Device Discovery:**
- Problem: Device lookup scans entire `/sys/class/nvme/` on every connection; slow with many devices
- Files: `pkg/nvme/nvme.go` (line 328-340)
- Cause: Linear scan of controller directories; no caching or indexing
- Improvement path: Cache controller-to-NQN mappings with TTL; use inotify for device changes; parallelize reading

**Single SSH Connection Bottleneck:**
- Problem: Controller creates multiple SSH connections sequentially when pool capacity exhausted
- Files: `pkg/rds/pool.go` (line 25-39)
- Cause: Connection pool creation waits for SSH handshake (10s timeout) synchronously
- Improvement path: Implement connection pool pre-warming; add async connection establishment with fallback

**Polling Intervals Not Adaptive:**
- Problem: Fixed 100ms/500ms sleeps don't adapt to system load
- Files: `pkg/nvme/nvme.go` (line 150, 171)
- Cause: Hardcoded timers
- Improvement path: Implement exponential backoff for device polling; measure device appearance times dynamically

**SSH Command Retry Logic Without Jitter:**
- Problem: All failed commands retry with same backoff (1s, 2s, 4s), potentially causing thundering herd
- Files: `pkg/rds/ssh_client.go` (line 180-215)
- Cause: Exponential backoff but no jitter
- Improvement path: Add ±10% jitter; implement full jitter algorithm for distributed retries

---

## Fragile Areas

**NVMe Device Path Resolution:**
- Files: `pkg/nvme/nvme.go` (line 325-399)
- Why fragile: Multiple fallback code paths with different naming conventions (nvmeXnY vs nvmeXcYnZ), precedence unclear, regex parsing complex
- Safe modification: Add comprehensive unit tests for each device naming pattern; add comments explaining fallback order
- Test coverage: Partial - unit tests cover basic cases but not edge cases like multi-controller devices

**RouterOS Output Parsing:**
- Files: `pkg/rds/commands.go`, output parsing code
- Why fragile: Parses RouterOS CLI output using string matching and regex; format changes in RouterOS could break parsing silently
- Safe modification: Add RouterOS version detection; implement version-specific parsers; add integration tests with real RouterOS
- Test coverage: Limited - unit tests use mock output; no integration tests with real RouterOS

**SSH Connection Pool State Machine:**
- Files: `pkg/rds/pool.go` (line 25-289)
- Why fragile: Complex state management with multiple lock acquire/release points; potential for deadlock or inconsistent state
- Safe modification: Add invariant checks in debug builds; simulate concurrent access patterns in tests; consider channels instead of mutexes
- Test coverage: Good - pool_test.go has concurrent tests, but no extreme concurrency stress tests

**Orphan Reconciler Kubernetes Integration:**
- Files: `pkg/reconciler/orphan_reconciler.go`
- Why fragile: Depends on Kubernetes API availability; grace period logic could delete volumes in transition
- Safe modification: Add dry-run validation tests; increase grace period in production; add operator-visible events
- Test coverage: Unit tests exist but no integration tests with real Kubernetes API

---

## Scaling Limits

**Connection Pool Maximum Size:**
- Current capacity: Configurable via PoolConfig but default unclear
- Limit: No documented limit per RDS instance; TCP limits on RDS unknown
- Scaling path: Profile RDS under concurrent load (50, 100, 500 ops); document safe limits; add rate limiter

**NVMe Connection Limits:**
- Current capacity: No documented limit per node; kernel module has undocumented limits
- Limit: Likely 10-20 volumes per node based on experiential evidence
- Scaling path: Profile with many volumes; add warning logs near limits; implement connection pooling across pods

**RDS SSH Concurrency:**
- Current capacity: Single SSH connection handled by pool; serial command execution
- Limit: Unknown - depends on RouterOS config and RDS hardware
- Scaling path: Benchmark RDS with concurrent sessions; consider multiple SSH connections per pool

**Kubernetes API Calls in Orphan Reconciler:**
- Current capacity: Lists all PVs on each cycle without pagination
- Limit: Timeout on clusters with 10,000+ PVs
- Scaling path: Implement pagination; add caching with TTL; use label selectors to filter PVs

---

## Dependencies at Risk

**nvme-cli Binary Availability:**
- Risk: Code assumes `nvme`, `mkfs.*`, `mount`, `umount` binaries exist on all nodes
- Files: `pkg/nvme/nvme.go`, `pkg/mount/mount.go`
- Impact: Pod creation fails if binaries not installed (common in minimal container images)
- Migration plan: Add init container to install; pre-build custom image; add startup binary availability check

**RouterOS SSH Command Format Stability:**
- Risk: RDS implements RouterOS CLI which may change format in future versions
- Files: `pkg/rds/commands.go`, output parsing
- Impact: Driver incompatible with RouterOS version changes
- Migration plan: Document tested RouterOS versions; add version detection; maintain parsers for multiple versions

**Kubernetes CSI API Stability:**
- Risk: CSI spec imports may have breaking changes
- Files: All `pkg/driver/*.go`
- Impact: Requires code updates on library version bumps
- Migration plan: Pin CSI library version; add CI tests on multiple versions

**Go Version Compatibility:**
- Risk: Code may use Go 1.24+ features not available in older versions
- Files: `cmd/rds-csi-plugin/main.go`, `go.mod`
- Impact: Cannot build on Go < 1.24
- Migration plan: Document minimum Go version; add CI matrix; avoid new-only features

---

## Missing Critical Features

**No Volume Snapshots:**
- Problem: CSI snapshot operations unimplemented
- Blocks: Users cannot create backups, clone volumes, disaster recovery
- Status: ROADMAP.md Phase 2; estimated 4-6 weeks post-v0.1.0

**No High Availability for Controller:**
- Problem: Single controller pod is single point of failure
- Blocks: Production deployments require HA
- Status: ROADMAP.md Phase 3; requires leader election

**No Volume Encryption:**
- Problem: Data at rest unencrypted; data in transit uses only network isolation
- Blocks: Production deployments with data security requirements
- Status: ROADMAP.md Phase 3

**Limited Observability:**
- Problem: No Prometheus metrics; minimal structured logging
- Blocks: Operators cannot monitor driver health proactively
- Status: Partially addressed - security logger exists but no metrics endpoint

---

## Test Coverage Gaps (v0.7.1 Priority)

**CSI Sanity Tests Not Run:**
- What's not tested: Full CSI spec compliance validation
- Files: `test/integration/` exists but may not run full sanity suite
- Risk: Driver may not comply with CSI spec in edge cases; sidecars may fail
- Priority: High - blocks production readiness

**No Chaos/Failure Injection Tests:**
- What's not tested: Behavior when RDS unavailable, network partitions, NVMe connection failures
- Files: No chaos test files
- Risk: Reliability under failure unknown; pods may hang indefinitely
- Priority: High - required for production safety

**No Stress/Load Tests:**
- What's not tested: Behavior with 100+ concurrent volumes, high I/O throughput
- Files: No load test files
- Risk: Scaling limits and bottlenecks undiscovered until production
- Priority: Medium - document limits

**No Integration Tests with Real RDS:**
- What's not tested: Actual RouterOS command compatibility, file-backed disk behavior, NVMe export
- Files: `test/mock/` has mock servers, but no real hardware tests
- Risk: Output parsing may be incorrect; file operations may fail in production
- Priority: Critical - failures only discovered in cluster deployment

**Orphan Reconciler Edge Cases:**
- What's not tested: Reconciler during rapid PV deletion, grace period expiry mid-cleanup, concurrent deletions
- Files: `test/integration/orphan_reconciler_integration_test.go` may have gaps
- Risk: Reconciler could delete wrong volumes; miss orphans if timing unlucky
- Priority: Medium - dry-run mode provides safety

**No End-to-End Tests in CI:**
- What's not tested: Full volume lifecycle (create → mount → write/read → unmount → delete) in automated environment
- Files: No CI pipeline integration tests
- Risk: Regressions discovered only when deployed to clusters
- Priority: High - essential for release confidence

---

*Concerns audit: 2026-02-04*
