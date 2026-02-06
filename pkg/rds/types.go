package rds

import (
	"fmt"
	"time"
)

// VolumeInfo represents an RDS volume
type VolumeInfo struct {
	Slot          string // Volume identifier (slot name)
	Type          string // "file" for file-backed volumes
	FilePath      string // Path to backing file
	FileSizeBytes int64  // Size in bytes
	NVMETCPExport bool   // Whether NVMe/TCP export is enabled
	NVMETCPPort   int    // NVMe/TCP server port
	NVMETCPNQN    string // NVMe Qualified Name
	Status        string // "ready", "formatting", "error"
}

// CapacityInfo represents filesystem capacity information
type CapacityInfo struct {
	TotalBytes     int64
	FreeBytes      int64
	UsedBytes      int64
	LastUpdateTime time.Time
}

// CreateVolumeOptions contains parameters for creating a volume
type CreateVolumeOptions struct {
	Slot          string // Unique identifier (pvc-<uuid>)
	FilePath      string // Full path to backing file
	FileSizeBytes int64  // Size in bytes
	NVMETCPPort   int    // NVMe/TCP port (default 4420)
	NVMETCPNQN    string // NVMe Qualified Name
}

// FileInfo represents a file on the RDS filesystem
type FileInfo struct {
	Name      string    // File name
	Path      string    // Full path to file
	SizeBytes int64     // Size in bytes
	Type      string    // "file" or "directory"
	CreatedAt time.Time // Creation time (if available)
}

// VolumeNotFoundError is returned when a volume is not found
type VolumeNotFoundError struct {
	Slot string
}

func (e *VolumeNotFoundError) Error() string {
	return fmt.Sprintf("volume not found: %s", e.Slot)
}

// SnapshotInfo represents an RDS Btrfs snapshot
type SnapshotInfo struct {
	Name          string    // Snapshot name (snap-<uuid>)
	SourceVolume  string    // Source volume slot (pvc-<uuid>)
	FileSizeBytes int64     // Size of snapshot (same as source volume)
	CreatedAt     time.Time // Creation timestamp
	ReadOnly      bool      // Should always be true for snapshots
	FSLabel       string    // Btrfs filesystem label
}

// CreateSnapshotOptions contains parameters for creating a snapshot
type CreateSnapshotOptions struct {
	Name         string // snap-<uuid>
	SourceVolume string // pvc-<uuid> (parent subvolume)
	FSLabel      string // Btrfs filesystem label
}

// SnapshotNotFoundError is returned when a snapshot is not found
type SnapshotNotFoundError struct {
	Name string
}

func (e *SnapshotNotFoundError) Error() string {
	return fmt.Sprintf("snapshot not found: %s", e.Name)
}
