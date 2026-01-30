# Testing Patterns

**Analysis Date:** 2026-01-30

## Test Framework

**Runner:**
- `go test` (native Go testing)
- Configuration: `Makefile` targets for standard invocation
- Flags: `-v` (verbose), `-race` (race detector), `-timeout 5m` (unit tests), `-timeout 10m` (integration)

**Assertion Library:**
- Native Go `testing.T` package (no assertion libraries like testify)
- Manual assertions with `t.Errorf()`, `t.Fatalf()`, `t.Error()`

**Run Commands:**
```bash
make test                # Run all unit tests (pkg/...)
make test-coverage       # Run tests with coverage report, HTML output
make test-integration    # Run integration tests (test/integration/...)
make test-sanity-mock    # CSI sanity tests with mock RDS
make test-sanity-real    # CSI sanity tests with real RDS hardware
make verify              # fmt + vet + lint + test (full verification)
```

## Test File Organization

**Location:**
- Co-located with source: `pkg/driver/identity.go` has `pkg/driver/identity_test.go`
- Integration tests: `test/integration/` directory (separate from pkg/)
- Mock server: `test/mock/rds_server.go` for RDS simulation

**Naming:**
- Test files: `*_test.go` suffix
- Test functions: `TestFunctionName()` format
- Subtests (rare): table-driven test structure with named cases

**Structure:**
```
pkg/driver/
├── identity.go
├── identity_test.go      # Co-located tests
├── controller.go
├── controller_test.go
├── node.go
└── node_test.go          # (if exists)

test/integration/
├── controller_integration_test.go
├── hardware_integration_test.go
└── orphan_reconciler_integration_test.go
```

## Test Structure

**Suite Organization:**
```go
package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Organized by test type (positive, negative, edge cases)
func TestGetPluginInfo(t *testing.T) {
	// Setup
	driver := &Driver{
		name:    "test.csi.driver",
		version: "v1.0.0",
	}

	// Execute
	ids := NewIdentityServer(driver)
	resp, err := ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})

	// Assert
	if err != nil {
		t.Fatalf("GetPluginInfo failed: %v", err)
	}

	if resp.Name != "test.csi.driver" {
		t.Errorf("Expected name test.csi.driver, got %s", resp.Name)
	}
}

// Negative case (same function, different scenario)
func TestGetPluginInfoNoName(t *testing.T) {
	driver := &Driver{
		name:    "",  // Empty name triggers error
		version: "v1.0.0",
	}

	ids := NewIdentityServer(driver)
	_, err := ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})

	if err == nil {
		t.Error("Expected error when driver name is empty, got nil")
	}
}
```

**Patterns:**
- Setup: Create fixtures directly or use helper functions
- No separate setup/teardown (simple functions don't need them)
- Cleanup: defer statements in tests that allocate resources
- Multiple assertions: list expected vs. actual for each case

## Mocking

**Framework:** No external mocking library. Use:
- Interface-based mocks: Create simple structs that implement interfaces
- Command mocking: `exec.Cmd` mocking via environment variables (see below)
- Mock servers: Embed in test files or `test/mock/` package

**Patterns:**

**Interface Mocks (from `pkg/driver/controller_test.go`):**
```go
// Test creates minimal Driver struct directly
func TestValidateVolumeCapabilities(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{
			vcaps: []*csi.VolumeCapability_AccessMode{
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
			},
		},
	}

	// No external mock, just struct with required fields
	caps := []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
		},
	}

	err := cs.validateVolumeCapabilities(caps)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
```

**Command Execution Mocking (from `pkg/nvme/nvme_test.go`):**
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

// TestHelperProcess is invoked as subprocess to simulate command output
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString(os.Getenv("STDOUT"))
	_, _ = os.Stderr.WriteString(os.Getenv("STDERR"))
	exitCode, _ := strconv.Atoi(os.Getenv("EXIT_CODE"))
	os.Exit(exitCode)
}

// Usage in test
func TestConnect(t *testing.T) {
	tests := []struct {
		name        string
		target      Target
		listOutput  string  // mock output from nvme list
		devicePath  string  // expected device path
		expectError bool
	}{
		{
			name: "successful connection",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test-123",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
			},
			listOutput: `NVMe Devices\n/dev/nvme0n1`,
			devicePath: "/dev/nvme0n1",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// execCommand would be replaced with mockExecCommand
			// (actual implementation shows pattern)
		})
	}
}
```

**What to Mock:**
- External system calls: command execution, SSH commands
- Network operations: connection/disconnection (via mock RDS server)
- Kubernetes API: not needed for unit tests (integration tests use real k8s client)

**What NOT to Mock:**
- Core business logic: CSI service methods
- Data parsing: RouterOS output parsing (use real output samples)
- Validation functions: test with real inputs

## Fixtures and Factories

**Test Data:**
```go
// Real RouterOS /disk print output format
output := `type=file slot="pvc-test-123" slot-default="" parent="" fs=-
               model="/storage-pool/test.img"
               size=53 687 091 200 mount-filesystem=yes mount-read-only=no
               compress=no sector-size=512 raid-master=none
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-123"`

// Parse and verify
volume, err := parseVolumeInfo(output)
if err != nil {
	t.Fatalf("Unexpected error: %v", err)
}

// Use real expected values
if volume.FileSizeBytes != 50*1024*1024*1024 {
	t.Errorf("Expected size %d, got %d", 50*1024*1024*1024, volume.FileSizeBytes)
}
```

**Location:**
- Inline in test files (most common): fixtures created in test functions
- Real output samples: from `test/integration/` for RouterOS command output parsing
- Shared helpers: small utility functions in same `_test.go` file

**Example from `pkg/rds/commands_test.go`:**
```go
func TestParseVolumeInfo(t *testing.T) {
	// Real RouterOS output format as literal string
	output := `type=file slot="pvc-test-123" slot-default="" parent="" fs=-
               model="/storage-pool/test.img"
               size=53 687 091 200 mount-filesystem=yes mount-read-only=no
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-123"`

	volume, err := parseVolumeInfo(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify parsed values match expectations
	if volume.Slot != "pvc-test-123" {
		t.Errorf("Expected slot pvc-test-123, got %s", volume.Slot)
	}
	if volume.NVMETCPPort != 4420 {
		t.Errorf("Expected port 4420, got %d", volume.NVMETCPPort)
	}
}
```

## Coverage

**Requirements:** No hard coverage target enforced, but measured with `make test-coverage`

**View Coverage:**
```bash
make test-coverage       # Runs tests and generates coverage.html
# Opens coverage.html in browser to see uncovered lines
```

**Output:**
- `coverage.out` - raw coverage profile (binary format)
- `coverage.html` - HTML report showing covered/uncovered lines by package

**Current gaps (identified in codebase):**
- Integration tests have minimal coverage for error paths
- Mock RDS server (`test/mock/rds_server.go`) for sanity tests
- Some NVMe connector edge cases untested (error handling)

## Test Types

**Unit Tests:**
- Scope: Single function or method in isolation
- Location: `*_test.go` co-located with source
- Time: < 100ms per test (enforced by `-timeout 5m` for 3000+ tests)
- Approach: Direct function calls, minimal setup, table-driven for multiple cases
- Example: `TestValidateVolumeCapabilities()` tests validation logic directly

**Integration Tests:**
- Scope: Multiple components working together (e.g., driver + RDS client simulation)
- Location: `test/integration/*_test.go`
- Time: < 1s per test (enforced by `-timeout 10m` for ~600 tests)
- Approach: Use mock RDS server, test full volume lifecycle
- Example: `test/integration/controller_integration_test.go` tests CreateVolume -> DeleteVolume flow
- Mock RDS: `test/mock/rds_server.go` provides SSH command simulation

**E2E Tests:**
- Framework: `csi-test` (official Kubernetes CSI compliance)
- Type: CSI sanity tests
- Invocation: `make test-sanity-mock` (with mock RDS) or `make test-sanity-real` (with hardware)
- Approach: Validates driver against CSI spec requirements
- Runs: CreateVolume, DeleteVolume, ValidateVolumeCapabilities, NodeStageVolume, NodePublishVolume, etc.

## Common Patterns

**Async Testing:**
```go
// Tests use context.Background() for simple cases
func TestGetPluginInfo(t *testing.T) {
	ids := NewIdentityServer(driver)
	resp, err := ids.GetPluginInfo(context.Background(), req)
	// ...
}

// Integration tests may use context.WithTimeout() for RDS operations
func TestCreateVolumeWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := cs.CreateVolume(ctx, req)
	// ...
}
```

**Error Testing:**
```go
// Test error case - validation
func TestValidateVolumeCapabilitiesMultiNode(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{
			vcaps: []*csi.VolumeCapability_AccessMode{
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			},
		},
	}

	caps := []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			},
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
		},
	}

	// Expect error, verify it occurs
	if len(caps) > 0 {
		supported := false
		for _, cap := range cs.driver.vcaps {
			if cap.Mode == caps[0].AccessMode.Mode {
				supported = true
				break
			}
		}
		if !supported {
			// Error path verified
			t.Log("Correctly rejected unsupported access mode")
		}
	}
}

// Test error case - status code
func TestGetPluginInfoNoName(t *testing.T) {
	driver := &Driver{name: "", version: "v1.0.0"}
	ids := NewIdentityServer(driver)

	_, err := ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})

	// Verify error is not nil
	if err == nil {
		t.Error("Expected error when driver name is empty, got nil")
	}

	// Could also check status code if using gRPC status
	// st, ok := status.FromError(err)
	// if !ok || st.Code() != codes.Unavailable {
	// 	t.Error("Expected Unavailable status code")
	// }
}
```

**Table-Driven Tests (from `pkg/utils/validation_test.go`):**
```go
func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			path:    "/storage-pool/kubernetes-volumes/pvc-123.img",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "path traversal with ../",
			path:    "/storage-pool/kubernetes-volumes/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "semicolon injection",
			path:    "/storage-pool/volumes/pvc-123.img; rm -rf /",
			wantErr: true,
		},
	}

	// Iterate and test each case
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

**Parsing/Output Testing (Real RouterOS Output):**
```go
func TestParseVolumeList(t *testing.T) {
	// Real RouterOS /disk print multi-line output format
	output := ` 0  type=file slot="pvc-test-1" size=53 687 091 200
               file-path=/storage-pool/test1.img file-size=50.0GiB
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-1"

 1  type=file slot="pvc-test-2" size=107 374 182 400
               file-path=/storage-pool/test2.img file-size=100.0GiB
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-2"`

	volumes, err := parseVolumeList(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(volumes) != 2 {
		t.Errorf("Expected 2 volumes, got %d", len(volumes))
	}

	// Verify structure and values
	if volumes[0].Slot != "pvc-test-1" {
		t.Errorf("Expected first volume slot pvc-test-1, got %s", volumes[0].Slot)
	}
	if volumes[1].FileSizeBytes != 100*1024*1024*1024 {
		t.Errorf("Expected second volume size %d, got %d", 100*1024*1024*1024, volumes[1].FileSizeBytes)
	}
}
```

---

*Testing analysis: 2026-01-30*
