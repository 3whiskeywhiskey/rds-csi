package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// SnapshotIDPrefix is prepended to all snapshot IDs
	SnapshotIDPrefix = "snap-"
)

var (
	// snapshotIDPattern matches the production format:
	// snap-<source-uuid>-at-<suffix>
	// where suffix is either a Unix timestamp (digits) or a deterministic hex hash (hex chars).
	//
	// Examples:
	//   snap-a1b2c3d4-e5f6-7890-abcd-ef1234567890-at-1739800000   (timestamp-based)
	//   snap-a1b2c3d4-e5f6-7890-abcd-ef1234567890-at-3a9f8c02d1   (deterministic hash)
	snapshotIDPattern = regexp.MustCompile(`^snap-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}-at-[a-f0-9]+$`)

	// snapshotIDLegacyPattern matches the old format (snap-<uuid> without -at-<suffix>)
	// kept for backward compatibility validation during migration
	snapshotIDLegacyPattern = regexp.MustCompile(`^snap-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

// GenerateSnapshotID generates a deterministic snapshot ID from the CSI snapshot name,
// satisfying CSI idempotency and uniqueness requirements.
//
// Format: snap-<uuid5-of-name>-at-<first10hex>
// Example: snap-a1b2c3d4-e5f6-7890-abcd-ef1234567890-at-3a9f8c02d1
//
// The snapshot ID is derived solely from the CSI snapshot name (not the source volume).
// This is required by the CSI spec: snapshot names are the unique identity — the same
// CSI name MUST produce the same snapshot ID regardless of the source volume. This
// allows the controller to detect "same name, different source" conflicts by looking up
// the ID and comparing the stored source volume.
//
// The sourceVolumeID parameter is accepted for backward compatibility but not used in
// ID generation. Source volume verification is the caller's responsibility.
//
// Use this function in CreateSnapshot to generate stable, idempotent snapshot IDs.
func GenerateSnapshotID(csiName string, sourceVolumeID string) string {
	// Generate a deterministic UUID from the CSI snapshot name using UUID v5 (SHA1).
	// The snapshot ID is based only on the CSI name per CSI spec requirements.
	nameUUID := uuid.NewSHA1(volumeNamespace, []byte(csiName))

	// Take the first 10 hex characters of the UUID (strip dashes, take prefix)
	nameHex := strings.ReplaceAll(nameUUID.String(), "-", "")
	suffix := nameHex[:10]

	return fmt.Sprintf("snap-%s-at-%s", nameUUID.String(), suffix)
}

// GenerateSnapshotIDFromSource generates a snapshot ID embedding the source volume UUID
// and the current Unix timestamp, producing a unique ID per creation call.
//
// NOTE: This function is NOT idempotent — each call at a different second produces a
// different ID. Use GenerateSnapshotID(csiName, sourceVolumeID) for CSI-compliant
// idempotent snapshot creation.
//
// Format: snap-<source-uuid>-at-<unix-timestamp>
// Example: snap-a1b2c3d4-e5f6-7890-abcd-ef1234567890-at-1739800000
func GenerateSnapshotIDFromSource(sourceVolumeID string) string {
	// Strip the "pvc-" prefix to get the UUID part
	sourceUUID := strings.TrimPrefix(sourceVolumeID, VolumeIDPrefix)
	timestamp := time.Now().Unix()
	return fmt.Sprintf("snap-%s-at-%d", sourceUUID, timestamp)
}

// ExtractSourceVolumeIDFromSnapshotID parses a snapshot ID and returns the source volume ID.
//
// Deprecated: As of the name-based snapshot ID generation change, the UUID portion of the
// snapshot ID is no longer the source volume UUID — it is derived from the CSI snapshot name.
// This function is kept for backward compatibility but should NOT be used to recover source
// volume information. Use the source-volume field from the snapshot disk entry instead.
//
// Snapshot ID format: snap-<uuid5-of-name>-at-<suffix>
// Returns an error if the ID doesn't match the expected format.
func ExtractSourceVolumeIDFromSnapshotID(snapshotID string) (string, error) {
	// Must start with "snap-"
	if !strings.HasPrefix(snapshotID, SnapshotIDPrefix) {
		return "", fmt.Errorf("snapshot ID %q does not have snap- prefix", snapshotID)
	}

	// Strip "snap-" prefix → "<uuid>-at-<suffix>"
	rest := strings.TrimPrefix(snapshotID, SnapshotIDPrefix)

	// UUID part is exactly 36 chars: 8-4-4-4-12 = 32 hex + 4 hyphens
	const uuidLen = 36
	if len(rest) <= uuidLen+4 { // 4 = len("-at-")
		return "", fmt.Errorf("snapshot ID %q does not match expected format snap-<uuid>-at-<suffix>", snapshotID)
	}

	separator := rest[uuidLen:]
	if !strings.HasPrefix(separator, "-at-") {
		return "", fmt.Errorf("snapshot ID %q does not contain -at- separator after UUID", snapshotID)
	}

	sourceUUID := rest[:uuidLen]
	// Validate UUID portion
	uuidPattern := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	if !uuidPattern.MatchString(sourceUUID) {
		return "", fmt.Errorf("snapshot ID %q contains invalid UUID portion: %q", snapshotID, sourceUUID)
	}

	return VolumeIDPrefix + sourceUUID, nil
}

// ExtractTimestampFromSnapshotID parses the -at-<timestamp> suffix from a snapshot ID.
// Returns the Unix timestamp as int64. Only valid for timestamp-based IDs generated by
// GenerateSnapshotIDFromSource (not for deterministic hash IDs from GenerateSnapshotID).
func ExtractTimestampFromSnapshotID(snapshotID string) (int64, error) {
	atIdx := strings.LastIndex(snapshotID, "-at-")
	if atIdx < 0 {
		return 0, fmt.Errorf("snapshot ID %q does not contain -at- suffix", snapshotID)
	}

	timestampStr := snapshotID[atIdx+4:] // +4 to skip "-at-"
	if timestampStr == "" {
		return 0, fmt.Errorf("snapshot ID %q has empty timestamp after -at-", snapshotID)
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("snapshot ID %q has non-numeric timestamp %q: %w", snapshotID, timestampStr, err)
	}

	return ts, nil
}

// ValidateSnapshotID validates that a snapshot ID is safe for use in commands.
//
// Accepted formats:
//  1. Production format:  snap-<lowercase-uuid>-at-<suffix> (digits or hex hash)
//  2. Legacy format:      snap-<lowercase-uuid>  (for backward compatibility)
//  3. CSI sanity test IDs: alphanumeric + hyphens, no snap- prefix
//
// SECURITY: Prevents command injection by restricting to safe characters only.
func ValidateSnapshotID(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("snapshot ID cannot be empty")
	}

	// Check production format (timestamp-based or deterministic hash suffix)
	if snapshotIDPattern.MatchString(snapshotID) {
		return nil // Valid production snapshot ID
	}

	// Check legacy format (snap-<uuid> without suffix, for backward compat)
	if snapshotIDLegacyPattern.MatchString(snapshotID) {
		return nil // Valid legacy snapshot ID
	}

	// For safety, reject anything with special characters first
	if !safeSlotPattern.MatchString(snapshotID) {
		return fmt.Errorf("invalid snapshot ID format: %s (only alphanumeric and hyphen allowed)", snapshotID)
	}

	// Reject if it starts with "snap-" but doesn't match either known format
	if strings.HasPrefix(snapshotID, SnapshotIDPrefix) {
		return fmt.Errorf("invalid snapshot ID format: %s (expected snap-<lowercase-uuid>-at-<suffix>)", snapshotID)
	}

	// Reject UUID-like strings without proper prefix (e.g., "12345678-1234-...")
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

// GenerateSnapshotIDLegacy generates a new unique snapshot ID using a random UUID.
// Deprecated: Use GenerateSnapshotID(csiName, sourceVolumeID) for new code.
// Kept for backward compatibility.
func GenerateSnapshotIDLegacy() string {
	return SnapshotIDPrefix + uuid.New().String()
}

// SnapshotNameToID generates a deterministic snapshot ID from a snapshot name.
// Deprecated: Use GenerateSnapshotID(csiName, sourceVolumeID) which also embeds
// source volume lineage. This function is kept for backward compatibility.
func SnapshotNameToID(name string) string {
	id := uuid.NewSHA1(volumeNamespace, []byte(name))
	return SnapshotIDPrefix + id.String()
}
