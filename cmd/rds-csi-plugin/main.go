package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/driver"
)

var (
	// Driver configuration
	endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/rds.csi.srvlab.io/csi.sock", "CSI endpoint")
	nodeID     = flag.String("node-id", "", "Node ID (required for node service)")
	driverName = flag.String("driver-name", "rds.csi.srvlab.io", "Name of the CSI driver")

	// RDS configuration
	rdsAddress        = flag.String("rds-address", "", "RDS server IP address (required for controller)")
	rdsPort           = flag.Int("rds-port", 22, "RDS SSH port")
	rdsUser           = flag.String("rds-user", "admin", "RDS SSH user")
	rdsKeyFile        = flag.String("rds-key-file", "/etc/rds-csi/ssh-key/id_rsa", "Path to RDS SSH private key")
	rdsHostKey        = flag.String("rds-host-key", "", "Path to RDS SSH host public key (required for secure verification)")
	rdsInsecure       = flag.Bool("rds-insecure-skip-verify", false, "Skip SSH host key verification (INSECURE - for testing only)")
	rdsVolumeBasePath = flag.String("rds-volume-base-path", "", "Base path for volumes on RDS (e.g., /storage-pool/metal-csi, required for file orphan detection)")

	// Mode flags
	controllerMode = flag.Bool("controller", false, "Run in controller mode")
	nodeMode       = flag.Bool("node", false, "Run in node mode")

	// Orphan reconciler flags
	enableOrphanReconciler = flag.Bool("enable-orphan-reconciler", false, "Enable orphan volume detection and cleanup")
	orphanCheckInterval    = flag.Duration("orphan-check-interval", 1*time.Hour, "Interval between orphan checks")
	orphanGracePeriod      = flag.Duration("orphan-grace-period", 5*time.Minute, "Minimum age before considering a volume orphaned")
	orphanDryRun           = flag.Bool("orphan-dry-run", true, "Dry-run mode for orphan cleanup (only log, don't delete)")

	// Kubernetes configuration
	kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig file (optional, uses in-cluster config if not specified)")

	// Version flag
	version = flag.Bool("version", false, "Print version and exit")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if *version {
		fmt.Println(driver.DriverName)
		// Version info will be displayed by driver
		os.Exit(0)
	}

	// Validate mode flags
	if !*controllerMode && !*nodeMode {
		klog.Fatal("Must specify at least one of --controller or --node")
	}

	// Validate required flags
	if *controllerMode && *rdsAddress == "" {
		klog.Fatal("--rds-address is required in controller mode")
	}

	if *nodeMode && *nodeID == "" {
		klog.Fatal("--node-id is required in node mode")
	}

	// Read SSH private key and host key if controller mode
	var privateKey []byte
	var hostKey []byte
	var err error
	if *controllerMode {
		privateKey, err = os.ReadFile(*rdsKeyFile)
		if err != nil {
			klog.Fatalf("Failed to read SSH key from %s: %v", *rdsKeyFile, err)
		}
		klog.V(4).Infof("Loaded SSH key from %s", *rdsKeyFile)

		// Enforce host key verification in production
		if *rdsHostKey == "" && !*rdsInsecure {
			klog.Fatal("SECURITY: --rds-host-key is required for production use. Use --rds-insecure-skip-verify ONLY for testing.")
		}

		// Read host key if provided
		if *rdsHostKey != "" {
			hostKey, err = os.ReadFile(*rdsHostKey)
			if err != nil {
				klog.Fatalf("Failed to read SSH host key from %s: %v", *rdsHostKey, err)
			}
			klog.V(4).Infof("Loaded SSH host key from %s", *rdsHostKey)
		} else if *rdsInsecure {
			klog.Warning("SECURITY WARNING: SSH host key verification is disabled. This is INSECURE and should only be used for testing!")
		}
	}

	// Create Kubernetes client if needed (for orphan reconciler)
	var k8sClient kubernetes.Interface
	if *controllerMode && *enableOrphanReconciler {
		k8sClient, err = createKubernetesClient(*kubeconfig)
		if err != nil {
			klog.Fatalf("Failed to create Kubernetes client: %v", err)
		}
		klog.Info("Kubernetes client initialized for orphan reconciler")
	}

	// Create driver configuration
	config := driver.DriverConfig{
		DriverName:             *driverName,
		NodeID:                 *nodeID,
		RDSAddress:             *rdsAddress,
		RDSPort:                *rdsPort,
		RDSUser:                *rdsUser,
		RDSPrivateKey:          privateKey,
		RDSHostKey:             hostKey,
		RDSInsecureSkipVerify:  *rdsInsecure,
		RDSVolumeBasePath:      *rdsVolumeBasePath,
		K8sClient:              k8sClient,
		EnableOrphanReconciler: *enableOrphanReconciler,
		OrphanCheckInterval:    *orphanCheckInterval,
		OrphanGracePeriod:      *orphanGracePeriod,
		OrphanDryRun:           *orphanDryRun,
		EnableController:       *controllerMode,
		EnableNode:             *nodeMode,
	}

	// Create driver
	klog.Info("Creating RDS CSI driver")
	drv, err := driver.NewDriver(config)
	if err != nil {
		klog.Fatalf("Failed to create driver: %v", err)
	}

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		klog.Infof("Received signal %s, shutting down", sig)
		drv.Stop()
		os.Exit(0)
	}()

	// Run driver
	klog.Infof("Starting driver in modes: controller=%v node=%v", *controllerMode, *nodeMode)
	if err := drv.Run(*endpoint); err != nil {
		klog.Fatalf("Failed to run driver: %v", err)
	}

	// Keep running
	select {}
}

// createKubernetesClient creates a Kubernetes client using in-cluster config or kubeconfig file
func createKubernetesClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		// Use kubeconfig file
		klog.V(2).Infof("Using kubeconfig file: %s", kubeconfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	} else {
		// Use in-cluster config
		klog.V(2).Info("Using in-cluster Kubernetes config")
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return clientset, nil
}
