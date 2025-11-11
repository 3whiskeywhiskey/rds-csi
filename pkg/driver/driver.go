package driver

import (
	"context"
	"fmt"
	"time"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/reconciler"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
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

	// Orphan reconciler (optional)
	reconciler *reconciler.OrphanReconciler

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

	// Kubernetes client (required for orphan reconciler)
	K8sClient kubernetes.Interface

	// Orphan reconciler settings
	EnableOrphanReconciler bool
	OrphanCheckInterval    time.Duration
	OrphanGracePeriod      time.Duration
	OrphanDryRun           bool

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

	driver := &Driver{
		name:    config.DriverName,
		version: config.Version,
		nodeID:  config.NodeID,
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
		d.ns = NewNodeServer(d, d.nodeID)
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
