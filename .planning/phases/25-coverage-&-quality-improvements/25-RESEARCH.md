# Phase 25: Coverage & Quality Improvements - Research

**Researched:** 2026-02-05
**Domain:** Go test coverage enforcement and quality improvements
**Confidence:** HIGH

## Summary

This phase focuses on increasing test coverage to 70% with emphasis on error path validation and quality enforcement. The project currently has 65.3% overall coverage with go-test-coverage tool already configured (v2, latest from github.com/vladopajic/go-test-coverage). The research reveals that Go 1.24 provides built-in coverage tools with atomic mode for race-safe coverage collection, table-driven tests are the idiomatic pattern for comprehensive error path testing, and CSI driver testing requires specific attention to gRPC error codes and negative scenarios.

The current project has strong testing infrastructure in place: 52 test files, comprehensive mock RDS server, CSI sanity tests with csi-test/v5, E2E tests using Ginkgo/Gomega, and CI coverage enforcement at 60% threshold. The coverage configuration is already sophisticated with package-specific overrides (pkg/rds: 70%, pkg/utils: 80%, pkg/attachment: 80%).

**Primary recommendation:** Focus on adding table-driven error path tests to packages below target thresholds (node service error handling, controller SSH failure scenarios), implement systematic negative test scenarios for CSI gRPC error codes, detect and fix flaky tests using -count=N pattern, and raise CI threshold from 60% to 65% baseline.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go test (built-in) | 1.24 | Native coverage with -coverprofile and -covermode=atomic | Official Go toolchain, no dependencies required |
| go-test-coverage | v2 (latest) | Coverage threshold enforcement and reporting | Already integrated, widely used for CI enforcement |
| testing package | stdlib | Built-in testing framework | Standard Go testing approach |
| stretchr/testify | v1.11.1 | Assertion library with require/assert | Already in project, reduces boilerplate |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Ginkgo/Gomega | v2.22.0/v1.36.1 | BDD-style testing framework | Already used for E2E tests, provides Eventually/Consistently for async testing |
| csi-test | v5.4.0 | CSI specification compliance testing | Already integrated for sanity tests |
| golang.org/x/tools/cover | stdlib | Coverage visualization and reporting | For generating HTML coverage reports |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| go-test-coverage | go tool cover with bash scripts | go-test-coverage provides package-level thresholds and YAML config |
| testify | gomock + bare testing package | testify is simpler, already integrated, gomock adds complexity |
| Ginkgo | bare testing package | Ginkgo already used for E2E, provides better async primitives |

**Installation:**
```bash
# Already in go.mod, but for reference:
go install github.com/vladopajic/go-test-coverage/v2@latest
go install github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@latest
```

## Architecture Patterns

### Recommended Project Structure
Current structure is already optimal:
```
pkg/
├── driver/          # CSI service implementations - needs error path coverage
├── rds/             # RDS client - strong coverage already (70%+)
├── nvme/            # NVMe operations - hardware-dependent, 55% acceptable
├── mount/           # Mount operations - needs error scenarios
├── utils/           # Utilities - target 80%, currently good
└── attachment/      # State management - target 80%

test/
├── e2e/             # End-to-end tests with Ginkgo
├── integration/     # Integration tests with mock RDS
├── mock/            # Mock RDS server implementation
└── sanity/          # CSI spec compliance tests
```

### Pattern 1: Table-Driven Error Path Tests
**What:** Define test cases as slice of structs with inputs, expected outputs, and error expectations
**When to use:** For any function with multiple error paths (validation, I/O operations, gRPC handlers)
**Example:**
```go
// Source: Existing pattern in pkg/rds/client_test.go
func TestNodeStageVolume_ErrorPaths(t *testing.T) {
    tests := []struct {
        name      string
        volumeID  string
        target    string
        setupMock func(*mockNVME, *mockMounter)
        expectErr bool
        errCode   codes.Code
        errMsg    string
    }{
        {
            name:     "nvme connection failure",
            volumeID: "pvc-test-123",
            target:   "/var/lib/kubelet/stage/vol",
            setupMock: func(nvme *mockNVME, mounter *mockMounter) {
                nvme.connectErr = errors.New("nvme: connection timeout")
            },
            expectErr: true,
            errCode:   codes.Internal,
            errMsg:    "failed to connect NVMe",
        },
        {
            name:     "device path not found",
            volumeID: "pvc-test-123",
            target:   "/var/lib/kubelet/stage/vol",
            setupMock: func(nvme *mockNVME, mounter *mockMounter) {
                nvme.devicePath = "" // Empty path indicates not found
            },
            expectErr: true,
            errCode:   codes.Internal,
            errMsg:    "device not found",
        },
        // ... more error scenarios
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockNVME := &mockNVME{}
            mockMounter := &mockMounter{}
            tt.setupMock(mockNVME, mockMounter)

            ns := &NodeServer{nvme: mockNVME, mounter: mockMounter}
            _, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
                VolumeId:          tt.volumeID,
                StagingTargetPath: tt.target,
            })

            if tt.expectErr {
                require.Error(t, err)
                st, ok := status.FromError(err)
                require.True(t, ok, "error should be gRPC status")
                assert.Equal(t, tt.errCode, st.Code())
                assert.Contains(t, st.Message(), tt.errMsg)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Pattern 2: Negative Test Scenarios for CSI gRPC Errors
**What:** Test that CSI RPCs return correct gRPC error codes per CSI spec
**When to use:** For all CSI Controller and Node service methods
**Example:**
```go
// Source: CSI spec requirements for error codes
func TestCreateVolume_NegativeScenarios(t *testing.T) {
    tests := []struct {
        name           string
        request        *csi.CreateVolumeRequest
        mockRDSError   error
        expectedCode   codes.Code
        expectedMsg    string
    }{
        {
            name: "invalid volume ID format",
            request: &csi.CreateVolumeRequest{
                Name: "invalid; drop table",
                VolumeCapabilities: []*csi.VolumeCapability{validCap},
                CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
            },
            expectedCode: codes.InvalidArgument,
            expectedMsg:  "invalid characters",
        },
        {
            name: "capacity exceeds RDS limit",
            request: &csi.CreateVolumeRequest{
                Name: "pvc-test-123",
                VolumeCapabilities: []*csi.VolumeCapability{validCap},
                CapacityRange: &csi.CapacityRange{RequiredBytes: 20 * TiB}, // Over 16 TiB limit
            },
            expectedCode: codes.OutOfRange,
            expectedMsg:  "exceeds maximum",
        },
        {
            name: "RDS disk full",
            request: &csi.CreateVolumeRequest{
                Name: "pvc-test-123",
                VolumeCapabilities: []*csi.VolumeCapability{validCap},
                CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
            },
            mockRDSError: errors.New("not enough space"),
            expectedCode: codes.ResourceExhausted,
            expectedMsg:  "insufficient capacity",
        },
        {
            name: "SSH connection failure",
            request: &csi.CreateVolumeRequest{
                Name: "pvc-test-123",
                VolumeCapabilities: []*csi.VolumeCapability{validCap},
                CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
            },
            mockRDSError: errors.New("ssh: handshake failed"),
            expectedCode: codes.Unavailable,
            expectedMsg:  "RDS unavailable",
        },
        {
            name: "volume already exists (idempotency check)",
            request: &csi.CreateVolumeRequest{
                Name: "pvc-existing-123",
                VolumeCapabilities: []*csi.VolumeCapability{validCap},
                CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
            },
            expectedCode: codes.AlreadyExists, // Or OK if idempotent
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mock RDS with error behavior
            mockRDS := setupMockWithError(tt.mockRDSError)
            cs := &ControllerServer{rdsClient: mockRDS}

            resp, err := cs.CreateVolume(context.Background(), tt.request)

            require.Error(t, err)
            st, ok := status.FromError(err)
            require.True(t, ok, "must return gRPC status error")
            assert.Equal(t, tt.expectedCode, st.Code())
            assert.Contains(t, st.Message(), tt.expectedMsg)
            assert.Nil(t, resp)
        })
    }
}
```

### Pattern 3: Flaky Test Detection with -count
**What:** Run tests multiple times to identify non-deterministic failures
**When to use:** Before phase completion, and when investigating test failures
**Example:**
```bash
# Run all tests 10 times to detect flaky behavior
go test -count=10 -race ./pkg/...

# Run specific package suspected of flakiness
go test -count=50 -v ./pkg/mount/...

# With JSON output for automated analysis
go test -count=100 -json ./pkg/... > test-results.json
```

### Pattern 4: Mock Injection for Error Scenarios
**What:** Use mock implementations with configurable error behavior
**When to use:** Testing error paths without requiring hardware or network failures
**Example:**
```go
// Source: Existing pattern in pkg/driver/node_test.go
type mockNVME struct {
    connectErr    error
    disconnectErr error
    devicePath    string
    resolveErr    error
}

func (m *mockNVME) Connect(ctx context.Context, target nvme.Target) error {
    if m.connectErr != nil {
        return m.connectErr
    }
    return nil
}

// Usage in test
func TestNVMEConnectionFailure(t *testing.T) {
    mock := &mockNVME{
        connectErr: errors.New("connection timeout"),
    }
    ns := &NodeServer{nvme: mock}

    _, err := ns.NodeStageVolume(ctx, req)
    require.Error(t, err)
    // Verify error handling behavior
}
```

### Anti-Patterns to Avoid
- **Sleeping for synchronization:** Use Eventually/Consistently from Gomega or channels for proper synchronization, never time.Sleep() in tests
- **Global state pollution:** Always reset test state in t.Cleanup() or AfterEach(), avoid shared variables between tests
- **Ignoring race detector warnings:** Run with -race, treat all race conditions as bugs even if tests pass
- **Testing implementation details:** Test public API behavior, not internal implementation
- **Quarantining flaky tests without investigation:** Understand root cause, fix or document why skipped

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Coverage threshold enforcement | Custom bash scripts parsing go tool cover output | go-test-coverage tool (already integrated) | Handles package-level thresholds, YAML config, badge generation, already configured |
| Test retry logic | Custom retry loops | Ginkgo's Eventually/Consistently (already available) | Handles timing, provides clear failure messages, prevents infinite loops |
| Mock SSH server | Custom TCP server with command parsing | test/mock/rds_server.go (already exists) | Full RouterOS CLI emulation, maintains state, used across test suites |
| CSI gRPC client setup | Manual grpc.Dial and client creation | test/e2e/e2e_suite_test.go patterns (already exists) | Handles Unix socket, cleanup, timeout, consistent across tests |
| Coverage report visualization | Parsing coverage.out manually | go tool cover -html=coverage.out | Built-in, generates interactive HTML, works with all coverage modes |

**Key insight:** The project already has sophisticated test infrastructure. Don't rebuild what exists - extend it to cover missing error paths.

## Common Pitfalls

### Pitfall 1: Race Detector Disabled During Coverage Collection
**What goes wrong:** Coverage reports collected without -race may miss concurrency bugs
**Why it happens:** Performance concern or incorrect belief they're incompatible
**How to avoid:** Always use -covermode=atomic with -race, Go 1.3+ handles this automatically
**Warning signs:** Tests pass but production has race conditions, coverage CI job doesn't use -race flag

### Pitfall 2: Testing Happy Paths Only
**What goes wrong:** Coverage metrics look good but error paths are untested, production failures not caught
**Why it happens:** Error scenarios require more setup, developers focus on feature completion
**How to avoid:** Require at least 2 error cases per success case in table-driven tests, review coverage of error returns specifically
**Warning signs:** High function coverage but low branch coverage, error handling code never executed in tests

### Pitfall 3: Flaky Tests Due to Timing Assumptions
**What goes wrong:** Tests pass locally but fail intermittently in CI
**Why it happens:** Using fixed time.Sleep() instead of polling, map iteration order randomness, shared resources
**How to avoid:** Use Eventually() with timeout for async operations, sort map keys before assertions, isolate test resources
**Warning signs:** Test failures disappear on retry, failures increase under load, different results between CI runs
**Detection:** Run go test -count=10 to reproduce locally

### Pitfall 4: Incorrect gRPC Error Codes
**What goes wrong:** CSI driver returns wrong error code, breaks Kubernetes CSI behavior
**Why it happens:** Using generic errors.New() instead of status.Error(), misunderstanding CSI spec error semantics
**How to avoid:** Use google.golang.org/grpc/status package, map error conditions to CSI-required codes (InvalidArgument, ResourceExhausted, Unavailable)
**Warning signs:** Kubernetes retries when it shouldn't, PVC stuck in Pending with wrong error message

### Pitfall 5: Coverage Threshold Too Aggressive
**What goes wrong:** CI fails on legitimate hardware-dependent code that can't be tested
**Why it happens:** Setting uniform threshold without considering package characteristics
**How to avoid:** Use package-specific overrides in .go-test-coverage.yml (already configured correctly in this project)
**Warning signs:** Developers skip coverage CI, add unnecessary mock complexity, test code becomes longer than implementation

### Pitfall 6: Not Testing Idempotency
**What goes wrong:** CSI driver creates duplicate resources or fails on retry
**Why it happens:** Only testing first call, not subsequent identical calls
**How to avoid:** Every CSI RPC test should call twice with same parameters, verify second call succeeds or returns AlreadyExists
**Warning signs:** Volume leaked after pod deletion, duplicate NVMe exports, CSI sanity idempotency tests fail

## Code Examples

Verified patterns from official sources:

### Coverage Collection with Race Detector
```bash
# Source: Go official documentation - go.dev/doc/articles/race_detector
# Run tests with coverage and race detection together
go test -race -covermode=atomic -coverprofile=coverage.out ./pkg/...

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

### Table-Driven Test Structure
```go
// Source: Go Wiki TableDrivenTests - github.com/golang/go/wiki/TableDrivenTests
func TestValidateVolumeID(t *testing.T) {
    tests := []struct {
        name      string
        volumeID  string
        wantErr   bool
        errSubstr string
    }{
        {
            name:     "valid pvc format",
            volumeID: "pvc-12345678-1234-1234-1234-123456789abc",
            wantErr:  false,
        },
        {
            name:      "missing pvc prefix",
            volumeID:  "12345678-1234-1234-1234-123456789abc",
            wantErr:   true,
            errSubstr: "must start with pvc-",
        },
        {
            name:      "invalid characters",
            volumeID:  "pvc-test; rm -rf /",
            wantErr:   true,
            errSubstr: "invalid characters",
        },
        {
            name:      "empty string",
            volumeID:  "",
            wantErr:   true,
            errSubstr: "cannot be empty",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateVolumeID(tt.volumeID)

            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errSubstr)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### CSI Error Code Mapping
```go
// Source: CSI spec and kubernetes-csi/csi-test patterns
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

// Return correct CSI error codes
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
    // Validation error
    if err := validateRequest(req); err != nil {
        return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
    }

    // Resource exhausted
    if err := cs.rds.CreateVolume(ctx, vol); err != nil {
        if strings.Contains(err.Error(), "not enough space") {
            return nil, status.Errorf(codes.ResourceExhausted, "insufficient capacity: %v", err)
        }
        if strings.Contains(err.Error(), "connection refused") {
            return nil, status.Errorf(codes.Unavailable, "RDS unavailable: %v", err)
        }
        return nil, status.Errorf(codes.Internal, "failed to create volume: %v", err)
    }

    return &csi.CreateVolumeResponse{Volume: vol}, nil
}

// Test the error codes
func TestCreateVolume_ErrorCodes(t *testing.T) {
    tests := []struct {
        name       string
        mockErr    error
        expectCode codes.Code
    }{
        {"disk full", errors.New("not enough space"), codes.ResourceExhausted},
        {"ssh failure", errors.New("connection refused"), codes.Unavailable},
        {"unknown error", errors.New("something else"), codes.Internal},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cs := &ControllerServer{rds: &mockRDSWithError{err: tt.mockErr}}
            _, err := cs.CreateVolume(ctx, req)

            st, ok := status.FromError(err)
            require.True(t, ok, "must be gRPC status error")
            assert.Equal(t, tt.expectCode, st.Code())
        })
    }
}
```

### Flaky Test Detection
```go
// Source: Testing patterns from betterstack.com/community/guides/testing
// Use subtests with t.Parallel() for race detection
func TestConcurrentVolumeOperations(t *testing.T) {
    tests := []struct {
        name string
        op   func() error
    }{
        {"create_vol_1", func() error { return cs.CreateVolume(ctx, req1) }},
        {"create_vol_2", func() error { return cs.CreateVolume(ctx, req2) }},
        {"delete_vol_1", func() error { return cs.DeleteVolume(ctx, delReq1) }},
    }

    for _, tt := range tests {
        tt := tt // Capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            err := tt.op()
            require.NoError(t, err)
        })
    }
}

// Use Eventually for async operations (Ginkgo pattern)
Eventually(func() bool {
    vol, err := cs.GetVolume(volumeID)
    return err == nil && vol.Status == "ready"
}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).Should(BeTrue())
```

### Coverage Threshold Enforcement
```yaml
# Source: github.com/vladopajic/go-test-coverage configuration
# .go-test-coverage.yml (already in project, example of proper config)
threshold:
  file: 0       # Disabled - enforce at package level
  package: 60   # Default minimum
  total: 55     # Overall minimum

override:
  # Critical packages with higher requirements
  - threshold: 70
    path: ^pkg/rds$
  - threshold: 70
    path: ^pkg/mount$
  - threshold: 55
    path: ^pkg/nvme$  # Lower due to hardware dependencies
  - threshold: 80
    path: ^pkg/utils$ # Pure utility code
  - threshold: 80
    path: ^pkg/attachment$

exclude:
  - path: _test\.go$
  - path: ^cmd/
  - path: ^test/integration/
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| go test -cover (set mode) | go test -covermode=atomic with -race | Go 1.3+ (2014) | Atomic mode prevents false race detection, safe for concurrent tests |
| Manual coverage scripts | go-test-coverage tool with YAML config | 2020+ | Package-level thresholds, CI integration, prevents regression |
| Ginkgo v1 | Ginkgo v2 with Context/Describe improvements | 2021 | Better async primitives (Eventually/Consistently), improved parallel execution |
| csi-test v3 | csi-test v5 with CSI spec v1.12 | 2023 | Updated CSI spec compliance, better error validation |
| go build then test coverage | go build -cover for integration tests | Go 1.20+ (2023) | Coverage for binaries, enables integration test coverage |

**Deprecated/outdated:**
- go test -covermode=set: Use atomic mode for race-safe coverage collection
- Fixed time.Sleep in tests: Use Eventually/Consistently for proper async testing
- Generic errors.New() in CSI handlers: Use status.Error() with proper gRPC codes

## Open Questions

Things that couldn't be fully resolved:

1. **Hardware-Dependent Code Coverage**
   - What we know: pkg/nvme has hardware dependencies (NVMe/TCP connections), currently at 55% coverage threshold
   - What's unclear: Optimal coverage target for hardware-dependent code, whether additional mocking helps
   - Recommendation: Keep 55% threshold for pkg/nvme, focus on logic validation over hardware interaction, consider hardware-in-the-loop testing for full validation

2. **Flaky Test Baseline**
   - What we know: Project uses -race flag in CI, has E2E tests with async operations
   - What's unclear: Current flaky test status, whether existing tests are deterministic
   - Recommendation: Run go test -count=50 ./... in pre-phase validation to establish baseline, document any intentionally skipped tests in TESTING.md

3. **CSI Sanity Test Coverage Mapping**
   - What we know: csi-sanity tests CSI spec compliance, runs against mock RDS
   - What's unclear: Which specific error scenarios csi-sanity validates, gaps in negative testing
   - Recommendation: Review csi-sanity source to identify tested error paths, add supplementary negative tests for gaps

4. **E2E Test Isolation**
   - What we know: E2E tests use Ginkgo with BeforeSuite/AfterSuite, mock RDS maintains state
   - What's unclear: Whether test isolation is complete, potential for state pollution between specs
   - Recommendation: Each E2E spec should use unique volume IDs, consider resetting mock RDS state between specs if flakiness occurs

## Sources

### Primary (HIGH confidence)
- Go Official Documentation - Data Race Detector: https://go.dev/doc/articles/race_detector
- Go Official Documentation - Coverage for Integration Tests: https://go.dev/blog/integration-test-coverage
- Go Wiki - TableDrivenTests: https://github.com/golang/go/wiki/TableDrivenTests
- go-test-coverage GitHub Repository: https://github.com/vladopajic/go-test-coverage
- Kubernetes CSI Developer Documentation: https://kubernetes-csi.github.io/docs/testing-drivers.html
- CSI Specification: https://github.com/container-storage-interface/spec

### Secondary (MEDIUM confidence)
- Go Code Coverage Best Practices (OtterWise): https://getotterwise.com/blog/go-code-coverage-tracking-best-practices-cicd
- Table-Driven Tests Practical Guide (2026): https://medium.com/@mojimich2015/table-driven-tests-in-go-a-practical-guide-8135dcbc27ca
- Flaky Tests in Go - Why They Happen: https://levelup.gitconnected.com/flaky-tests-in-go-why-they-happen-and-how-to-eliminate-them-cc19d8404ad6
- Testing of CSI Drivers (Kubernetes Blog): https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/
- Test Fixtures in Go (Dave Cheney): https://dave.cheney.net/2016/05/10/test-fixtures-in-go

### Tertiary (LOW confidence)
- Coverage threshold enforcement GitHub Actions: https://github.com/marketplace/actions/go-test-coverage
- CSI driver error handling examples from real-world issues
- Go testing patterns community articles

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Tools already integrated and documented in Go official sources
- Architecture: HIGH - Patterns verified in project codebase and official Go documentation
- Error path patterns: HIGH - Based on CSI spec requirements and existing test patterns in project
- Flaky test detection: MEDIUM - Techniques verified but current project status unknown
- Coverage thresholds: HIGH - Configuration already exists and well-documented

**Research date:** 2026-02-05
**Valid until:** 2026-03-05 (30 days - stable Go testing practices, tools mature)

## Project-Specific Context

**Current State:**
- Overall coverage: 65.3% (target: 70%)
- Coverage tool: go-test-coverage v2 (configured)
- Test files: 52 across pkg/ and test/
- CI threshold: 60% (enforced in .gitea/workflows/full-test.yml)
- Package thresholds: rds 70%, utils 80%, attachment 80%, nvme 55%

**Gaps Identified:**
1. Node service error paths (mount failures, NVMe disconnection)
2. Controller service SSH failure scenarios
3. Negative test scenarios for invalid parameters
4. CSI gRPC error code validation
5. Idempotency testing completeness
6. Flaky test status unknown

**Ready for Planning:**
- Use existing test infrastructure (mock RDS, Ginkgo/Gomega, testify)
- Add table-driven error path tests following pkg/rds patterns
- Implement negative test scenarios for CSI spec compliance
- Run flaky test detection before declaring phase complete
- Raise CI threshold to 65% after coverage improvements
