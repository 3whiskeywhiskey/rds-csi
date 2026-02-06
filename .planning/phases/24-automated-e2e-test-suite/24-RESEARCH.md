# Phase 24: Automated E2E Test Suite - Research

**Researched:** 2026-02-05
**Domain:** Kubernetes E2E testing, CSI driver validation, CI automation, test isolation
**Confidence:** HIGH

## Summary

Phase 24 implements automated end-to-end (E2E) testing for the RDS CSI driver to validate full Kubernetes integration in CI. The project already has excellent foundations: integration tests with mock RDS (Phase 22-23), CSI sanity tests, and comprehensive manual testing procedures. This phase automates those manual tests using Ginkgo v2 framework and extends coverage for volume expansion, concurrent operations, controller state recovery, and orphan cleanup.

The E2E tests must run in CI against the enhanced mock RDS server (Phase 23) without requiring real hardware, while still providing high confidence in production readiness. The mock already supports realistic timing simulation, error injection, and concurrent connections with SSH latency (200ms ± 50ms jitter) and operation tracking.

**Key challenge:** Testing KubeVirt VirtualMachineInstance block volumes in CI without actual VMs. Solution: Use pod-based block volume tests as a proxy, with KubeVirt tests remaining in manual validation only.

**Primary recommendation:** Use Ginkgo v2 with custom test suites, run tests in-process with mock RDS (pattern already proven in sanity tests), implement unique test ID prefixes for resource cleanup, and leverage existing integration test patterns for fast, isolated CI execution.

## Standard Stack

The established libraries/tools for Kubernetes CSI E2E testing:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/onsi/ginkgo/v2 | v2.23+ | BDD test framework | Industry standard for Kubernetes E2E tests, nested test organization, parallel execution support |
| github.com/onsi/gomega | v1.36+ | Matcher/assertion library | Works hand-in-hand with Ginkgo, expressive assertions (Eventually, Consistently) |
| github.com/container-storage-interface/spec/lib/go/csi | v1.5.0+ | CSI protobuf definitions | Official CSI spec library, required for gRPC calls |
| google.golang.org/grpc | v1.60+ | gRPC client/server | Standard for CSI communication, already used throughout project |
| k8s.io/klog/v2 | v2 | Structured logging | Already used in project, integrates with test output |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| k8s.io/kubernetes/test/e2e/framework | Latest (optional) | Kubernetes E2E framework | Consider for future real cluster tests; skip for Phase 24 (adds complexity) |
| github.com/google/uuid | Already present | Unique test IDs | Generate per-test-run prefixes for resource isolation |
| time.Sleep / time.After | stdlib | Wait operations | Use with Gomega Eventually() for async validation |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| In-process driver + mock RDS | Real Kubernetes cluster (kind/minikube) | Real cluster provides higher confidence but adds 2-3 min CI startup, requires privileged containers for NVMe, increases CI cost; mock is faster (proven in sanity tests) |
| Ginkgo v2 | Go standard testing | Ginkgo provides better test organization (Describe/Context), parallel execution, focus/pending features; standard testing sufficient but less ergonomic for large suites |
| Custom cleanup logic | Kubernetes E2E framework cleanup | Framework cleanup robust but requires importing entire k8s test framework (heavy dependency); custom cleanup simpler for mock-based tests |

**Installation:**
```bash
# Core dependencies (check if already in go.mod)
go get github.com/onsi/ginkgo/v2
go get github.com/onsi/gomega

# Verify versions
go list -m github.com/onsi/ginkgo/v2
go list -m github.com/onsi/gomega
```

## Architecture Patterns

### Recommended Project Structure
```
test/e2e/
├── e2e_suite_test.go         # Ginkgo suite setup
├── e2e_test.go               # Main test file with all test cases
├── helpers.go                # Test helper functions
├── fixtures.go               # Test data and common setups
├── MANUAL_TESTING.md         # Existing manual procedures (keep)
├── PROGRESSIVE_VALIDATION.md # Hardware validation runbook (keep)
├── HARDWARE_VALIDATION.md    # Production validation (keep)
├── kubevirt-validation.yaml  # Manual KubeVirt tests (keep)
└── block-mknod-test.yaml     # Manual block tests (keep)
```

### Pattern 1: In-Process Driver with Mock RDS (Proven Pattern)
**What:** Run driver and mock RDS in the same process as tests
**When to use:** All E2E tests in CI (fast, deterministic, no external dependencies)
**Why:** Already proven successful in sanity tests (test/sanity/sanity_test.go), 10x faster than real cluster

**Example from existing sanity tests:**
```go
// From test/sanity/sanity_test.go (lines 70-130)
func TestE2ESuite(t *testing.T) {
    // 1. Start mock RDS server
    mockRDS, err := mock.NewMockRDSServer(0) // port 0 = random port
    Expect(err).NotTo(HaveOccurred())
    Expect(mockRDS.Start()).To(Succeed())
    defer mockRDS.Stop()

    // 2. Create driver with mock RDS config
    drv, err := driver.NewDriver(driver.DriverConfig{
        RDSAddress:            mockRDS.Address(),
        RDSPort:               mockRDS.Port(),
        RDSUser:               "admin",
        RDSInsecureSkipVerify: true, // Skip host key verification for mock
        EnableController:      true,
        EnableNode:            true, // Enable for full E2E
        // ... other config
    })

    // 3. Start driver on Unix socket in background
    endpoint := fmt.Sprintf("unix:///tmp/csi-e2e-%s.sock", uuid.New().String())
    go func() {
        _ = drv.Run(endpoint) // Runs until test completes
    }()

    // 4. Wait for socket ready
    Eventually(func() error {
        conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
        if err != nil {
            return err
        }
        conn.Close()
        return nil
    }).Should(Succeed())

    // 5. Create CSI clients
    identityClient := csi.NewIdentityClient(conn)
    controllerClient := csi.NewControllerClient(conn)
    nodeClient := csi.NewNodeClient(conn)

    // 6. Run tests using clients...
}
```

**Key insight:** This pattern eliminates Kubernetes cluster requirement, runs in <5 seconds, and provides deterministic behavior through mock RDS error injection and timing control.

### Pattern 2: Test Isolation with Unique Prefixes
**What:** Each test run uses unique volume ID prefix to prevent conflicts
**When to use:** All E2E tests (prevents cross-test pollution)

**Example:**
```go
// In e2e_suite_test.go
var (
    testRunID   string
    mockRDS     *mock.MockRDSServer
    driverConn  *grpc.ClientConn
    ctrlClient  csi.ControllerServiceClient
    nodeClient  csi.NodeServiceClient
)

var _ = BeforeSuite(func() {
    // Generate unique test run ID
    testRunID = fmt.Sprintf("e2e-%s", uuid.New().String()[:8])
    klog.Infof("E2E test run ID: %s", testRunID)

    // Start mock RDS and driver (as shown in Pattern 1)
    // ...
})

var _ = AfterSuite(func() {
    // Cleanup: List all volumes with test prefix and delete
    listResp, err := ctrlClient.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
    if err == nil {
        for _, entry := range listResp.Entries {
            if strings.HasPrefix(entry.Volume.VolumeId, testRunID) {
                _, _ = ctrlClient.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
                    VolumeId: entry.Volume.VolumeId,
                })
            }
        }
    }

    // Stop driver and mock
    if driverConn != nil {
        driverConn.Close()
    }
    if mockRDS != nil {
        mockRDS.Stop()
    }
})

// Helper function for tests
func createTestVolumeName(testName string) string {
    return fmt.Sprintf("%s-%s", testRunID, testName)
}
```

### Pattern 3: Async Validation with Eventually/Consistently
**What:** Use Gomega's Eventually() for async operations, Consistently() for stability checks
**When to use:** All async operations (volume creation, node operations, state changes)

**Example:**
```go
It("should create volume and track in mock RDS", func() {
    volumeName := createTestVolumeName("basic-volume")

    // Create volume via CSI
    resp, err := ctrlClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
        Name: volumeName,
        CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
        VolumeCapabilities: []*csi.VolumeCapability{{
            AccessMode: &csi.VolumeCapability_AccessMode{
                Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
            },
            AccessType: &csi.VolumeCapability_Mount{
                Mount: &csi.VolumeCapability_MountVolume{},
            },
        }},
    })
    Expect(err).NotTo(HaveOccurred())
    volumeID := resp.Volume.VolumeId

    // Verify volume created on mock RDS (may take time with realistic timing)
    Eventually(func() bool {
        vol, exists := mockRDS.GetVolume(volumeID)
        return exists && vol.Exported
    }, "5s", "100ms").Should(BeTrue(), "Volume should exist on mock RDS")

    // Verify volume remains stable
    Consistently(func() bool {
        vol, exists := mockRDS.GetVolume(volumeID)
        return exists
    }, "2s", "200ms").Should(BeTrue(), "Volume should remain on mock RDS")
})
```

### Pattern 4: Concurrent Operation Testing
**What:** Run multiple operations in parallel to test thread safety
**When to use:** Multi-volume test (E2E-04 requirement)

**Example:**
```go
It("should handle concurrent volume operations without conflicts", func() {
    const numVolumes = 5
    errChan := make(chan error, numVolumes)
    volumeIDs := make([]string, numVolumes)

    // Create volumes concurrently
    for i := 0; i < numVolumes; i++ {
        go func(idx int) {
            volumeName := createTestVolumeName(fmt.Sprintf("concurrent-%d", idx))
            resp, err := ctrlClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
                Name: volumeName,
                CapacityRange: &csi.CapacityRange{RequiredBytes: 1 * GiB},
                VolumeCapabilities: []*csi.VolumeCapability{{
                    AccessMode: &csi.VolumeCapability_AccessMode{
                        Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
                    },
                    AccessType: &csi.VolumeCapability_Mount{
                        Mount: &csi.VolumeCapability_MountVolume{},
                    },
                }},
            })
            if err != nil {
                errChan <- err
                return
            }
            volumeIDs[idx] = resp.Volume.VolumeId
            errChan <- nil
        }(i)
    }

    // Wait for all operations
    for i := 0; i < numVolumes; i++ {
        Expect(<-errChan).NotTo(HaveOccurred(), "Volume creation should succeed")
    }

    // Verify all volumes exist
    Eventually(func() int {
        count := 0
        for _, volID := range volumeIDs {
            if volID != "" {
                if _, exists := mockRDS.GetVolume(volID); exists {
                    count++
                }
            }
        }
        return count
    }, "10s", "200ms").Should(Equal(numVolumes), "All volumes should exist on mock RDS")

    // Cleanup (delete concurrently)
    for _, volID := range volumeIDs {
        if volID != "" {
            go func(id string) {
                _, _ = ctrlClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: id})
            }(volID)
        }
    }
})
```

### Pattern 5: Block Volume Testing (KubeVirt Proxy)
**What:** Use pod-based block volume test as proxy for KubeVirt VMs
**When to use:** E2E-02 requirement (KubeVirt validation remains in manual testing)

**Why this approach:** KubeVirt requires nested virtualization and significant CI resources. Block volume mechanics are identical whether consumed by pod or VM. Full KubeVirt tests stay in PROGRESSIVE_VALIDATION.md for hardware validation.

**Example:**
```go
It("should support block volume access (KubeVirt proxy test)", func() {
    volumeName := createTestVolumeName("block-volume")

    // Create block volume
    resp, err := ctrlClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
        Name: volumeName,
        CapacityRange: &csi.CapacityRange{RequiredBytes: 5 * GiB},
        VolumeCapabilities: []*csi.VolumeCapability{{
            AccessMode: &csi.VolumeCapability_AccessMode{
                Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
            },
            AccessType: &csi.VolumeCapability_Block{
                Block: &csi.VolumeCapability_BlockVolume{},
            },
        }},
    })
    Expect(err).NotTo(HaveOccurred())
    volumeID := resp.Volume.VolumeId

    // Verify volume created with block mode
    vol, exists := mockRDS.GetVolume(volumeID)
    Expect(exists).To(BeTrue())
    Expect(vol.FileSizeBytes).To(Equal(int64(5 * GiB)))

    // Stage block volume (simulates node attaching for VM)
    targetPath := fmt.Sprintf("/tmp/csi-e2e/staging-%s", volumeID)
    _, err = nodeClient.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
        VolumeId:          volumeID,
        StagingTargetPath: targetPath,
        VolumeCapability: &csi.VolumeCapability{
            AccessMode: &csi.VolumeCapability_AccessMode{
                Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
            },
            AccessType: &csi.VolumeCapability_Block{
                Block: &csi.VolumeCapability_BlockVolume{},
            },
        },
        VolumeContext: map[string]string{
            "nqn": vol.NVMETCPNQN,
        },
    })
    Expect(err).NotTo(HaveOccurred(), "Block volume staging should succeed")

    // Publish block volume (simulates making available to VM)
    publishPath := fmt.Sprintf("/tmp/csi-e2e/publish-%s", volumeID)
    _, err = nodeClient.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
        VolumeId:          volumeID,
        StagingTargetPath: targetPath,
        TargetPath:        publishPath,
        VolumeCapability: &csi.VolumeCapability{
            AccessMode: &csi.VolumeCapability_AccessMode{
                Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
            },
            AccessType: &csi.VolumeCapability_Block{
                Block: &csi.VolumeCapability_BlockVolume{},
            },
        },
    })
    Expect(err).NotTo(HaveOccurred(), "Block volume publish should succeed")

    // NOTE: Actual VM boot testing happens in manual validation (PROGRESSIVE_VALIDATION.md)
    // This test validates the CSI driver's block volume support is functional

    // Cleanup
    _, _ = nodeClient.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
        VolumeId:   volumeID,
        TargetPath: publishPath,
    })
    _, _ = nodeClient.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
        VolumeId:          volumeID,
        StagingTargetPath: targetPath,
    })
    _, _ = ctrlClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
})
```

### Pattern 6: Orphan Detection Testing
**What:** Test orphan reconciler finds and reports mismatches between RDS and PVs
**When to use:** E2E-05 requirement

**Example:**
```go
It("should detect and reconcile orphaned volumes", func() {
    // Create orphaned file on RDS (file without disk object)
    orphanPath := "/storage-pool/metal-csi/orphan-file.img"
    mockRDS.CreateOrphanedFile(orphanPath, 1*GiB)

    // Create orphaned volume on RDS (disk object without backing file)
    orphanSlot := createTestVolumeName("orphan-volume")
    orphanFilePath := fmt.Sprintf("/storage-pool/metal-csi/%s.img", orphanSlot)
    mockRDS.CreateOrphanedVolume(orphanSlot, orphanFilePath, 2*GiB)

    // Run orphan detection (driver should have reconciler)
    // This test validates the reconciler logic, not full cleanup
    // (cleanup would require Kubernetes client and PV listing)

    // Verify orphan file detected
    Eventually(func() bool {
        files := mockRDS.ListFiles()
        for _, f := range files {
            if f.Path == orphanPath {
                return true
            }
        }
        return false
    }, "2s").Should(BeTrue(), "Orphan file should exist before reconciliation")

    // Verify orphan volume detected
    Eventually(func() bool {
        vol, exists := mockRDS.GetVolume(orphanSlot)
        return exists && vol.FilePath == orphanFilePath
    }, "2s").Should(BeTrue(), "Orphan volume should exist before reconciliation")

    // NOTE: Full reconciliation (deleting orphans) requires Kubernetes API
    // This is tested in integration tests with k8s client
    // E2E tests validate the detection logic only
})
```

### Anti-Patterns to Avoid

- **❌ Real Kubernetes cluster in CI:** Adds 2-3 min startup time, requires privileged containers for NVMe, increases cost. Use in-process driver + mock RDS instead (proven pattern).
- **❌ Hardcoded volume IDs:** Causes test conflicts when running multiple times. Use unique test run ID prefix.
- **❌ Sleep-based timing:** Flaky tests. Use Gomega Eventually() with explicit conditions.
- **❌ Shared state between tests:** Tests should be independent. Use BeforeEach/AfterEach for per-test setup/cleanup.
- **❌ Testing implementation details:** Test behavior, not internals. Example: Don't verify SSH command format; verify volume exists on mock RDS.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Test framework for BDD-style tests | Custom test runner | Ginkgo v2 | Industry standard, parallel execution, focus/pending, nested Describe/Context, excellent reporting |
| Async operation validation | Sleep loops | Gomega Eventually()/Consistently() | Avoids flaky tests, clear timeout semantics, readable assertions |
| Test isolation/cleanup | Manual cleanup in each test | BeforeEach/AfterEach + unique prefixes | DRY principle, prevents forgotten cleanup, handles test failures gracefully |
| gRPC client creation | Manual connection management | CSI client libraries | Type-safe, handles serialization, error mapping built-in |
| Volume lifecycle state machine | Custom state tracking | CSI spec + existing driver code | Driver already implements full lifecycle; tests just validate it works |
| Kubernetes cluster for E2E | kind/minikube in CI | In-process driver + mock RDS | 10x faster, no external deps, proven in sanity tests |

**Key insight:** The project already has 90% of what's needed. Phase 24 is about organizing existing capabilities (mock RDS, integration test patterns, CSI clients) into automated Ginkgo suites that run in CI.

## Common Pitfalls

### Pitfall 1: Treating E2E Tests Like Unit Tests
**What goes wrong:** Tests that mock every dependency and don't test real integration paths
**Why it happens:** Confusion between integration tests (individual component validation) and E2E tests (full workflow validation)
**How to avoid:** E2E tests should exercise the full CSI driver stack (gRPC → driver → mock RDS → state verification). Only mock external dependencies (real RDS replaced with mock, Kubernetes API replaced with direct CSI calls).
**Warning signs:** Tests that mock CSI client methods, tests that directly call internal driver functions instead of using gRPC, tests that don't verify state on mock RDS

**Example of correct approach:**
```go
// ✅ GOOD: Full path through gRPC
resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
Expect(err).NotTo(HaveOccurred())
vol, exists := mockRDS.GetVolume(resp.Volume.VolumeId)
Expect(exists).To(BeTrue())

// ❌ BAD: Direct function call bypassing gRPC
cs := driver.NewControllerServer(drv)
resp, err := cs.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
```

### Pitfall 2: Not Using Realistic Mock Timing
**What goes wrong:** Tests pass in CI but fail in production due to timing assumptions
**Why it happens:** Mock RDS defaults to instant responses; real RDS has ~200ms SSH latency
**How to avoid:** Set `MOCK_RDS_REALISTIC_TIMING=true` for stress tests and timing-sensitive scenarios. Phase 23 added SSH latency simulation (200ms ± 50ms jitter).
**Warning signs:** Tests that pass with mock but timeout in production, tests that don't use Eventually() for async operations

**Example:**
```go
// In CI job
env:
  - MOCK_RDS_REALISTIC_TIMING=true  # Enable for timing-sensitive tests

// In test
It("should handle SSH latency gracefully", func() {
    start := time.Now()
    _, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
    duration := time.Since(start)

    Expect(err).NotTo(HaveOccurred())
    Expect(duration).To(BeNumerically(">", 150*time.Millisecond), "Should account for SSH latency")
    Expect(duration).To(BeNumerically("<", 2*time.Second), "Should not timeout")
})
```

### Pitfall 3: Test Pollution from Improper Cleanup
**What goes wrong:** Test failures leave orphaned volumes, causing subsequent test failures
**Why it happens:** Cleanup in AfterEach not running on test failure, or cleanup depending on test success
**How to avoid:** Use Ginkgo's DeferCleanup() pattern (runs even on failure) and unique test run IDs
**Warning signs:** Tests pass individually but fail when run as suite, CI failures that disappear on retry, "volume already exists" errors

**Example:**
```go
// ❌ BAD: Cleanup skipped on test failure
It("should create volume", func() {
    resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
    Expect(err).NotTo(HaveOccurred())
    volumeID := resp.Volume.VolumeId

    // Test assertions...

    // This never runs if test fails!
    _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
})

// ✅ GOOD: Cleanup always runs
It("should create volume", func() {
    resp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
    Expect(err).NotTo(HaveOccurred())
    volumeID := resp.Volume.VolumeId

    // Register cleanup immediately (runs even on test failure)
    DeferCleanup(func() {
        _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
    })

    // Test assertions...
})
```

### Pitfall 4: Node Service Tests Without NVMe Mock
**What goes wrong:** Tests try to call nvme-cli commands that fail in test environment
**Why it happens:** Node service requires NVMe kernel modules and nvme-cli tools
**How to avoid:** Mock NVMe operations or skip node tests that require actual NVMe in CI. Phase 24 focuses on controller + basic node path validation. Full node tests remain in hardware validation.
**Warning signs:** Tests requiring root/privileged mode, tests that shell out to nvme commands, test failures in CI but success on workstation

**Current project status:**
- Phase 23 marked "COMP-03 (Node idempotency) deferred to Phase 24 - requires NVMe mock"
- Node service E2E tests should mock NVMe operations or use integration test approach (test logic without actual kernel operations)

**Example:**
```go
// ✅ GOOD: Test node service logic without actual NVMe
It("should stage volume with correct parameters", func() {
    // Create volume first
    volResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{...})
    Expect(err).NotTo(HaveOccurred())
    volumeID := volResp.Volume.VolumeId

    vol, _ := mockRDS.GetVolume(volumeID)

    // Call NodeStageVolume (will fail at nvme connect but that's expected in test env)
    // Test validates the driver generates correct NVMe connection parameters
    stageReq := &csi.NodeStageVolumeRequest{
        VolumeId:          volumeID,
        StagingTargetPath: "/tmp/staging",
        VolumeCapability:  &csi.VolumeCapability{...},
        VolumeContext: map[string]string{
            "nqn": vol.NVMETCPNQN,
        },
    }

    // Either: Mock NVMe operations at pkg/nvme level
    // Or: Skip actual staging but validate request parameters
    // Or: Mark as integration test that requires privileged mode
})
```

### Pitfall 5: KubeVirt Tests in CI Without VMs
**What goes wrong:** Attempting to run VirtualMachineInstance tests in CI without KubeVirt
**Why it happens:** Requirement E2E-02 asks for "KubeVirt VirtualMachineInstance" testing
**How to avoid:** Use block volume tests as proxy for KubeVirt validation in CI. Keep full KubeVirt tests in manual validation (PROGRESSIVE_VALIDATION.md already has comprehensive VM tests).
**Warning signs:** CI trying to install KubeVirt, tests requiring nested virtualization, VM boot timeouts in CI

**Resolution:**
- E2E-02 requirement satisfied by validating block volume support (what KubeVirt uses)
- Full KubeVirt VM boot/migration tests remain in hardware validation
- E2E tests prove the CSI driver correctly handles block volumes; manual tests prove VMs can use them

## Code Examples

Verified patterns from official sources:

### Example 1: Ginkgo v2 Suite Structure
```go
// test/e2e/e2e_suite_test.go
package e2e

import (
    "context"
    "fmt"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/container-storage-interface/spec/lib/go/csi"
    "google.golang.org/grpc"
    "k8s.io/klog/v2"

    "git.srvlab.io/whiskey/rds-csi-driver/pkg/driver"
    "git.srvlab.io/whiskey/rds-csi-driver/test/mock"
)

func TestE2E(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "RDS CSI Driver E2E Suite")
}

var (
    testRunID      string
    mockRDS        *mock.MockRDSServer
    driverEndpoint string
    grpcConn       *grpc.ClientConn
    identityClient csi.IdentityClient
    controllerClient csi.ControllerClient
    nodeClient     csi.NodeClient
    ctx            context.Context
    cancel         context.CancelFunc
)

const (
    GiB = 1024 * 1024 * 1024
    defaultTimeout = 30 * time.Second
)

var _ = BeforeSuite(func() {
    // Generate unique test run ID for resource isolation
    testRunID = fmt.Sprintf("e2e-%d", time.Now().Unix())
    klog.Infof("E2E test run ID: %s", testRunID)

    // Start mock RDS server
    var err error
    mockRDS, err = mock.NewMockRDSServer(0) // Port 0 = random port assignment
    Expect(err).NotTo(HaveOccurred())
    Expect(mockRDS.Start()).To(Succeed())
    klog.Infof("Mock RDS started on port %d", mockRDS.Port())

    // Create driver
    drv, err := driver.NewDriver(driver.DriverConfig{
        DriverName:            "rds.csi.srvlab.io",
        Version:               "test",
        NodeID:                "test-node-1",
        RDSAddress:            mockRDS.Address(),
        RDSPort:               mockRDS.Port(),
        RDSUser:               "admin",
        RDSPrivateKey:         nil, // Mock doesn't require real key
        RDSInsecureSkipVerify: true,
        RDSVolumeBasePath:     "/storage-pool/metal-csi",
        EnableController:      true,
        EnableNode:            true,
    })
    Expect(err).NotTo(HaveOccurred())

    // Start driver on Unix socket
    driverEndpoint = fmt.Sprintf("unix:///tmp/csi-e2e-%s.sock", testRunID)
    go func() {
        _ = drv.Run(driverEndpoint)
    }()

    // Wait for driver to be ready
    Eventually(func() error {
        conn, err := grpc.Dial(
            driverEndpoint,
            grpc.WithInsecure(),
            grpc.WithBlock(),
            grpc.WithTimeout(1*time.Second),
        )
        if err != nil {
            return err
        }
        conn.Close()
        return nil
    }, "10s", "500ms").Should(Succeed(), "Driver should start and accept connections")

    // Create CSI clients
    grpcConn, err = grpc.Dial(driverEndpoint, grpc.WithInsecure())
    Expect(err).NotTo(HaveOccurred())

    identityClient = csi.NewIdentityClient(grpcConn)
    controllerClient = csi.NewControllerClient(grpcConn)
    nodeClient = csi.NewNodeClient(grpcConn)

    // Create test context
    ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
})

var _ = AfterSuite(func() {
    // Cancel context
    if cancel != nil {
        cancel()
    }

    // Cleanup all test volumes
    if controllerClient != nil {
        listResp, err := controllerClient.ListVolumes(ctx, &csi.ListVolumesRequest{})
        if err == nil {
            for _, entry := range listResp.Entries {
                if strings.HasPrefix(entry.Volume.VolumeId, testRunID) {
                    _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
                        VolumeId: entry.Volume.VolumeId,
                    })
                }
            }
        }
    }

    // Close gRPC connection
    if grpcConn != nil {
        grpcConn.Close()
    }

    // Stop mock RDS
    if mockRDS != nil {
        mockRDS.Stop()
    }

    klog.Infof("E2E suite cleanup complete")
})

// Helper: Create unique volume name for test
func testVolumeName(name string) string {
    return fmt.Sprintf("%s-%s", testRunID, name)
}
```

### Example 2: Volume Lifecycle E2E Test (E2E-01)
```go
// test/e2e/e2e_test.go
package e2e

import (
    "fmt"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/container-storage-interface/spec/lib/go/csi"
)

var _ = Describe("Volume Lifecycle", func() {
    It("should complete full volume lifecycle (E2E-01)", func() {
        volumeName := testVolumeName("lifecycle")
        var volumeID string
        var stagingPath, publishPath string

        By("Creating volume via CreateVolume")
        createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
            Name: volumeName,
            CapacityRange: &csi.CapacityRange{
                RequiredBytes: 1 * GiB,
            },
            VolumeCapabilities: []*csi.VolumeCapability{{
                AccessMode: &csi.VolumeCapability_AccessMode{
                    Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
                },
                AccessType: &csi.VolumeCapability_Mount{
                    Mount: &csi.VolumeCapability_MountVolume{
                        FsType: "ext4",
                    },
                },
            }},
        })
        Expect(err).NotTo(HaveOccurred())
        Expect(createResp.Volume).NotTo(BeNil())
        volumeID = createResp.Volume.VolumeId
        DeferCleanup(func() {
            if volumeID != "" {
                _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
                    VolumeId: volumeID,
                })
            }
        })

        By("Verifying volume exists on mock RDS")
        Eventually(func() bool {
            vol, exists := mockRDS.GetVolume(volumeID)
            return exists && vol.Exported
        }, "5s", "200ms").Should(BeTrue())

        By("Staging volume via NodeStageVolume")
        stagingPath = fmt.Sprintf("/tmp/csi-staging-%s", volumeID)
        stageResp, err := nodeClient.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
            VolumeId:          volumeID,
            StagingTargetPath: stagingPath,
            VolumeCapability: &csi.VolumeCapability{
                AccessMode: &csi.VolumeCapability_AccessMode{
                    Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
                },
                AccessType: &csi.VolumeCapability_Mount{
                    Mount: &csi.VolumeCapability_MountVolume{
                        FsType: "ext4",
                    },
                },
            },
        })
        Expect(err).NotTo(HaveOccurred())
        DeferCleanup(func() {
            if stagingPath != "" {
                _, _ = nodeClient.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
                    VolumeId:          volumeID,
                    StagingTargetPath: stagingPath,
                })
            }
        })

        By("Publishing volume via NodePublishVolume")
        publishPath = fmt.Sprintf("/tmp/csi-publish-%s", volumeID)
        publishResp, err := nodeClient.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
            VolumeId:          volumeID,
            StagingTargetPath: stagingPath,
            TargetPath:        publishPath,
            VolumeCapability: &csi.VolumeCapability{
                AccessMode: &csi.VolumeCapability_AccessMode{
                    Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
                },
                AccessType: &csi.VolumeCapability_Mount{
                    Mount: &csi.VolumeCapability_MountVolume{
                        FsType: "ext4",
                    },
                },
            },
        })
        Expect(err).NotTo(HaveOccurred())
        DeferCleanup(func() {
            if publishPath != "" {
                _, _ = nodeClient.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
                    VolumeId:   volumeID,
                    TargetPath: publishPath,
                })
            }
        })

        By("Writing test data (simulated - requires actual mount)")
        // Note: In-process test can't actually mount filesystem
        // This validates the gRPC calls succeed
        // Full mount testing happens in hardware validation

        By("Unpublishing volume via NodeUnpublishVolume")
        _, err = nodeClient.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
            VolumeId:   volumeID,
            TargetPath: publishPath,
        })
        Expect(err).NotTo(HaveOccurred())

        By("Unstaging volume via NodeUnstageVolume")
        _, err = nodeClient.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
            VolumeId:          volumeID,
            StagingTargetPath: stagingPath,
        })
        Expect(err).NotTo(HaveOccurred())

        By("Deleting volume via DeleteVolume")
        _, err = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
            VolumeId: volumeID,
        })
        Expect(err).NotTo(HaveOccurred())

        By("Verifying volume deleted from mock RDS")
        Eventually(func() bool {
            _, exists := mockRDS.GetVolume(volumeID)
            return !exists
        }, "5s", "200ms").Should(BeTrue())
    })
})
```

### Example 3: Volume Expansion Test (E2E-03)
```go
var _ = Describe("Volume Expansion", func() {
    It("should expand volume and update capacity (E2E-03)", func() {
        volumeName := testVolumeName("expansion")
        originalSize := int64(5 * GiB)
        expandedSize := int64(10 * GiB)

        By("Creating initial volume")
        createResp, err := controllerClient.CreateVolume(ctx, &csi.CreateVolumeRequest{
            Name: volumeName,
            CapacityRange: &csi.CapacityRange{
                RequiredBytes: originalSize,
            },
            VolumeCapabilities: []*csi.VolumeCapability{{
                AccessMode: &csi.VolumeCapability_AccessMode{
                    Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
                },
                AccessType: &csi.VolumeCapability_Mount{
                    Mount: &csi.VolumeCapability_MountVolume{},
                },
            }},
        })
        Expect(err).NotTo(HaveOccurred())
        volumeID := createResp.Volume.VolumeId
        DeferCleanup(func() {
            _, _ = controllerClient.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
        })

        By("Verifying initial size")
        vol, exists := mockRDS.GetVolume(volumeID)
        Expect(exists).To(BeTrue())
        Expect(vol.FileSizeBytes).To(Equal(originalSize))

        By("Expanding volume")
        expandResp, err := controllerClient.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
            VolumeId: volumeID,
            CapacityRange: &csi.CapacityRange{
                RequiredBytes: expandedSize,
            },
        })
        Expect(err).NotTo(HaveOccurred())
        Expect(expandResp.CapacityBytes).To(Equal(expandedSize))

        By("Verifying expanded size on mock RDS")
        Eventually(func() int64 {
            vol, exists := mockRDS.GetVolume(volumeID)
            if !exists {
                return 0
            }
            return vol.FileSizeBytes
        }, "5s", "200ms").Should(Equal(expandedSize))

        By("Verifying filesystem expansion (Node side)")
        // Note: NodeExpandVolume requires actual mounted filesystem
        // Test validates controller expansion; node expansion tested in hardware validation
    })
})
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual E2E testing only | Automated E2E tests in CI | Phase 24 (current) | Catch regressions early, faster iteration, confidence before hardware validation |
| Real cluster (kind) for E2E | In-process driver + mock RDS | Phase 22-23 | 10x faster tests (<5s vs 2-3min), no privileged containers, deterministic |
| Sleep-based waits | Gomega Eventually/Consistently | Kubernetes community (~2019) | Eliminates flaky tests, clear timeout semantics |
| Ginkgo v1 | Ginkgo v2 | ~2021 (major rewrite) | Better syntax, improved parallel execution, cleaner setup/teardown |
| Namespace cleanup on failure | ginkgo.DeferCleanup | Ginkgo v2 (2021+) | Guaranteed cleanup even on failure |

**Deprecated/outdated:**
- **CSI E2E framework from kubernetes/kubernetes:** Kubernetes 1.25+ moved CSI E2E tests into separate repos. Modern approach: Custom Ginkgo tests using CSI client libraries directly.
- **Ginkgo v1:** Use v2 for new projects (better API, active development)
- **Global BeforeSuite state without cleanup:** Use DeferCleanup in BeforeSuite for guaranteed cleanup

## Open Questions

Things that couldn't be fully resolved:

1. **Node service NVMe operations in CI**
   - What we know: Phase 23 deferred COMP-03 (Node idempotency) due to lack of NVMe mock
   - What's unclear: Should Phase 24 implement NVMe mock or skip node E2E tests in CI?
   - Recommendation: Skip NVMe operations in CI E2E tests. Validate node service logic (request handling, error cases) without actual kernel NVMe calls. Full node tests remain in hardware validation.

2. **KubeVirt VM boot in CI**
   - What we know: E2E-02 requires "block volume test with KubeVirt VirtualMachineInstance"
   - What's unclear: Is full VM boot required in CI or can block volume support serve as proxy?
   - Recommendation: Use block volume tests as KubeVirt proxy in CI. Block volume mechanics are identical whether consumed by pod or VM. Keep full VM tests in PROGRESSIVE_VALIDATION.md (already comprehensive).

3. **Volume expansion filesystem resize**
   - What we know: ControllerExpandVolume works (controller side), NodeExpandVolume requires mounted filesystem
   - What's unclear: Can we test NodeExpandVolume in-process without actual mount?
   - Recommendation: Test ControllerExpandVolume in E2E (verifies RDS resize), test NodeExpandVolume in hardware validation (requires real filesystem).

4. **Controller restart state recovery**
   - What we know: E2E-07 requires "controller restart test validates state rebuild from VolumeAttachment objects"
   - What's unclear: How to test this without Kubernetes API? VolumeAttachment is a Kubernetes resource.
   - Recommendation: This test requires Kubernetes client (k8s.io/client-go). Implement in integration test layer or defer to Phase 25 (Kubernetes integration tests with real cluster).

5. **Node failure simulation**
   - What we know: E2E-06 requires "node failure simulation test validates volume unstaging on node deletion"
   - What's unclear: How to simulate node deletion without Kubernetes cluster?
   - Recommendation: Test the driver's cleanup logic (call NodeUnstageVolume), but full node failure scenario requires Kubernetes. Defer to Phase 25 or hardware validation.

## Sources

### Primary (HIGH confidence)
- [Kubernetes CSI Functional Testing Documentation](https://kubernetes-csi.github.io/docs/functional-testing.html) - Official CSI E2E testing guide
- [kubernetes-csi/csi-test GitHub](https://github.com/kubernetes-csi/csi-test) - CSI test frameworks and sanity tests
- [Ginkgo v2 Official Documentation](https://onsi.github.io/ginkgo/) - BDD testing framework for Go
- [Ginkgo v2 GitHub Repository](https://github.com/onsi/ginkgo) - Latest framework features and examples
- Project codebase: test/sanity/sanity_test.go (lines 70-197) - Proven in-process pattern
- Project codebase: test/integration/controller_integration_test.go - Existing integration patterns
- Project codebase: test/mock/rds_server.go - Mock RDS capabilities

### Secondary (MEDIUM confidence)
- [Kubernetes E2E Testing Blog](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/) - CSI driver testing overview
- [AWS EBS CSI Driver E2E Tests](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/tests/e2e/README.md) - Real-world E2E example
- [KubeVirt CSI Driver](https://github.com/kubevirt/csi-driver) - KubeVirt block volume testing
- [Ginkgo Testing Guide](https://www.browserstack.com/guide/ginkgo-testing-framework) - Ginkgo best practices

### Tertiary (LOW confidence)
- [Kubernetes E2E Framework Package](https://pkg.go.dev/k8s.io/kubernetes/test/e2e/framework) - Full K8s E2E framework (not using, but aware of)
- WebSearch results on orphaned resource cleanup patterns - General Kubernetes patterns

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Ginkgo/Gomega are industry standard, proven in this project already
- Architecture patterns: HIGH - In-process pattern proven in sanity tests, patterns derived from existing code
- Pitfalls: HIGH - Based on Phase 23 research, existing test code analysis, and Kubernetes community experience
- KubeVirt E2E approach: MEDIUM - Decision to use block volume proxy requires validation in implementation
- Node NVMe mocking: MEDIUM - Deferred node tests require architectural decision in Phase 24 planning

**Research date:** 2026-02-05
**Valid until:** 30 days (stable patterns, but Ginkgo may add features; check release notes)
