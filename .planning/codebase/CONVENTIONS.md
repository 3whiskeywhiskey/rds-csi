# Coding Conventions

**Analysis Date:** 2026-01-30

## Naming Patterns

**Files:**
- Package files: lowercase with underscores for multiple words (e.g., `ssh_client.go`, `controller_test.go`)
- Test files: `*_test.go` suffix (e.g., `identity_test.go`, `commands_test.go`)
- Main entry point: `cmd/rds-csi-plugin/main.go`

**Functions:**
- Public functions: PascalCase (e.g., `NewIdentityServer()`, `GetPluginInfo()`, `CreateVolume()`)
- Private functions: camelCase (e.g., `newSSHClient()`, `parseVolumeInfo()`, `sanitizePaths()`)
- Interface methods: PascalCase following receiver convention (e.g., `(cs *ControllerServer) CreateVolume()`)
- Test functions: `Test` prefix with PascalCase (e.g., `TestValidateVolumeCapabilities()`, `TestParseVolumeInfo()`)

**Variables:**
- Package-level private: camelCase (e.g., `globalLogger`, `loggerOnce`, `defaultFSType`)
- Constants (package-level): ALL_CAPS with underscores for multi-word (e.g., `DriverName`, `defaultVersion`, `minVolumeSizeBytes`, `volumeContextNQN`)
- Local variables: camelCase (e.g., `volumeID`, `stagingPath`, `requiredBytes`)
- Error returns: often short variable names `err` for last return value

**Types:**
- Structs: PascalCase (e.g., `ControllerServer`, `Driver`, `SanitizedError`, `SecurityEvent`)
- Interfaces: PascalCase ending with "er" for behavioral contracts (e.g., `RDSClient`, `Connector`, `Mounter`)
- Type aliases for enums: PascalCase (e.g., `ErrorType`, `EventCategory`)
- Embedded fields: unnamed embedding with interface types (e.g., `csi.UnimplementedIdentityServer`)

## Code Style

**Formatting:**
- Uses `go fmt` for automatic formatting
- Goimports with local module path: `git.srvlab.io/whiskey/rds-csi-driver`
- Line length: follows Go conventions (no hard limit enforced, but readable)
- Indentation: tabs (Go standard)

**Linting:**
- Uses `golangci-lint` with 5-minute timeout for CI
- Code must pass: `fmt`, `vet`, and `lint` before test
- Called via: `make verify` which runs fmt, vet, lint, and test sequentially
- Individual targets: `make fmt`, `make vet`, `make lint`

## Import Organization

**Order:**
1. Standard library imports (e.g., `context`, `fmt`, `time`)
2. Third-party public imports (e.g., `github.com/...`, `k8s.io/...`, `google.golang.org/...`)
3. Local project imports (e.g., `git.srvlab.io/whiskey/rds-csi-driver/pkg/...`)

**Path Aliases:**
- Local imports: `git.srvlab.io/whiskey/rds-csi-driver/pkg/driver`
- Kubernetes: `k8s.io/klog/v2` for logging (never `log` package)
- CSI: `github.com/container-storage-interface/spec/lib/go/csi`
- gRPC: `google.golang.org/grpc` with `codes` and `status` subpackages

**Example from `pkg/driver/controller.go`:**
```go
import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/security"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
)
```

## Error Handling

**Patterns:**
- Use `fmt.Errorf()` for wrapping errors with context: `fmt.Errorf("operation failed: %w", err)`
- CSI gRPC errors: use `status.Error(codes.Code, "message")` for returning to kubelet
- Sanitized errors: use `utils.SanitizedError` for sensitive information removal
- Error classification: `ErrorTypeInternal`, `ErrorTypeUser`, `ErrorTypeValidation`

**Error Wrapping (from `pkg/utils/errors.go`):**
- `NewInternalError(err, userMsg)` - Internal errors hidden from users, logged fully
- `NewUserError(err, operation)` - User-facing errors with sanitization
- `NewValidationError(field, reason)` - Validation errors (safe to show)
- `WrapError(err, format, args...)` - Wraps with context, sanitizes message
- `SanitizeErrorMessage(msg)` - Removes IPs, paths, hostnames, stack traces

**CSI Status Codes (from `pkg/driver/controller.go`):**
- `codes.InvalidArgument` - Validation failures (missing fields, bad format)
- `codes.OutOfRange` - Size constraints violated (too large/small)
- `codes.Unavailable` - Service not ready (RDS not connected)
- `codes.Internal` - Unexpected errors (wrapped as status.Error)

**Example from `pkg/driver/identity.go`:**
```go
if ids.driver.name == "" {
	return nil, status.Error(codes.Unavailable, "driver name not configured")
}
```

## Logging

**Framework:** `k8s.io/klog/v2` (Kubernetes standard)

**Patterns:**
- Verbosity levels: `V(0)=Error`, `V(1)=Warning`, `V(2)=Info`, `V(4)=Debug`, `V(5)=Trace`
- Always use structured logging with `.Infof()`, `.Warningf()`, `.Errorf()` methods
- Log entry point calls: `V(2).Infof("MethodName called with param: %s", value)`
- Log success/completion: `V(2).Infof("MethodName completed successfully")`
- Security events: use `pkg/security/logger.go` for structured security logging

**Example from `pkg/driver/controller.go`:**
```go
klog.V(2).Infof("CreateVolume called with name: %s", req.GetName())
```

**Security Logging (from `pkg/security/logger.go`):**
- Use `security.GetLogger().LogEvent(event)` for access control, authentication, volume operations
- Event types: `EventSSHConnectionAttempt`, `EventVolumeCreateRequest`, `EventVolumeDeleteRequest`
- Severities: `SeverityInfo`, `SeverityWarning`, `SeverityError`
- Categories: `CategoryAuthentication`, `CategoryVolumeOperation`, `CategorySecurityValidation`

## Comments

**When to Comment:**
- All public functions and types must have doc comments starting with the name
- Explain "why", not "what" (the code shows what)
- Complex algorithms or non-obvious logic
- Tricky workarounds or temporary fixes

**JSDoc/Go Doc Pattern:**
```go
// ControllerServer implements the CSI Controller service
type ControllerServer struct {
	// driver holds reference to parent Driver instance
	driver *Driver
}

// NewControllerServer creates a new Controller service
func NewControllerServer(driver *Driver) *ControllerServer {
	return &ControllerServer{
		driver: driver,
	}
}

// CreateVolume provisions a new volume on RDS
// This involves:
// 1. Validating the request
// 2. Checking if volume already exists (idempotency)
// 3. Creating the file-backed disk via SSH
// 4. Waiting for NVMe/TCP export to be ready
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
```

**Constants and "why" comments:**
```go
const (
	// Default values for storage parameters
	defaultVolumeBasePath = "/storage-pool/metal-csi"
	defaultNVMETCPPort    = 4420

	// Minimum/maximum volume sizes
	minVolumeSizeBytes = 1 * 1024 * 1024 * 1024         // 1 GiB
	maxVolumeSizeBytes = 16 * 1024 * 1024 * 1024 * 1024 // 16 TiB
)
```

## Function Design

**Size:** Keep functions under 50 lines when possible. Complex operations (like CreateVolume) are broken into helper functions.

**Parameters:**
- Use context as first parameter in all async/network operations: `func (...) methodName(ctx context.Context, ...)`
- Receiver types as pointers for methods that modify state: `func (cs *ControllerServer) Method()`
- Use structs for multiple related parameters instead of long argument lists (e.g., `CreateVolumeOptions`, `ClientConfig`)

**Return Values:**
- Error as last return value: `func Foo(x int) (string, error)`
- Use pointer receivers to return data and error: `(*VolumeInfo, error)`
- Boolean success indicator not used; use error return instead

**Example from `pkg/rds/client.go`:**
```go
// RDSClient interface defines clean contract
type RDSClient interface {
	Connect() error
	CreateVolume(opts CreateVolumeOptions) error
	DeleteVolume(slot string) error
	GetVolume(slot string) (*VolumeInfo, error)
	ListVolumes() ([]VolumeInfo, error)
	GetCapacity(basePath string) (*CapacityInfo, error)
}

// NewClient uses configuration struct
func NewClient(config ClientConfig) (RDSClient, error) {
	// Route to appropriate implementation
	switch config.Protocol {
	case "ssh":
		return newSSHClient(config)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", config.Protocol)
	}
}
```

## Module Design

**Exports:**
- All public types/functions exported: PascalCase
- All private types/functions unexported: camelCase
- Constructor functions: `New<TypeName>()` returns interface when possible (e.g., `NewClient()` returns `RDSClient`)

**Barrel Files:** Not used. Import specific package paths directly.

**Package Responsibilities:**
- `pkg/driver/` - CSI gRPC services (Identity, Controller, Node)
- `pkg/rds/` - RDS/RouterOS SSH client and command parsing
- `pkg/nvme/` - NVMe/TCP connector for block device operations
- `pkg/mount/` - Filesystem mounting operations
- `pkg/security/` - Security logging, metrics, event tracking
- `pkg/utils/` - Common utilities (error sanitization, validation, volume ID generation)
- `pkg/reconciler/` - Orphan volume detection and cleanup

**Example from `pkg/driver/driver.go`:**
```go
// Driver is the main CSI driver instance
type Driver struct {
	// Public exports via GetPluginInfo()
	name    string
	version string

	// CSI service implementations
	ids csi.IdentityServer
	cs  csi.ControllerServer
	ns  csi.NodeServer

	// RDS backend
	rdsClient rds.RDSClient
	reconciler *reconciler.OrphanReconciler
}

// NewDriver is the public constructor
func NewDriver(config DriverConfig) (*Driver, error) {
	// Initialization logic
}
```

---

*Convention analysis: 2026-01-30*
