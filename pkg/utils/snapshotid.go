package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	// SnapshotIDPrefix is prepended to all snapshot IDs
	SnapshotIDPrefix = "snap-"
)

var (
	// snapshotIDPattern matches strict UUID format with snap- prefix
	// Format: snap-<lowercase-uuid>
	// Example: snap-12345678-1234-1234-1234-123456789abc
	snapshotIDPattern = regexp.MustCompile(`^snap-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

// GenerateSnapshotID generates a new unique snapshot ID
func GenerateSnapshotID() string {
	return SnapshotIDPrefix + uuid.New().String()
}

// SnapshotNameToID generates a deterministic snapshot ID from a snapshot name
// This ensures the same name always produces the same ID (for idempotency)
func SnapshotNameToID(name string) string {
	// Use UUID v5 (SHA-1 based) to generate deterministic UUID from name
	// Reuse volumeNamespace since volume names and snapshot names are inherently different strings
	id := uuid.NewSHA1(volumeNamespace, []byte(name))
	return SnapshotIDPrefix + id.String()
}

// ValidateSnapshotID validates that a snapshot ID is safe for use in commands
// For production snapshot IDs: must match "snap-<lowercase-uuid>" format
// For CSI sanity tests: accepts alphanumeric with hyphens (safe pattern) but not UUID-like strings
// SECURITY: Prevents command injection by restricting to safe characters only
func ValidateSnapshotID(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("snapshot ID cannot be empty")
	}

	// Check if it matches strict UUID pattern (production snapshot IDs)
	if snapshotIDPattern.MatchString(snapshotID) {
		return nil // Valid production snapshot ID
	}

	// For safety, reject anything with special characters first
	if !safeSlotPattern.MatchString(snapshotID) {
		return fmt.Errorf("invalid snapshot ID format: %s (only alphanumeric and hyphen allowed)", snapshotID)
	}

	// Reject if it starts with "snap-" but doesn't match UUID format
	// This catches malformed production IDs like "snap-invalid" or "snap-UPPERCASE"
	if strings.HasPrefix(snapshotID, SnapshotIDPrefix) {
		return fmt.Errorf("invalid snapshot ID format: %s (expected snap-<lowercase-uuid>)", snapshotID)
	}

	// Reject UUID-like strings without proper prefix (e.g., "12345678-1234-...")
	// This pattern detects UUID format without "snap-" prefix
	uuidLikePattern := regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	if uuidLikePattern.MatchString(snapshotID) {
		return fmt.Errorf("invalid snapshot ID format: %s (missing snap- prefix)", snapshotID)
	}

	// Additional length check to prevent excessively long snapshot IDs
	if len(snapshotID) > 250 {
		return fmt.Errorf("snapshot ID too long: %d characters (max 250)", len(snapshotID))
	}

	return nil // Valid CSI sanity test ID (alphanumeric, no snap-, not UUID-like)
}
