package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/driver"
)

var (
	// Driver configuration
	endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/rds.csi.srvlab.io/csi.sock", "CSI endpoint")
	nodeID     = flag.String("node-id", "", "Node ID (required for node service)")
	driverName = flag.String("driver-name", "rds.csi.srvlab.io", "Name of the CSI driver")

	// RDS configuration
	rdsAddress    = flag.String("rds-address", "", "RDS server IP address (required for controller)")
	rdsPort       = flag.Int("rds-port", 22, "RDS SSH port")
	rdsUser       = flag.String("rds-user", "admin", "RDS SSH user")
	rdsKeyFile    = flag.String("rds-key-file", "/etc/rds-csi/ssh-key/id_rsa", "Path to RDS SSH private key")
	rdsHostKey    = flag.String("rds-host-key", "", "Path to RDS SSH host public key (required for secure verification)")
	rdsInsecure   = flag.Bool("rds-insecure-skip-verify", false, "Skip SSH host key verification (INSECURE - for testing only)")

	// Mode flags
	controllerMode = flag.Bool("controller", false, "Run in controller mode")
	nodeMode       = flag.Bool("node", false, "Run in node mode")

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

	// Create driver configuration
	config := driver.DriverConfig{
		DriverName:            *driverName,
		NodeID:                *nodeID,
		RDSAddress:            *rdsAddress,
		RDSPort:               *rdsPort,
		RDSUser:               *rdsUser,
		RDSPrivateKey:         privateKey,
		RDSHostKey:            hostKey,
		RDSInsecureSkipVerify: *rdsInsecure,
		EnableController:      *controllerMode,
		EnableNode:            *nodeMode,
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
