package rds

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
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

	// Normalize output and check if volume exists
	// RouterOS returns flags header even when volume doesn't exist
	normalized := normalizeRouterOSOutput(output)
	if strings.TrimSpace(normalized) == "" {
		return nil, fmt.Errorf("volume not found: %s", slot)
	}

	volume, err := parseVolumeInfo(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse volume info: %w", err)
	}

	// Additional check: if slot is empty, volume wasn't found
	if volume.Slot == "" {
		return nil, fmt.Errorf("volume not found: %s", slot)
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

	// SECURITY: Validate base path
	if basePath != "" {
		sanitized, err := utils.SanitizeBasePath(basePath)
		if err != nil {
			return nil, fmt.Errorf("invalid base path: %w", err)
		}
		basePath = sanitized
	}

	// Extract mount point from base path
	// Examples:
	//   /storage-pool/metal-csi/volumes → storage-pool
	//   /nvme1/kubernetes → nvme1
	mountPoint := extractMountPoint(basePath)
	klog.V(4).Infof("Extracted mount point: %s", mountPoint)

	// Query disk capacity using mount point
	// Use /disk print to get filesystem capacity information
	cmd := fmt.Sprintf(`/disk print detail where mount-point="%s"`, mountPoint)

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

// ListFiles lists files in a directory on RDS
func (c *sshClient) ListFiles(path string) ([]FileInfo, error) {
	klog.V(4).Infof("Listing files in %s", path)

	// SECURITY: Validate path to prevent command injection
	if err := utils.ValidateFilePath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Build /file print command
	// Use "where name~" for pattern matching (~ is RouterOS regex match operator)
	// RouterOS file paths don't include leading /, so strip it if present
	searchPath := strings.TrimPrefix(path, "/")
	cmd := fmt.Sprintf(`/file print detail where name~"%s"`, regexp.QuoteMeta(searchPath))

	// Execute command
	output, err := c.runCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	// Parse file list
	files, err := parseFileList(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file list: %w", err)
	}

	return files, nil
}

// DeleteFile deletes a file on RDS
func (c *sshClient) DeleteFile(path string) error {
	klog.V(4).Infof("Deleting file: %s", path)

	// SECURITY: Validate path to prevent command injection
	if err := utils.ValidateFilePath(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// RouterOS file paths don't include leading / in commands
	searchPath := strings.TrimPrefix(path, "/")

	// Build /file remove command
	cmd := fmt.Sprintf(`/file remove [find name="%s"]`, searchPath)

	// Execute command
	output, err := c.runCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// RouterOS doesn't return output on successful delete, but check for errors
	if strings.Contains(strings.ToLower(output), "error") || strings.Contains(strings.ToLower(output), "failure") {
		return fmt.Errorf("error deleting file: %s", output)
	}

	klog.V(4).Infof("Successfully deleted file: %s", path)
	return nil
}

// parseVolumeInfo parses RouterOS disk print output for a single volume
func parseVolumeInfo(output string) (*VolumeInfo, error) {
	volume := &VolumeInfo{}

	// Normalize multi-line output: join continuation lines (lines starting with spaces)
	normalized := normalizeRouterOSOutput(output)

	// Extract slot
	if match := regexp.MustCompile(`slot="([^"]+)"`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.Slot = match[1]
	} else if match := regexp.MustCompile(`slot=([^\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.Slot = match[1]
	}

	// Extract type
	if match := regexp.MustCompile(`type="?([^"\s]+)"?`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.Type = match[1]
	}

	// Extract file-path (can be quoted or with equals sign in path)
	// Match: file-path=/path/to/file.img or file-path="/path/to/file.img"
	if match := regexp.MustCompile(`file-path=([^\s]+\.img)`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.FilePath = match[1]
	} else if match := regexp.MustCompile(`file-path="([^"]+)"`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.FilePath = match[1]
	}
	// Normalize to absolute path format
	if volume.FilePath != "" && !strings.HasPrefix(volume.FilePath, "/") {
		volume.FilePath = "/" + volume.FilePath
	}

	// Extract file-size (human-readable format like "50.0GiB" or "1024.0MiB")
	// This is more reliable than the raw size field with spaces
	if match := regexp.MustCompile(`file-size=([\d.]+)\s*([KMGT]i?B)`).FindStringSubmatch(normalized); len(match) > 2 {
		if bytes, err := parseSize(match[1], match[2]); err == nil {
			volume.FileSizeBytes = bytes
		}
	} else {
		// Fallback: try to parse raw size field (with spaces removed)
		if match := regexp.MustCompile(`size=([\d\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
			// Remove all spaces from the number
			sizeStr := strings.ReplaceAll(match[1], " ", "")
			if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				volume.FileSizeBytes = size
			}
		}
	}

	// Extract nvme-tcp-export
	if match := regexp.MustCompile(`nvme-tcp-export=(yes|no)`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.NVMETCPExport = match[1] == "yes"
	}

	// Extract nvme-tcp-server-port
	if match := regexp.MustCompile(`nvme-tcp-server-port=(\d+)`).FindStringSubmatch(normalized); len(match) > 1 {
		if port, err := strconv.Atoi(match[1]); err == nil {
			volume.NVMETCPPort = port
		}
	}

	// Extract nvme-tcp-server-nqn
	if match := regexp.MustCompile(`nvme-tcp-server-nqn="([^"]+)"`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.NVMETCPNQN = match[1]
	}

	// Extract status (if available)
	// Note: Real RouterOS doesn't always provide a status field for file-backed disks
	if match := regexp.MustCompile(`status="?([^"\s]+)"?`).FindStringSubmatch(normalized); len(match) > 1 {
		volume.Status = match[1]
	} else {
		// For file-backed volumes with nvme-tcp-export=yes, assume "ready"
		if volume.Type == "file" && volume.NVMETCPExport {
			volume.Status = "ready"
		} else {
			volume.Status = "unknown"
		}
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

	// Normalize multi-line output
	normalized := normalizeRouterOSOutput(output)

	// RouterOS /file print detail output format uses space-separated numbers:
	// size=7 681 574 174 720 free=7 301 927 047 168 use=5%

	// Extract size (total capacity) - numbers may have spaces
	if match := regexp.MustCompile(`size=([\d\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
		sizeStr := strings.ReplaceAll(match[1], " ", "")
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			capacity.TotalBytes = size
		}
	}

	// Extract free capacity - numbers may have spaces
	if match := regexp.MustCompile(`free=([\d\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
		freeStr := strings.ReplaceAll(match[1], " ", "")
		if free, err := strconv.ParseInt(freeStr, 10, 64); err == nil {
			capacity.FreeBytes = free
		}
	}

	// Calculate used capacity
	if capacity.TotalBytes > 0 && capacity.FreeBytes > 0 {
		capacity.UsedBytes = capacity.TotalBytes - capacity.FreeBytes
	}

	// If we didn't get values, return error
	if capacity.TotalBytes == 0 {
		return nil, fmt.Errorf("could not parse capacity from output")
	}

	return capacity, nil
}

// parseFileList parses RouterOS file print output for multiple files
func parseFileList(output string) ([]FileInfo, error) {
	var files []FileInfo

	// Normalize multi-line output
	normalized := normalizeRouterOSOutput(output)

	// Split by file entries (each starts with a number)
	entries := regexp.MustCompile(`(?m)^\s*\d+\s+`).Split(normalized, -1)

	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}

		file, err := parseFileInfo(entry)
		if err != nil {
			klog.V(4).Infof("Skipping unparseable file entry: %v", err)
			continue
		}

		files = append(files, *file)
	}

	return files, nil
}

// parseFileInfo parses RouterOS file print output for a single file
func parseFileInfo(output string) (*FileInfo, error) {
	file := &FileInfo{}

	// Normalize multi-line output
	normalized := normalizeRouterOSOutput(output)

	// Extract name (path)
	if match := regexp.MustCompile(`name="([^"]+)"`).FindStringSubmatch(normalized); len(match) > 1 {
		file.Path = match[1]
	} else if match := regexp.MustCompile(`name=([^\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
		file.Path = match[1]
	}

	// Normalize to absolute path format
	if file.Path != "" && !strings.HasPrefix(file.Path, "/") {
		file.Path = "/" + file.Path
	}

	// Extract filename from path
	if file.Path != "" {
		parts := strings.Split(file.Path, "/")
		if len(parts) > 0 {
			file.Name = parts[len(parts)-1]
		}
	}

	// Extract type
	if match := regexp.MustCompile(`type="?([^"\s]+)"?`).FindStringSubmatch(normalized); len(match) > 1 {
		file.Type = match[1]
	}

	// Extract size (numbers may have spaces like "10 737 418 240")
	if match := regexp.MustCompile(`size=([\d\s]+)`).FindStringSubmatch(normalized); len(match) > 1 {
		sizeStr := strings.ReplaceAll(match[1], " ", "")
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			file.SizeBytes = size
		}
	}

	// Extract creation time (if available)
	// RouterOS format: creation-time=jan/02/2025 10:30:45
	if match := regexp.MustCompile(`creation-time=(\w+/\d+/\d+\s+\d+:\d+:\d+)`).FindStringSubmatch(normalized); len(match) > 1 {
		// Parse RouterOS time format
		if t, err := time.Parse("jan/02/2006 15:04:05", match[1]); err == nil {
			file.CreatedAt = t
		}
	}

	return file, nil
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

	// SECURITY: Validate file path to prevent command injection and path traversal
	if err := utils.ValidateFilePath(opts.FilePath); err != nil {
		return fmt.Errorf("security validation failed for file path: %w", err)
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

// normalizeRouterOSOutput normalizes multi-line RouterOS output by joining continuation lines
// RouterOS CLI output often spans multiple lines with properties wrapped across lines.
// Continuation lines start with whitespace. This function joins them into a single line.
func normalizeRouterOSOutput(output string) string {
	lines := strings.Split(output, "\n")
	var normalized strings.Builder

	for _, line := range lines {
		// Remove \r if present (Windows-style line endings)
		line = strings.TrimRight(line, "\r")

		// Skip the "Flags:" header lines
		if strings.HasPrefix(line, "Flags:") || strings.Contains(line, "disabled") {
			continue
		}

		// If line starts with whitespace (continuation line), append to current line with space
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			normalized.WriteString(" ")
			normalized.WriteString(strings.TrimSpace(line))
		} else {
			// New entry - add newline before (except for first line)
			if normalized.Len() > 0 {
				normalized.WriteString("\n")
			}
			normalized.WriteString(line)
		}
	}

	return normalized.String()
}

// extractMountPoint extracts the mount point from a full path
// Examples:
//
//	/storage-pool/metal-csi/volumes → storage-pool
//	/nvme1/kubernetes → nvme1
//	storage-pool/volumes → storage-pool
func extractMountPoint(path string) string {
	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	// Split by slash and take first component
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return path
}
