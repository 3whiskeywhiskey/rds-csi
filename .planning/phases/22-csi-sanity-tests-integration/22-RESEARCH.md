# Phase 22: CSI Sanity Tests Integration - Research

**Researched:** 2026-02-04
**Domain:** CSI Specification Compliance Testing
**Confidence:** HIGH

## Summary

CSI sanity testing is a well-established practice for validating CSI driver compliance with the Container Storage Interface specification. The `csi-test` repository maintained by Kubernetes-CSI SIG provides the authoritative `csi-sanity` tool that performs comprehensive API validation by calling gRPC methods in various ways and checking that outcomes match specification requirements.

The RDS CSI driver already has foundational infrastructure for sanity testing:
- An existing shell script (`test/sanity/run-sanity-tests.sh`) that runs csi-sanity
- A mock SSH server (`test/mock/rds_server.go`) that simulates RouterOS CLI responses
- Integration tests that validate idempotency and error codes
- GitHub Actions CI workflow that can be extended for sanity tests

**Primary recommendation:** Enhance the existing test infrastructure to run csi-sanity with mock RDS in CI, focusing on Identity + Controller services with comprehensive artifact capture for debugging failures.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| csi-test/csi-sanity | v5.x | CSI spec compliance testing | Official Kubernetes-CSI SIG tool, de facto standard |
| Ginkgo | v2.x | BDD test framework | Used by csi-sanity internally, enables `-ginkgo.skip` filtering |
| Gomega | v1.x | Matcher library | Paired with Ginkgo for assertions |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| grpc-go | v1.69.x | gRPC client/server | Already in project, csi-sanity connects via gRPC |
| container-storage-interface/spec | v1.10.0 | CSI protobuf definitions | Already in project |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| csi-sanity CLI | pkg/sanity Go integration | CLI better for CI, Go integration better for custom test setups |
| Mock SSH server | Real RDS hardware | Mock is faster/reliable for CI, hardware for final validation |

**Installation:**
```bash
go install github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@latest
```

## Architecture Patterns

### Recommended Project Structure
```
test/
├── sanity/
│   ├── run-sanity-tests.sh     # Main test runner (exists)
│   ├── sanity_config.go        # NEW: Programmatic sanity test config
│   └── sanity_test.go          # NEW: Go-based sanity tests
├── mock/
│   └── rds_server.go           # Mock RDS SSH server (exists)
└── integration/
    └── controller_integration_test.go  # Integration tests (exists)
```

### Pattern 1: In-Process Driver Testing (Recommended)
**What:** Start the CSI driver as a goroutine within the test process, connect csi-sanity via Unix socket
**When to use:** Local development and CI - faster startup, easier debugging
**Example:**
```go
// Source: https://pkg.go.dev/github.com/kubernetes-csi/csi-test/v5/pkg/sanity
func TestCSISanity(t *testing.T) {
    // Start mock RDS server
    mockRDS, _ := mock.NewMockRDSServer(12345)
    mockRDS.Start()
    defer mockRDS.Stop()

    // Create driver with mock RDS
    config := driver.DriverConfig{
        EnableController: true,
        RDSAddress:       "localhost",
        RDSPort:          12345,
    }
    drv, _ := driver.NewDriver(config)

    // Start driver in background on Unix socket
    socketPath := "/tmp/csi-sanity.sock"
    go drv.Run("unix://" + socketPath)
    defer drv.Stop()

    // Configure sanity tests
    sanityConfig := sanity.NewTestConfig()
    sanityConfig.Address = "unix://" + socketPath
    sanityConfig.TargetPath = "/tmp/csi-target"
    sanityConfig.StagingPath = "/tmp/csi-staging"
    sanityConfig.TestVolumeSize = 10 * 1024 * 1024 * 1024 // 10GB
    sanityConfig.IdempotentCount = 2  // Test idempotency

    // Run sanity tests
    sanity.Test(t, sanityConfig)
}
```

### Pattern 2: CLI-Based Testing (Current Implementation)
**What:** Build driver binary, start as subprocess, run csi-sanity CLI
**When to use:** CI environments where subprocess isolation is preferred
**Example:**
```bash
# Source: Existing run-sanity-tests.sh
csi-sanity \
    --csi.endpoint="unix:///tmp/csi-sanity.sock" \
    --csi.testvolumesize="${TEST_VOLUME_SIZE}" \
    --ginkgo.skip="Node" \
    --ginkgo.v
```

### Anti-Patterns to Avoid
- **Running Node tests without NVMe/TCP mock:** Node service requires actual block device operations, cannot be mocked without significant infrastructure
- **Testing with 1GB volumes:** Small sizes may pass tests but miss size-related edge cases (use 10GB+)
- **Masking flaky tests with retries:** Fix root cause rather than adding retry logic
- **Testing against real RDS in CI:** Unreliable, slow, creates external dependency

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CSI spec validation | Custom gRPC test clients | csi-sanity | Covers 100+ edge cases, maintained by SIG |
| Idempotency testing | Manual duplicate call tests | sanity.IdempotentCount config | Automatically repeats all operations |
| Error code validation | Switch statements on codes | csi-sanity negative tests | Tests all error scenarios per spec |
| Test filtering | Custom test selection | Ginkgo -skip/-focus flags | Built-in, well-documented |

**Key insight:** csi-sanity tests CSI API capability comprehensively. The existing integration tests in `test/integration/controller_integration_test.go` are valuable but redundant with csi-sanity for spec compliance. Keep them for quick unit-level validation, but rely on csi-sanity for formal compliance.

## Common Pitfalls

### Pitfall 1: Incomplete Volume ID Generation
**What goes wrong:** csi-sanity generates random volume names that don't match the `pvc-<uuid>` pattern expected by the driver
**Why it happens:** Driver validates volume IDs strictly, csi-sanity uses `DefaultIDGenerator`
**How to avoid:** The driver should accept the name provided by CreateVolume request as-is (already implemented - see line 93 of controller.go: `volumeID := req.GetName()`)
**Warning signs:** "invalid volume name format" errors in sanity tests

### Pitfall 2: Node Service Without Hardware
**What goes wrong:** Attempting to run Node service tests without NVMe/TCP target capability
**Why it happens:** Node tests require actual block device mount/unmount operations
**How to avoid:** Always skip Node tests with `--ginkgo.skip="Node"` when using mock RDS
**Warning signs:** "nvme connect failed" or "mount: special device not found" errors

### Pitfall 3: Staging/Target Path Permissions
**What goes wrong:** csi-sanity fails to create directories for staging/target paths
**Why it happens:** Tests running without root privileges, or paths don't exist
**How to avoid:** Use TestConfig with CreateTargetDir/CreateStagingDir callbacks, or use `-csi.createstagingpathcmd` and `-csi.createmountpathcmd` flags
**Warning signs:** "permission denied" or "no such file or directory" errors

### Pitfall 4: Controller Capabilities Mismatch
**What goes wrong:** Tests fail expecting capabilities that driver doesn't report
**Why it happens:** Driver reports PUBLISH_UNPUBLISH_VOLUME but mock doesn't support attachment tracking
**How to avoid:** Ensure mock RDS properly supports GetVolume for ControllerPublishVolume validation, or disable attachment manager in test mode
**Warning signs:** "controller does not support" errors or attachment-related failures

### Pitfall 5: Idempotency Edge Cases
**What goes wrong:** CreateVolume returns different volume ID on repeated calls
**Why it happens:** Driver generates new UUID for each call instead of returning existing volume
**How to avoid:** Check if volume exists by name before creating (already implemented - see lines 101-135 of controller.go)
**Warning signs:** "volume ID mismatch" errors in idempotency tests

## Code Examples

Verified patterns from official sources:

### Running csi-sanity with Ginkgo Skip
```bash
# Source: https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/
# Skip Node tests (requires hardware)
csi-sanity \
    --csi.endpoint="unix:///tmp/csi.sock" \
    --ginkgo.skip="Node" \
    --ginkgo.v

# Focus on specific tests
csi-sanity \
    --csi.endpoint="unix:///tmp/csi.sock" \
    --ginkgo.focus="CreateVolume.*idempotent" \
    --ginkgo.v
```

### Go-Based Sanity Test Configuration
```go
// Source: https://pkg.go.dev/github.com/kubernetes-csi/csi-test/v5/pkg/sanity
func TestCSISanity(t *testing.T) {
    config := sanity.NewTestConfig()

    // Connection
    config.Address = "unix:///tmp/csi.sock"
    config.ControllerAddress = config.Address  // Same endpoint

    // Paths (Controller tests don't need these, but Node tests do)
    config.TargetPath = "/tmp/csi-target"
    config.StagingPath = "/tmp/csi-staging"

    // Volume configuration
    config.TestVolumeSize = 10 * 1024 * 1024 * 1024  // 10 GiB
    config.TestVolumeExpandSize = 20 * 1024 * 1024 * 1024  // 20 GiB

    // Idempotency testing (critical per CONTEXT.md decisions)
    config.IdempotentCount = 2  // Repeat each operation twice

    // Custom parameters for StorageClass simulation
    config.TestVolumeParameters = map[string]string{
        "volumePath": "/storage-pool/metal-csi",
    }

    sanity.Test(t, config)
}
```

### CI Artifact Capture Pattern
```bash
# Source: Best practice from Kubernetes CSI driver repos
# Capture debug artifacts on failure
capture_artifacts() {
    local exit_code=$1
    local artifact_dir="${CI_ARTIFACT_DIR:-/tmp/sanity-artifacts}"

    mkdir -p "${artifact_dir}"

    # Capture driver logs
    if [ -f /tmp/driver.log ]; then
        cp /tmp/driver.log "${artifact_dir}/"
    fi

    # Capture mock RDS command history
    if [ -f /tmp/mock-rds.log ]; then
        cp /tmp/mock-rds.log "${artifact_dir}/"
    fi

    # Capture test output
    if [ -f /tmp/sanity-output.log ]; then
        cp /tmp/sanity-output.log "${artifact_dir}/"
    fi
}

trap 'capture_artifacts $?' EXIT
```

### CSI Error Code Validation
```go
// Source: CSI spec + existing controller.go patterns
// The driver already returns proper CSI error codes:

// INVALID_ARGUMENT - missing or invalid parameters
if req.GetName() == "" {
    return nil, status.Error(codes.InvalidArgument, "volume name is required")
}

// NOT_FOUND - volume doesn't exist
if _, err := cs.driver.rdsClient.GetVolume(volumeID); err != nil {
    return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
}

// ALREADY_EXISTS - idempotent CreateVolume returns existing volume (not an error)
// Note: CSI spec says CreateVolume with same name/capacity should succeed

// RESOURCE_EXHAUSTED - out of capacity
if stderrors.Is(err, utils.ErrResourceExhausted) {
    return nil, status.Errorf(codes.ResourceExhausted, "insufficient storage on RDS: %v", err)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| csi-test v3/v4 | csi-test v5 | 2023 | v5 is current, import path changed |
| Manual gRPC testing | csi-sanity automation | Established | Standard practice for all CSI drivers |
| Single test run | IdempotentCount repeats | csi-test v4+ | Built-in idempotency validation |

**Deprecated/outdated:**
- `github.com/kubernetes-csi/csi-test/v4`: Use v5 instead
- Manual idempotency test loops: Use `IdempotentCount` config option

## CSI Capability Matrix

Document in TESTING.md per CONTEXT.md decisions:

### Controller Service Capabilities
| Capability | Implemented | Tested by Sanity | Notes |
|------------|-------------|------------------|-------|
| CREATE_DELETE_VOLUME | Yes | Yes | Core functionality |
| PUBLISH_UNPUBLISH_VOLUME | Yes | Yes | Attachment tracking |
| GET_CAPACITY | Yes | Yes | Returns RDS pool capacity |
| EXPAND_VOLUME | Yes | Yes | Online expansion |
| CREATE_DELETE_SNAPSHOT | No | Skip | Deferred to Phase 26 |
| CLONE_VOLUME | No | Skip | Not implemented |
| LIST_VOLUMES | Yes | Yes | Returns all CSI-managed volumes |

### Identity Service Capabilities
| Capability | Implemented | Tested by Sanity | Notes |
|------------|-------------|------------------|-------|
| CONTROLLER_SERVICE | Yes | Yes | Driver has controller |
| VOLUME_ACCESSIBILITY_CONSTRAINTS | Yes | Yes | Topology support |

### Node Service Capabilities (Deferred)
| Capability | Implemented | Tested by Sanity | Notes |
|------------|-------------|------------------|-------|
| STAGE_UNSTAGE_VOLUME | Yes | No (skip) | Requires NVMe/TCP hardware |
| EXPAND_VOLUME | Yes | No (skip) | Requires mounted filesystem |
| GET_VOLUME_STATS | Yes | No (skip) | Requires mounted filesystem |
| VOLUME_CONDITION | Yes | No (skip) | Requires NVMe connection |

## Open Questions

Things that couldn't be fully resolved:

1. **Mock RDS State Persistence Between Test Runs**
   - What we know: Mock server resets on each test; csi-sanity may expect persistent state
   - What's unclear: Whether sanity tests create volumes in one test and expect them in another
   - Recommendation: Run sanity tests with fresh mock per test suite; verify no cross-test dependencies

2. **Attachment Manager in Sanity Tests**
   - What we know: Driver uses Kubernetes client for attachment tracking
   - What's unclear: Whether csi-sanity's ControllerPublishVolume tests work without k8s client
   - Recommendation: Test with attachment manager disabled first; enable if tests pass

3. **Volume Parameter Propagation**
   - What we know: csi-sanity uses TestVolumeParameters for CreateVolume
   - What's unclear: Whether all required parameters (volumePath, nqnPrefix) are properly passed
   - Recommendation: Verify parameters in mock RDS logs; adjust TestVolumeParameters as needed

## Sources

### Primary (HIGH confidence)
- [csi-test GitHub](https://github.com/kubernetes-csi/csi-test) - Official repository, README, source code
- [pkg.go.dev csi-test/v5/pkg/sanity](https://pkg.go.dev/github.com/kubernetes-csi/csi-test/v5/pkg/sanity) - TestConfig struct, API documentation
- [Kubernetes CSI Functional Testing](https://kubernetes-csi.github.io/docs/functional-testing.html) - Official CSI documentation

### Secondary (MEDIUM confidence)
- [Kubernetes Blog: Testing of CSI Drivers](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/) - Detailed tutorial with examples (2020, concepts still valid)
- [csi-test/cmd/csi-sanity/README.md](https://github.com/kubernetes-csi/csi-test/blob/master/cmd/csi-sanity/README.md) - CLI flags documentation

### Tertiary (LOW confidence)
- WebSearch results for CI integration patterns - Community practices, not verified against current versions

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official Kubernetes-CSI SIG tooling, verified via pkg.go.dev
- Architecture: HIGH - Based on existing project structure and official patterns
- Pitfalls: MEDIUM - Derived from source analysis and common CSI driver issues

**Research date:** 2026-02-04
**Valid until:** 30 days (stable tooling, low churn)
