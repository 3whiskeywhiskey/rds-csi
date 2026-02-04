# Phase 12: Compatibility and Quality - Research

**Researched:** 2026-02-03
**Domain:** CSI driver compatibility testing, regression prevention, error message design
**Confidence:** HIGH

## Summary

Phase 12 ensures that block volume support (Phase 11) doesn't break existing filesystem volume functionality and provides clear, actionable error messages for invalid volume mode combinations. This is a quality assurance phase focused on regression testing and user experience.

The standard approach for CSI driver regression testing uses table-driven unit tests to verify both block and filesystem volume paths, following Go testing best practices. Tests should cover all CSI operations (Stage, Publish, Unstage, Unpublish) for both volume modes to ensure no regressions in existing filesystem code paths.

Error message validation follows CSI community patterns: return appropriate gRPC status codes (InvalidArgument for validation errors) with messages that explain WHAT is wrong, WHY it's wrong, and HOW to fix it. The existing RWX filesystem rejection error in this codebase is an excellent example: "RWX access mode requires volumeMode: Block. Filesystem volumes risk data corruption with multi-node access. For KubeVirt VM live migration, use volumeMode: Block in your PVC".

**Primary recommendation:** Use table-driven unit tests with comprehensive test cases for both block and filesystem volumes across all node operations. Structure tests with clear test names, expected outcomes, and descriptive error messages that help developers quickly identify regression failures.

## Standard Stack

The established libraries/tools for CSI driver testing:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing | Go stdlib | Unit testing framework | Official Go testing package, supports table-driven tests with t.Run() |
| csi-test/pkg/sanity | v5.0.0+ | CSI spec compliance | Official Kubernetes CSI test suite, validates spec requirements |
| github.com/container-storage-interface/spec/lib/go/csi | v1.5.0+ | CSI types and interfaces | Official CSI protobuf definitions for Go |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| google.golang.org/grpc/codes | Latest | gRPC status codes | Error handling in CSI methods (InvalidArgument, FailedPrecondition, etc.) |
| google.golang.org/grpc/status | Latest | gRPC status creation | Wrapping errors with proper codes and messages |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| stdlib testing | testify/assert | testify adds dependencies and less idiomatic; stdlib sufficient for CSI testing |
| table-driven tests | separate test functions | separate functions = code duplication, harder to maintain |

**Installation:**
```bash
# Already in project (check go.mod)
go get github.com/container-storage-interface/spec/lib/go/csi@v1.5.0
go get google.golang.org/grpc
```

## Architecture Patterns

### Recommended Test Structure
```
pkg/driver/
├── node.go                    # Implementation
├── node_test.go              # Tests for node operations
│   ├── TestNodeStageVolume_FilesystemVolume
│   ├── TestNodeStageVolume_BlockVolume
│   ├── TestNodePublishVolume_FilesystemVolume
│   ├── TestNodePublishVolume_BlockVolume
│   └── (etc for all operations × both modes)
└── controller.go / controller_test.go
```

### Pattern 1: Table-Driven Regression Tests
**What:** Structure tests as slices of test cases with inputs and expected outcomes
**When to use:** For testing multiple scenarios of the same operation (e.g., block vs filesystem volumes)
**Example:**
```go
// Source: https://go.dev/wiki/TableDrivenTests
func TestNodeStageVolume_BothModes(t *testing.T) {
    tests := []struct {
        name              string
        volumeCapability  *csi.VolumeCapability
        expectFormat      bool  // Should Format() be called?
        expectMount       bool  // Should Mount() be called?
        expectMetadataFile bool  // Should device metadata file exist?
    }{
        {
            name: "filesystem volume - existing behavior",
            volumeCapability: &csi.VolumeCapability{
                AccessType: &csi.VolumeCapability_Mount{
                    Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
                },
            },
            expectFormat:      true,
            expectMount:       true,
            expectMetadataFile: false,
        },
        {
            name: "block volume - new behavior",
            volumeCapability: &csi.VolumeCapability{
                AccessType: &csi.VolumeCapability_Block{
                    Block: &csi.VolumeCapability_BlockVolume{},
                },
            },
            expectFormat:      false,
            expectMount:       false,
            expectMetadataFile: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
            // Verify expectations match actual behavior
            if mounter.formatCalled != tt.expectFormat {
                t.Errorf("Format called = %v, want %v", mounter.formatCalled, tt.expectFormat)
            }
        })
    }
}
```

### Pattern 2: Named Subtests for Regression Detection
**What:** Use t.Run() with descriptive names to identify which specific scenario failed
**When to use:** Always, for every table-driven test
**Example:**
```go
// Source: https://go.dev/wiki/TableDrivenTests
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) {
        // Test runs independently
        // Failure output shows: "TestName/subtest_name"
    })
}
```

### Pattern 3: Actionable Error Messages in CSI Responses
**What:** Error messages that explain the problem AND how to fix it
**When to use:** All validation errors, especially user-facing capability validation
**Example:**
```go
// Source: Existing codebase (pkg/driver/controller.go:949)
// Good: Actionable error with WHY and HOW
return fmt.Errorf("RWX access mode requires volumeMode: Block. " +
    "Filesystem volumes risk data corruption with multi-node access. " +
    "For KubeVirt VM live migration, use volumeMode: Block in your PVC")

// Bad: Generic error with no guidance
return fmt.Errorf("invalid volume capability")
```

### Anti-Patterns to Avoid
- **Separate test functions per scenario:** Leads to massive code duplication; use table-driven tests instead
- **Testing implementation details:** Test behavior (does staging create correct state?), not internals (was function X called?)
- **Vague error messages:** "invalid request" tells user nothing; explain what's invalid and how to fix it
- **No regression coverage:** Adding block volumes without testing filesystem volumes = risk of breaking existing users

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CSI spec compliance testing | Custom validation scripts | csi-test/pkg/sanity | Official test suite covers edge cases, protocol requirements, error codes |
| Test case organization | Ad-hoc test functions | Table-driven tests with t.Run() | Standard Go pattern, reduces duplication, clear failure reporting |
| gRPC error codes | Custom error types | google.golang.org/grpc/status | CSI spec requires specific codes (InvalidArgument, FailedPrecondition, etc.) |

**Key insight:** CSI drivers interact with Kubernetes via well-defined protocols (gRPC). Use official types and test frameworks rather than custom solutions to ensure spec compliance.

## Common Pitfalls

### Pitfall 1: Testing Only New Functionality
**What goes wrong:** Adding block volume tests without verifying filesystem volumes still work leads to regressions
**Why it happens:** Focus on new feature, assume existing code is "safe"
**How to avoid:** Write paired tests for both modes (filesystem AND block) for every operation
**Warning signs:** Test files only contain "Block" in test names, no "Filesystem" regression tests

### Pitfall 2: Implementation-Focused Tests Instead of Behavior Tests
**What goes wrong:** Tests verify mocks were called instead of verifying actual behavior (state changes, files created, etc.)
**Why it happens:** Testing frameworks make it easy to track function calls
**How to avoid:** Assert on observable outcomes (does staging directory exist? does metadata file contain device path?) rather than mock call counts
**Warning signs:** Tests check `mounter.mountCalled == true` without verifying mount actually worked

### Pitfall 3: Cryptic Validation Error Messages
**What goes wrong:** Users get "invalid volume capability" without understanding what's invalid or how to fix it
**Why it happens:** Developers focus on detecting errors, not explaining them
**How to avoid:** Follow pattern: "X is invalid because Y. To fix: Z" (e.g., "volumeMode must be Block for RWX because filesystem risks corruption. Use volumeMode: Block in PVC")
**Warning signs:** Error messages don't mention specific field names, don't explain consequences, don't provide remediation

### Pitfall 4: No Cross-Mode Test Coverage
**What goes wrong:** Block volume code accidentally breaks filesystem volumes (e.g., wrong branch condition)
**Why it happens:** Tests only cover one code path per operation
**How to avoid:** Use table-driven tests with both block and filesystem cases for EVERY CSI operation
**Warning signs:** `go test -cover` shows partial coverage; no tests verify filesystem volume path after block changes

### Pitfall 5: Missing Error Path Testing
**What goes wrong:** Error messages added but never actually tested for correctness
**Why it happens:** Focus on happy path testing
**How to avoid:** Include negative test cases in table (e.g., RWX filesystem should return specific error)
**Warning signs:** Test tables only have `expectError: false` cases

## Code Examples

Verified patterns from existing codebase and Go official docs:

### Table-Driven Test Structure
```go
// Source: pkg/driver/controller_test.go:908-994
func TestValidateVolumeCapabilities_RWX(t *testing.T) {
    tests := []struct {
        name          string
        caps          []*csi.VolumeCapability
        expectError   bool
        errorContains string
    }{
        {
            name: "RWX block - should succeed",
            caps: []*csi.VolumeCapability{
                {
                    AccessMode: &csi.VolumeCapability_AccessMode{
                        Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
                    },
                    AccessType: &csi.VolumeCapability_Block{
                        Block: &csi.VolumeCapability_BlockVolume{},
                    },
                },
            },
            expectError: false,
        },
        {
            name: "RWX filesystem - should fail with actionable error",
            caps: []*csi.VolumeCapability{
                {
                    AccessMode: &csi.VolumeCapability_AccessMode{
                        Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
                    },
                    AccessType: &csi.VolumeCapability_Mount{
                        Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
                    },
                },
            },
            expectError:   true,
            errorContains: "volumeMode: Block",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            err := cs.validateVolumeCapabilities(tc.caps)

            if tc.expectError {
                if err == nil {
                    t.Errorf("expected error but got nil")
                } else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
                    t.Errorf("expected error containing %q, got %q", tc.errorContains, err.Error())
                }
            } else {
                if err != nil {
                    t.Errorf("unexpected error: %v", err)
                }
            }
        })
    }
}
```

### Actionable Error Message Pattern
```go
// Source: pkg/driver/controller.go:945-955
// RWX block-only validation (ROADMAP-4)
// RWX with filesystem volumes risks data corruption - reject with actionable error
if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
    if cap.GetMount() != nil {
        return fmt.Errorf("RWX access mode requires volumeMode: Block. " +
            "Filesystem volumes risk data corruption with multi-node access. " +
            "For KubeVirt VM live migration, use volumeMode: Block in your PVC")
    }
    // Log valid RWX block usage for debugging/auditing
    klog.V(2).Info("RWX block volume capability validated (KubeVirt live migration use case)")
}
```

### Regression Test Pattern for Block vs Filesystem
```go
// Source: pkg/driver/node_test.go:755-819
// TestNodeStageVolume_FilesystemVolume_Unchanged tests that filesystem volumes still work
func TestNodeStageVolume_FilesystemVolume_Unchanged(t *testing.T) {
    // Setup for filesystem volume
    req := &csi.NodeStageVolumeRequest{
        VolumeCapability: createFilesystemVolumeCapability(), // Mount volume
        // ... other fields
    }

    _, err := ns.NodeStageVolume(context.Background(), req)
    if err != nil {
        t.Fatalf("NodeStageVolume failed: %v", err)
    }

    // Verify: Format WAS called for filesystem volumes
    if !mounter.formatCalled {
        t.Error("Format should be called for filesystem volumes")
    }

    // Verify: Mount WAS called for filesystem volumes
    if !mounter.mountCalled {
        t.Error("Mount should be called for filesystem volumes")
    }

    // Verify: Device metadata file was NOT created for filesystem volumes
    metadataPath := filepath.Join(stagingPath, "device")
    if _, err := os.Stat(metadataPath); err == nil {
        t.Error("device metadata file should not exist for filesystem volumes")
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate test per scenario | Table-driven tests with t.Run() | Go 1.7+ (2016) | Reduces duplication, clearer failure output |
| Generic error messages | Actionable 3-part errors (WHAT/WHY/HOW) | CSI community practice | Users can self-serve fixes instead of filing issues |
| Test only happy path | Include error cases in test tables | Go testing best practice | Error messages actually get tested |
| Manual spec compliance | csi-sanity test suite | CSI v1.0+ (2019) | Automated validation of protocol requirements |

**Deprecated/outdated:**
- **Asserting on mock call counts:** Modern Go tests verify behavior (state changes) rather than implementation details
- **Single-path testing:** Adding features without regression tests for existing paths is outdated; comprehensive coverage is standard

## Open Questions

None - this is a well-understood domain with established patterns.

## Sources

### Primary (HIGH confidence)
- [Go Wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests) - Official Go testing documentation
- [Kubernetes CSI Developer Documentation - Unit Testing](https://kubernetes-csi.github.io/docs/unit-testing.html) - Official CSI testing guide
- [Kubernetes CSI Developer Documentation - Functional Testing](https://kubernetes-csi.github.io/docs/functional-testing.html) - E2E testing guidance
- [csi-test repository](https://github.com/kubernetes-csi/csi-test) - Official CSI test frameworks
- Existing codebase: pkg/driver/node_test.go (block/filesystem regression tests), pkg/driver/controller_test.go (validation error testing)

### Secondary (MEDIUM confidence)
- [How to Write Table-Driven Tests in Go (2026)](https://oneuptime.com/blog/post/2026-01-07-go-table-driven-tests/view) - Current best practices
- [Table-Driven Tests in Go: A Practical Guide (2026)](https://medium.com/@mojimich2015/table-driven-tests-in-go-a-practical-guide-8135dcbc27ca) - Pattern guidance

### Tertiary (LOW confidence)
None - all findings verified with official sources or existing codebase

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official CSI test tools and Go stdlib testing well-documented
- Architecture: HIGH - Existing codebase already follows table-driven pattern correctly
- Pitfalls: HIGH - Based on direct code inspection and CSI community patterns

**Research date:** 2026-02-03
**Valid until:** 60 days (stable domain - Go testing patterns and CSI spec change slowly)
