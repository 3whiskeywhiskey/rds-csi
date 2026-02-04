# Coding Conventions

**Analysis Date:** 2026-02-04

## Naming Patterns

**Files:**
- Lowercase with underscores for utilities: `ssh_client.go`, `proc_mounts.go`
- Descriptive names matching primary type: `controller.go`, `node.go`, `driver.go`
- Test files follow Go convention: `*_test.go` (43 test files across codebase)
- Package organization reflects functionality: `pkg/driver/`, `pkg/nvme/`, `pkg/rds/`

**Functions:**
- Exported functions: PascalCase (e.g., `CreateVolume`, `NewConnector`, `GetVolume`)
- Unexported functions: camelCase (e.g., `validateCreateVolumeOptions`, `runCommandWithRetry`)
- Helper methods use consistent prefixes: `Track`, `Log`, `Get`, `Set`, `Is`
- Logger methods follow pattern: `Log{Entity}{Action}` (e.g., `LogVolumeCreate`, `LogSSHConnectionFailure`)

**Variables:**
- Interface variables: PascalCase (e.g., `Connector`, `Mounter`, `RDSClient`)
- Struct fields: PascalCase
- Local variables: camelCase (e.g., `volumeID`, `stagingPath`, `isBlockVolume`)
- Constants: UPPER_SNAKE_CASE (e.g., `defaultVolumeBasePath`)

**Types:**
- Interfaces: PascalCase with verb/noun structure (e.g., `Connector`, `Mounter`, `RDSClient`)
- Structs: PascalCase descriptive names (e.g., `ControllerServer`, `NodeServer`, `SecurityEvent`)
- Type definitions for enums: PascalCase (e.g., `EventType`, `EventSeverity`, `EventOutcome`)

## Code Style

**Formatting:**
- Standard Go fmt (enforced via `make fmt`)
- Uses goimports with local import path: `git.srvlab.io/whiskey/rds-csi-driver`
- Line length: standard Go (no hard limit enforced)
- Indentation: tabs (Go standard)

**Linting:**
- Tool: golangci-lint with 5-minute timeout
- Run: `make lint`
- No custom `.golangci.yml` file - uses default rules
- Coverage: run with `make test-coverage`

## Import Organization

**Order:**
1. Standard library imports (e.g., `"context"`, `"fmt"`, `"time"`)
2. Blank line
3. Third-party imports (e.g., `"github.com/..."`, `"k8s.io/..."`, `"google.golang.org/..."`)
4. Blank line
5. Local imports (e.g., `"git.srvlab.io/whiskey/rds-csi-driver/pkg/..."`)

**Path Aliases:**
- Local path: `git.srvlab.io/whiskey/rds-csi-driver/pkg/`
- Kubernetes imports: `metav1`, `corev1`, `"k8s.io/klog/v2"`
- Never use `"log"` package - always use `k8s.io/klog/v2`

**Example from `pkg/driver/controller.go`:**
```go
import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/security"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
)
```

## Error Handling

**Patterns:**
- CSI errors use `status.Error()` and `status.Errorf()` with gRPC codes: `codes.InvalidArgument`, `codes.Internal`, `codes.ResourceExhausted`
- Internal errors use `fmt.Errorf()` with verb `%w` for error wrapping
- Error wrapping adds context: `fmt.Errorf("failed to create volume: %w", err)`
- All error paths must include context for troubleshooting

**CSI Service Layer (driver/controller.go, driver/node.go):**
```go
if req.GetName() == "" {
	return nil, status.Error(codes.InvalidArgument, "volume name is required")
}
if err := someOperation(); err != nil {
	return nil, status.Errorf(codes.Internal, "failed to operation: %v", err)
}
```

**Internal Packages (rds/commands.go, nvme/nvme.go):**
```go
if err := validateSlotName(slot); err != nil {
	return err  // Already wrapped
}
if newSizeBytes <= 0 {
	return fmt.Errorf("new size must be positive")
}
return fmt.Errorf("failed to get volume info: %w", err)
```

## Logging

**Framework:** Kubernetes klog/v2 (468 total logging calls across codebase)

**Verbosity Levels in Use:**
- `V(0)`: Errors (klog.Errorf, klog.Error)
- `V(1)`: Warnings (klog.Warningf, klog.Warning)
- `V(2)`: Key operations (CreateVolume entry/exit, DeleteVolume, NodeStageVolume) - 54 occurrences
- `V(3)`: Intermediate steps (volume exists checks, file cleanup) - DeleteVolume logs multiple V(3) steps
- `V(4)`: Detailed queries (ValidateVolumeCapabilities, GetCapacity)
- `V(5)`: Framework calls (ControllerGetCapabilities)

**Current Distribution:**
- klog.V(2): 54 occurrences (primary operation logging)
- klog.V(3): Intermediate checks
- klog.V(4): Capability validation, capacity queries
- klog.V(5): Framework discovery
- klog.Errorf/Warningf: 100+ error/warning paths

**Patterns in Controller (`pkg/driver/controller.go`):**
```go
klog.V(2).Infof("CreateVolume called with name: %s", req.GetName())
klog.V(2).Infof("Using volume ID: %s (from volume name: %s)", volumeID, req.GetName())
klog.V(3).Infof("Volume %s not found on RDS, assuming already deleted", volumeID)
klog.V(3).Infof("Deleting volume %s (path=%s, size=%d bytes, nvme_export=%v)", volumeID, filePath, size, export)
klog.Errorf("Failed to delete volume %s: %v", volumeID, err)
```

**Patterns in Node (`pkg/driver/node.go`):**
```go
klog.V(2).Infof("NodeStageVolume called for volume: %s, staging path: %s", volumeID, stagingPath)
klog.V(3).Infof("Volume %s already connected, fetching device path", volumeID)
klog.Warningf("Failed to format device: %v", err)
```

**Patterns in RDS Commands (`pkg/rds/commands.go`):**
```go
klog.V(2).Infof("Creating volume %s (size: %d bytes, path: %s)", opts.Slot, opts.FileSizeBytes, opts.FilePath)
klog.V(2).Infof("Successfully created volume %s", opts.Slot)
klog.V(3).Infof("Volume %s not found, skipping deletion", slot)
klog.Warningf("Failed to delete backing file: %v", err)
```

**Security Logging (`pkg/security/logger.go`):**
- Centralized security event logging separate from operational logs
- Maps severity (Info/Warning/Error/Critical) to klog verbosity
- Structured fields: username, source_ip, volume_id, node_id, operation, duration
- Critical events also logged as JSON: `"CRITICAL_SECURITY_EVENT: {...}"`
- Provides 20+ helper methods for common events (LogVolumeCreate, LogSSHConnectionFailure, etc.)

**Guidelines for v0.7.1:**
- Log operation entry with inputs at V(2)
- Log operation completion at V(2)
- Log intermediate steps at V(3) ONLY for troubleshooting, not on every path
- Log errors at klog.Errorf() level (always visible)
- Use Warningf for recoverable issues
- Avoid logging sensitive data (passwords, private keys)
- Include context: volumeID, nodeID, operation, duration

## Comments

**When to Comment:**
- Package-level documentation: every public package has a comment explaining purpose
- Exported functions/types: brief description of behavior
- Complex logic: explain non-obvious decisions or algorithms
- Workarounds/hacks: explain why workaround is needed and plan to fix
- Business logic: explain CSI spec requirements or design decisions

**Example from `pkg/attachment/manager.go`:**
```go
// TrackAttachment records that a volume is attached to a node.
// This method is idempotent - if the volume is already attached to the same node,
// it returns nil. If the volume is attached to a different node, it returns an error.
// For RWX dual-attach, use TrackAttachmentWithMode or AddSecondaryAttachment instead.
func (am *AttachmentManager) TrackAttachment(ctx context.Context, volumeID, nodeID string) error {
```

**Go Doc Pattern:**
```go
// TypeName represents purpose
// Additional context if needed.
type TypeName struct {
	// Field explains purpose
	Field string
}

// MethodName does something
// Multi-line comments explain complex behavior.
func (t *TypeName) MethodName() error {
```

## Function Design

**Size:**
- Target: 20-60 lines
- Larger functions (100-200+ lines): ControllerPublishVolume, ControllerUnpublishVolume, NodeStageVolume
- Large functions use clear section comments separating concerns

**Parameters:**
- Context as first parameter (required for timeout/cancellation)
- Group related parameters together
- Use structs for >3 parameters (e.g., `CreateVolumeOptions` struct)
- Avoid pass-by-value for large structs

**Return Values:**
- CSI methods: `(response, error)`
- Single-value methods: `(value, error)`
- Helper functions: `(data, error)`
- Never return nil response when error is present

**Error Propagation:**
- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Each layer adds information rather than hiding original error
- Avoid re-throwing without context

## Module Design

**Exports:**
- Exported functions document public API clearly
- Unexported helpers keep implementation private
- Interfaces exported for dependency injection (Connector, Mounter, RDSClient)
- Configuration constants exported (DefaultOrphanCheckInterval)

**Barrel Files:**
- Not used - no index.go files
- Direct imports from specific packages

**Package Structure (9 packages in pkg/):**
- `pkg/driver/`: CSI services (Identity, Controller, Node) - 830+ lines
- `pkg/rds/`: RDS/RouterOS SSH client and command parsing - 600+ lines
- `pkg/nvme/`: NVMe/TCP connector, device resolution - 500+ lines
- `pkg/mount/`: Filesystem operations, mount recovery - 600+ lines
- `pkg/attachment/`: Volume-to-node attachment state tracking - 400+ lines
- `pkg/reconciler/`: Orphan volume detection and cleanup - 300+ lines
- `pkg/utils/`: Validation, retry, error handling, volume ID generation - 300+ lines
- `pkg/security/`: Security event logging and metrics - 600+ lines
- `pkg/observability/`: Prometheus metrics collection - 200+ lines

---

*Convention analysis: 2026-02-04*
