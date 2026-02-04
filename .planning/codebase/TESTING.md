# Testing Patterns

**Analysis Date:** 2026-02-04

## Test Framework

**Runner:**
- Go `testing` package (built-in)
- Go 1.24+

**Assertion Library:**
- Standard library `testing.T` with manual assertions

**Run Commands:**
```bash
make test                  # Run all unit tests with -race flag
make test-coverage         # Run tests with coverage report to coverage.html
make test-integration      # Run integration tests with mock RDS
make test-sanity          # Run CSI sanity tests (auto-detects RDS or uses mock)
make test-sanity-mock     # CSI sanity tests with mock RDS (no real RDS needed)
make test-sanity-real     # CSI sanity tests with real RDS (requires RDS_ADDRESS env var)
make lint                 # Run golangci-lint
make verify               # Run fmt + vet + lint + test
```

## Test Organization and Coverage

### Overall Coverage by Package

| Package | Coverage | Status | Notes |
|---------|----------|--------|-------|
| `pkg/utils` | 92.3% | GOOD | Validation, errors, regex, retry helpers |
| `pkg/circuitbreaker` | 90.2% | GOOD | Circuit breaker implementation |
| `pkg/attachment` | 84.5% | GOOD | Attachment tracking and reconciliation |
| `pkg/security` | 79.8% | GOOD | Metrics, logger, event recording |
| `pkg/observability` | 75.0% | ACCEPTABLE | Prometheus metrics |
| `pkg/reconciler` | 66.4% | GAPS | Orphan reconciliation logic |
| `pkg/mount` | 55.9% | **CRITICAL** | Core mounting operations (20KB code, 18KB tests) |
| `pkg/rds` | 44.5% | **CRITICAL** | SSH client completely untested (341 lines) |
| `pkg/nvme` | 43.3% | **CRITICAL** | 4031 lines, 2376 test lines - missing edge cases |
| `pkg/driver` | FAILING | **BLOCKING** | Block volume tests have nil pointer dereference |

## File and Test Structure

**Location Pattern:** Co-located

Tests are in same directory as implementation:
- `pkg/driver/controller.go` → `pkg/driver/controller_test.go`
- `pkg/nvme/nvme.go` → `pkg/nvme/nvme_test.go`
- `pkg/mount/mount.go` → `pkg/mount/mount_test.go`

**Naming Convention:**
- Test functions: `Test<Function><Scenario>` (e.g., `TestNodeStageVolume_BlockVolume`)
- Helper functions: Lowercase with clear purpose (e.g., `mockExecCommand`, `createBlockVolumeCapability`)
- Test data: Inline or simple mock structs

**Files Without Tests (COVERAGE GAPS):**
- `pkg/rds/ssh_client.go` - 341 lines - SSH connection, authentication, command execution
- `pkg/driver/server.go` - 145 lines - gRPC server setup, endpoint parsing
- `pkg/attachment/persist.go` - 147 lines - PV annotation persistence
- `pkg/rds/client.go` - 69 lines - RDS client factory (thin wrapper)

## Test Structure Pattern

### Standard Unit Test Structure

```go
// From pkg/driver/controller_test.go
func TestValidateVolumeCapabilities(t *testing.T) {
    // 1. Setup
    cs := &ControllerServer{
        driver: &Driver{
            vcaps: []*csi.VolumeCapability_AccessMode{
                {Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
            },
        },
    }

    // 2. Define test cases as table-driven
    tests := []struct {
        name      string
        caps      []*csi.VolumeCapability
        expectErr bool
    }{
        {
            name: "valid single node writer with mount",
            caps: []*csi.VolumeCapability{...},
            expectErr: false,
        },
        // More cases...
    }

    // 3. Run test cases
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := cs.validateVolumeCapabilities(tt.caps)
            if tt.expectErr && err == nil {
                t.Error("Expected error but got nil")
            }
            if !tt.expectErr && err != nil {
                t.Errorf("Unexpected error: %v", err)
            }
        })
    }
}
```

## Mocking Patterns

### Mock Types Strategy

**By Package:**

#### pkg/driver/node_test.go
```go
// mockMounter implements mount.Mounter interface for testing
type mockMounter struct {
    formatCalled    bool
    mountCalled     bool
    unmountCalled   bool
    mountErr        error
    unmountErr      error
    formatErr       error
    isLikelyMounted bool
    // ... more fields
}

func (m *mockMounter) Mount(source, target, fsType string, options []string) error {
    m.mountCalled = true
    return m.mountErr
}
// ... implement all interface methods
```

**Pattern:**
- Mocks implement interfaces (e.g., `mount.Mounter`, `RDSClient`)
- Tracking fields (e.g., `mountCalled`, `formatErr`) record what was called and with what errors
- Simple default behavior (e.g., return nil) unless test specifically configures error

#### pkg/nvme/nvme_test.go (exec.Cmd mocking)
```go
// mockExecCommand creates a mock exec.Cmd for testing
func mockExecCommand(stdout, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
    return func(command string, args ...string) *exec.Cmd {
        cs := []string{"-test.run=TestHelperProcess", "--", command}
        cs = append(cs, args...)
        cmd := exec.Command(os.Args[0], cs...)
        cmd.Env = []string{
            "GO_WANT_HELPER_PROCESS=1",
            "STDOUT=" + stdout,
            "STDERR=" + stderr,
            "EXIT_CODE=" + fmt.Sprintf("%d", exitCode),
        }
        return cmd
    }
}

// TestHelperProcess is called by the mocked command
func TestHelperProcess(t *testing.T) {
    if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
        return
    }
    // Read STDOUT/STDERR/EXIT_CODE from env and output accordingly
}
```

### What to Mock

**DO Mock:**
- External dependencies: SSH, NVMe commands, Kubernetes API (`k8s.io/client-go/kubernetes/fake`)
- System calls: `exec.Command`, filesystem operations (for unit tests)
- Time: For timeout/retry testing (use standard `time` package)
- RDS client: Use `test/mock/rds_server.go` for integration tests

**DO NOT Mock:**
- Core driver logic (test actual implementation)
- CSI structures (use real `csi.VolumeCapability`, `csi.NodeStageVolumeRequest`)
- Volume ID generation (test real `pkg/utils/volumeid.go`)
- Validation functions (test real validation, not mocked)

## Key Testing Patterns Observed

### Pattern 1: Error Injection via Struct Fields

From `pkg/attachment/manager_test.go`:
```go
// Setup mocks with error injection
mounter := &mockMounter{
    mountErr:    fmt.Errorf("mount failed"),
    formatErr:   nil,
    unmountErr: nil,
}

// Test calls method, mock returns configured error
err := ns.NodeStageVolume(ctx, req)
```

### Pattern 2: Table-Driven Tests

Heavily used throughout for testing multiple scenarios:
```go
tests := []struct {
    name      string
    input     string
    expected  string
    shouldErr bool
}{
    {"valid case", "input1", "expected1", false},
    {"error case", "input2", "", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test implementation
    })
}
```

### Pattern 3: Temporary Filesystem for Tests

From `pkg/driver/node_test.go`:
```go
tmpDir, err := os.MkdirTemp("", "node-test-*")
if err != nil {
    t.Fatalf("failed to create temp dir: %v", err)
}
defer os.RemoveAll(tmpDir)

// Test operations on temp directory
stagingPath := filepath.Join(tmpDir, "staging")
```

### Pattern 4: Fake Kubernetes Client

From `pkg/driver/controller_test.go`:
```go
import "k8s.io/client-go/kubernetes/fake"

// Use fake client for integration tests
clientset := fake.NewSimpleClientset()
// Tests can interact with Kubernetes API without real cluster
```

### Pattern 5: Context-Based Test Isolation

All CSI service tests use `context.Background()`:
```go
ctx := context.Background()
_, err = ns.NodeStageVolume(ctx, req)
```

No context timeouts observed - tests run to completion without interruption.

## Test Quality Assessment

### Strong Areas

**1. Validation Testing** (`pkg/utils/validation_test.go`)
- Comprehensive test cases: 20+ subtests for NQN validation
- Security-focused: Tests for shell injection, special characters
- Example: `TestValidateNQN` covers 20+ injection attack patterns

**2. Error Handling** (`pkg/utils/errors_test.go`)
- 482 lines of thorough error tests
- Tests sanitization of IPs, paths, hostnames, fingerprints
- Tests context preservation in error chains
- Complex scenario testing (multi-component errors)

**3. Attachment Management** (`pkg/attachment/manager_test.go`)
- Idempotency testing (same call twice should be safe)
- Conflict detection (volume already attached to different node)
- State tracking and cleanup

**4. Mount Operations** (`pkg/mount/mount_test.go`)
- 18KB of tests for 20KB of mount logic
- Tests for stale mount detection, recovery, health checks
- Tests for /proc/mounts parsing edge cases

### Critical Gaps (for v0.7.1)

**1. SSH Client Completely Untested** - `pkg/rds/ssh_client.go` (341 lines)
- No tests for SSH key parsing
- No tests for host key verification
- No tests for SSH connection establishment
- No tests for SSH command execution
- No tests for error handling (timeout, auth failure, connection refused)
- **Impact:** Control plane (volume CRUD) is untested at SSH transport level
- **Risk Level:** HIGH - This is the critical path for all volume operations

**2. gRPC Server Setup Untested** - `pkg/driver/server.go` (145 lines)
- `parseEndpoint()` function untested
- No tests for socket cleanup, TCP binding
- No tests for service registration
- **Impact:** Driver startup and initialization untested
- **Risk Level:** MEDIUM - Only runs once at startup

**3. Block Volume Support Broken** - `pkg/driver/node_test.go`
- `TestNodeStageVolume_BlockVolume` - FAILING (staging directory not created)
- `TestNodePublishVolume_BlockVolume` - FAILING (nil pointer dereference)
- Root cause: NodeStageVolume for block volumes doesn't create staging dir or device metadata file
- **Impact:** Block volumes (KubeVirt, raw disks) don't work
- **Risk Level:** CRITICAL for v0.7.1 (blocks VMs on NVMe/TCP)

**4. Attachment Persistence Untested** - `pkg/attachment/persist.go` (147 lines)
- PV annotation persistence never tested
- No tests for retry logic with conflicts
- No tests for "not found" handling
- **Impact:** Debugging attachments may be unreliable
- **Risk Level:** LOW - Informational only, doesn't affect attachment state

**5. RDS Package Coverage Only 44.5%** - `pkg/rds/` (3024 lines code, 1211 test lines)
- `ssh_client.go` - 0% coverage (341 lines)
- `client.go` - 0% coverage (69 lines)
- Only `commands_test.go` and `pool_test.go` have tests
- Missing: SSH connection lifecycle, key loading, host verification
- **Impact:** RDS integration untested except parsing
- **Risk Level:** HIGH - Volume creation/deletion depends on this

**6. NVMe Package Only 43.3%** - `pkg/nvme/` (4031 lines)
- Coverage gaps in `nvme.go` (NVMe connect/disconnect)
- Limited error path testing
- No timeout/retry scenario tests
- **Impact:** Data plane (device connection) has edge cases untested
- **Risk Level:** MEDIUM - Most paths exercised but error handling thin

**7. Mount Package Only 55.9%** - `pkg/mount/` (3523 lines, 2190 test lines)
- 20KB code, 18KB tests but still only 55% coverage
- Likely missing: Permission errors, filesystem full, path escaping
- **Impact:** Pod mounting may fail ungracefully
- **Risk Level:** MEDIUM

**8. Driver Package Test Failures**
- Block volume support tests failing with nil pointer panic
- This indicates incomplete implementation or test mismatch
- **Impact:** v0.7.1 block volume feature incomplete
- **Risk Level:** CRITICAL

## Coverage Gaps Summary for v0.7.1

### By Severity

**BLOCKING (must fix for release):**
1. Block volume tests failing - `pkg/driver/node_test.go:728-889`
   - NodeStageVolume not writing device metadata
   - NodePublishVolume dereferencing nil pointer
   - Fix: Complete block volume implementation in `pkg/driver/node.go`

2. SSH client untested - `pkg/rds/ssh_client.go`
   - 341 lines of critical code with 0% test coverage
   - Tests needed for:
     - SSH key parsing and authentication
     - Host key verification
     - Connection timeouts and retries
     - Command execution
     - Connection cleanup

### HIGH PRIORITY (may impact v0.7.1)

3. RDS client untested - `pkg/rds/client.go`, `ssh_client.go`
   - Volume creation/deletion path needs SSH tests
   - Mock RDS server exists but SSH client itself untested

4. Attachment persistence untested - `pkg/attachment/persist.go`
   - Needs tests for PV annotation updates
   - Conflict retry logic untested

### MEDIUM PRIORITY (can defer if time-constrained)

5. gRPC server setup untested - `pkg/driver/server.go`
   - Endpoint parsing needs tests
   - Start/stop lifecycle untested

6. NVMe edge cases - `pkg/nvme/nvme.go`
   - Error scenarios need more coverage
   - Timeout handling untested

## Test Execution Environment

**Local Testing:**
```bash
# Run on macOS/Linux with no RDS required
make test
make test-integration      # Uses mock RDS server
make test-sanity-mock      # CSI sanity with mock

# With real RDS
RDS_ADDRESS=192.168.1.100 RDS_SSH_KEY=~/.ssh/id_rsa make test-sanity-real
```

**Docker Testing:**
```bash
make test-docker           # All tests in Docker Compose
make test-docker-sanity    # CSI sanity in Docker
make test-docker-integration # Integration tests in Docker
```

**CI Integration:**
- Makefile targets: `test`, `lint`, `verify`
- No external test runners configured (just `go test`)
- No coverage threshold enforcement (but should be added)

## Hard-to-Maintain Tests

### Test 1: Block Volume Mock Setup (node_test.go)

**Issue:** Complex mock filesystem setup for block volumes
```go
// Creates temporary /dev-like structures
tmpDir, _ := os.MkdirTemp("", "node-test-block-stage-*")
stagingPath := filepath.Join(tmpDir, "staging")
mockDevicePath := filepath.Join(tmpDir, "mock-nvme0n1")
// ... writes metadata files, creates directories
```

**Problem:** Test is fragile if NodeStageVolume implementation changes metadata location

**Recommendation:** Use `os.WriteFile` with known paths, assert file presence at end

### Test 2: NVMe ExecCommand Mocking (nvme_test.go)

**Issue:** TestHelperProcess pattern is complex and easy to break
```go
// Must coordinate between mockExecCommand and TestHelperProcess
// ENV variables pass state (STDOUT, STDERR, EXIT_CODE)
// Helper process must read correct env vars
```

**Problem:** If helper test is modified, all mocks break silently

**Recommendation:** Use `exec.CommandContext` with timeout for clearer semantics

### Test 3: Mount Stale Check with Mocking (node_test.go)

**Issue:** `createNodeServerWithStaleBehavior` has 150+ lines of setup
```go
// Simulates entire sysfs structure for NVMe device resolution
// Sets up directories, NQN files, block device entries
// Configures stale check behavior with multiple branches
```

**Problem:** Test is tightly coupled to resolver internals; changes to sysfs detection break test

**Recommendation:** Extract stale check setup into a test helper function with better documentation

## Recommendations for v0.7.1

1. **Fix Blocking Tests (IMMEDIATE)**
   - Implement block volume support in `pkg/driver/node.go` (staging dir, metadata file)
   - Or remove/disable tests until feature is ready

2. **Add SSH Client Tests (HIGH)**
   - Create `pkg/rds/ssh_client_test.go` (target: 80%+ coverage)
   - Test: key loading, host verification, command execution, timeouts
   - Use mock SSH server or `golang.org/x/crypto/ssh/test` if available

3. **Add Persistence Tests (MEDIUM)**
   - Create `pkg/attachment/persist_test.go`
   - Test: PV annotation updates, retry on conflict, "not found" handling

4. **Add Server Tests (MEDIUM)**
   - Add `server_test.go` for `parseEndpoint()` and server lifecycle

5. **Improve Coverage Enforcement**
   - Add coverage threshold to CI (e.g., min 70% per package)
   - Pre-commit hook to run `make verify`
   - Document coverage expectations per package

6. **Document Mock Strategy**
   - Document when to use interface mocks vs. fake clients
   - Add comments explaining mock setup patterns
   - Create test utility package for common mocks

---

*Testing analysis: 2026-02-04*
