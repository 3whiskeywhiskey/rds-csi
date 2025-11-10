package utils

import (
	"fmt"
	"net"
	"regexp"
	"strconv"

	"github.com/google/uuid"
)

const (
	// VolumeIDPrefix is prepended to all volume IDs
	VolumeIDPrefix = "pvc-"

	// NQNPrefix is the NVMe Qualified Name prefix for MikroTik
	NQNPrefix = "nqn.2000-02.com.mikrotik"
)

var (
	// volumeIDPattern matches valid volume IDs (pvc-<uuid>)
	volumeIDPattern = regexp.MustCompile(`^pvc-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

	// safeSlotPattern matches safe slot names (alphanumeric and hyphen only)
	safeSlotPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

	// Namespace UUID for generating deterministic volume IDs
	volumeNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // DNS namespace UUID
)

// GenerateVolumeID generates a new unique volume ID
func GenerateVolumeID() string {
	return VolumeIDPrefix + uuid.New().String()
}

// VolumeNameToID generates a deterministic volume ID from a volume name
// This ensures the same name always produces the same ID (for idempotency)
func VolumeNameToID(name string) string {
	// Use UUID v5 (SHA-1 based) to generate deterministic UUID from name
	id := uuid.NewSHA1(volumeNamespace, []byte(name))
	return VolumeIDPrefix + id.String()
}

// ValidateVolumeID validates that a volume ID is in the correct format
func ValidateVolumeID(volumeID string) error {
	if volumeID == "" {
		return fmt.Errorf("volume ID cannot be empty")
	}

	if !volumeIDPattern.MatchString(volumeID) {
		return fmt.Errorf("invalid volume ID format: %s (expected pvc-<uuid>)", volumeID)
	}

	return nil
}

// ValidateSlotName validates that a slot name is safe (prevents command injection)
func ValidateSlotName(slot string) error {
	if slot == "" {
		return fmt.Errorf("slot name cannot be empty")
	}

	if !safeSlotPattern.MatchString(slot) {
		return fmt.Errorf("invalid slot name: %s (only alphanumeric and hyphen allowed)", slot)
	}

	return nil
}

// VolumeIDToNQN converts a volume ID to an NVMe Qualified Name
func VolumeIDToNQN(volumeID string) (string, error) {
	if err := ValidateVolumeID(volumeID); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", NQNPrefix, volumeID), nil
}

// VolumeIDToFilePath generates the file path for a volume
func VolumeIDToFilePath(volumeID, basePath string) (string, error) {
	if err := ValidateVolumeID(volumeID); err != nil {
		return "", err
	}

	if basePath == "" {
		basePath = "/storage-pool/kubernetes-volumes"
	}

	return fmt.Sprintf("%s/%s.img", basePath, volumeID), nil
}

// ExtractVolumeIDFromNQN extracts the volume ID from an NQN
func ExtractVolumeIDFromNQN(nqn string) (string, error) {
	expectedPrefix := NQNPrefix + ":"
	if len(nqn) <= len(expectedPrefix) {
		return "", fmt.Errorf("invalid NQN format: %s", nqn)
	}

	if nqn[:len(expectedPrefix)] != expectedPrefix {
		return "", fmt.Errorf("NQN does not have expected prefix: %s", nqn)
	}

	volumeID := nqn[len(expectedPrefix):]
	if err := ValidateVolumeID(volumeID); err != nil {
		return "", fmt.Errorf("invalid volume ID in NQN: %w", err)
	}

	return volumeID, nil
}

// ValidateIPAddress validates that a string is a valid IPv4 or IPv6 address
func ValidateIPAddress(address string) error {
	if address == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	// Try to parse as IP
	ip := net.ParseIP(address)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", address)
	}

	return nil
}

// ValidatePort validates that a port number is in valid range
// Optionally checks against privileged port range (< 1024)
func ValidatePort(port int, allowPrivileged bool) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be in range 1-65535: got %d", port)
	}

	if !allowPrivileged && port < 1024 {
		return fmt.Errorf("privileged port (< 1024) not allowed: %d", port)
	}

	return nil
}

// ValidatePortString validates a port string and returns the port number
func ValidatePortString(portStr string, allowPrivileged bool) (int, error) {
	if portStr == "" {
		return 0, fmt.Errorf("port cannot be empty")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port format: %s", portStr)
	}

	if err := ValidatePort(port, allowPrivileged); err != nil {
		return 0, err
	}

	return port, nil
}

// ValidateNVMEAddress validates an NVMe target address (IP:Port combination)
func ValidateNVMEAddress(address string, port int) error {
	// Validate IP address
	if err := ValidateIPAddress(address); err != nil {
		return fmt.Errorf("invalid NVMe address: %w", err)
	}

	// Validate port (NVMe/TCP typically uses non-privileged ports)
	if err := ValidatePort(port, true); err != nil {
		return fmt.Errorf("invalid NVMe port: %w", err)
	}

	return nil
}

// ValidateNVMETargetContext validates volume context parameters for NVMe/TCP
// This includes NQN, address, and port validation
func ValidateNVMETargetContext(nqn, address string, port int, expectedAddress string) error {
	// Validate NQN (using existing function if available)
	if nqn == "" {
		return fmt.Errorf("NQN cannot be empty")
	}

	// Validate address and port
	if err := ValidateNVMEAddress(address, port); err != nil {
		return err
	}

	// Optionally verify address matches expected RDS address
	if expectedAddress != "" && address != expectedAddress {
		return fmt.Errorf("NVMe address %s does not match expected RDS address %s", address, expectedAddress)
	}

	return nil
}
