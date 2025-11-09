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
	rdsAddress = flag.String("rds-address", "", "RDS server IP address (required for controller)")
	rdsPort    = flag.Int("rds-port", 22, "RDS SSH port")
	rdsUser    = flag.String("rds-user", "admin", "RDS SSH user")
	rdsKeyFile = flag.String("rds-key-file", "/etc/rds-csi/ssh-key/id_rsa", "Path to RDS SSH private key")

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

	// Read SSH private key if controller mode
	var privateKey []byte
	var err error
	if *controllerMode {
		privateKey, err = os.ReadFile(*rdsKeyFile)
		if err != nil {
			klog.Fatalf("Failed to read SSH key from %s: %v", *rdsKeyFile, err)
		}
		klog.V(4).Infof("Loaded SSH key from %s", *rdsKeyFile)
	}

	// Create driver configuration
	config := driver.DriverConfig{
		DriverName:       *driverName,
		NodeID:           *nodeID,
		RDSAddress:       *rdsAddress,
		RDSPort:          *rdsPort,
		RDSUser:          *rdsUser,
		RDSPrivateKey:    privateKey,
		EnableController: *controllerMode,
		EnableNode:       *nodeMode,
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
