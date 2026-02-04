# Codebase Concerns

**Analysis Date:** 2026-02-04

## Code Smells & Quality Issues (v0.7.1 Priority)

### Excessive Logging in Helper Methods

**Status:** RESOLVED (Phase 18)

**Resolution:** Consolidated 11 nearly-identical helper methods using table-driven LogOperation helper (operationConfigs map). Reduced from 300+ lines to 47 lines. See Phase 18 summary.

**Original Issue:** `pkg/security/logger.go` contained 11 nearly-identical helper methods (LogVolumeCreate, LogVolumeDelete, LogVolumeStage, LogVolumeUnstage, LogVolumePublish, LogVolumeUnpublish, LogNVMEConnect, LogNVMEDisconnect, etc.) that followed identical patterns.

**Impact:** 300+ lines of duplicated logic eliminated; significantly improved maintainability

---

### Inconsistent Error Message Format

**Status:** RESOLVED (Phase 19)

**Resolution:** Established consistent error wrapping with %w format verb (96.1% compliance). Created sentinel errors for type-safe classification. Documented patterns in CONVENTIONS.md.

**Original Issue:** Error wrapping was inconsistent between `status.Errorf()` and direct string format, using both %v and %w across 160+ error returns.

**Impact:** Consistent error wrapping now enables proper error chain inspection and context preservation throughout the codebase

---

### Logging Verbosity Inconsistency

**Status:** RESOLVED (Phase 18)

**Resolution:** Audited all V(3) logs, moved intermediate steps to V(4). DeleteVolume reduced from 6 V(3) statements to 1 outcome log at V(2). V(3) eliminated from codebase.

**Original Issue:** V() level usage was inconsistent across modules; V(3) logged intermediate steps on every error path, creating excessive noise.

**Impact:** Operational logs at V(2) now provide clean outcome-focused logging; V(4) available for detailed diagnostic traces when needed

---

### Duplication in Attachment Manager Logger

**Status:** RESOLVED (Phase 21-01)

**Resolution:** Replaced switch statement in LogEvent() (lines 49-69) with severityMap table lookup. Reduced cyclomatic complexity from 5 to 1, eliminated 17 lines of duplicated switch logic.

**Original Issue:** `pkg/security/logger.go` duplicated the switch pattern within `LogEvent()` for severity-to-verbosity mapping.

**Impact:** Table-driven approach provides single source of truth for severity mappings; adding new severities now requires single map entry instead of multiple case statements

---

### Large Package Sizes

**Status:** DEFERRED to v0.9.0

**Issue:** pkg/driver (3552 lines), pkg/rds (1834 lines), pkg/nvme (1655 lines) exceed typical package thresholds.

**Rationale for deferral:**
- Packages have clear responsibilities aligned with CSI architecture (Controller, Node, Identity)
- Splitting would increase import complexity without clear benefit
- Current structure is well-tested with 65% coverage
- Refactoring risk outweighs maintainability benefit at current scale

**Review criteria:** Reconsider if:
- Package exceeds 5000 lines
- Adding new feature would blur responsibility boundaries
- Test coverage drops due to package complexity

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

*Concerns audit: 2026-02-04 (Phase 21 code smell resolution)*
