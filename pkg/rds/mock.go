package rds

import (
	"fmt"
	"sync"
)

// MockClient is a mock implementation of RDSClient for testing
type MockClient struct {
	mu      sync.RWMutex
	volumes map[string]*VolumeInfo
	address string
}

// NewMockClient creates a new MockClient for testing
func NewMockClient() *MockClient {
	return &MockClient{
		volumes: make(map[string]*VolumeInfo),
		address: "mock-rds-server",
	}
}

// AddVolume adds a volume to the mock (test helper)
func (m *MockClient) AddVolume(v *VolumeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volumes[v.Slot] = v
}

// RemoveVolume removes a volume from the mock (test helper)
func (m *MockClient) RemoveVolume(slot string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.volumes, slot)
}

// SetAddress sets the mock address (test helper)
func (m *MockClient) SetAddress(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.address = addr
}

// Connect implements RDSClient
func (m *MockClient) Connect() error {
	return nil
}

// Close implements RDSClient
func (m *MockClient) Close() error {
	return nil
}

// IsConnected implements RDSClient
func (m *MockClient) IsConnected() bool {
	return true
}

// GetAddress implements RDSClient
func (m *MockClient) GetAddress() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.address
}

// CreateVolume implements RDSClient
func (m *MockClient) CreateVolume(opts CreateVolumeOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.volumes[opts.Slot]; exists {
		return fmt.Errorf("volume %s already exists", opts.Slot)
	}

	m.volumes[opts.Slot] = &VolumeInfo{
		Slot:          opts.Slot,
		Type:          "file",
		FilePath:      opts.FilePath,
		FileSizeBytes: opts.FileSizeBytes,
		NVMETCPExport: true,
		NVMETCPPort:   opts.NVMETCPPort,
		NVMETCPNQN:    opts.NVMETCPNQN,
		Status:        "ready",
	}
	return nil
}

// DeleteVolume implements RDSClient
func (m *MockClient) DeleteVolume(slot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.volumes[slot]; !exists {
		// Idempotent - not an error if doesn't exist
		return nil
	}

	delete(m.volumes, slot)
	return nil
}

// ResizeVolume implements RDSClient
func (m *MockClient) ResizeVolume(slot string, newSizeBytes int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vol, exists := m.volumes[slot]
	if !exists {
		return &VolumeNotFoundError{Slot: slot}
	}

	vol.FileSizeBytes = newSizeBytes
	return nil
}

// GetVolume implements RDSClient
func (m *MockClient) GetVolume(slot string) (*VolumeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vol, exists := m.volumes[slot]
	if !exists {
		return nil, &VolumeNotFoundError{Slot: slot}
	}

	// Return a copy to prevent mutation
	copy := *vol
	return &copy, nil
}

// VerifyVolumeExists implements RDSClient
func (m *MockClient) VerifyVolumeExists(slot string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.volumes[slot]; !exists {
		return &VolumeNotFoundError{Slot: slot}
	}
	return nil
}

// ListVolumes implements RDSClient
func (m *MockClient) ListVolumes() ([]VolumeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]VolumeInfo, 0, len(m.volumes))
	for _, vol := range m.volumes {
		result = append(result, *vol)
	}
	return result, nil
}

// ListFiles implements RDSClient
func (m *MockClient) ListFiles(path string) ([]FileInfo, error) {
	return nil, nil
}

// DeleteFile implements RDSClient
func (m *MockClient) DeleteFile(path string) error {
	return nil
}

// GetCapacity implements RDSClient
func (m *MockClient) GetCapacity(basePath string) (*CapacityInfo, error) {
	return &CapacityInfo{
		TotalBytes: 1024 * 1024 * 1024 * 1024, // 1 TiB
		FreeBytes:  512 * 1024 * 1024 * 1024,  // 512 GiB
		UsedBytes:  512 * 1024 * 1024 * 1024,  // 512 GiB
	}, nil
}
