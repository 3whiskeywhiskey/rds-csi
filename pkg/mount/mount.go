package mount

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

// Dangerous mount options that should never be allowed
var dangerousMountOptions = map[string]bool{
	"suid": true, // Allow set-user-ID/set-group-ID bits
	"dev":  true, // Interpret character or block special devices
	"exec": true, // Permit execution of binaries
}

// Default secure mount options enforced for bind mounts
var defaultSecureMountOptions = []string{
	"nosuid", // Ignore set-user-ID/set-group-ID bits
	"nodev",  // Do not interpret character or block special devices
	"noexec", // Do not allow direct execution of binaries
}

// Whitelist of allowed mount options (beyond the defaults)
var allowedMountOptions = map[string]bool{
	// Security options
	"nosuid":   true,
	"nodev":    true,
	"noexec":   true,
	"ro":       true,
	"rw":       true,
	"relatime": true,
	"noatime":  true,
	"nodiratime": true,

	// Filesystem-specific options that are generally safe
	"defaults": true,
	"sync":     true,
	"async":    true,
	"auto":     true,
	"noauto":   true,
	"user":     true,
	"nouser":   true,
	"_netdev":  true,

	// Bind mount options
	"bind":     true,
	"rbind":    true,
	"remount":  true,

	// Additional safe options
	"strictatime": true,
	"lazytime":    true,
	"nolazytime":  true,
}

// Mounter handles filesystem operations
type Mounter interface {
	// Mount mounts source to target with the given fsType and options
	Mount(source, target, fsType string, options []string) error

	// Unmount unmounts the target
	Unmount(target string) error

	// IsLikelyMountPoint checks if a path is a mount point
	IsLikelyMountPoint(path string) (bool, error)

	// Format formats the device with the given filesystem type
	Format(device, fsType string) error

	// IsFormatted checks if device has a filesystem
	IsFormatted(device string) (bool, error)

	// GetDeviceStats returns filesystem statistics
	GetDeviceStats(path string) (*DeviceStats, error)
}

// DeviceStats represents filesystem statistics
type DeviceStats struct {
	// Total size in bytes
	TotalBytes int64

	// Used bytes
	UsedBytes int64

	// Available bytes
	AvailableBytes int64

	// Total inodes
	TotalInodes int64

	// Used inodes
	UsedInodes int64

	// Available inodes
	AvailableInodes int64
}

// mounter implements Mounter interface using system commands
type mounter struct {
	execCommand func(name string, args ...string) *exec.Cmd
}

// NewMounter creates a new filesystem mounter
func NewMounter() Mounter {
	return &mounter{
		execCommand: exec.Command,
	}
}

// ValidateMountOptions validates mount options against security policies
// Returns an error if any dangerous options are found or if options are not whitelisted
func ValidateMountOptions(options []string) error {
	if len(options) == 0 {
		// No options is safe
		return nil
	}

	for _, opt := range options {
		// Remove value if option has one (e.g., "uid=1000" -> "uid")
		optName := strings.Split(opt, "=")[0]

		// SECURITY: Check for dangerous options
		if dangerousMountOptions[optName] {
			return fmt.Errorf("dangerous mount option not allowed: %s", optName)
		}

		// Check if option is in whitelist
		if !allowedMountOptions[optName] {
			return fmt.Errorf("mount option not in whitelist: %s", optName)
		}
	}

	return nil
}

// SanitizeMountOptions adds default secure options and validates user-provided options
// For bind mounts, always enforces nosuid, nodev, noexec unless explicitly overridden
func SanitizeMountOptions(options []string, isBindMount bool) ([]string, error) {
	// Validate provided options first
	if err := ValidateMountOptions(options); err != nil {
		return nil, err
	}

	// For bind mounts, enforce secure defaults
	if isBindMount {
		// Create a set to track existing options
		existingOpts := make(map[string]bool)
		for _, opt := range options {
			optName := strings.Split(opt, "=")[0]
			existingOpts[optName] = true
		}

		// Add secure defaults if not already present
		secureOpts := make([]string, 0, len(options)+len(defaultSecureMountOptions))
		for _, secureOpt := range defaultSecureMountOptions {
			// Don't add if user explicitly specified the opposite
			opposite := ""
			switch secureOpt {
			case "nosuid":
				opposite = "suid"
			case "nodev":
				opposite = "dev"
			case "noexec":
				opposite = "exec"
			}

			if !existingOpts[secureOpt] && !existingOpts[opposite] {
				secureOpts = append(secureOpts, secureOpt)
			}
		}

		// Combine secure options with user options
		options = append(secureOpts, options...)
	}

	return options, nil
}

// Mount mounts source to target with the given filesystem type and options
func (m *mounter) Mount(source, target, fsType string, options []string) error {
	klog.V(2).Infof("Mounting %s to %s (fsType: %s, options: %v)", source, target, fsType, options)

	// SECURITY: Validate and sanitize mount options
	// Detect if this is a bind mount
	isBindMount := false
	for _, opt := range options {
		if opt == "bind" || opt == "rbind" {
			isBindMount = true
			break
		}
	}

	sanitizedOptions, err := SanitizeMountOptions(options, isBindMount)
	if err != nil {
		return fmt.Errorf("mount options validation failed: %w", err)
	}
	options = sanitizedOptions

	klog.V(4).Infof("Sanitized mount options: %v", options)

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(target, 0750); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Build mount command arguments
	args := []string{}

	// Add filesystem type if specified
	if fsType != "" {
		args = append(args, "-t", fsType)
	}

	// Add mount options if specified
	if len(options) > 0 {
		args = append(args, "-o", strings.Join(options, ","))
	}

	// Add source and target
	args = append(args, source, target)

	// Execute mount command
	cmd := m.execCommand("mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount failed: %w, output: %s", err, string(output))
	}

	klog.V(4).Infof("mount output: %s", string(output))
	klog.V(2).Infof("Successfully mounted %s to %s", source, target)
	return nil
}

// Unmount unmounts the target path
func (m *mounter) Unmount(target string) error {
	klog.V(2).Infof("Unmounting %s", target)

	// Check if it's actually mounted
	mounted, err := m.IsLikelyMountPoint(target)
	if err != nil {
		return fmt.Errorf("failed to check if mounted: %w", err)
	}

	if !mounted {
		klog.V(2).Infof("Path %s is not mounted, nothing to unmount", target)
		return nil
	}

	// Execute umount command
	cmd := m.execCommand("umount", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount failed: %w, output: %s", err, string(output))
	}

	klog.V(4).Infof("umount output: %s", string(output))
	klog.V(2).Infof("Successfully unmounted %s", target)
	return nil
}

// IsLikelyMountPoint checks if a path is a mount point
func (m *mounter) IsLikelyMountPoint(path string) (bool, error) {
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	// Use findmnt to check if path is a mount point
	cmd := m.execCommand("findmnt", "-o", "TARGET", "-n", "-M", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// findmnt returns non-zero if not a mount point
		if strings.Contains(string(output), "not found") {
			return false, nil
		}
		klog.V(5).Infof("findmnt failed: %v, output: %s", err, string(output))
		return false, nil
	}

	// If findmnt succeeded and returned output, it's a mount point
	return len(output) > 0, nil
}

// Format formats a device with the specified filesystem type
func (m *mounter) Format(device, fsType string) error {
	klog.V(2).Infof("Formatting device %s with %s", device, fsType)

	// Check if already formatted
	formatted, err := m.IsFormatted(device)
	if err != nil {
		return fmt.Errorf("failed to check if device is formatted: %w", err)
	}

	if formatted {
		klog.V(2).Infof("Device %s is already formatted, skipping", device)
		return nil
	}

	// Build mkfs command based on filesystem type
	var cmd *exec.Cmd
	switch fsType {
	case "ext4":
		// mkfs.ext4 -F (force) device
		cmd = m.execCommand("mkfs.ext4", "-F", device)
	case "ext3":
		cmd = m.execCommand("mkfs.ext3", "-F", device)
	case "xfs":
		cmd = m.execCommand("mkfs.xfs", "-f", device)
	default:
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}

	// Execute mkfs command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.%s failed: %w, output: %s", fsType, err, string(output))
	}

	klog.V(4).Infof("mkfs output: %s", string(output))
	klog.V(2).Infof("Successfully formatted %s with %s", device, fsType)
	return nil
}

// IsFormatted checks if a device has a filesystem
func (m *mounter) IsFormatted(device string) (bool, error) {
	// Use blkid to check for filesystem
	cmd := m.execCommand("blkid", "-o", "value", "-s", "TYPE", device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// blkid returns exit status 2 if no filesystem found
		// blkid returns exit status 1 if device not found or other error
		if strings.Contains(err.Error(), "exit status 2") || strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, fmt.Errorf("blkid failed: %w", err)
	}

	// If blkid returned a filesystem type, device is formatted
	fsType := strings.TrimSpace(string(output))
	return len(fsType) > 0, nil
}

// GetDeviceStats returns filesystem statistics for the given path
func (m *mounter) GetDeviceStats(path string) (*DeviceStats, error) {
	// Use df to get filesystem statistics
	cmd := m.execCommand("df", "--output=size,used,avail,itotal,iused,iavail", "-B1", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("df failed: %w, output: %s", err, string(output))
	}

	// Parse df output
	// Format: Size Used Avail Inodes IUsed IFree
	// Skip header line
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected df output: %s", string(output))
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return nil, fmt.Errorf("unexpected df output format: %s", lines[1])
	}

	stats := &DeviceStats{}

	// Parse size fields
	_, _ = fmt.Sscanf(fields[0], "%d", &stats.TotalBytes)
	_, _ = fmt.Sscanf(fields[1], "%d", &stats.UsedBytes)
	_, _ = fmt.Sscanf(fields[2], "%d", &stats.AvailableBytes)

	// Parse inode fields
	_, _ = fmt.Sscanf(fields[3], "%d", &stats.TotalInodes)
	_, _ = fmt.Sscanf(fields[4], "%d", &stats.UsedInodes)
	_, _ = fmt.Sscanf(fields[5], "%d", &stats.AvailableInodes)

	return stats, nil
}
