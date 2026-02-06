# Testing Guide

Comprehensive guide to testing the RDS CSI driver, including unit tests, integration tests, CSI sanity tests, and E2E tests.

## Overview

The RDS CSI driver employs a multi-layered testing strategy:

1. **Unit Tests** - Test individual functions and packages in isolation
2. **Integration Tests** - Test driver + mock RDS interaction
3. **Sanity Tests** - Validate CSI spec compliance (Identity + Controller services)
4. **E2E Tests** - Validate full stack with real Kubernetes and hardware (planned)

## Running Tests Locally

### Unit Tests

Run all unit tests with coverage:

```bash
make test
```

Run tests for a specific package:

```bash
go test -v ./pkg/driver/...
```

Generate coverage report:

```bash
make test-coverage
open coverage.html
```

### Integration Tests

Run integration tests with mock RDS:

```bash
make test-integration
```

These tests validate driver behavior with a simulated RouterOS CLI, covering:
- SSH connection handling
- Command execution and parsing
- Error handling and retries
- Volume lifecycle operations

### CSI Sanity Tests

Sanity tests validate CSI specification compliance using the official `csi-test` package.

#### With Mock RDS (Recommended for CI)

```bash
make test-sanity-mock
```

This runs the Go-based sanity test suite with an in-process mock RDS server. It validates Identity and Controller services against CSI spec v1.12.0.

#### With Real RDS Hardware

```bash
make test-sanity-real RDS_ADDRESS=10.42.68.1 RDS_SSH_KEY=~/.ssh/id_rsa
```

WARNING: This runs tests against real hardware and will create/delete actual volumes. Use a dedicated test RDS instance.

### Verification Suite

Run all code quality checks:

```bash
make verify
```

This runs:
- `go fmt` - Code formatting
- `go vet` - Static analysis
- `golangci-lint` - Comprehensive linting
- Unit tests

## CSI Capability Matrix

This table documents which CSI capabilities are implemented and tested by the driver.

### Identity Service

| Capability | Implemented | Tested | Notes |
|------------|-------------|--------|-------|
| CONTROLLER_SERVICE | Yes | Yes (sanity) | Driver has controller component |
| VOLUME_ACCESSIBILITY_CONSTRAINTS | Yes | Yes (sanity) | Topology support for node scheduling |

### Controller Service

| Capability | Implemented | Tested | Notes |
|------------|-------------|--------|-------|
| CREATE_DELETE_VOLUME | Yes | Yes (sanity) | Core volume provisioning functionality |
| PUBLISH_UNPUBLISH_VOLUME | Yes | Yes (sanity) | Volume attachment tracking |
| GET_CAPACITY | Yes | Yes (sanity) | Returns RDS storage pool capacity |
| LIST_VOLUMES | Yes | Yes (sanity) | Enumerates CSI-managed volumes |
| EXPAND_VOLUME | Yes | Yes (sanity) | Online volume expansion support |
| CREATE_DELETE_SNAPSHOT | No | Skipped | Deferred to Phase 26 (future milestone) |
| CLONE_VOLUME | No | Skipped | Not planned |
| GET_VOLUME | No | Skipped | Optional capability, not required |

### Node Service

Node service capabilities are implemented but require NVMe/TCP hardware for testing.

| Capability | Implemented | Tested | Notes |
|------------|-------------|--------|-------|
| STAGE_UNSTAGE_VOLUME | Yes | No (deferred) | Requires NVMe/TCP target connection |
| EXPAND_VOLUME | Yes | No (deferred) | Requires mounted filesystem |
| GET_VOLUME_STATS | Yes | No (deferred) | Requires mounted filesystem |
| VOLUME_CONDITION | Yes | No (deferred) | Requires active NVMe connection |

**Node service testing status:**
- Sanity tests skip Node service (no NVMe/TCP mock available)
- Node functionality tested manually with real hardware
- E2E tests with hardware environment planned for Phase 24

## Test Infrastructure

### Mock RDS Server

The mock RDS server (`test/mock/rds_server.go`) simulates RouterOS CLI behavior for testing without real hardware.

**Features:**
- SSH server on configurable port (default: 12222 for sanity tests, 2222 for integration tests)
- Simulates RouterOS CLI commands (`/disk add`, `/disk remove`, `/disk print`, `/file print`)
- Command history logging for debugging test failures
- Thread-safe state management

**Usage in tests:**

```go
mockRDS, err := mock.NewMockRDSServer(12222)
require.NoError(t, err)
mockRDS.Start()
defer mockRDS.Stop()

// Run tests against localhost:12222
```

### Mock RDS Server Configuration

The mock RDS server supports environment-based configuration for timing simulation and error injection, enabling various testing scenarios without modifying code.

#### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MOCK_RDS_REALISTIC_TIMING` | `false` | Enable realistic operation delays |
| `MOCK_RDS_SSH_LATENCY_MS` | `200` | Base SSH connection latency (ms) |
| `MOCK_RDS_SSH_LATENCY_JITTER_MS` | `50` | Latency jitter range +/- (ms) |
| `MOCK_RDS_DISK_ADD_DELAY_MS` | `500` | Disk add operation delay (ms) |
| `MOCK_RDS_DISK_REMOVE_DELAY_MS` | `300` | Disk remove operation delay (ms) |
| `MOCK_RDS_ERROR_MODE` | `none` | Error injection mode |
| `MOCK_RDS_ERROR_AFTER_N` | `0` | Fail after N operations (0 = immediate) |
| `MOCK_RDS_ENABLE_HISTORY` | `true` | Enable command history logging |
| `MOCK_RDS_HISTORY_DEPTH` | `100` | Max commands in history |
| `MOCK_RDS_ROUTEROS_VERSION` | `7.16` | RouterOS version to simulate |

#### Error Injection Modes

| Mode | Description | Error Message |
|------|-------------|---------------|
| `none` | No errors injected | - |
| `disk_full` | Simulate disk full condition | `failure: not enough space` |
| `ssh_timeout` | Simulate SSH connection timeout | (connection hangs) |
| `command_fail` | Simulate command execution failure | `failure: execution error` |

#### Usage Examples

**Run sanity tests with error injection:**
```bash
MOCK_RDS_ERROR_MODE=disk_full make test-sanity-mock
```

**Run tests with realistic timing:**
```bash
MOCK_RDS_REALISTIC_TIMING=true go test ./test/sanity/... -v
```

**Fail after 3rd operation (for idempotency testing):**
```bash
MOCK_RDS_ERROR_MODE=disk_full MOCK_RDS_ERROR_AFTER_N=3 make test-sanity-mock
```

**Test with realistic SSH latency (150-250ms):**
```bash
MOCK_RDS_REALISTIC_TIMING=true \
  MOCK_RDS_SSH_LATENCY_MS=200 \
  MOCK_RDS_SSH_LATENCY_JITTER_MS=50 \
  go test ./test/integration/... -v
```

#### Stress Testing

Run concurrent connection stress tests to validate MOCK-07 (concurrent SSH connections without state corruption):

```bash
# Run all stress tests
go test ./test/mock/... -run TestConcurrent -v

# Run with race detector
go test ./test/mock/... -run TestConcurrent -v -race

# Skip timing tests (faster CI)
go test ./test/mock/... -short -v
```

**Available stress tests:**
- `TestConcurrentConnections` - 50 parallel volume operations
- `TestConcurrentSameVolume` - Idempotency validation (10 goroutines, 1 volume)
- `TestConcurrentCreateDelete` - Race between create/delete operations
- `TestConcurrentMixedOperations` - Concurrent create/query/delete mix
- `TestConcurrentCommandHistory` - History tracking with concurrency

### Sanity Test Configuration

The sanity tests use the official `csi-test/v5/pkg/sanity` package with these settings:

- **Test volume size:** 10 GiB (realistic size validation)
- **Expand size:** 20 GiB (online expansion tests)
- **Idempotent count:** 2 (validates CreateVolume/DeleteVolume idempotency)
- **Target path:** `/tmp/csi-target` (unused for Controller-only tests)
- **Staging path:** `/tmp/csi-staging` (unused for Controller-only tests)

### In-Process Testing Pattern

The sanity tests use an in-process testing pattern:
1. Start mock RDS server as goroutine
2. Create driver instance with mock RDS config
3. Start driver gRPC server on Unix socket
4. Run csi-sanity tests against the socket
5. Clean up on test completion

This pattern provides:
- Faster startup (no subprocess overhead)
- Easier debugging (single process)
- Better CI integration (no cleanup issues)

## Debugging Test Failures

### Enable Verbose Logging

```bash
# Unit/integration tests
go test -v ./pkg/driver/...

# Sanity tests with detailed output
go test -v -race ./test/sanity/...
```

### Check Mock RDS Command History

The mock RDS server logs all executed commands. Enable logging in tests:

```go
mockRDS, _ := mock.NewMockRDSServer(12222)
mockRDS.Start()
defer func() {
    history := mockRDS.GetCommandHistory()
    for _, cmd := range history {
        t.Logf("RDS command: %s (success: %t)", cmd.Command, cmd.Success)
    }
    mockRDS.Stop()
}()
```

### CI Artifacts

GitHub Actions CI captures test logs as artifacts on failure:
- **sanity-test-logs** - Full sanity test output with timing and error details

To download artifacts:
1. Navigate to failed workflow run
2. Scroll to "Artifacts" section
3. Download `sanity-test-logs.zip`

### Common Issues

#### "invalid volume name format" in sanity tests

**Cause:** Volume ID validation is too strict.

**Fix:** Ensure `ValidateVolumeID` accepts alphanumeric names with hyphens, not just `pvc-<uuid>` format. CSI spec allows any safe volume name.

#### "nvme connect failed" in Node tests

**Cause:** Node tests require NVMe/TCP target hardware.

**Fix:** Skip Node tests with `--ginkgo.skip="Node"` when using mock RDS. Node tests deferred to E2E testing phase.

#### "connection refused" to driver socket

**Cause:** Driver failed to start or socket path already in use.

**Fix:** Check for existing socket file, verify driver startup logs, ensure no conflicting processes.

#### Mock RDS state inconsistencies

**Cause:** Tests creating volumes that persist between test runs.

**Fix:** Mock RDS resets state on restart. Each test run should start fresh. Call `mockRDS.ClearCommandHistory()` between test suites if needed.

## Contributing Tests

### Unit Test Guidelines

- Place tests in `*_test.go` files next to the code under test
- Use `testify/require` for assertions: `require.NoError(t, err)`
- Use table-driven tests for multiple scenarios
- Mock external dependencies (SSH client, Kubernetes client)

**Example:**

```go
func TestVolumeIDValidation(t *testing.T) {
    tests := []struct {
        name    string
        volumeID string
        wantErr bool
    }{
        {"valid pvc format", "pvc-12345678-1234-1234-1234-123456789abc", false},
        {"valid custom name", "my-volume-123", false},
        {"invalid shell chars", "vol;rm -rf /", true},
        {"too long", strings.Repeat("a", 300), true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateVolumeID(tt.volumeID)
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Integration Test Guidelines

- Place tests in `test/integration/*_test.go`
- Use mock RDS server for RouterOS CLI simulation
- Test full request/response cycles
- Validate error handling and retries

### Sanity Test Guidelines

The sanity tests use the official `csi-test` package and should not need modification unless:
- Adding new CSI capabilities (update capability reporting in driver)
- Adjusting test configuration (volume size, idempotent count)
- Fixing capability mismatches between driver and tests

## Test Coverage Goals

**Current coverage:** ~60% (established as realistic target for hardware-dependent code)

**Coverage by package:**
- `pkg/driver` - Target: 70% (core CSI logic, well testable)
- `pkg/rds` - Target: 50% (SSH client, hardware dependent)
- `pkg/utils` - Target: 80% (pure functions, fully testable)

**Untested code acceptable when:**
- Requires real hardware (NVMe/TCP operations)
- Requires real Kubernetes cluster (VolumeAttachment tracking)
- Trivial getters/setters

## Continuous Integration

All PRs run automated tests via GitHub Actions (`.github/workflows/pr.yml`):

1. **Verify job** - Code quality checks (fmt, vet, lint) + unit tests + integration tests
2. **Sanity-tests job** - CSI spec compliance validation with mock RDS
3. **Build-test job** - Docker image build for linux/amd64 and linux/arm64

**Build fails if:**
- Any unit test fails
- Any integration test fails
- Any sanity test fails (strict CSI compliance required)
- Linting errors detected
- Code coverage drops significantly

## Performance Testing

Performance testing is manual and hardware-dependent:

- **IOPS:** Test with `fio` on NVMe/TCP volumes
- **Latency:** Measure with `fio --rw=randread --iodepth=1`
- **Throughput:** Measure with `fio --rw=read --bs=1M`
- **Concurrent operations:** Create/delete multiple volumes simultaneously

See `docs/performance.md` (planned) for detailed benchmarking procedures.

## Related Documentation

- `CLAUDE.md` - Essential commands and architecture overview
- `docs/architecture.md` - System design and component interactions
- `.planning/phases/22-csi-sanity-tests-integration/22-RESEARCH.md` - CSI testing domain research
- [CSI Spec](https://github.com/container-storage-interface/spec) - Official Container Storage Interface specification
- [csi-test Documentation](https://github.com/kubernetes-csi/csi-test) - Official CSI testing framework

---

For questions or issues with testing, check existing test patterns in the codebase or refer to the CSI testing best practices at https://kubernetes-csi.github.io/docs/functional-testing.html.
