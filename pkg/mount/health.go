package mount

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"k8s.io/klog/v2"
)

const (
	// HealthCheckTimeout is the maximum time to wait for filesystem health check
	HealthCheckTimeout = 60 * time.Second
)

// CheckFilesystemHealth runs a read-only filesystem check before mounting.
// This detects corruption early before attempting mount operations.
// Returns nil if filesystem is healthy, error if corrupted or check fails.
//
// IMPORTANT: Only call this on UNMOUNTED devices. Running fsck on mounted
// filesystems can cause false positives or corruption.
func CheckFilesystemHealth(ctx context.Context, devicePath, fsType string) error {
	ctx, cancel := context.WithTimeout(ctx, HealthCheckTimeout)
	defer cancel()

	var cmd *exec.Cmd
	startTime := time.Now()

	switch fsType {
	case "ext4", "ext3", "ext2":
		// fsck.ext4 -n: read-only check, no modifications
		// -p: automatically repair if safe (not used with -n)
		cmd = exec.CommandContext(ctx, "fsck.ext4", "-n", devicePath)
	case "xfs":
		// xfs_repair -n: dry-run check only
		cmd = exec.CommandContext(ctx, "xfs_repair", "-n", devicePath)
	default:
		// Unknown filesystem - skip check (don't fail on unknown types)
		klog.V(2).Infof("Skipping health check for unsupported filesystem type: %s", fsType)
		return nil
	}

	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	if duration > 10*time.Second {
		klog.Warningf("Filesystem health check took %v (device: %s, fsType: %s)", duration, devicePath, fsType)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("filesystem health check timed out after %v for device %s. "+
			"Device may be unresponsive or severely corrupted", HealthCheckTimeout, devicePath)
	}

	if err != nil {
		return fmt.Errorf("filesystem health check failed for device %s (fsType: %s): %w. "+
			"Filesystem may be corrupted. Output: %s. "+
			"Consider running fsck manually after unmounting any existing mounts",
			devicePath, fsType, err, string(output))
	}

	klog.V(3).Infof("Filesystem health check passed for %s (fsType: %s, duration: %v)", devicePath, fsType, duration)
	return nil
}
