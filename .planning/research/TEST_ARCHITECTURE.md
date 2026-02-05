# Test Infrastructure Architecture Research

**Domain:** CSI Driver Testing Infrastructure
**Researched:** 2026-02-04
**Confidence:** HIGH

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Test Infrastructure                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                         Unit Tests (148)                          │  │
│  │  • pkg/driver/*_test.go    • pkg/rds/*_test.go                   │  │
│  │  • pkg/nvme/*_test.go      • pkg/mount/*_test.go                 │  │
│  │  • Uses testify/assert     • gomock for interfaces               │  │
│  │  • Command-aware mocking   • In-memory state                     │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                    │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    Integration Tests (test/integration/)          │  │
│  │  ┌─────────────────────┐    ┌────────────────────────────────┐   │  │
│  │  │   Mock RDS Server   │←───│  Driver with RDS Client       │   │  │
│  │  │  • SSH server       │    │  • Full Controller/Node       │   │  │
│  │  │  • RouterOS CLI     │    │  • gRPC API testing           │   │  │
│  │  │  • State tracking   │    │  • Error injection            │   │  │
│  │  └─────────────────────┘    └────────────────────────────────┘   │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                    │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │              CSI Sanity Tests (test/sanity/)                      │  │
│  │  • kubernetes-csi/csi-test/v5 framework                          │  │
│  │  • Validates CSI spec compliance                                  │  │
│  │  • Tests via unix socket: Identity + Controller (no Node)        │  │
│  │  • Both mock and real RDS modes                                   │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                    │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │            E2E Tests (test/e2e/ - manual validation)              │  │
│  │  • YAML manifests for hardware testing                           │  │
│  │  • Manual validation plans (HARDWARE_VALIDATION.md)              │  │
│  │  • Real cluster deployment verification                           │  │
│  │  • KubeVirt and block device scenarios                            │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │        Docker Compose Test Environment (docker-compose.test.yml)  │  │
│  │  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐         │  │
│  │  │   Mock RDS   │   │  Controller  │   │  csi-sanity  │         │  │
│  │  │  (openssh)   │──▶│   (driver)   │──▶│   (tests)    │         │  │
│  │  └──────────────┘   └──────────────┘   └──────────────┘         │  │
│  │          ↓                                      ↓                  │  │
│  │    Simulates               gRPC Socket     Test Runner            │  │
│  │  RouterOS CLI              (unix domain)   (Ginkgo)               │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **Mock RDS Server** | Simulates RouterOS CLI via SSH | In-process SSH server (golang.org/x/crypto/ssh), parses commands, maintains state |
| **Unit Tests** | Test individual functions/methods | Go testing + testify assertions, gomock for interface mocks |
| **Integration Tests** | Test driver + mock RDS interaction | Real gRPC calls to driver with mock backend |
| **CSI Sanity Tests** | Validate CSI spec compliance | kubernetes-csi/csi-test framework via unix socket |
| **E2E Tests** | Validate in real cluster | Manual validation with YAML manifests + hardware |
| **Docker Compose** | CI test environment | Orchestrates mock RDS + driver + sanity tests |

## Recommended Project Structure

### Current Structure (RDS CSI Driver)

```
rds-csi-driver/
├── pkg/                          # Source code
│   ├── driver/                   # CSI implementation
│   │   ├── controller.go
│   │   ├── controller_test.go    # Unit tests with mocks
│   │   ├── node.go
│   │   └── node_test.go
│   ├── rds/                      # RDS client
│   │   ├── client.go
│   │   ├── client_test.go        # Unit tests
│   │   ├── ssh_client.go
│   │   └── ssh_client_test.go
│   ├── nvme/                     # NVMe operations
│   │   ├── nvme.go
│   │   └── nvme_test.go
│   └── mount/                    # Filesystem operations
│       ├── mount.go
│       └── mount_test.go
├── test/                         # Test infrastructure
│   ├── mock/                     # Test doubles
│   │   └── rds_server.go         # Mock RDS SSH server
│   ├── integration/              # Integration tests
│   │   ├── controller_integration_test.go
│   │   ├── hardware_integration_test.go
│   │   └── orphan_reconciler_integration_test.go
│   ├── sanity/                   # CSI sanity tests
│   │   └── run-sanity-tests.sh   # Test runner script
│   ├── e2e/                      # E2E test plans
│   │   ├── HARDWARE_VALIDATION.md
│   │   ├── PROGRESSIVE_VALIDATION.md
│   │   └── *.yaml                # Test manifests
│   └── docker/                   # Docker Compose fixtures
│       ├── mock-rds-config/
│       └── mock-rds-scripts/
├── docker-compose.test.yml       # CI test orchestration
└── Makefile                      # Test targets
```

### Structure Rationale

- **test/mock/**: Centralized test doubles, reused across integration and sanity tests. Single source of truth for mock behavior.
- **test/integration/**: Tests that require real gRPC → driver → mock RDS flow. More complex than unit tests, faster than E2E.
- **test/sanity/**: CSI spec compliance via official framework. Separate because it uses external tool (csi-sanity).
- **test/e2e/**: Hardware validation plans and manifests. Manual because requires real RDS and cluster.
- **Co-located unit tests**: `*_test.go` next to source files follows Go convention, enables quick edit-test cycle.

### Pattern from Production CSI Drivers

AWS EBS CSI and other production drivers follow similar patterns:
- Unit tests co-located with source
- `tests/e2e/` directory for end-to-end tests (using Ginkgo framework)
- Mock implementations in dedicated packages
- Sanity tests as separate Make target
- Docker Compose or similar for CI automation

## Architectural Patterns

### Pattern 1: Mock RDS Server (In-Process SSH)

**What:** Embed an SSH server in the test process that parses and responds to RouterOS CLI commands.

**When to use:** Integration tests, CSI sanity tests with mock backend

**Trade-offs:**
- **Pros:** Fast (no network I/O), deterministic, can inject failures, works in CI
- **Cons:** Behavior divergence from real RDS, must maintain command parsers

**Example:**
```go
// test/mock/rds_server.go
type MockRDSServer struct {
    listener net.Listener
    config   *ssh.ServerConfig
    volumes  map[string]*MockVolume  // State tracking
    mu       sync.RWMutex
}

func (s *MockRDSServer) Start() error {
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.address, s.port))
    // ...
    go s.acceptConnections()
}

func (s *MockRDSServer) handleSession(channel ssh.Channel) {
    // Parse commands: /disk add, /disk remove, /disk print
    // Update state in s.volumes
    // Return RouterOS-formatted output
}
```

**Current implementation:** RDS CSI driver already has this in `test/mock/rds_server.go` (17KB, ~500 LOC). It:
- Accepts SSH connections without authentication (testing only)
- Parses `/disk add`, `/disk remove`, `/disk print`, `/file print` commands
- Maintains in-memory volume and file state
- Returns RouterOS-style table output

### Pattern 2: Command-Aware Test Mocking

**What:** Instead of mocking method calls, mock SSH command execution and parse the command string to return appropriate responses.

**When to use:** Testing RDS client interactions where command syntax matters

**Trade-offs:**
- **Pros:** Tests actual command strings (catches typos, format changes), closer to real behavior
- **Cons:** More complex than method mocks, requires regex/parsing in tests

**Example:**
```go
// pkg/rds/commands_test.go
func TestDiskAdd(t *testing.T) {
    mockSSH := &MockSSHClient{
        ExecuteFunc: func(cmd string) (string, error) {
            // Verify command format
            if !strings.Contains(cmd, "/disk add") {
                t.Errorf("Expected /disk add command")
            }
            // Verify required parameters
            if !strings.Contains(cmd, "type=file") {
                t.Errorf("Missing type=file")
            }
            // Return mocked RouterOS output
            return "Flags: B t\n  0 slot=pvc-123...", nil
        },
    }

    client := &Client{ssh: mockSSH}
    err := client.DiskAdd(/* params */)
    assert.NoError(t, err)
}
```

**Current implementation:** RDS CSI driver uses this extensively. All `pkg/rds/*_test.go` files mock the `ExecuteCommand` method and verify actual command strings.

### Pattern 3: CSI Sanity Testing via Unix Socket

**What:** Use the official kubernetes-csi/csi-test framework to validate CSI spec compliance by connecting to driver via unix domain socket.

**When to use:** Mandatory for CSI drivers, validates gRPC interface conformance

**Trade-offs:**
- **Pros:** Official validation, catches spec violations, widely recognized
- **Cons:** Requires driver to be running, slower than unit tests

**Example:**
```bash
#!/bin/bash
# test/sanity/run-sanity-tests.sh

# Start driver
./bin/rds-csi-plugin --endpoint=unix:///tmp/csi.sock --controller &
DRIVER_PID=$!

# Wait for socket
while [ ! -S /tmp/csi.sock ]; do sleep 1; done

# Run sanity tests (skip Node tests if no hardware)
csi-sanity \
    --csi.endpoint=unix:///tmp/csi.sock \
    --csi.testvolumesize=1073741824 \
    --ginkgo.skip="Node" \
    --ginkgo.v

# Cleanup
kill $DRIVER_PID
```

**Current implementation:** RDS CSI driver has `test/sanity/run-sanity-tests.sh` that supports:
- Mock mode (Identity service only)
- Real RDS mode (Controller + Identity)
- Skip Node tests (requires hardware)

### Pattern 4: Docker Compose for CI

**What:** Orchestrate mock RDS + driver + tests in containers for reproducible CI environment.

**When to use:** CI/CD pipelines, developer onboarding, integration testing

**Trade-offs:**
- **Pros:** Reproducible, isolated, easy to run locally, matches CI
- **Cons:** Slower than native tests, requires Docker, resource overhead

**Example:**
```yaml
# docker-compose.test.yml
services:
  mock-rds:
    image: linuxserver/openssh-server
    environment:
      - USER_NAME=admin
      - PASSWORD_ACCESS=false
    volumes:
      - ./test/docker/mock-rds-scripts:/scripts

  csi-controller:
    build: .
    depends_on:
      mock-rds:
        condition: service_healthy
    volumes:
      - csi-socket:/csi
    command:
      - --endpoint=unix:///csi/csi.sock
      - --controller-mode
      - --rds-address=mock-rds

  csi-sanity:
    image: golang:1.24-alpine
    depends_on:
      csi-controller:
        condition: service_healthy
    volumes:
      - csi-socket:/csi
    command: |
      go install github.com/kubernetes-csi/csi-test/cmd/csi-sanity@latest
      csi-sanity --csi.endpoint=unix:///csi/csi.sock
```

**Current implementation:** RDS CSI driver has full Docker Compose setup with:
- Mock RDS (openssh-server with custom scripts)
- CSI controller
- CSI sanity test runner
- Integration test runner
- Health checks and dependencies

### Pattern 5: TestDriver Interface for E2E

**What:** Implement Kubernetes storage test suite's `TestDriver` interface to run standard storage tests against your CSI driver in a real cluster.

**When to use:** E2E testing in actual Kubernetes cluster (post-v0.9.0)

**Trade-offs:**
- **Pros:** Standard test suite, comprehensive coverage, validates K8s integration
- **Cons:** Requires real cluster, slow, complex setup

**Example:**
```go
// test/e2e/driver.go
type rdsCSITestDriver struct {
    driverInfo driver.DriverInfo
}

func (d *rdsCSITestDriver) GetDriverInfo() *driver.DriverInfo {
    return &d.driverInfo
}

func (d *rdsCSITestDriver) SkipUnsupportedTest(pattern storageframework.TestPattern) {
    // Skip multi-node tests if RWO only, etc.
}

func (d *rdsCSITestDriver) PrepareTest(f *framework.Framework) (*storageframework.PerTestConfig, func()) {
    // Deploy driver, create StorageClass
    return testConfig, cleanupFunc
}
```

**Current implementation:** RDS CSI driver has manual E2E validation plans but not automated Ginkgo-based E2E tests yet. This is a v0.9.0 goal.

## Data Flow

### Unit Test Flow

```
Test Case
    ↓
Mock Interface (gomock)
    ↓ (method call)
Function Under Test
    ↓ (returns)
Assertion (testify)
```

### Integration Test Flow

```
Test Case
    ↓ (creates)
Mock RDS Server (in-process SSH)
    ↑           ↓ (SSH connection)
    │      RDS Client
    │           ↓ (method call)
    │      Driver (Controller/Node)
    │           ↓ (gRPC)
    └─────  Test Assertion
```

### CSI Sanity Test Flow

```
csi-sanity binary
    ↓ (gRPC over unix socket)
CSI Driver (running process)
    ↓ (SSH)
Mock RDS Server OR Real RDS
    ↓
State Change (volume created/deleted)
    ↓
Driver returns gRPC response
    ↓
csi-sanity validates spec compliance
```

### Docker Compose Test Flow

```
docker-compose up
    ↓
Mock RDS container starts
    ↓ (health check: SSH listening)
CSI Controller container starts
    ↓ (connects to mock-rds)
Driver ready (socket created)
    ↓ (health check: socket exists)
csi-sanity container starts
    ↓ (runs tests via socket)
Results printed
    ↓
docker-compose down (cleanup)
```

## State Management in Tests

### Unit Tests: Stateless Mocks

Most unit tests use stateless mocks that return canned responses. State is implicit in the test case.

```go
mockSSH.ExecuteFunc = func(cmd string) (string, error) {
    // Always return success for this test
    return "success", nil
}
```

### Integration Tests: Stateful Mock Server

Mock RDS server maintains state across commands:

```go
type MockRDSServer struct {
    volumes map[string]*MockVolume  // Persistent within test
    files   map[string]*MockFile
    mu      sync.RWMutex            // Thread-safe
}

// State transitions:
// Test creates volume → volumes[id] = &MockVolume{...}
// Test deletes volume → delete(volumes, id)
// Test lists volumes  → return all values from volumes map
```

**State initialization:**
- Clean state at test start (NewMockRDSServer creates empty maps)
- Can pre-populate for specific scenarios (test fixtures)
- State persists for duration of test, cleaned up in defer

### CSI Sanity Tests: Driver State

Driver maintains state internally (same as production):
- AttachmentManager: volume → node mappings
- RDS state: volumes exist or don't

Tests validate that state transitions are correct (create → exists, delete → doesn't exist).

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 0-100 tests | Current approach is fine: co-located unit tests, single mock server, Docker Compose for CI |
| 100-500 tests | Parallelize test execution (go test -p=4), consider table-driven tests to reduce duplication |
| 500+ tests | Split tests into packages by domain (controller_test/, node_test/), use test fixtures, consider test sharding in CI |

### Scaling Priorities

1. **First bottleneck:** Test execution time (unit tests fast, integration tests slower)
   - **Fix:** Run tests in parallel (`go test -p=N`), use `-short` flag to skip slow tests

2. **Second bottleneck:** Mock RDS server complexity (more commands = more parsers)
   - **Fix:** Generate mock from RouterOS API spec (if available), or extract common parser logic

3. **Third bottleneck:** E2E test flakiness (network, timing, cluster state)
   - **Fix:** Retries with exponential backoff, proper cleanup in defer/AfterEach, isolated namespaces

## Anti-Patterns

### Anti-Pattern 1: Mocking Only Methods, Not Behavior

**What people do:** Mock interface methods to return success, but don't validate that the behavior is correct.

```go
// BAD: Doesn't verify command format
mockClient.EXPECT().
    ExecuteCommand(gomock.Any()).
    Return("success", nil)
```

**Why it's wrong:** Test passes even if command is malformed or missing parameters.

**Do this instead:** Command-aware mocking that validates command strings.

```go
// GOOD: Validates actual command
mockClient.ExecuteFunc = func(cmd string) (string, error) {
    if !strings.Contains(cmd, "slot="+expectedSlot) {
        t.Errorf("Command missing expected slot: %s", cmd)
    }
    return routerosOutput, nil
}
```

### Anti-Pattern 2: Testing Implementation, Not Interface

**What people do:** Test internal implementation details instead of public API behavior.

```go
// BAD: Testing private method
func TestParseRouterOSOutput(t *testing.T) {
    // This is an implementation detail
}
```

**Why it's wrong:** Tests break when refactoring internals, even if public behavior is correct.

**Do this instead:** Test public API behavior, implementation details are exercised indirectly.

```go
// GOOD: Testing public API
func TestDiskAdd_CreatesVolume(t *testing.T) {
    // Public method that uses parseRouterOSOutput internally
    err := client.DiskAdd(params)
    assert.NoError(t, err)

    // Verify behavior, not implementation
    volume, exists := mockRDS.GetVolume("pvc-123")
    assert.True(t, exists)
}
```

### Anti-Pattern 3: Stateless Tests with Stateful System

**What people do:** Write tests that assume clean state but don't clean up.

```go
// BAD: Leaves state around
func TestCreateVolume(t *testing.T) {
    driver.CreateVolume(req)
    // No cleanup
}

func TestDeleteVolume(t *testing.T) {
    // Might fail if previous test left volume
    driver.DeleteVolume(req)
}
```

**Why it's wrong:** Tests become order-dependent, flaky, hard to debug.

**Do this instead:** Clean state in setup/teardown.

```go
// GOOD: Isolated state
func TestCreateVolume(t *testing.T) {
    mockRDS, _ := mock.NewMockRDSServer(12345)
    mockRDS.Start()
    defer mockRDS.Stop()  // Cleanup

    driver := setupDriverWithMock(mockRDS)
    driver.CreateVolume(req)
}
```

### Anti-Pattern 4: Testing Happy Path Only

**What people do:** Only test success cases, ignore error conditions.

```go
// BAD: Only tests success
func TestCreateVolume(t *testing.T) {
    resp, err := driver.CreateVolume(validReq)
    assert.NoError(t, err)
    assert.NotNil(t, resp)
}
```

**Why it's wrong:** Production systems encounter errors. Tests don't catch error handling bugs.

**Do this instead:** Test error paths explicitly.

```go
// GOOD: Tests error conditions
func TestCreateVolume_Errors(t *testing.T) {
    tests := []struct{
        name string
        req *csi.CreateVolumeRequest
        mockSetup func(*mock.MockRDSServer)
        wantErr codes.Code
    }{
        {"missing name", &csi.CreateVolumeRequest{}, nil, codes.InvalidArgument},
        {"disk full", validReq, func(m *mock.MockRDSServer) {
            m.InjectError("disk add", "not enough space")
        }, codes.ResourceExhausted},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test each error condition
        })
    }
}
```

### Anti-Pattern 5: E2E Tests Without Cleanup

**What people do:** Create resources in cluster but don't delete them after test.

**Why it's wrong:** Resources leak, cluster becomes polluted, affects other tests.

**Do this instead:** Always cleanup with defer or Ginkgo AfterEach.

```go
// GOOD: Cleanup guaranteed
BeforeEach(func() {
    namespace = framework.CreateTestNamespace(f)
})

AfterEach(func() {
    framework.DeleteTestNamespace(f, namespace)
})
```

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| **kubernetes-csi/csi-test** | CLI tool via shell script | Installed with `go install`, run via `test/sanity/run-sanity-tests.sh` |
| **Docker Compose** | Container orchestration | Used for CI, `make test-docker` target |
| **Real RDS** | SSH connection | Only for hardware validation, not automated tests |
| **Real Kubernetes** | kubectl + YAML manifests | Manual E2E validation, test/e2e/ directory |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| **Unit test ↔ Source** | Direct function calls | Co-located, fast, import package under test |
| **Integration test ↔ Mock RDS** | SSH over localhost | Mock server in same process, different goroutine |
| **Sanity test ↔ Driver** | gRPC over unix socket | Driver runs as separate process |
| **E2E test ↔ Cluster** | kubectl (REST API) | Real cluster, real resources |

## Mock RDS Architecture Deep Dive

The Mock RDS Server is the linchpin of the test infrastructure. Here's its architecture:

### Component Structure

```
MockRDSServer
├── SSH Server (golang.org/x/crypto/ssh)
│   ├── Listener (net.Listener)
│   ├── ServerConfig (authentication)
│   └── Session Handlers
├── Command Parser
│   ├── Regex patterns for each command
│   ├── Parameter extraction
│   └── Validation logic
├── State Management
│   ├── volumes map[string]*MockVolume
│   ├── files map[string]*MockFile
│   └── sync.RWMutex (thread-safe)
└── Response Formatter
    ├── RouterOS table format
    ├── Column alignment
    └── Flag encoding (B, t, etc.)
```

### State Tracking

```go
type MockVolume struct {
    Slot          string  // pvc-<uuid>
    FilePath      string  // /storage-pool/metal-csi/pvc-<uuid>.img
    FileSizeBytes int64   // Capacity
    NVMETCPPort   int     // Usually 4420
    NVMETCPNQN    string  // nqn.2000-02.com.mikrotik:pvc-<uuid>
    Exported      bool    // Is NVMe/TCP export enabled
}

type MockFile struct {
    Path      string
    SizeBytes int64
    Type      string  // "file" or "directory"
    CreatedAt string
}
```

**State transitions:**
1. `/disk add` → Create MockVolume + MockFile
2. `/disk remove` → Delete MockVolume (file may remain if orphaned)
3. `/disk print` → List volumes (query volumes map)
4. `/file print` → List files (query files map)

### Command Parsing

Uses regex to extract parameters from RouterOS CLI commands:

```go
// Example: /disk add type=file file-path=/path slot=id file-size=10G nvme-tcp-export=yes
diskAddRegex := regexp.MustCompile(`/disk add\s+(.*)`)
params := parseParams(matches[1])  // type=file, file-path=/path, etc.

// Validate required parameters
if params["type"] != "file" {
    return errorResponse("only type=file supported")
}
```

### Response Format

Returns RouterOS-style table output:

```
Flags: B t
  0 slot=pvc-123
    file=/storage-pool/metal-csi/pvc-123.img
    size=10737418240
    nvme-tcp-server-port=4420
    nvme-tcp-server-nqn=nqn.2000-02.com.mikrotik:pvc-123
```

**Key challenge:** Matching real RouterOS output format exactly. Any discrepancy causes parser failures in driver code.

## Test Organization for v0.9.0

For the test infrastructure integration milestone (v0.9.0), build in this order:

### Phase 1: CSI Sanity Integration (1-2 days)

**Goal:** Automate CSI sanity tests in CI

**Tasks:**
1. Enhance `test/sanity/run-sanity-tests.sh` to support CI mode (no TTY)
2. Add `make test-sanity-ci` target to Makefile
3. Ensure Docker Compose sanity service works reliably
4. Document how to run sanity tests locally

**Deliverables:**
- CI can run sanity tests automatically
- Developers can run `make test-sanity` locally

**Why first:** CSI sanity tests are the gold standard for spec compliance. Get this working early.

### Phase 2: E2E Test Framework (2-3 days)

**Goal:** Automated E2E tests using Kubernetes storage test suite

**Tasks:**
1. Implement `TestDriver` interface in `test/e2e/driver.go`
2. Create test suite in `test/e2e/e2e_test.go` using Ginkgo
3. Skip unsupported tests (e.g., snapshots)
4. Add `make test-e2e` target

**Deliverables:**
- Automated E2E tests run against real cluster
- Tests validate basic volume lifecycle (create, mount, write, unmount, delete)

**Why second:** E2E tests validate the full stack. Having this framework enables catching regressions.

### Phase 3: Enhanced Mock RDS (1-2 days)

**Goal:** Mock RDS supports NVMe/TCP simulation (for future Node tests)

**Tasks:**
1. Add NVMe/TCP simulation to Mock RDS (fake nvme-cli responses)
2. Support `/disk print detail` with NVMe connection status
3. Add error injection capabilities (network failures, disk full)

**Deliverables:**
- Mock RDS can simulate NVMe/TCP targets
- Can test Node service operations in integration tests

**Why third:** Node tests require NVMe simulation. This unblocks future test expansion.

### Phase 4: Test Data and Fixtures (1 day)

**Goal:** Standardized test fixtures for consistent testing

**Tasks:**
1. Create `test/fixtures/` directory
2. Define standard PVC manifests, StorageClass configs
3. Add test data generators (volume sizes, names, etc.)
4. Document fixture usage

**Deliverables:**
- Test fixtures reduce duplication
- Consistent test data across test types

**Why fourth:** Once test frameworks are in place, fixtures reduce maintenance burden.

### Build Order Rationale

- **Sanity first:** Validates CSI spec compliance, highest ROI
- **E2E second:** Validates Kubernetes integration, catches cluster-specific issues
- **Mock enhancement third:** Enables more comprehensive integration tests
- **Fixtures last:** Improves maintainability after test infrastructure is stable

## Sources

### High Confidence (Official Documentation)

- [Testing of CSI Drivers | Kubernetes](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/) - CSI testing layers (sanity, E2E)
- [kubernetes-csi/csi-test GitHub](https://github.com/kubernetes-csi/csi-test) - Official CSI test framework
- [CSI Developer Documentation - Testing](https://kubernetes-csi.github.io/docs/testing-drivers.html) - Test organization
- [Functional Testing - Kubernetes CSI](https://kubernetes-csi.github.io/docs/functional-testing.html) - TestDriver interface

### Medium Confidence (Production Implementations)

- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver) - Test structure reference
- [kubernetes/kubernetes - test/e2e/storage/](https://github.com/kubernetes/kubernetes/tree/master/test/e2e/storage) - E2E test patterns
- [Mock CSI Driver (csi-test)](https://pkg.go.dev/github.com/kubernetes-csi/csi-test/v4/mock) - GoMock integration

### Low Confidence (Needs Verification)

- Exact test count (claimed 148, actually 1015 in `go test` output) - discrepancy suggests some tests are subtests
- Docker Compose test reliability - needs CI validation
- E2E test framework integration complexity - estimated 2-3 days, may vary

---
*Architecture research for: RDS CSI Driver Test Infrastructure*
*Researched: 2026-02-04*
*Confidence: HIGH - Based on official K8s CSI documentation, production driver patterns, and existing codebase analysis*
