package utils

import (
	"fmt"
	"regexp"
	"strings"

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

	// nqnPattern matches valid NVMe Qualified Names (NQN)
	// Format: nqn.YYYY-MM.reversed.domain:identifier
	// Example: nqn.2000-02.com.mikrotik:pvc-12345678-1234-1234-1234-123456789abc
	// SECURITY: This strict pattern prevents command injection via NQN parameter
	nqnPattern = regexp.MustCompile(`^nqn\.[0-9]{4}-[0-9]{2}\.[a-z0-9.-]+:[a-z0-9._-]+$`)

	// Namespace UUID for generating deterministic volume IDs
	volumeNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // DNS namespace UUID
)

// Shell metacharacters that are dangerous in NQN context
var dangerousNQNChars = []string{
	";", "|", "&", "$", "`", "(", ")", "<", ">", "\n", "\r", " ", "\t", "\"", "'", "\\", "*", "?", "[", "]",
}

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

// ValidateNQN validates that an NQN (NVMe Qualified Name) is in the correct format
// and does not contain dangerous characters that could enable command injection
func ValidateNQN(nqn string) error {
	if nqn == "" {
		return fmt.Errorf("NQN cannot be empty")
	}

	// SECURITY: Check for dangerous shell metacharacters first
	for _, char := range dangerousNQNChars {
		if strings.Contains(nqn, char) {
			return fmt.Errorf("NQN contains dangerous character %q: %s", char, nqn)
		}
	}

	// Validate NQN format using strict regex
	if !nqnPattern.MatchString(nqn) {
		return fmt.Errorf("invalid NQN format: %s (expected format: nqn.YYYY-MM.domain:identifier)", nqn)
	}

	// Additional length check to prevent excessively long NQNs
	if len(nqn) > 223 {
		// NVMe spec limits NQN to 223 bytes (NVM Express 1.3 spec)
		return fmt.Errorf("NQN too long: %d bytes (max 223)", len(nqn))
	}

	return nil
}

// VolumeIDToNQN converts a volume ID to an NVMe Qualified Name
func VolumeIDToNQN(volumeID string) (string, error) {
	if err := ValidateVolumeID(volumeID); err != nil {
		return "", err
	}

	nqn := fmt.Sprintf("%s:%s", NQNPrefix, volumeID)

	// SECURITY: Validate the generated NQN before returning
	if err := ValidateNQN(nqn); err != nil {
		return "", fmt.Errorf("generated NQN failed validation: %w", err)
	}

	return nqn, nil
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
