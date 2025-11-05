package rds

import "time"

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
