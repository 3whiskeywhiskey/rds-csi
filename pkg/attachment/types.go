package attachment

import "time"

// NodeAttachment represents a single node's attachment to a volume.
// Used within AttachmentState.Nodes to track attachment order.
type NodeAttachment struct {
	// NodeID is the Kubernetes node identifier
	NodeID string

	// AttachedAt is when this specific node attached
	AttachedAt time.Time
}

// AttachmentState represents a tracked volume-to-node binding.
// For RWO volumes, Nodes will have at most 1 entry.
// For RWX volumes during migration, Nodes can have up to 2 entries.
type AttachmentState struct {
	// VolumeID is the CSI volume identifier (e.g., "pvc-uuid")
	VolumeID string

	// NodeID is the primary node (first attached) - kept for backward compatibility
	// Deprecated: Use Nodes[0].NodeID instead
	NodeID string

	// Nodes tracks all attached nodes in attachment order.
	// Index 0 = primary (first attached), Index 1 = secondary (migration target)
	// For RWO: len(Nodes) <= 1
	// For RWX during migration: len(Nodes) <= 2
	Nodes []NodeAttachment

	// AttachedAt is the timestamp when the volume was first attached
	AttachedAt time.Time

	// DetachedAt is the timestamp when the volume was detached.
	// nil if volume is currently attached. Used for grace period calculation.
	DetachedAt *time.Time

	// AccessMode tracks whether this is RWO or RWX attachment
	// Needed to determine if dual-attach is allowed
	AccessMode string // "RWO" or "RWX"

	// MigrationStartedAt is when dual-attach began (secondary node attached).
	// nil if not currently in migration state. Used for timeout calculation.
	MigrationStartedAt *time.Time

	// MigrationTimeout is the maximum duration allowed for migration dual-attach.
	// Parsed from StorageClass parameter migrationTimeoutSeconds.
	// Zero value means use default (5 minutes).
	MigrationTimeout time.Duration
}

// GetNodeIDs returns a slice of all attached node IDs.
func (as *AttachmentState) GetNodeIDs() []string {
	ids := make([]string, len(as.Nodes))
	for i, na := range as.Nodes {
		ids[i] = na.NodeID
	}
	return ids
}

// IsAttachedToNode checks if volume is attached to a specific node.
func (as *AttachmentState) IsAttachedToNode(nodeID string) bool {
	for _, na := range as.Nodes {
		if na.NodeID == nodeID {
			return true
		}
	}
	return false
}

// NodeCount returns the number of attached nodes.
func (as *AttachmentState) NodeCount() int {
	return len(as.Nodes)
}

// IsMigrating returns true if volume is in dual-attach migration state.
func (as *AttachmentState) IsMigrating() bool {
	return as.MigrationStartedAt != nil && len(as.Nodes) > 1
}

// IsMigrationTimedOut returns true if migration exceeded configured timeout.
// Returns false if not migrating or timeout is zero (disabled).
func (as *AttachmentState) IsMigrationTimedOut() bool {
	if as.MigrationStartedAt == nil || as.MigrationTimeout == 0 {
		return false
	}
	return time.Since(*as.MigrationStartedAt) > as.MigrationTimeout
}
