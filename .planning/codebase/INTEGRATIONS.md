# External Integrations

**Analysis Date:** 2026-01-30

## APIs & External Services

**RouterOS CLI (SSH):**
- MikroTik ROSE Data Server (RDS) - NVMe/TCP volume management
  - SDK/Client: Custom SSH wrapper in `pkg/rds/ssh_client.go`
  - Protocol: SSH (TCP port 22 by default)
  - Commands: Disk create/delete via `/disk` RouterOS CLI commands in `pkg/rds/commands.go`
  - Auth: Ed25519 SSH private key stored in Kubernetes Secret
  - Example: `/disk add type=file file-path=/storage-pool/kubernetes-volumes/pvc-<uuid>.img file-size=<size> slot=pvc-<uuid> nvme-tcp-export=yes nvme-tcp-server-port=4420`

**Kubernetes API Server:**
- Access method: In-cluster service account or kubeconfig file
  - Client: `k8s.io/client-go/kubernetes` in `cmd/rds-csi-plugin/main.go`
  - Resources accessed: PersistentVolumes, PersistentVolumeClaims, Nodes, Events
  - Purpose: Orphan volume reconciliation (optional, enabled via `--enable-orphan-reconciler` flag)
  - Auth: Service account token mounted at `/var/run/secrets/kubernetes.io/serviceaccount`

**NVMe/TCP (Data Plane):**
- Target: RDS NVMe/TCP export
  - Client: System `nvme-cli` via exec (no Go SDK)
  - Protocol: NVMe/TCP (port 4420)
  - Commands: `nvme connect -t tcp`, `nvme disconnect` executed in `pkg/nvme/nvme.go`
  - Device path: `/dev/nvmeXnY` after successful connection
  - NQN format: `nqn.2000-02.com.mikrotik:pvc-<uuid>`

## Data Storage

**Databases:**
- None - driver is stateless (state stored in Kubernetes PVs and RDS)

**File Storage:**
- MikroTik RDS (network block storage) - File-backed disk images via NVMe/TCP
  - Storage format: `.img` files on Btrfs RAID6
  - Base path: `/storage-pool/kubernetes-volumes/` (configurable via `--rds-volume-base-path`)
  - File naming: `pvc-<uuid>.img`
  - Access: SSH commands to create/delete files on RDS

**Caching:**
- None - operations are direct

## Authentication & Identity

**Auth Provider:**
- Custom SSH key-based authentication
  - Implementation: Standard SSH public/private key authentication (Ed25519 format)
  - Private key location: `/etc/rds-csi/ssh-key/id_rsa` (mounted from Kubernetes Secret)
  - Host key verification: SSH host public key in `/etc/rds-csi/ssh-key/rds-host-key` (mounted from Kubernetes Secret)
  - Security: Enforced host key verification; can skip only with `--rds-insecure-skip-verify` flag for testing
  - Code: `pkg/rds/ssh_client.go` lines 103-140 (SSH client config), 290-340 (host key verification)

**Kubernetes RBAC:**
- Service Account: `rds-csi-controller` for controller pod, `rds-csi-node` for node DaemonSet
- Roles/ClusterRoles defined in `deploy/kubernetes/rbac.yaml`
- Permissions: PV, PVC, Node, StorageClass, Event access

## Monitoring & Observability

**Error Tracking:**
- None - errors logged to klog (Kubernetes logging system)

**Logs:**
- Standard output/stderr captured by container runtime
- Structured logging via `k8s.io/klog/v2` throughout driver
- Log levels: Verbosity controlled via `-v` flag (default 0, can increase to `-v=4` for detailed debugging)
- Security events logged to security logger in `pkg/security/logger.go` (SSH connection attempts, host key verification)

**Metrics:**
- Basic operation metrics collected in `pkg/security/metrics.go`
- No Prometheus integration; metrics available via code inspection or logs

## CI/CD & Deployment

**Hosting:**
- Container registry: `ghcr.io/3whiskeywhiskey/rds-csi-driver`
- Deployment target: Kubernetes 1.26+ cluster
- Controller: Single replica Deployment (prefer control-plane affinity)
- Node plugin: DaemonSet on all worker nodes

**CI Pipeline:**
- None detected in current state (no GitHub Actions, no CI config files found)
- Manual build via `make build`, `make docker`, `make test`
- Build targets available in Makefile lines 77-180

## Environment Configuration

**Required env vars for controller mode:**
- `RDS_ADDRESS` - RDS server IP (no default, required)
- `RDS_VOLUME_BASE_PATH` - Base path on RDS for volume storage (required for orphan detection)

**Optional env vars:**
- `RDS_PORT` - SSH port (default: 22)
- `RDS_USER` - SSH username (default: admin)
- `NODE_ID` - Node identifier (required for node mode)
- `CSI_ENDPOINT` - CSI socket path (default: `unix:///var/lib/kubelet/plugins/rds.csi.srvlab.io/csi.sock`)

**Secrets location:**
- Kubernetes Secret `rds-csi-secret` with keys:
  - `rds-private-key` - SSH private key content
  - `rds-host-key` - SSH host public key content
- Mounted to controller at `/etc/rds-csi/ssh-key/` (mode 0600)

**ConfigMap location:**
- Kubernetes ConfigMap `rds-csi-config` with keys:
  - `rds-address` - RDS IP
  - `rds-port` - SSH port
  - `rds-user` - SSH username
  - `rds-volume-base-path` - Base path for volumes

## Webhooks & Callbacks

**Incoming:**
- None - driver is not a webhook server

**Outgoing:**
- Kubernetes events: Driver writes events to PVC objects for user visibility (status updates, errors)
  - Implementation: Uses Kubernetes event recorder (standard Kubernetes pattern)
  - No external webhook calls

## Network Requirements

**Control Plane (SSH):**
- RDS SSH management interface: TCP port 22
- Should be on separate VLAN from data plane (security best practice)

**Data Plane (NVMe/TCP):**
- RDS NVMe/TCP export: TCP port 4420 (default, configurable on RDS)
- High throughput, low latency requirement (~1ms target)
- Should be on dedicated network for performance

## Storage Class Parameters

**Supported parameters:**
- None - driver accepts generic CSI parameters only
- Volume IDs format: `pvc-<uuid>`
- Volume access mode: ReadWriteOnce only (enforced in `pkg/driver/controller.go`)

---

*Integration audit: 2026-01-30*
