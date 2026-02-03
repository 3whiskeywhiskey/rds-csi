package nvme

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	// DeviceCheckTimeout is the maximum time to wait for lsof to respond.
	// If exceeded, assume device is unresponsive and proceed with disconnect.
	DeviceCheckTimeout = 5 * time.Second
)

// DeviceUsageResult contains the result of a device-in-use check.
type DeviceUsageResult struct {
	// InUse is true if processes have the device open
	InUse bool
	// Processes is a list of "command[PID]" strings holding the device open
	Processes []string
	// TimedOut is true if the check timed out (device may be unresponsive)
	TimedOut bool
	// Error is set if the check failed for reasons other than timeout
	Error error
}

// CheckDeviceInUse checks if a device has open file descriptors using lsof.
// Returns InUse=true if processes are holding the device open.
// Returns TimedOut=true if lsof didn't respond within DeviceCheckTimeout (device may be unresponsive).
// Uses context for cancellation.
func CheckDeviceInUse(ctx context.Context, devicePath string) DeviceUsageResult {
	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, DeviceCheckTimeout)
	defer cancel()

	// Run lsof with timeout
	cmd := exec.CommandContext(checkCtx, "lsof", devicePath)
	out, err := cmd.Output()

	// Check for timeout
	if checkCtx.Err() == context.DeadlineExceeded {
		klog.Warningf("Device busy check timed out for %s after %v (device may be unresponsive)",
			devicePath, DeviceCheckTimeout)
		return DeviceUsageResult{
			InUse:    false, // Treat as not busy - device unresponsive, proceed with disconnect
			TimedOut: true,
		}
	}

	if err != nil {
		// lsof returns exit code 1 if no processes using device
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No processes using device - this is the normal "not busy" case
			return DeviceUsageResult{InUse: false}
		}

		// Other error (lsof not found, permission denied, etc.)
		klog.Warningf("lsof command failed for %s: %v", devicePath, err)
		return DeviceUsageResult{
			InUse: false, // Can't determine - proceed with caution
			Error: fmt.Errorf("lsof failed: %w", err),
		}
	}

	// Parse lsof output
	// Format: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 {
		// Only header line or empty - no processes
		return DeviceUsageResult{InUse: false}
	}

	// Extract process info from output (skip header line)
	processes := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Format as "command[PID]" for readable error messages
			processes = append(processes, fmt.Sprintf("%s[PID:%s]", fields[0], fields[1]))
		}
	}

	if len(processes) > 0 {
		klog.V(2).Infof("Device %s in use by %d process(es): %v", devicePath, len(processes), processes)
		return DeviceUsageResult{
			InUse:     true,
			Processes: processes,
		}
	}

	return DeviceUsageResult{InUse: false}
}
