package rds

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

// CreateVolume creates a file-backed NVMe/TCP volume on RDS
func (c *sshClient) CreateVolume(opts CreateVolumeOptions) error {
	klog.V(2).Infof("Creating volume %s (size: %d bytes, path: %s)", opts.Slot, opts.FileSizeBytes, opts.FilePath)

	// Validate options
	if err := validateCreateVolumeOptions(opts); err != nil {
		return fmt.Errorf("invalid volume options: %w", err)
	}

	// Convert size to human-readable format (e.g., "50G", "100G")
	sizeStr := formatBytes(opts.FileSizeBytes)

	// Build /disk add command
	cmd := fmt.Sprintf(
		`/disk add type=file file-path=%s file-size=%s slot=%s nvme-tcp-export=yes nvme-tcp-server-port=%d nvme-tcp-server-nqn=%s`,
		opts.FilePath,
		sizeStr,
		opts.Slot,
		opts.NVMETCPPort,
		opts.NVMETCPNQN,
	)

	// Execute command with retry
	_, err := c.runCommandWithRetry(cmd, 3)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	// Verify volume was created
	if err := c.VerifyVolumeExists(opts.Slot); err != nil {
		return fmt.Errorf("volume creation verification failed: %w", err)
	}

	klog.V(2).Infof("Successfully created volume %s", opts.Slot)
	return nil
}

// DeleteVolume removes a volume from RDS
func (c *sshClient) DeleteVolume(slot string) error {
	klog.V(2).Infof("Deleting volume %s", slot)

	// Validate slot name
	if err := validateSlotName(slot); err != nil {
		return err
	}

	// Build /disk remove command
	cmd := fmt.Sprintf(`/disk remove [find slot=%s]`, slot)

	// Execute command with retry
	_, err := c.runCommandWithRetry(cmd, 3)
	if err != nil {
		// If volume doesn't exist, that's okay (idempotent)
		if strings.Contains(err.Error(), "no such item") {
			klog.V(3).Infof("Volume %s does not exist, skipping deletion", slot)
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	klog.V(2).Infof("Successfully deleted volume %s", slot)
	return nil
}

// GetVolume retrieves information about a specific volume
func (c *sshClient) GetVolume(slot string) (*VolumeInfo, error) {
	klog.V(4).Infof("Getting volume info for %s", slot)

	// Validate slot name
	if err := validateSlotName(slot); err != nil {
		return nil, err
	}

	// Build /disk print command
	cmd := fmt.Sprintf(`/disk print detail where slot=%s`, slot)

	// Execute command
	output, err := c.runCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume info: %w", err)
	}

	// Parse output
	if strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("volume not found: %s", slot)
	}

	volume, err := parseVolumeInfo(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse volume info: %w", err)
	}

	return volume, nil
}

// VerifyVolumeExists checks if a volume exists and is ready
func (c *sshClient) VerifyVolumeExists(slot string) error {
	volume, err := c.GetVolume(slot)
	if err != nil {
		return err
	}

	if volume.Status != "ready" {
		return fmt.Errorf("volume %s is not ready (status: %s)", slot, volume.Status)
	}

	return nil
}

// GetCapacity queries the available storage capacity on RDS
func (c *sshClient) GetCapacity(basePath string) (*CapacityInfo, error) {
	klog.V(4).Infof("Getting capacity for %s", basePath)

	// Build /file print command
	cmd := fmt.Sprintf(`/file print detail where name="%s"`, basePath)

	// Execute command
	output, err := c.runCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get capacity: %w", err)
	}

	// Parse capacity info
	capacity, err := parseCapacityInfo(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse capacity info: %w", err)
	}

	return capacity, nil
}

// ListVolumes lists all volumes on RDS
func (c *sshClient) ListVolumes() ([]VolumeInfo, error) {
	klog.V(4).Info("Listing all volumes")

	// Build /disk print command
	cmd := `/disk print detail`

	// Execute command
	output, err := c.runCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	// Parse all volumes
	volumes, err := parseVolumeList(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse volume list: %w", err)
	}

	return volumes, nil
}

// parseVolumeInfo parses RouterOS disk print output for a single volume
func parseVolumeInfo(output string) (*VolumeInfo, error) {
	volume := &VolumeInfo{}

	// Extract slot
	if match := regexp.MustCompile(`slot="([^"]+)"`).FindStringSubmatch(output); len(match) > 1 {
		volume.Slot = match[1]
	}

	// Extract type
	if match := regexp.MustCompile(`type="?([^"\s]+)"?`).FindStringSubmatch(output); len(match) > 1 {
		volume.Type = match[1]
	}

	// Extract file-path
	if match := regexp.MustCompile(`file-path="([^"]+)"`).FindStringSubmatch(output); len(match) > 1 {
		volume.FilePath = match[1]
	}

	// Extract file-size
	if match := regexp.MustCompile(`file-size=(\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if size, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			volume.FileSizeBytes = size
		}
	}

	// Extract nvme-tcp-export
	if match := regexp.MustCompile(`nvme-tcp-export=(yes|no)`).FindStringSubmatch(output); len(match) > 1 {
		volume.NVMETCPExport = match[1] == "yes"
	}

	// Extract nvme-tcp-server-port
	if match := regexp.MustCompile(`nvme-tcp-server-port=(\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if port, err := strconv.Atoi(match[1]); err == nil {
			volume.NVMETCPPort = port
		}
	}

	// Extract nvme-tcp-server-nqn
	if match := regexp.MustCompile(`nvme-tcp-server-nqn="([^"]+)"`).FindStringSubmatch(output); len(match) > 1 {
		volume.NVMETCPNQN = match[1]
	}

	// Extract status
	if match := regexp.MustCompile(`status="?([^"\s]+)"?`).FindStringSubmatch(output); len(match) > 1 {
		volume.Status = match[1]
	} else {
		volume.Status = "unknown"
	}

	return volume, nil
}

// parseVolumeList parses RouterOS disk print output for multiple volumes
func parseVolumeList(output string) ([]VolumeInfo, error) {
	var volumes []VolumeInfo

	// Split by volume entries (each starts with a number)
	entries := regexp.MustCompile(`(?m)^\s*\d+\s+`).Split(output, -1)

	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}

		volume, err := parseVolumeInfo(entry)
		if err != nil {
			klog.V(4).Infof("Skipping unparseable volume entry: %v", err)
			continue
		}

		volumes = append(volumes, *volume)
	}

	return volumes, nil
}

// parseCapacityInfo parses RouterOS file print output for capacity
func parseCapacityInfo(output string) (*CapacityInfo, error) {
	capacity := &CapacityInfo{}

	// Look for lines like "Total: 7.23TiB" or "Free: 5.12TiB"
	totalRe := regexp.MustCompile(`(?i)Total:\s+([\d.]+)\s*([KMGT]i?B)`)
	freeRe := regexp.MustCompile(`(?i)Free:\s+([\d.]+)\s*([KMGT]i?B)`)
	usedRe := regexp.MustCompile(`(?i)Used:\s+([\d.]+)\s*([KMGT]i?B)`)

	if match := totalRe.FindStringSubmatch(output); len(match) > 2 {
		if bytes, err := parseSize(match[1], match[2]); err == nil {
			capacity.TotalBytes = bytes
		}
	}

	if match := freeRe.FindStringSubmatch(output); len(match) > 2 {
		if bytes, err := parseSize(match[1], match[2]); err == nil {
			capacity.FreeBytes = bytes
		}
	}

	if match := usedRe.FindStringSubmatch(output); len(match) > 2 {
		if bytes, err := parseSize(match[1], match[2]); err == nil {
			capacity.UsedBytes = bytes
		}
	}

	// If we didn't get values, return error
	if capacity.TotalBytes == 0 {
		return nil, fmt.Errorf("could not parse capacity from output")
	}

	return capacity, nil
}

// validateCreateVolumeOptions validates volume creation options
func validateCreateVolumeOptions(opts CreateVolumeOptions) error {
	if opts.Slot == "" {
		return fmt.Errorf("slot is required")
	}
	if err := validateSlotName(opts.Slot); err != nil {
		return err
	}
	if opts.FilePath == "" {
		return fmt.Errorf("file path is required")
	}
	if opts.FileSizeBytes <= 0 {
		return fmt.Errorf("file size must be positive")
	}
	if opts.NVMETCPPort == 0 {
		opts.NVMETCPPort = 4420
	}
	if opts.NVMETCPNQN == "" {
		return fmt.Errorf("NVMe/TCP NQN is required")
	}
	return nil
}

// validateSlotName ensures slot name is safe (prevents command injection)
func validateSlotName(slot string) error {
	if slot == "" {
		return fmt.Errorf("slot name cannot be empty")
	}

	// Only allow alphanumeric and hyphen (pvc-<uuid> format)
	if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(slot) {
		return fmt.Errorf("invalid slot name format: %s (only alphanumeric and hyphen allowed)", slot)
	}

	return nil
}

// formatBytes converts bytes to human-readable format (50G, 100G, 1T)
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%dT", bytes/TB)
	case bytes >= GB:
		return fmt.Sprintf("%dG", bytes/GB)
	case bytes >= MB:
		return fmt.Sprintf("%dM", bytes/MB)
	case bytes >= KB:
		return fmt.Sprintf("%dK", bytes/KB)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// parseSize converts human-readable size to bytes
func parseSize(value, unit string) (int64, error) {
	num, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}

	multiplier := int64(1)
	switch strings.ToUpper(unit) {
	case "KIB", "KB", "K":
		multiplier = 1024
	case "MIB", "MB", "M":
		multiplier = 1024 * 1024
	case "GIB", "GB", "G":
		multiplier = 1024 * 1024 * 1024
	case "TIB", "TB", "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(num * float64(multiplier)), nil
}
