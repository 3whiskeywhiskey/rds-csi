package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/attachment"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/observability"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/reconciler"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
)

const (
	// DriverName is the official name of this CSI driver
	DriverName = "rds.csi.srvlab.io"

	// DriverVersion is the version of the driver
	// These will be set via ldflags during build
	defaultVersion = "dev"
)

var (
	version   = defaultVersion
	gitCommit = "unknown"
	buildDate = "unknown"
)

// Driver implements the CSI Controller, Node, and Identity services
type Driver struct {
	name    string
	version string
	nodeID  string

	// CSI services
	ids csi.IdentityServer
	cs  csi.ControllerServer
	ns  csi.NodeServer

	// RDS client (interface allows different implementations: SSH, API, mock)
	rdsClient rds.RDSClient

	// Kubernetes client (for events and reconciler)
	k8sClient kubernetes.Interface

	// Prometheus metrics (may be nil if disabled)
	metrics *observability.Metrics

	// Orphan reconciler (optional)
	reconciler *reconciler.OrphanReconciler

	// Attachment manager (for controller only)
	attachmentManager *attachment.AttachmentManager

	// Attachment reconciler (for controller only)
	attachmentReconciler *attachment.AttachmentReconciler

	// Grace period for attachment handoff during live migration
	attachmentGracePeriod time.Duration

	// VMI grouper for per-VMI operation serialization
	vmiGrouper *VMIGrouper

	// Capabilities
	vcaps  []*csi.VolumeCapability_AccessMode
	cscaps []*csi.ControllerServiceCapability
	nscaps []*csi.NodeServiceCapability
}

// DriverConfig contains configuration for creating a driver instance
type DriverConfig struct {
	DriverName string
	NodeID     string
	Version    string

	// RDS connection settings
	RDSAddress            string
	RDSPort               int
	RDSUser               string
	RDSPrivateKey         []byte
	RDSHostKey            []byte // SSH host public key for verification
	RDSInsecureSkipVerify bool   // Skip host key verification (INSECURE)
	RDSVolumeBasePath     string // Base path for volumes on RDS (e.g., /storage-pool/metal-csi)

	// Kubernetes client (required for orphan reconciler)
	K8sClient kubernetes.Interface

	// Prometheus metrics (optional, nil to disable)
	Metrics *observability.Metrics

	// Orphan reconciler settings
	EnableOrphanReconciler bool
	OrphanCheckInterval    time.Duration
	OrphanGracePeriod      time.Duration
	OrphanDryRun           bool

	// Attachment reconciler settings
	EnableAttachmentReconciler  bool
	AttachmentReconcileInterval time.Duration // Default: 5 minutes
	AttachmentGracePeriod       time.Duration // Default: 30 seconds

	// VMI serialization settings (for kubevirt concurrent operation mitigation)
	EnableVMISerialization bool          // Enable per-VMI operation locks
	VMICacheTTL            time.Duration // Cache TTL for PVC->VMI mapping (default: 60s)

	// Mode flags
	EnableController bool
	EnableNode       bool
}

// NewDriver creates a new RDS CSI driver
func NewDriver(config DriverConfig) (*Driver, error) {
	if config.DriverName == "" {
		config.DriverName = DriverName
	}
	if config.Version == "" {
		config.Version = version
	}

	klog.Infof("Driver: %s Version: %s GitCommit: %s BuildDate: %s", config.DriverName, config.Version, gitCommit, buildDate)

	// Add configured base path to security allowlist
	if config.RDSVolumeBasePath != "" {
		if err := utils.AddAllowedBasePath(config.RDSVolumeBasePath); err != nil {
			return nil, fmt.Errorf("failed to add base path to allowlist: %w", err)
		}
		klog.Infof("Added volume base path to security allowlist: %s", config.RDSVolumeBasePath)
	}

	driver := &Driver{
		name:      config.DriverName,
		version:   config.Version,
		nodeID:    config.NodeID,
		k8sClient: config.K8sClient,
		metrics:   config.Metrics,
	}

	// Initialize RDS client if controller is enabled
	if config.EnableController {
		rdsClient, err := rds.NewClient(rds.ClientConfig{
			Address:            config.RDSAddress,
			Port:               config.RDSPort,
			User:               config.RDSUser,
			PrivateKey:         config.RDSPrivateKey,
			HostKey:            config.RDSHostKey,
			InsecureSkipVerify: config.RDSInsecureSkipVerify,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create RDS client: %w", err)
		}

		// Connect to RDS
		if err := rdsClient.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect to RDS: %w", err)
		}

		driver.rdsClient = rdsClient
		klog.Infof("Connected to RDS at %s:%d", config.RDSAddress, config.RDSPort)
	}

	// Initialize attachment manager if controller is enabled
	if config.EnableController && config.K8sClient != nil {
		driver.attachmentManager = attachment.NewAttachmentManager(config.K8sClient)
		klog.Info("Attachment manager created")
	}

	// Initialize attachment reconciler if enabled
	if config.EnableController && config.EnableAttachmentReconciler && config.K8sClient != nil && driver.attachmentManager != nil {
		// Create EventPoster for posting lifecycle events
		var eventPoster attachment.EventPoster
		if config.K8sClient != nil {
			eventPoster = NewEventPoster(config.K8sClient)
		}

		reconcilerConfig := attachment.ReconcilerConfig{
			Manager:     driver.attachmentManager,
			K8sClient:   config.K8sClient,
			Interval:    config.AttachmentReconcileInterval,
			GracePeriod: config.AttachmentGracePeriod,
			Metrics:     config.Metrics,
			EventPoster: eventPoster,
		}

		attachmentReconciler, err := attachment.NewAttachmentReconciler(reconcilerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create attachment reconciler: %w", err)
		}

		driver.attachmentReconciler = attachmentReconciler
		driver.attachmentGracePeriod = config.AttachmentGracePeriod
		if driver.attachmentGracePeriod <= 0 {
			driver.attachmentGracePeriod = 30 * time.Second
		}
		klog.Infof("Attachment reconciler enabled (interval=%v, grace_period=%v)",
			config.AttachmentReconcileInterval, config.AttachmentGracePeriod)
	}

	// Initialize VMI grouper for per-VMI operation serialization
	if config.EnableController && config.EnableVMISerialization && config.K8sClient != nil {
		driver.vmiGrouper = NewVMIGrouper(VMIGrouperConfig{
			K8sClient: config.K8sClient,
			CacheTTL:  config.VMICacheTTL,
			Enabled:   true,
		})
		klog.Infof("VMI serialization enabled (cache_ttl=%v)", config.VMICacheTTL)
	}

	// Add volume capabilities
	driver.addVolumeCapabilities()

	// Add controller service capabilities
	if config.EnableController {
		driver.addControllerServiceCapabilities()
	}

	// Add node service capabilities
	if config.EnableNode {
		driver.addNodeServiceCapabilities()
	}

	// Initialize orphan reconciler if enabled and we have controller + k8s client
	if config.EnableController && config.EnableOrphanReconciler && config.K8sClient != nil {
		reconcilerConfig := reconciler.OrphanReconcilerConfig{
			RDSClient:     driver.rdsClient,
			K8sClient:     config.K8sClient,
			CheckInterval: config.OrphanCheckInterval,
			GracePeriod:   config.OrphanGracePeriod,
			DryRun:        config.OrphanDryRun,
			Enabled:       true,
			BasePath:      config.RDSVolumeBasePath,
		}

		orphanReconciler, err := reconciler.NewOrphanReconciler(reconcilerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create orphan reconciler: %w", err)
		}

		driver.reconciler = orphanReconciler
		klog.Infof("Orphan reconciler enabled (interval=%v, grace_period=%v, dry_run=%v)",
			config.OrphanCheckInterval, config.OrphanGracePeriod, config.OrphanDryRun)
	}

	return driver, nil
}

// addVolumeCapabilities adds supported volume access modes
func (d *Driver) addVolumeCapabilities() {
	d.vcaps = []*csi.VolumeCapability_AccessMode{
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		},
	}
}

// addControllerServiceCapabilities adds controller service capabilities
func (d *Driver) addControllerServiceCapabilities() {
	d.cscaps = []*csi.ControllerServiceCapability{
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				},
			},
		},
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
				},
			},
		},
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
				},
			},
		},
	}
}

// addNodeServiceCapabilities adds node service capabilities
func (d *Driver) addNodeServiceCapabilities() {
	d.nscaps = []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
				},
			},
		},
	}
}

// Run starts the CSI driver gRPC server
func (d *Driver) Run(endpoint string) error {
	klog.Infof("Starting RDS CSI driver at endpoint %s", endpoint)

	// Initialize identity service (always available)
	d.ids = NewIdentityServer(d)

	// Initialize controller service if enabled
	if d.rdsClient != nil {
		klog.Info("Controller service enabled")
		d.cs = NewControllerServer(d)
	}

	// Initialize node service if enabled
	if d.nodeID != "" {
		klog.Info("Node service enabled")
		d.ns = NewNodeServer(d, d.nodeID, d.k8sClient)
	}

	// Initialize attachment manager state
	if d.attachmentManager != nil {
		ctx := context.Background()
		if err := d.attachmentManager.Initialize(ctx); err != nil {
			return fmt.Errorf("failed to initialize attachment manager: %w", err)
		}
		klog.Info("Attachment manager initialized")
	}

	// Start attachment reconciler if configured
	if d.attachmentReconciler != nil {
		ctx := context.Background()
		if err := d.attachmentReconciler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start attachment reconciler: %w", err)
		}
		klog.Info("Attachment reconciler started")
	}

	// Start orphan reconciler if configured
	if d.reconciler != nil {
		ctx := context.Background()
		if err := d.reconciler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start orphan reconciler: %w", err)
		}
		klog.Info("Orphan reconciler started")
	}

	// Start gRPC server
	server := NewNonBlockingGRPCServer(endpoint)
	if err := server.Start(d.ids, d.cs, d.ns); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	klog.Info("Driver initialization complete, server running")

	// Block forever (shutdown handled by Stop method via signal handler)
	select {}
}

// Stop stops the driver and cleans up resources
func (d *Driver) Stop() {
	klog.Info("Stopping RDS CSI driver")

	// Stop attachment reconciler if running
	if d.attachmentReconciler != nil {
		d.attachmentReconciler.Stop()
		klog.Info("Attachment reconciler stopped")
	}

	// Stop orphan reconciler if running
	if d.reconciler != nil {
		d.reconciler.Stop()
		klog.Info("Orphan reconciler stopped")
	}

	if d.rdsClient != nil {
		if err := d.rdsClient.Close(); err != nil {
			klog.Errorf("Error closing RDS client: %v", err)
		}
	}
}

// SetRDSClient sets the RDS client (for testing)
func (d *Driver) SetRDSClient(client rds.RDSClient) {
	d.rdsClient = client
}

// AddVolumeCapabilities adds volume capabilities (exported for testing)
func (d *Driver) AddVolumeCapabilities() {
	d.addVolumeCapabilities()
}

// AddControllerServiceCapabilities adds controller service capabilities (exported for testing)
func (d *Driver) AddControllerServiceCapabilities() {
	d.addControllerServiceCapabilities()
}

// AddNodeServiceCapabilities adds node service capabilities (exported for testing)
func (d *Driver) AddNodeServiceCapabilities() {
	d.addNodeServiceCapabilities()
}

// GetMetrics returns the Prometheus metrics instance (may be nil if disabled)
func (d *Driver) GetMetrics() *observability.Metrics {
	return d.metrics
}

// GetAttachmentManager returns the attachment manager (may be nil if controller disabled)
func (d *Driver) GetAttachmentManager() *attachment.AttachmentManager {
	return d.attachmentManager
}

// GetAttachmentGracePeriod returns the configured grace period for attachment handoff.
func (d *Driver) GetAttachmentGracePeriod() time.Duration {
	return d.attachmentGracePeriod
}

// GetVMIGrouper returns the VMI grouper for per-VMI operation serialization (may be nil if disabled).
func (d *Driver) GetVMIGrouper() *VMIGrouper {
	return d.vmiGrouper
}
