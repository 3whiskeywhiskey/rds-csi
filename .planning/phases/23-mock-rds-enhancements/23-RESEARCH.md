# Phase 23: Mock RDS Enhancements - Research

**Researched:** 2026-02-04
**Domain:** SSH server mocking, concurrent state management, error injection, RouterOS CLI simulation
**Confidence:** HIGH

## Summary

This phase enhances the existing mock RDS server (`test/mock/rds_server.go`) to match real hardware behavior for reliable CI testing. The mock already implements basic SSH server functionality using `golang.org/x/crypto/ssh` and handles core RouterOS commands (`/disk add`, `/disk remove`, `/disk print detail`, `/file print detail`). Enhancements focus on four areas: realistic timing simulation, comprehensive error injection, concurrent connection handling, and RouterOS output format validation.

The current mock uses in-memory state with basic mutex locking and implements idempotent command handling. It successfully supports CSI sanity tests (Phase 22) but lacks timing realism, structured error injection, and operation history tracking needed for advanced testing scenarios.

**Primary recommendation:** Use environment variable-based configuration for timing and error injection, structured logging for test observability, and extend existing command handling to support all documented RouterOS quirks.

## Standard Stack

The established libraries/tools for mock SSH server testing in Go:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/crypto/ssh | Latest (Jan 2026) | SSH server/client implementation | Official Go crypto library, complete SSH protocol support, widely used for SSH mocking |
| sync.RWMutex | stdlib | Concurrent state protection | Standard Go concurrency primitive for read-heavy workloads |
| testing/synctest | Go 1.24 (experimental) | Deterministic concurrent testing with fake time | Official Go solution for testing concurrent code without flakiness |
| k8s.io/klog/v2 | v2 | Structured logging | Already used throughout codebase, supports verbosity levels |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/gliderlabs/ssh | Latest | Simplified SSH server creation | Consider if x/crypto/ssh proves too low-level (not needed - current implementation works) |
| net.Pipe() | stdlib | In-memory network connections for testing | Use with synctest for deterministic network testing |
| time.Sleep | stdlib | Latency simulation | Use for realistic timing when MOCK_RDS_REALISTIC_TIMING=true |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| golang.org/x/crypto/ssh | gliderlabs/ssh | Higher-level API but adds dependency; current low-level implementation provides full control |
| Environment variables | Config file | Config file more complex; env vars simpler for CI and test isolation |
| Structured logging (JSON) | Standard klog | JSON enables parsing by test framework but adds noise; use klog V-levels and only JSON for critical events |

**Installation:**
```bash
# Core dependencies (already in go.mod)
# golang.org/x/crypto/ssh - already present
# k8s.io/klog/v2 - already present

# Experimental synctest for Go 1.24+
# Requires GOEXPERIMENT=synctest environment variable
# No explicit import needed - part of testing package
```

## Architecture Patterns

### Recommended Project Structure
```
test/mock/
├── rds_server.go          # Main mock server (existing)
├── error_injection.go     # NEW: Error injection configuration
├── timing.go              # NEW: Timing simulation configuration
└── routeros_output.go     # NEW: RouterOS output format validation
```

### Pattern 1: Environment-Based Configuration
**What:** Configure mock behavior via environment variables read at startup
**When to use:** Test-specific behavior without code changes

**Example:**
```go
// In rds_server.go
type MockRDSConfig struct {
    // Timing control
    RealisticTiming bool          // MOCK_RDS_REALISTIC_TIMING
    SSHLatencyMs    int           // MOCK_RDS_SSH_LATENCY_MS (default: 200)
    DiskAddDelayMs  int           // MOCK_RDS_DISK_ADD_DELAY_MS (default: 500)
    DiskRemoveDelayMs int         // MOCK_RDS_DISK_REMOVE_DELAY_MS (default: 300)

    // Error injection
    ErrorMode       string        // MOCK_RDS_ERROR_MODE (disk_full|ssh_timeout|command_fail|none)
    ErrorAfterN     int           // MOCK_RDS_ERROR_AFTER_N (fail Nth operation)

    // Observability
    EnableHistory   bool          // MOCK_RDS_ENABLE_HISTORY (default: false)
    HistoryDepth    int           // MOCK_RDS_HISTORY_DEPTH (default: 100)
    RouterOSVersion string        // MOCK_RDS_ROUTEROS_VERSION (default: "7.16")
}

func NewMockRDSServer(port int) (*MockRDSServer, error) {
    config := loadConfigFromEnv()
    // Apply config to server...
}

func loadConfigFromEnv() MockRDSConfig {
    return MockRDSConfig{
        RealisticTiming: os.Getenv("MOCK_RDS_REALISTIC_TIMING") == "true",
        SSHLatencyMs:    getEnvInt("MOCK_RDS_SSH_LATENCY_MS", 200),
        // ... other fields
    }
}
```

### Pattern 2: Layered Error Injection
**What:** Inject errors at specific operation layers (SSH connect, command parse, execution)
**When to use:** Test error handling at each layer of the driver

**Example:**
```go
// error_injection.go
type ErrorInjector struct {
    mode         ErrorMode
    operationNum int
    triggerAfter int
}

type ErrorMode int
const (
    ErrorModeNone ErrorMode = iota
    ErrorModeDiskFull
    ErrorModeSSHTimeout
    ErrorModeCommandParseFail
    ErrorModeConcurrentConflict
)

func (e *ErrorInjector) ShouldFailSSHConnect() bool {
    if e.mode == ErrorModeSSHTimeout {
        e.operationNum++
        return e.operationNum >= e.triggerAfter
    }
    return false
}

func (e *ErrorInjector) ShouldFailDiskAdd() (bool, string) {
    if e.mode == ErrorModeDiskFull {
        e.operationNum++
        if e.operationNum >= e.triggerAfter {
            return true, "failure: not enough space\n"
        }
    }
    return false, ""
}

// Usage in handleDiskAdd
func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
    // Check error injection BEFORE normal processing
    if shouldFail, errMsg := s.errorInjector.ShouldFailDiskAdd(); shouldFail {
        klog.V(2).Infof("MOCK ERROR INJECTION: Disk add failed - %s", errMsg)
        return errMsg, 1
    }

    // Normal processing...
}
```

### Pattern 3: Concurrent Connection Multiplexing
**What:** Handle multiple SSH connections concurrently with proper state isolation
**When to use:** Stress testing concurrent CreateVolume/DeleteVolume operations

**Example:**
```go
// Current pattern (GOOD - already correct)
func (s *MockRDSServer) acceptConnections() {
    for {
        conn, err := s.listener.Accept()
        if err != nil {
            return
        }
        go s.handleConnection(conn)  // Each connection in separate goroutine
    }
}

func (s *MockRDSServer) handleConnection(conn net.Conn) {
    defer conn.Close()
    sshConn, chans, reqs, _ := ssh.NewServerConn(conn, s.config)
    defer sshConn.Close()

    go ssh.DiscardRequests(reqs)  // Prevent goroutine leak

    for newChannel := range chans {
        go s.handleSession(channel, requests)  // Each session concurrent
    }
}

// State access (GOOD - already uses RWMutex)
func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Check for duplicate
    if _, exists := s.volumes[slot]; exists {
        return "failure: volume already exists\n", 1
    }

    // Create volume
    s.volumes[slot] = &MockVolume{...}
}
```

**Key insight:** Current implementation already follows best practices for concurrent SSH. Only enhancement needed is stress testing validation.

### Pattern 4: Timing Simulation with Selective Delays
**What:** Add realistic delays only when explicitly enabled, default to fast testing
**When to use:** Fast tests by default, realistic timing for timeout/latency validation

**Example:**
```go
// timing.go
type TimingSimulator struct {
    enabled       bool
    sshLatency    time.Duration
    diskAddDelay  time.Duration
    diskRemoveDelay time.Duration
}

func (t *TimingSimulator) SimulateSSHLatency() {
    if t.enabled && t.sshLatency > 0 {
        time.Sleep(t.sshLatency)
    }
}

func (t *TimingSimulator) SimulateDiskOperation(opType string) {
    if !t.enabled {
        return
    }

    switch opType {
    case "add":
        time.Sleep(t.diskAddDelay)
    case "remove":
        time.Sleep(t.diskRemoveDelay)
    }
}

// Usage in handleSession (after SSH handshake completes)
func (s *MockRDSServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
    defer channel.Close()

    // Simulate SSH latency at session start
    s.timing.SimulateSSHLatency()

    for req := range requests {
        // Handle request...
    }
}

// Usage in handleDiskAdd
func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
    // Parse and validate...

    // Simulate disk operation delay BEFORE creating volume
    s.timing.SimulateDiskOperation("add")

    // Create volume...
}
```

### Pattern 5: RouterOS Output Format Validation
**What:** Validate mock output matches real RouterOS format for each version
**When to use:** Ensure parser compatibility across RouterOS versions

**Example:**
```go
// routeros_output.go
type RouterOSFormatter struct {
    version string  // e.g., "7.1", "7.16"
}

func (f *RouterOSFormatter) FormatDiskDetail(vol *MockVolume) string {
    // RouterOS 7.16 format (current production)
    if f.version >= "7.16" {
        return fmt.Sprintf(
            `slot="%s" type="file" file-path="%s" file-size=%d nvme-tcp-export=%s nvme-tcp-server-port=%d nvme-tcp-server-nqn="%s" status="ready"`,
            vol.Slot, vol.FilePath, vol.FileSizeBytes,
            boolToYesNo(vol.Exported), vol.NVMETCPPort, vol.NVMETCPNQN,
        )
    }

    // RouterOS 7.1 format (for backward compatibility testing)
    // May have different field order or naming
    return fmt.Sprintf(/* 7.1 format */)
}

// Documented RouterOS quirks to simulate
func (f *RouterOSFormatter) ShouldSimulateQuirk(quirkName string) bool {
    quirks := map[string]string{
        "multiline_paths":     "7.1+",  // Long paths split across lines
        "space_separated_numbers": "7.0+",  // size=7 949 127 950 336
        "flags_header":        "7.0+",  // "Flags: X - disabled" header
    }

    minVersion, exists := quirks[quirkName]
    if !exists {
        return false
    }

    return f.version >= minVersion
}
```

### Anti-Patterns to Avoid
- **Global timing delays:** Don't add `time.Sleep()` to every operation unconditionally - kills test speed
- **Test-specific code in production mock:** Use config/injection instead of `if testName == "xyz"` checks
- **Ignoring SSH request drain:** Always `go ssh.DiscardRequests(reqs)` to prevent goroutine leaks
- **Mutable shared state without locking:** All volume/file map access must be mutex-protected

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SSH server implementation | Custom SSH protocol handler | golang.org/x/crypto/ssh with ssh.ServerConfig | SSH protocol is complex (key exchange, auth, channels); existing implementation is battle-tested |
| Concurrent state management | Custom lock-free data structures | sync.RWMutex on maps | Reader-writer locks are optimized for read-heavy workloads like mock servers |
| Time-based testing | Real sleep delays with timeouts | testing/synctest (Go 1.24+) | Eliminates flaky tests, instant execution, deterministic behavior |
| Structured logging | Custom JSON logger | klog.V() with verbosity levels | Already integrated, supports filtering, no dependency churn |

**Key insight:** The current mock implementation already avoids hand-rolling SSH and state management. Primary risk is adding custom time simulation when `testing/synctest` exists.

## Common Pitfalls

### Pitfall 1: SSH Goroutine Leaks
**What goes wrong:** Forgetting to drain global requests causes goroutines to hang indefinitely
**Why it happens:** `ssh.NewServerConn` returns a channel of global requests that must be consumed
**How to avoid:**
```go
// WRONG - goroutine leak
serverConn, chans, reqs, _ := ssh.NewServerConn(conn, config)
// Never consume reqs - goroutine hangs

// RIGHT - drain requests
go ssh.DiscardRequests(reqs)
```
**Warning signs:** Test hangs after completion, goroutine count grows with each test run

### Pitfall 2: Non-Deterministic Concurrent Tests
**What goes wrong:** Tests pass locally but fail in CI due to timing variations
**Why it happens:** Real `time.Sleep()` depends on system load and scheduler
**How to avoid:** Use `testing/synctest.Run()` with fake time for deterministic execution
**Warning signs:** Tests marked `continue-on-error: true` in CI, intermittent failures under load

### Pitfall 3: Command Injection via Unvalidated Slot Names
**What goes wrong:** Test passes malicious slot name like `vol; rm -rf /`, mock executes it
**Why it happens:** Mock doesn't validate input assuming test code is trusted
**How to avoid:** Apply SAME validation as production code in mock
```go
// WRONG - trust test input
slot := extractParam(command, "slot")
s.volumes[slot] = &MockVolume{...}

// RIGHT - validate even in mock
slot := extractParam(command, "slot")
if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(slot) {
    return "failure: invalid slot name\n", 1
}
```
**Warning signs:** Mock accepts invalid input that production rejects

### Pitfall 4: RouterOS Output Format Drift
**What goes wrong:** Mock output diverges from real RouterOS, parser fails in production
**Why it happens:** Mock is updated without checking real hardware output
**How to avoid:**
- Document real RouterOS output examples in `docs/rds-commands.md` (already done ✓)
- Add output validation tests comparing mock vs documented format
- Use version-specific formatters to handle RouterOS upgrades

**Warning signs:** Parser works in tests but fails with real RDS

### Pitfall 5: Error Injection Lacks Cleanup
**What goes wrong:** Error injection in one test affects subsequent tests
**Why it happens:** Error injector state persists across tests
**How to avoid:**
```go
// In each test
func TestCreateVolumeDiskFull(t *testing.T) {
    os.Setenv("MOCK_RDS_ERROR_MODE", "disk_full")
    os.Setenv("MOCK_RDS_ERROR_AFTER_N", "1")
    defer os.Unsetenv("MOCK_RDS_ERROR_MODE")  // CRITICAL
    defer os.Unsetenv("MOCK_RDS_ERROR_AFTER_N")

    // Test runs...
}
```
**Warning signs:** Tests fail when run together but pass individually

## Code Examples

Verified patterns from golang.org/x/crypto/ssh and existing mock:

### SSH Server with Per-Connection Goroutines
```go
// Source: golang.org/x/crypto/ssh documentation + test/mock/rds_server.go
func (s *MockRDSServer) acceptConnections() {
    for {
        select {
        case <-s.shutdown:
            return
        default:
            conn, err := s.listener.Accept()
            if err != nil {
                select {
                case <-s.shutdown:
                    return
                default:
                    klog.Errorf("Failed to accept connection: %v", err)
                    continue
                }
            }

            go s.handleConnection(conn)  // Concurrent per connection
        }
    }
}

func (s *MockRDSServer) handleConnection(conn net.Conn) {
    defer conn.Close()

    sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
    if err != nil {
        klog.Errorf("Failed to handshake: %v", err)
        return
    }
    defer sshConn.Close()

    // CRITICAL: Drain global requests to prevent goroutine leak
    go ssh.DiscardRequests(reqs)

    // Handle channels concurrently
    for newChannel := range chans {
        if newChannel.ChannelType() != "session" {
            newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
            continue
        }

        channel, requests, _ := newChannel.Accept()
        go s.handleSession(channel, requests)
    }
}
```

### Error Injection with Environment Variables
```go
// NEW: error_injection.go
package mock

import (
    "os"
    "strconv"
)

type ErrorInjector struct {
    mode         ErrorMode
    operationNum int
    triggerAfter int
    mu           sync.Mutex  // Protect operation counter
}

type ErrorMode int
const (
    ErrorModeNone ErrorMode = iota
    ErrorModeDiskFull
    ErrorModeSSHTimeout
    ErrorModeCommandParseFail
)

func NewErrorInjector() *ErrorInjector {
    mode := parseErrorMode(os.Getenv("MOCK_RDS_ERROR_MODE"))
    triggerAfter := getEnvInt("MOCK_RDS_ERROR_AFTER_N", 0)

    return &ErrorInjector{
        mode:         mode,
        triggerAfter: triggerAfter,
    }
}

func (e *ErrorInjector) ShouldFailDiskAdd() (bool, string) {
    e.mu.Lock()
    defer e.mu.Unlock()

    if e.mode != ErrorModeDiskFull {
        return false, ""
    }

    e.operationNum++
    if e.operationNum >= e.triggerAfter {
        return true, "failure: not enough space\n"
    }

    return false, ""
}

func parseErrorMode(s string) ErrorMode {
    switch s {
    case "disk_full":
        return ErrorModeDiskFull
    case "ssh_timeout":
        return ErrorModeSSHTimeout
    case "command_fail":
        return ErrorModeCommandParseFail
    default:
        return ErrorModeNone
    }
}

func getEnvInt(key string, defaultVal int) int {
    if val := os.Getenv(key); val != "" {
        if i, err := strconv.Atoi(val); err == nil {
            return i
        }
    }
    return defaultVal
}
```

### Timing Simulation with Environment Control
```go
// NEW: timing.go
package mock

import (
    "os"
    "time"
)

type TimingSimulator struct {
    enabled         bool
    sshLatency      time.Duration
    diskAddDelay    time.Duration
    diskRemoveDelay time.Duration
}

func NewTimingSimulator() *TimingSimulator {
    enabled := os.Getenv("MOCK_RDS_REALISTIC_TIMING") == "true"

    return &TimingSimulator{
        enabled:         enabled,
        sshLatency:      time.Duration(getEnvInt("MOCK_RDS_SSH_LATENCY_MS", 200)) * time.Millisecond,
        diskAddDelay:    time.Duration(getEnvInt("MOCK_RDS_DISK_ADD_DELAY_MS", 500)) * time.Millisecond,
        diskRemoveDelay: time.Duration(getEnvInt("MOCK_RDS_DISK_REMOVE_DELAY_MS", 300)) * time.Millisecond,
    }
}

func (t *TimingSimulator) SimulateSSHLatency() {
    if t.enabled && t.sshLatency > 0 {
        time.Sleep(t.sshLatency)
    }
}

func (t *TimingSimulator) SimulateDiskOperation(opType string) {
    if !t.enabled {
        return
    }

    switch opType {
    case "add":
        if t.diskAddDelay > 0 {
            time.Sleep(t.diskAddDelay)
        }
    case "remove":
        if t.diskRemoveDelay > 0 {
            time.Sleep(t.diskRemoveDelay)
        }
    }
}
```

### Concurrent State Access with RWMutex
```go
// Source: test/mock/rds_server.go (existing pattern - CORRECT)
type MockRDSServer struct {
    volumes        map[string]*MockVolume
    files          map[string]*MockFile
    commandHistory []CommandLog
    mu             sync.RWMutex  // Protects all state
}

func (s *MockRDSServer) GetVolume(slot string) (*MockVolume, bool) {
    s.mu.RLock()  // Read lock - multiple concurrent readers OK
    defer s.mu.RUnlock()

    vol, ok := s.volumes[slot]
    return vol, ok
}

func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
    // Parse parameters WITHOUT lock (no shared state access)
    slot := extractParam(command, "slot")
    filePath := extractParam(command, "file-path")
    fileSize, _ := parseSize(extractParam(command, "file-size"))

    // Acquire write lock ONLY when modifying state
    s.mu.Lock()
    defer s.mu.Unlock()

    // Check for duplicates
    if _, exists := s.volumes[slot]; exists {
        return "failure: volume already exists\n", 1
    }

    // Create volume and file
    s.volumes[slot] = &MockVolume{...}
    s.files[filePath] = &MockFile{...}

    return "", 0
}
```

### RouterOS Output Format Matching
```go
// Source: docs/rds-commands.md + pkg/rds/commands.go parser expectations
func (s *MockRDSServer) formatDiskDetail(vol *MockVolume) string {
    exported := "no"
    if vol.Exported {
        exported = "yes"
    }

    // Match exact RouterOS 7.16 format from docs/rds-commands.md line 73-74:
    // slot="disk1" type="file" file-path="/storage-pool/..." file-size=53687091200
    // nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn="..." status="ready"
    return fmt.Sprintf(
        `slot="%s" type="file" file-path="%s" file-size=%d nvme-tcp-export=%s nvme-tcp-server-port=%d nvme-tcp-server-nqn="%s" status="ready"`,
        vol.Slot, vol.FilePath, vol.FileSizeBytes,
        exported, vol.NVMETCPPort, vol.NVMETCPNQN,
    )
}

func (s *MockRDSServer) formatMountPointCapacity() string {
    // Match RouterOS format from parseCapacityInfo() expectations (commands.go:447)
    // Space-separated numbers: size=7 949 127 950 336 free=5 963 595 964 416
    return fmt.Sprintf(
        `slot=storage-pool type=partition mount-point=storage-pool file-system=btrfs size=7 949 127 950 336 free=5 963 595 964 416 use=25%%
`,
    )
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Real time.Sleep() in tests | testing/synctest with fake time | Go 1.24 (2025) | Eliminates flaky concurrent tests, instant execution |
| Global config variables | Environment variable per test | Test isolation best practice | Tests run in parallel without interference |
| Manual SSH protocol handling | golang.org/x/crypto/ssh | Go 1.4+ | Production-grade SSH with crypto library team maintenance |
| Single-threaded mock server | Concurrent connection handling | Mock server pattern | Matches real SSH server behavior, enables stress testing |

**Deprecated/outdated:**
- Manual goroutine management: Use `go ssh.DiscardRequests()` pattern instead
- Custom time mocking libraries: Use official `testing/synctest` in Go 1.24+
- Shared global mock state: Use isolated server per test

## Open Questions

Things that couldn't be fully resolved:

1. **Exact operation timing from real RDS hardware**
   - What we know: docs/rds-commands.md documents "typical latencies" (50-1000ms range)
   - What's unclear: Exact timing under load, variability range (±50ms? ±200ms?)
   - Recommendation: Use documented values as defaults, add 150-250ms jitter range for SSH latency to expose timeout bugs (per CONTEXT.md decision)

2. **RouterOS quirks across versions**
   - What we know: docs/rds-commands.md documents 7.16 format
   - What's unclear: Specific differences between 7.1, 7.16, and future versions
   - Recommendation: Implement 7.16 format (production), defer 7.1 compatibility until hardware testing identifies differences

3. **Error message exact format**
   - What we know: Common errors listed - "failure: not enough space", "failure: no such item"
   - What's unclear: Exact capitalization, punctuation, error codes
   - Recommendation: Match format already validated by existing parser tests (commands_test.go line 169: `err: errors.New("failure: no such item")`)

4. **Concurrent operation limits**
   - What we know: Real RDS handles multiple SSH connections
   - What's unclear: Maximum concurrent connections, throttling behavior
   - Recommendation: No artificial limits in mock (rely on test framework to control concurrency), log warning if >10 concurrent connections

## Sources

### Primary (HIGH confidence)
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) - Official SSH package documentation
- test/mock/rds_server.go - Existing mock implementation (validated by CSI sanity tests)
- pkg/rds/commands.go - Production RouterOS command parsing (defines expected format)
- docs/rds-commands.md - RouterOS command reference with output examples
- [Go synctest blog](https://go.dev/blog/synctest) - Official testing/synctest documentation

### Secondary (MEDIUM confidence)
- [Mocking in Go tests](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/mocking) - General Go mocking patterns
- [Testing concurrent code with synctest](https://go.dev/blog/testing-time) - Time-based testing patterns
- [SSH server testing utilities](https://github.com/folbricht/sshtest) - Community SSH testing patterns

### Tertiary (LOW confidence)
- [gliderlabs/ssh](https://github.com/gliderlabs/ssh) - Higher-level SSH library (not needed but documents patterns)
- WebSearch results on error injection patterns (general guidance, not Go-specific)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official Go libraries with existing validated implementation
- Architecture: HIGH - Patterns validated by existing mock and Go standard library
- Pitfalls: HIGH - Identified from existing codebase and official documentation
- RouterOS format: MEDIUM - Based on documented format but not all versions validated
- Timing values: MEDIUM - Documented ranges exist but not hardware-validated

**Research date:** 2026-02-04
**Valid until:** 30 days (stable technology - Go stdlib and SSH protocol don't change rapidly)
