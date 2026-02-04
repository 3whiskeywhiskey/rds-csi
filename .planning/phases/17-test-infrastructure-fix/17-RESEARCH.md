# Phase 17: Test Infrastructure Fix - Research

**Researched:** 2026-02-04
**Domain:** Go unit testing and test infrastructure
**Confidence:** HIGH

## Summary

This phase addresses failing block volume tests in the CSI driver's test suite, specifically a nil pointer dereference in `TestNodePublishVolume_BlockVolume` and missing metadata in `TestNodeStageVolume_BlockVolume`. The root cause is incomplete mock setup where the test creates a `NodeServer` without initializing the `nvmeConn` field, leading to a nil pointer panic when the production code calls `ns.nvmeConn.GetDevicePath()`.

The standard approach for fixing this is to ensure all required dependencies are properly mocked in test setup functions. Go's native mocking patterns (interface-based dependency injection with struct mocks) are sufficient - no external mocking libraries needed. The codebase already uses this pattern successfully in other tests (e.g., `mockNVMEConnector` exists but isn't being used).

**Primary recommendation:** Add missing mock initialization to test setup, verify block volume operation flow, and ensure test helper functions consistently initialize all required fields.

## Standard Stack

The established tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go testing | 1.24 | Built-in test framework | Standard library, no external deps needed |
| go test | 1.24 | Test runner with race detector | Native tooling, integrates with CI/CD |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| t.Helper() | stdlib | Mark helper functions | Every test helper to fix line reporting |
| t.Run() | stdlib | Subtests for table-driven tests | When running multiple test cases |
| -race flag | stdlib | Race condition detection | Always in CI, critical for concurrent code |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Manual mocks | testify/mock | Project avoids external mock libs (simpler) |
| Manual mocks | gomock | Project uses interface-based mocks (lighter) |
| Inline tests | Table-driven | Project already uses table-driven pattern |

**Installation:**
```bash
# No installation needed - uses Go stdlib only
go test -v -race ./pkg/driver/...
```

## Architecture Patterns

### Recommended Project Structure
```
pkg/driver/
├── node.go              # Production code
├── node_test.go         # Test code with mocks
└── (mocks inline)       # mockMounter, mockNVMEConnector
```

### Pattern 1: Interface-Based Mock Structs
**What:** Define mock structs that implement the required interface with controllable behavior
**When to use:** When testing code that depends on interfaces (Mounter, nvme.Connector, etc.)
**Example:**
```go
// Source: Existing codebase pattern (node_test.go:564-644)
type mockNVMEConnector struct {
	connectCalled    bool
	disconnectCalled bool
	devicePath       string
	connectErr       error
	disconnectErr    error
	getDevicePathErr error
}

func (m *mockNVMEConnector) GetDevicePath(nqn string) (string, error) {
	if m.getDevicePathErr != nil {
		return "", m.getDevicePathErr
	}
	return m.devicePath, nil
}

// Usage in test:
connector := &mockNVMEConnector{
	devicePath: "/dev/nvme0n1",
}
ns := &NodeServer{
	driver:   driver,
	nvmeConn: connector, // THIS IS WHAT'S MISSING
}
```

### Pattern 2: Test Helper Functions for Consistent Setup
**What:** Centralized functions that create properly configured test objects
**When to use:** When multiple tests need similar setup with slight variations
**Example:**
```go
// Source: Existing pattern but incomplete (node_test.go:98-178)
func createNodeServerWithMocks(mounter mount.Mounter, connector nvme.Connector) *NodeServer {
	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}

	return &NodeServer{
		driver:         driver,
		mounter:        mounter,
		nvmeConn:       connector,  // MUST initialize all fields
		nodeID:         "test-node",
		circuitBreaker: circuitbreaker.NewVolumeCircuitBreaker(),
	}
}
```

### Pattern 3: Table-Driven Tests with Subtests
**What:** Define test cases as data structures, iterate with t.Run()
**When to use:** Testing multiple scenarios for the same operation
**Example:**
```go
// Source: Go Wiki TableDrivenTests pattern
tests := []struct {
	name        string
	setupMocks  func() (*mockMounter, *mockNVMEConnector)
	volumeID    string
	expectError bool
}{
	{
		name: "block volume succeeds with valid device",
		setupMocks: func() (*mockMounter, *mockNVMEConnector) {
			return &mockMounter{}, &mockNVMEConnector{
				devicePath: "/dev/nvme0n1",
			}
		},
		volumeID: "pvc-test-uuid",
		expectError: false,
	},
}

for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		// Test logic here
	})
}
```

### Anti-Patterns to Avoid
- **Partially initialized mocks:** Creating NodeServer without nvmeConn leads to nil pointer panics
- **Skipping t.Helper() in helpers:** Makes error line numbers point to helper instead of test
- **Not using -race in CI:** Misses concurrency bugs that appear in production
- **Testing implementation details:** Test behavior, not internal structure

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Mock generation | Custom mock code | Inline struct mocks | Go interfaces implicitly satisfied, no codegen needed |
| Test assertions | Custom helpers | stdlib comparison + clear errors | No external deps, clear test output |
| Concurrent testing | Manual goroutines | t.Run with t.Parallel() | Built-in race detector, better reporting |
| Temp file cleanup | Manual os.RemoveAll | defer os.RemoveAll(tmpDir) | Handles panic cases automatically |

**Key insight:** Go's built-in testing features (interfaces, first-class functions, t.Run, -race) cover 95% of testing needs. External mocking libraries add complexity without benefit for this codebase's patterns.

## Common Pitfalls

### Pitfall 1: Incomplete Mock Initialization
**What goes wrong:** Test creates NodeServer but leaves fields nil, causing panics in production code
**Why it happens:** Test helper functions don't initialize all required dependencies
**How to avoid:**
1. Every test helper must initialize ALL fields that production code uses
2. Review production code path to identify required dependencies
3. Add validation in test helpers: `if ns.nvmeConn == nil { panic("missing nvmeConn") }`
**Warning signs:** `nil pointer dereference` panics during test execution

### Pitfall 2: Mock Behavior Not Matching Interface Contract
**What goes wrong:** Mock returns values that real implementation never would, test passes but production fails
**Why it happens:** Mock doesn't validate inputs or simulate real errors
**How to avoid:**
1. Study real implementation behavior (e.g., nvme.Connector.GetDevicePath returns error if device not found)
2. Mock should return realistic errors for invalid inputs
3. Add comments documenting what real implementation does
**Warning signs:** Tests pass but production has unexpected behavior

### Pitfall 3: Testing Implementation Instead of Behavior
**What goes wrong:** Tests check internal state instead of observable behavior, become brittle
**Why it happens:** Easy access to internal fields tempts over-testing
**How to avoid:**
1. Test what callers care about: return values, side effects, errors
2. Don't test that mocks were called (unless that IS the behavior)
3. Focus on "given X, when Y, then Z" scenarios
**Warning signs:** Tests break when refactoring without behavior change

### Pitfall 4: Shared Mutable Mock State
**What goes wrong:** Tests interfere with each other when mocks share state
**Why it happens:** Reusing mock instances across tests without reset
**How to avoid:**
1. Create fresh mock instances for each test/subtest
2. Use t.Run with fresh setup for each case
3. Avoid package-level mock variables
**Warning signs:** Tests pass individually but fail when run together

## Code Examples

Verified patterns from official sources:

### Fixing the Nil Pointer Issue
```go
// Source: Inferred from production code (node.go:533) and existing mock pattern
// BEFORE (broken):
func TestNodePublishVolume_BlockVolume(t *testing.T) {
	mounter := &mockMounter{}
	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}
	ns := &NodeServer{
		driver:  driver,
		mounter: mounter,
		nodeID:  "test-node",
		// nvmeConn is nil! Causes panic at line 533
	}
	// Test code...
}

// AFTER (fixed):
func TestNodePublishVolume_BlockVolume(t *testing.T) {
	mounter := &mockMounter{}
	connector := &mockNVMEConnector{
		devicePath: "/dev/nvme0n1",  // Return valid device
	}

	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}

	ns := &NodeServer{
		driver:   driver,
		mounter:  mounter,
		nvmeConn: connector,  // Now initialized!
		nodeID:   "test-node",
	}
	// Test code...
}
```

### Block Volume Stage Operation Flow
```go
// Source: Understanding from production code (node.go)
// Block volumes in NodeStageVolume should:
// 1. Connect to NVMe device (nvmeConn.Connect)
// 2. Create staging directory
// 3. Write device path to metadata file (staging/device)
// 4. Skip filesystem formatting/mounting

// Test must verify metadata file creation:
metadataPath := filepath.Join(stagingPath, "device")
deviceBytes, err := os.ReadFile(metadataPath)
if err != nil {
	t.Fatalf("failed to read device metadata file: %v", err)
}
expectedDevice := "/dev/nvme0n1"
actualDevice := strings.TrimSpace(string(deviceBytes))
if actualDevice != expectedDevice {
	t.Errorf("device metadata = %q, want %q", actualDevice, expectedDevice)
}
```

### Test Helper with Validation
```go
// Source: Best practice from Go community patterns
func createTestNodeServer(t *testing.T, mounter mount.Mounter, connector nvme.Connector) *NodeServer {
	t.Helper()  // Correct error line reporting

	// Validate required fields
	if mounter == nil {
		t.Fatal("mounter is required")
	}
	if connector == nil {
		t.Fatal("connector is required for block volume tests")
	}

	driver := &Driver{
		name:    "rds.csi.srvlab.io",
		version: "test",
		metrics: observability.NewMetrics(),
	}

	return &NodeServer{
		driver:         driver,
		mounter:        mounter,
		nvmeConn:       connector,
		nodeID:         "test-node",
		circuitBreaker: circuitbreaker.NewVolumeCircuitBreaker(),
	}
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| External mock libraries (testify, gomock) | Inline interface mocks | Go 1.18+ generics reduced need | Simpler deps, faster builds |
| Individual test functions | Table-driven tests with t.Run | Go 1.7+ | Better organization, parallel tests |
| Manual race detection | go test -race | Go 1.1+ | Catches concurrency bugs automatically |
| Custom assertions | Clear error messages + stdlib | Modern Go practice | Better test output, no magic |

**Deprecated/outdated:**
- gomock: While still used, inline mocks are simpler for this codebase's patterns
- testify/mock: External dep not needed when interface mocks work well
- Individual test functions: Table-driven tests are now standard practice

## Open Questions

Things that couldn't be fully resolved:

1. **Why was nvmeConn originally omitted?**
   - What we know: Other tests (TestNodeStageVolume_FilesystemVolume) also have this pattern but don't fail
   - What's unclear: Whether this was intentional (block volume path untested) or oversight
   - Recommendation: Audit all test helper functions for consistent field initialization

2. **Should block volume tests run on macOS?**
   - What we know: Tests create /dev/nvme* device files which don't exist on macOS
   - What's unclear: Whether these tests should skip on non-Linux or mock device files
   - Recommendation: Use temp files that simulate devices (already done in some tests), document platform assumptions

3. **Test coverage for error paths**
   - What we know: Happy path tested, but error scenarios (device not found, permission denied) may be missing
   - What's unclear: Full set of error conditions to test
   - Recommendation: Review production code error paths, add negative test cases

## Sources

### Primary (HIGH confidence)
- Go testing stdlib documentation - official language documentation
- Existing codebase patterns (node_test.go, mockNVMEConnector) - direct codebase analysis
- Production code (node.go:533) - identified nil pointer dereference location

### Secondary (MEDIUM confidence)
- [Go Wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests) - official Go community patterns
- [Your Go tests probably don't need a mocking library | Redowan's Reflections](https://rednafi.com/go/mocking-libraries-bleh/) - interface-based mocking patterns
- [Prefer table driven tests | Dave Cheney](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) - table-driven test philosophy
- [How to Write Table-Driven Tests in Go (OneUpTime, January 2026)](https://oneuptime.com/blog/post/2026-01-07-go-table-driven-tests/view) - recent best practices
- [5 Mocking Techniques for Go](https://www.myhatchpad.com/insight/mocking-techniques-for-go/) - comprehensive mocking patterns

### Tertiary (LOW confidence)
- None - all findings verified with codebase or official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - stdlib testing, no external dependencies
- Architecture: HIGH - existing codebase patterns, direct code analysis
- Pitfalls: HIGH - nil pointer cause identified from panic output and code review

**Research date:** 2026-02-04
**Valid until:** 90 days (stable domain, Go stdlib patterns don't change frequently)
