package attachment

import "time"

// AttachmentState represents a tracked volume-to-node binding
// This struct stores the current attachment state of a volume,
// tracking which node it's attached to and when the attachment occurred.
type AttachmentState struct {
	// VolumeID is the CSI volume identifier (e.g., "pvc-uuid")
	VolumeID string

	// NodeID is the Kubernetes node identifier where the volume is attached
	NodeID string

	// AttachedAt is the timestamp when the volume was attached to the node
	AttachedAt time.Time
}
