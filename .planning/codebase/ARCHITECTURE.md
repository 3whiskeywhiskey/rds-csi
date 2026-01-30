# Architecture

**Analysis Date:** 2026-01-30

## Pattern Overview

**Overall:** Microservices pattern with separation of control plane and data plane

The RDS CSI driver implements a standard Kubernetes Container Storage Interface (CSI) pattern with two independent services: a centralized controller and distributed node plugins. The controller handles volume lifecycle management via SSH to RouterOS CLI, while node plugins handle NVMe/TCP connectivity and filesystem mounting.

**Key Characteristics:**
- **Layered service architecture** with clear separation between CSI API, business logic, and infrastructure clients
- **Interface-driven design** for testability and extensibility (RDSClient, Connector, Mounter interfaces)
- **Security-first approach** with input validation, SSH host key verification, and comprehensive logging
- **Asynchronous background reconciliation** for orphaned volume detection and cleanup

## Layers

**Presentation Layer (gRPC):**
- Purpose: Expose CSI-compliant gRPC services to Kubernetes kubelet and external-provisioner
- Location: `pkg/driver/server.go`, `pkg/driver/identity.go`, `pkg/driver/controller.go`, `pkg/driver/node.go`
- Contains: Service implementations that receive CSI requests and return responses
- Depends on: Business logic layer, security logging
- Used by: Kubernetes kubelet (NodeStageVolume, NodePublishVolume), external-provisioner (CreateVolume, DeleteVolume)

**Business Logic Layer:**
- Purpose: Implement CSI operations with validation, error handling, and orchestration
- Location: `pkg/driver/controller.go` (CreateVolume, DeleteVolume, GetCapacity, ValidateVolumeCapabilities, ControllerGetCapabilities), `pkg/driver/node.go` (NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume)
- Contains: Volume provisioning logic, capacity calculations, validation rules
- Depends on: RDS client, NVMe connector, mount handler, validation utilities, security logging
- Used by: Presentation layer (gRPC services)

**Infrastructure Layer:**
- **RDS Client** (`pkg/rds/client.go`, `pkg/rds/ssh_client.go`, `pkg/rds/commands.go`): Communicates with RouterOS via SSH, executes `/disk add/remove/print` commands, manages connection pooling
- **NVMe Connector** (`pkg/nvme/nvme.go`): Wraps `nvme-cli` binary, handles NVMe/TCP connections, device discovery with retry logic and timeout handling
- **Mount Handler** (`pkg/mount/mount.go`): Wraps `mount` and `mkfs` commands, enforces secure mount options (nosuid, nodev, noexec)
- Location: `pkg/rds/`, `pkg/nvme/`, `pkg/mount/`
- Depends on: External commands (SSH client, nvme-cli, mount/mkfs), validation utilities, logging
- Used by: Business logic layer

**Support Layers:**
- **Utilities** (`pkg/utils/`): Volume ID generation/validation, regex parsing for RouterOS output, port/IP validation, error classification
- **Security** (`pkg/security/`): Centralized event logging, metrics collection, structured logging for audit trails
- **Reconciler** (`pkg/reconciler/orphan_reconciler.go`): Background task that detects orphaned volumes (exist on RDS but not in Kubernetes) and cleans them up with grace period and dry-run support

## Data Flow

**Volume Create Flow (Controller):**

1. External-provisioner calls `CreateVolume(req)` gRPC
2. ControllerServer validates request (name, capabilities, size bounds: 1GB-16TB)
3. Generate volumeID from PVC name (already unique from external-provisioner)
4. Validate volumeID format: `^pvc-[a-f0-9-]+$` to prevent command injection
5. Check if volume already exists via RDSClient.GetVolume (idempotency)
6. If exists, return existing volume with metadata
7. If not exists:
   - Generate NQN: `nqn.2000-02.com.mikrotik:<volumeID>`
   - Generate file path: `<basePath>/<volumeID>.img`
   - Log security event via security.GetLogger().LogVolumeCreate()
   - Call RDSClient.CreateVolume(CreateVolumeOptions) via SSH
   - SSH command: `/disk add type=file file-path=... file-size=... slot=... nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn=...`
   - Verify creation via RDSClient.VerifyVolumeExists
8. Return CSI CreateVolumeResponse with volumeContext containing NQN, address, port, fsType

**Volume Stage Flow (Node):**

1. Kubelet calls `NodeStageVolume(req)` gRPC
2. NodeServer validates request and extracts volumeContext (NQN, address, port)
3. Validate inputs: NQN format, IP address format, port range (1-65535)
4. Log security event via security.GetLogger().LogVolumeStage()
5. Create NVMe target object with extracted context
6. Call NVMe Connector.Connect(target) - executes `nvme connect -t tcp -a <ip> -s <port> -n <nqn>`
7. Connector polls for device appearance at `/dev/nvmeXnY` with timeout (default 30s)
8. Call Mounter.Format(devicePath, fsType) - executes `mkfs.<fsType> <device>`
9. Call Mounter.Mount(devicePath, stagingPath, fsType, mountOptions) - applies secure mount options (nosuid, nodev, noexec)
10. On any failure: disconnect NVMe target, log error, return gRPC error
11. Return CSI NodeStageVolumeResponse

**Volume Cleanup Flow (Orphan Reconciler):**

1. Reconciler runs on configurable interval (default 1 hour)
2. List all volumes from RDSClient.ListVolumes() (SSH: `/disk print detail`)
3. List all PersistentVolumes from Kubernetes API
4. For each volume on RDS with prefix `pvc-`:
   - Check if corresponding PV exists in Kubernetes
   - Check if volume age > grace period (default 5 minutes)
   - If orphaned: log detection, then either dry-run log or execute RDSClient.DeleteVolume() and RDSClient.DeleteFile()
5. SSH delete commands:
   - `/disk remove [find slot=<volumeID>]`
   - `/file remove <filePath>`

**State Management:**

- **Volume State Progression:** Non-existent → Creating (SSH in progress) → Available (exists on RDS, NVMe exported)
- **Device State Progression:** Not connected → Connecting (nvme-cli running) → Staged (mounted at staging path)
- **Idempotency:** CreateVolume checks for existing volume before creation; NodeStageVolume checks if already mounted
- **Error State Handling:** All error paths either fail the operation or roll back (e.g., disconnect NVMe if mount fails)

## Key Abstractions

**RDSClient Interface** (`pkg/rds/client.go`):
- Purpose: Abstract SSH implementation, allow future API protocol support, enable testing with mocks
- Examples: `pkg/rds/ssh_client.go` (production), `test/mock/rds_server.go` (testing)
- Pattern: Factory pattern via `NewClient(config)` that routes to SSH implementation
- Methods: Connect, Close, IsConnected, CreateVolume, DeleteVolume, ResizeVolume, GetVolume, ListVolumes, GetCapacity

**Connector Interface** (`pkg/nvme/nvme.go`):
- Purpose: Abstract nvme-cli invocation, handle retries and timeouts
- Examples: `pkg/nvme/nvme.go` (production implementation with exec.Command wrapping)
- Pattern: Wrapper around system `nvme-cli` binary with exponential backoff retry (3 attempts, 5s between)
- Methods: Connect, ConnectWithContext, Disconnect, IsConnected, GetDevicePath, WaitForDevice, GetMetrics

**Mounter Interface** (`pkg/mount/mount.go`):
- Purpose: Abstract filesystem operations, enforce secure mount options
- Examples: `pkg/mount/mount.go` (production with system mount/mkfs commands)
- Pattern: Command wrapper with validation of mount options against whitelist
- Methods: Mount, Unmount, Format, IsMounted
- Security: Enforces default options (nosuid, nodev, noexec), rejects dangerous options (suid, dev, exec)

## Entry Points

**Driver Initialization:**
- Location: `cmd/rds-csi-plugin/main.go`
- Triggers: Container startup with flags
- Responsibilities: Parse flags, load SSH keys, create Kubernetes client, instantiate Driver instance, start gRPC server

**Driver Main Struct:**
- Location: `pkg/driver/driver.go`
- Purpose: Coordinates lifecycle of all CSI services
- Creates: RDSClient, Identity/Controller/Node services, OrphanReconciler
- Entry methods: `NewDriver(config)`, `Run(endpoint)`, `Stop()`

**gRPC Server:**
- Location: `pkg/driver/server.go` (NonBlockingGRPCServer)
- Purpose: Listen on endpoint (unix socket or TCP), register service handlers, start serving
- Endpoint format: `unix:///var/lib/kubelet/plugins/rds.csi.srvlab.io/csi.sock` (default)

**CSI Service Entry Points:**
- **Identity Service** (`pkg/driver/identity.go`): GetPluginInfo, GetPluginCapabilities, Probe
- **Controller Service** (`pkg/driver/controller.go`): CreateVolume, DeleteVolume, ValidateVolumeCapabilities, GetCapacity, ControllerGetCapabilities, ControllerPublishVolume
- **Node Service** (`pkg/driver/node.go`): NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo

## Error Handling

**Strategy:** Classification-based error handling with CSI gRPC status codes

**Patterns:**

1. **Input Validation Errors** → `codes.InvalidArgument`:
   - Missing required fields (volumeName, volumeID, stagingPath)
   - Invalid format (volumeID fails regex, port out of range, IP address invalid)
   - Unsupported capabilities (access mode not SINGLE_NODE_*)
   - File: `pkg/utils/validation.go` performs ValidateVolumeID, ValidateIPAddress, ValidatePortString, ValidateFilePath

2. **Resource Exhaustion** → `codes.ResourceExhausted`:
   - RDS disk full detected from SSH output "not enough space"
   - Size exceeds limits (min 1GB, max 16TB)
   - File: `pkg/driver/controller.go` checks error message for capacity keywords

3. **Not Found Errors** → `codes.NotFound`:
   - Volume doesn't exist when deleting
   - Device path cannot be found
   - File: `pkg/rds/commands.go` returns specific error types

4. **Already Exists** → `codes.AlreadyExists`:
   - Volume already created (idempotency check)
   - File: `pkg/driver/controller.go` checks `GetVolume` result

5. **Internal Errors** → `codes.Internal`:
   - SSH connection failure
   - NVMe connection failure
   - Mount/format failures
   - Regex parsing failures
   - File: All infrastructure layer errors default to Internal

6. **Unavailable** → `codes.Unavailable`:
   - Driver health check (Probe) fails because RDS client not connected
   - File: `pkg/driver/identity.go` Probe method

**Error Classification** (`pkg/utils/errors.go`):
- Maps RouterOS output patterns to CSI error codes
- Detects "not enough space" → ResourceExhausted
- Detects "not found" patterns → NotFound
- Default: Internal for unknown errors

## Cross-Cutting Concerns

**Logging:**
- Framework: `k8s.io/klog/v2` (Kubernetes standard logging)
- Levels: V(5) for trace (detailed gRPC calls), V(2) for operation flow, V(1) for warnings, V(0) for errors
- Pattern: Every major step logs with volumeID and context (e.g., "NodeStageVolume called for volume: pvc-abc, staging path: /var/lib/kubelet/pods/xyz")
- File: All files use `klog.Infof`, `klog.V(N).Infof`, `klog.Warning`, `klog.Error`

**Validation:**
- All user inputs and external data validated before use
- Volume IDs: Must match `^pvc-[a-f0-9-]+$` (command injection prevention)
- File paths: Must not contain shell metacharacters, must be within allowed base paths
- IP addresses: Standard dotted-quad format with range checking
- Ports: Must be 1-65535
- File: `pkg/utils/validation.go` exports ValidateVolumeID, ValidateIPAddress, ValidatePortString, ValidateFilePath, ValidateNVMETargetContext

**Authentication & Security:**
- SSH: Private key stored in Kubernetes Secret, mounted as `/etc/rds-csi/ssh-key/id_rsa` with 0600 permissions
- Host key verification: Enforced in production, optional with --rds-insecure-skip-verify for testing
- Mount options: Whitelist validation, dangerous options (suid, dev, exec) always blocked
- Command injection: All inputs validated, no shell injection possible
- File: `pkg/rds/ssh_client.go` handles SSH config, `pkg/mount/mount.go` validates options

**Security Auditing:**
- Framework: `pkg/security/logger.go` (centralized event logging), `pkg/security/metrics.go` (operation metrics)
- Events logged: VolumeCreate (start/success/failure with duration), VolumeStage (start/success/failure), VolumeUnstage, VolumeDelete
- Outcome types: Unknown (start), Success, Failure (with error details)
- Metrics tracked: Operation count/duration, failure count/reasons
- File: All CSI service methods call `security.GetLogger().LogVolume*()` at start and end

---

*Architecture analysis: 2026-01-30*
