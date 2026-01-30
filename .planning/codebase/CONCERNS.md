# Codebase Concerns

**Analysis Date:** 2026-01-30

## Tech Debt

**SSH Host Key Verification Bypass:**
- Issue: Default configuration allows skipping SSH host key verification via `--rds-insecure-skip-verify` flag, which is enabled in test deployments
- Files: `cmd/rds-csi-plugin/main.go` (line 31, 87-100), `pkg/rds/ssh_client.go` (line 26, 95-100)
- Impact: Man-in-the-middle attacks possible in production if flag is inadvertently used. Clear security warning messages are logged, but the danger exists
- Fix approach: Enforce host key verification requirement in production builds; consider removing InsecureSkipVerify from release builds or adding build-time check

**Orphaned NVMe Subsystems Detection:**
- Issue: Controller may encounter orphaned NVMe subsystems that appear connected but have no actual device paths accessible
- Files: `pkg/nvme/nvme.go` (line 495-504 with fallback logic, line 307-323 legacy implementation)
- Impact: Connection attempts may appear successful but subsequent device operations could fail, causing delayed error detection
- Fix approach: Improve `GetDevicePath()` robustness with more comprehensive orphan subsystem detection; add metrics for orphan detection frequency

**Hard-coded Device Lookup Delays:**
- Issue: Fixed 100ms sleep in device polling and 500ms ticker for device discovery
- Files: `pkg/nvme/nvme.go` (around line 150-200)
- Impact: Tuning relies on empirical observation; may fail on slower hardware or under high load
- Fix approach: Make polling intervals configurable via Config struct; add jitter to prevent thundering herd

**Legacy NVMe Connection Check Implementation:**
- Issue: Deprecated `isConnectedLegacy()` method still exists and uses string matching instead of JSON parsing
- Files: `pkg/nvme/nvme.go` (line 307-323)
- Impact: Code duplication; if legacy code is accidentally used, robustness suffers
- Fix approach: Remove deprecated method; ensure new implementation is used consistently everywhere

## Known Bugs

**Potential Goroutine Leak in gRPC Server:**
- Symptoms: Server goroutine started in `Start()` may not cleanly exit if listener fails after goroutine creation
- Files: `pkg/driver/server.go` (line 85-89)
- Trigger: Race condition between listener creation and Serve() call in goroutine
- Workaround: Server gracefully shuts down via `Stop()` call, but lingering goroutine could prevent clean process exit

**NVMe-oF Device Naming Race Condition:**
- Symptoms: Device path lookup may return incorrect subsystem-based paths vs controller-based paths inconsistently
- Files: `pkg/nvme/nvme.go` (line 343-399)
- Trigger: Multiple NVMe connections to same target across different controllers
- Workaround: Current implementation prefers simple paths but doesn't validate they're functional before returning

**Potential Resource Leak in Orphan Reconciler:**
- Symptoms: If `Stop()` is called while reconciliation loop is in middle of cleanup, pending goroutine may continue
- Files: `pkg/reconciler/orphan_reconciler.go` (line 103-116, 119-125)
- Trigger: Rapid start/stop cycles during deployment
- Workaround: Orphan reconciler is disabled by default; enable only with careful operational planning

## Security Considerations

**Command Injection in SSH Operations:**
- Risk: SSH commands constructed with volume IDs and file paths could be vulnerable if validation is bypassed
- Files: `pkg/rds/commands.go`, `pkg/rds/ssh_client.go`, `pkg/utils/volumeid.go`
- Current mitigation: Strict validation of volume IDs (must match `^pvc-[a-f0-9-]+$`), file paths normalized, slot names validated
- Recommendations: Add integration tests that attempt common injection patterns; consider pre-compiled command templates instead of string formatting

**NQN Validation Coverage:**
- Risk: NQN format validation may not catch all malformed input
- Files: `pkg/utils/validation.go`, `pkg/nvme/nvme.go` (line 456-458, 461-465)
- Current mitigation: NQN validated before use in nvme-cli commands
- Recommendations: Add fuzzing tests for NQN validation; verify all code paths validate NQN

**Mount Options Validation:**
- Risk: Mount options validation uses whitelist approach but regex-based validation in `pkg/utils/regex.go` could have edge cases
- Files: `pkg/mount/mount.go` (line 13-56), `pkg/utils/regex.go`
- Current mitigation: Dangerous options (suid, dev, exec) explicitly blocked; whitelist enforced
- Recommendations: Add unit tests with malicious option combinations (e.g., `nosuid,suid`, `no^X` patterns); consider using explicit option parser instead of regex

**SSH Key File Permissions:**
- Risk: SSH private key could be readable by unauthorized users if Kubernetes Secret permissions are misconfigured
- Files: `cmd/rds-csi-plugin/main.go` (line 80)
- Current mitigation: Key path expected to be `/etc/rds-csi/ssh-key/id_rsa` with restricted permissions via Kubernetes Secret
- Recommendations: Add runtime check that key file has mode 0600; log warning if readable by others; document Secret RBAC requirements

## Performance Bottlenecks

**Synchronous NVMe Device Discovery:**
- Problem: Device lookup scans entire `/sys/class/nvme/` on every connection; becomes slow with many devices
- Files: `pkg/nvme/nvme.go` (line 328-340)
- Cause: Linear scan of controller directories; no caching or indexing
- Improvement path: Cache controller-to-NQN mappings with TTL; use inotify for device change notifications; parallelize subsystem reading

**Single SSH Connection Bottleneck:**
- Problem: Controller service may create multiple SSH connections sequentially when pool capacity is exhausted
- Files: `pkg/rds/pool.go` (line 25-39)
- Cause: Connection pool creation waits for SSH handshake (10s timeout) synchronously
- Improvement path: Implement connection pool pre-warming; add async connection establishment with fallback

**Polling Intervals Not Adaptive:**
- Problem: Fixed 100ms/500ms sleeps in device polling don't adapt to system load
- Files: `pkg/nvme/nvme.go` (line 150, 171)
- Cause: Hardcoded timers
- Improvement path: Implement exponential backoff for device polling; measure actual device appearance times and adjust dynamically

**SSH Command Retry Logic Without Jitter:**
- Problem: All failed commands retry with same backoff sequence (1s, 2s, 4s), potentially causing thundering herd
- Files: `pkg/rds/ssh_client.go` (line 180-215)
- Cause: Exponential backoff but no random jitter
- Improvement path: Add ±10% jitter to backoff times; implement full jitter algorithm for distributed retries

## Fragile Areas

**NVMe Device Path Resolution:**
- Files: `pkg/nvme/nvme.go` (line 325-399)
- Why fragile: Multiple fallback code paths with different naming conventions (nvmeXnY vs nvmeXcYnZ), precedence unclear, regex parsing complex
- Safe modification: Add comprehensive unit tests for each device naming pattern before any changes; add comments explaining fallback order
- Test coverage: Partial - unit tests cover basic cases but not edge cases like multi-controller devices

**RouterOS Output Parsing:**
- Files: `pkg/rds/commands.go`, `pkg/rds/parser.go` (if exists)
- Why fragile: Parses RouterOS CLI output using string matching and regex; format changes in RouterOS version could break parsing silently
- Safe modification: Add RouterOS version detection and version-specific parsers; add integration tests with real RouterOS instances
- Test coverage: Limited - unit tests use mock output; no integration tests with real RouterOS

**SSH Connection Pool State Machine:**
- Files: `pkg/rds/pool.go` (line 25-289)
- Why fragile: Complex state management with multiple lock acquire/release points; potential for deadlock or inconsistent state
- Safe modification: Add invariant checks in debug builds; simulate concurrent access patterns in tests; consider using channels instead of mutexes
- Test coverage: Good - pool_test.go has concurrent access tests, but no stress tests with extreme concurrency

**Orphan Reconciler Kubernetes Integration:**
- Files: `pkg/reconciler/orphan_reconciler.go`
- Why fragile: Depends on Kubernetes API availability and consistency; grace period logic could delete volumes in transition
- Safe modification: Add dry-run validation tests; increase grace period in production; add operator-visible events when reconciler acts
- Test coverage: Unit tests exist but no integration tests with real Kubernetes API

## Scaling Limits

**Connection Pool Maximum Size:**
- Current capacity: Configurable via `PoolConfig.MaxSize` but default unclear
- Limit: No documented limit per RDS instance; TCP connection limits on RDS side unknown
- Scaling path: Profile RDS behavior under concurrent load (load test with 50, 100, 500 concurrent operations); document safe limits; add rate limiter

**NVMe Connection Limits:**
- Current capacity: No documented limit per node; kernel module has undocumented limits
- Limit: Likely limited by number of NVMe subsystems kernel can track (experiential evidence: works with 10-20 volumes per node)
- Scaling path: Profile with many volumes; add warning logs when approaching limits; implement connection pooling across pods

**RDS SSH Concurrency:**
- Current capacity: Single SSH connection handled by pool; serial execution of commands
- Limit: Unknown - depends on RouterOS configuration and RDS hardware
- Scaling path: Benchmark RDS with concurrent SSH sessions; consider multiple SSH connections per pool; profile response times under load

**Kubernetes API Calls in Orphan Reconciler:**
- Current capacity: Lists all PVs on each reconciliation cycle without pagination
- Limit: Will timeout on clusters with 10,000+ PVs
- Scaling path: Implement pagination in orphan reconciler; add caching with TTL; consider label selectors to filter PVs

## Dependencies at Risk

**nvme-cli Binary Availability:**
- Risk: Code assumes `nvme`, `nvme-cli`, `mkfs.*`, `mount`, `umount` binaries exist on all nodes
- Files: `pkg/nvme/nvme.go`, `pkg/mount/mount.go`
- Impact: Pod creation fails if binaries not installed (common in minimal container images)
- Migration plan: Add init container to install binaries; or pre-build custom image with dependencies; add startup check for binary availability

**RouterOS SSH Command Format Stability:**
- Risk: RDS implements RouterOS CLI which may change format in future versions
- Files: `pkg/rds/commands.go`, output parsing code
- Impact: Driver becomes incompatible with RouterOS version changes
- Migration plan: Document tested RouterOS versions; add version detection in SSH client; maintain parsers for multiple RouterOS versions; add integration tests with multiple versions

**Kubernetes CSI API Stability:**
- Risk: CSI spec imports may have breaking changes
- Files: All `pkg/driver/*.go`
- Impact: Requires code updates on Kubernetes or CSI library version bumps
- Migration plan: Pin CSI library to specific version; add CI tests on multiple CSI versions; consider CSI shim if needed

**Go Version Compatibility:**
- Risk: Code may use Go 1.24+ features not available in older versions
- Files: `cmd/rds-csi-plugin/main.go`, `go.mod`
- Impact: Cannot build on Go < 1.24
- Migration plan: Document minimum Go version; add CI matrix for multiple Go versions; avoid new-only language features for at least 2 release cycles

## Missing Critical Features

**No Volume Snapshots:**
- Problem: CSI snapshot operations are unimplemented
- Blocks: Users cannot create backups, clone volumes, or implement disaster recovery
- Status: Documented in ROADMAP.md as Phase 2; estimated 4-6 weeks post-v0.1.0

**No High Availability for Controller:**
- Problem: Single controller pod is single point of failure
- Blocks: Production deployments require HA
- Status: Documented in ROADMAP.md Phase 3; requires leader election implementation

**No Volume Encryption:**
- Problem: Data at rest is unencrypted; data in transit uses only network isolation
- Blocks: Production deployments with data security requirements
- Status: Documented in ROADMAP.md Phase 3

**Limited Monitoring and Observability:**
- Problem: No Prometheus metrics; minimal structured logging for operators
- Blocks: Operators cannot monitor driver health, detect issues proactively
- Status: Partially addressed - security logger exists but no metrics endpoint; ROADMAP.md lists as Milestone 5 TODO

## Test Coverage Gaps

**CSI Sanity Tests Not Run:**
- What's not tested: Full CSI specification compliance validation
- Files: `test/integration/` (exists but may not run full sanity suite)
- Risk: Driver may not comply with CSI spec in edge cases; sidecars may fail
- Priority: High - blocks production readiness

**No Chaos/Failure Injection Tests:**
- What's not tested: Behavior when RDS becomes unavailable, network partitions, NVMe connection failures
- Files: No chaos test files
- Risk: Reliability under failure conditions unknown; pods may hang indefinitely
- Priority: High - required for production safety

**No Stress/Load Tests:**
- What's not tested: Behavior with 100+ concurrent volumes, high I/O throughput
- Files: No load test files
- Risk: Scaling limits and bottlenecks undiscovered until production
- Priority: Medium - document limits even if not addressed

**No Integration Tests with Real RDS:**
- What's not tested: Actual RouterOS command compatibility, file-backed disk behavior, NVMe export functionality
- Files: `test/mock/` has mock servers, but no real hardware tests
- Risk: Output parsing may be incorrect for real RouterOS instances; file creation/deletion may fail in production
- Priority: Critical - test failures only discovered in cluster deployment

**Orphan Reconciler Edge Cases Not Covered:**
- What's not tested: Reconciler behavior during rapid PV deletion, when grace period expires mid-cleanup, concurrent deletions
- Files: `test/integration/orphan_reconciler_integration_test.go` exists but may have gaps
- Risk: Reconciler could delete volumes that shouldn't be deleted; miss orphans if timing is unlucky
- Priority: Medium - dry-run mode provides safety but doesn't test actual deletion logic

**No End-to-End Tests in CI:**
- What's not tested: Full volume lifecycle (create → mount → write/read → unmount → delete) in automated environment
- Files: No CI pipeline integration tests
- Risk: Regressions discovered only when deployed to clusters
- Priority: High - essential for release confidence

---

*Concerns audit: 2026-01-30*
