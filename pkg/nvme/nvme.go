package nvme

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
	"k8s.io/klog/v2"
)

// Connector handles NVMe/TCP connections
type Connector interface {
	// Connect establishes connection to NVMe/TCP target
	Connect(target Target) (string, error)

	// Disconnect terminates connection to NVMe/TCP target
	Disconnect(nqn string) error

	// IsConnected checks if NVMe target is connected
	IsConnected(nqn string) (bool, error)

	// GetDevicePath returns block device path for connected target
	GetDevicePath(nqn string) (string, error)

	// WaitForDevice waits for device to appear after connection
	WaitForDevice(nqn string, timeout time.Duration) (string, error)
}

// Target represents an NVMe/TCP connection target
type Target struct {
	// Transport type (always "tcp" for NVMe/TCP)
	Transport string

	// NQN (NVMe Qualified Name) of the subsystem
	NQN string

	// TargetAddress is the IP address of the NVMe/TCP target
	TargetAddress string

	// TargetPort is the port number of the NVMe/TCP target
	TargetPort int

	// HostNQN is the NQN of the host initiator (optional)
	HostNQN string
}

// connector implements Connector interface using nvme-cli
type connector struct {
	execCommand func(name string, args ...string) *exec.Cmd
}

// NewConnector creates a new NVMe connector
func NewConnector() Connector {
	return &connector{
		execCommand: exec.Command,
	}
}

// Connect establishes NVMe/TCP connection and returns device path
func (c *connector) Connect(target Target) (string, error) {
	klog.V(2).Infof("Connecting to NVMe/TCP target: %s at %s:%d",
		target.NQN, target.TargetAddress, target.TargetPort)

	// SECURITY: Validate NQN format before using in commands
	if err := utils.ValidateNQN(target.NQN); err != nil {
		return "", fmt.Errorf("invalid target NQN: %w", err)
	}

	// Validate host NQN if specified
	if target.HostNQN != "" {
		if err := utils.ValidateNQN(target.HostNQN); err != nil {
			return "", fmt.Errorf("invalid host NQN: %w", err)
		}
	}

	// Check if already connected
	connected, err := c.IsConnected(target.NQN)
	if err != nil {
		return "", fmt.Errorf("failed to check connection status: %w", err)
	}

	if connected {
		klog.V(2).Infof("Already connected to NQN: %s", target.NQN)
		return c.GetDevicePath(target.NQN)
	}

	// Build nvme connect command
	args := []string{
		"connect",
		"-t", target.Transport,
		"-a", target.TargetAddress,
		"-s", fmt.Sprintf("%d", target.TargetPort),
		"-n", target.NQN,
	}

	// Add host NQN if specified
	if target.HostNQN != "" {
		args = append(args, "-q", target.HostNQN)
	}

	// Execute nvme connect
	cmd := c.execCommand("nvme", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nvme connect failed: %w, output: %s", err, string(output))
	}

	klog.V(4).Infof("nvme connect output: %s", string(output))

	// Wait for device to appear
	devicePath, err := c.WaitForDevice(target.NQN, 30*time.Second)
	if err != nil {
		// Cleanup: disconnect on failure
		_ = c.Disconnect(target.NQN)
		return "", fmt.Errorf("device did not appear: %w", err)
	}

	// Wait for device node to be accessible in /dev
	// Give udev time to create the device node and set permissions
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(devicePath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Final check that device is accessible
	if _, err := os.Stat(devicePath); err != nil {
		_ = c.Disconnect(target.NQN)
		return "", fmt.Errorf("device %s not accessible: %w", devicePath, err)
	}

	klog.V(2).Infof("Successfully connected to NVMe target, device: %s", devicePath)
	return devicePath, nil
}

// Disconnect terminates NVMe/TCP connection
func (c *connector) Disconnect(nqn string) error {
	klog.V(2).Infof("Disconnecting from NVMe target: %s", nqn)

	// SECURITY: Validate NQN format before using in commands
	if err := utils.ValidateNQN(nqn); err != nil {
		return fmt.Errorf("invalid NQN: %w", err)
	}

	// Check if connected
	connected, err := c.IsConnected(nqn)
	if err != nil {
		return fmt.Errorf("failed to check connection status: %w", err)
	}

	if !connected {
		klog.V(2).Infof("Not connected to NQN: %s, nothing to disconnect", nqn)
		return nil
	}

	// Execute nvme disconnect
	cmd := c.execCommand("nvme", "disconnect", "-n", nqn)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvme disconnect failed: %w, output: %s", err, string(output))
	}

	klog.V(4).Infof("nvme disconnect output: %s", string(output))
	klog.V(2).Infof("Successfully disconnected from NVMe target: %s", nqn)
	return nil
}

// IsConnected checks if NVMe target is currently connected
func (c *connector) IsConnected(nqn string) (bool, error) {
	// SECURITY: Validate NQN format
	if err := utils.ValidateNQN(nqn); err != nil {
		return false, fmt.Errorf("invalid NQN: %w", err)
	}

	// List all NVMe subsystems
	cmd := c.execCommand("nvme", "list-subsys", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// nvme list-subsys may fail if no devices, that's ok
		if strings.Contains(string(output), "No NVMe subsystems") {
			return false, nil
		}
		klog.V(4).Infof("nvme list-subsys failed (may be normal): %v, output: %s", err, string(output))
		return false, nil
	}

	// Simple string search for NQN
	// TODO: Parse JSON for more robust checking in future
	return strings.Contains(string(output), nqn), nil
}

// GetDevicePath returns the block device path for a connected NVMe target
func (c *connector) GetDevicePath(nqn string) (string, error) {
	// Scan /sys/class/nvme for controllers
	controllers, err := filepath.Glob("/sys/class/nvme/nvme*")
	if err != nil {
		return "", fmt.Errorf("failed to scan nvme devices: %w", err)
	}

	for _, controller := range controllers {
		// Read subsystem NQN
		nqnPath := filepath.Join(controller, "subsysnqn")
		data, err := os.ReadFile(nqnPath)
		if err != nil {
			klog.V(5).Infof("Failed to read %s: %v", nqnPath, err)
			continue
		}

		deviceNQN := strings.TrimSpace(string(data))
		if deviceNQN == nqn {
			// Found matching controller, now find the block device in /sys/class/block
			controllerName := filepath.Base(controller)

			// Search for block devices matching this controller
			// For NVMe-oF, the device is usually nvmeXnY (not nvmeXcYnZ)
			blockDevices, err := filepath.Glob("/sys/class/block/" + controllerName + "n*")
			if err != nil {
				return "", fmt.Errorf("failed to scan block devices: %w", err)
			}

			// Return first namespace found
			for _, blockDev := range blockDevices {
				deviceName := filepath.Base(blockDev)
				// Skip controller-specific paths like nvme1c1n1, prefer nvme1n1
				if !strings.Contains(deviceName, "c") {
					return "/dev/" + deviceName, nil
				}
			}

			// If no simple path found, use any available
			if len(blockDevices) > 0 {
				deviceName := filepath.Base(blockDevices[0])
				return "/dev/" + deviceName, nil
			}
		}
	}

	return "", fmt.Errorf("no device found for NQN: %s", nqn)
}

// WaitForDevice waits for block device to appear after connection
func (c *connector) WaitForDevice(nqn string, timeout time.Duration) (string, error) {
	klog.V(4).Infof("Waiting for device with NQN: %s (timeout: %v)", nqn, timeout)

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			devicePath, err := c.GetDevicePath(nqn)
			if err == nil {
				klog.V(4).Infof("Device appeared: %s", devicePath)
				return devicePath, nil
			}

			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout waiting for device with NQN %s", nqn)
			}

		case <-time.After(timeout):
			return "", fmt.Errorf("timeout waiting for device with NQN %s", nqn)
		}
	}
}
