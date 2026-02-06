package rds

import (
	"fmt"
	"sync"
)

// MockClient is a mock implementation of RDSClient for testing
type MockClient struct {
	mu            sync.RWMutex
	volumes       map[string]*VolumeInfo
	address       string
	connected     bool  // Connection state (for testing connection manager)
	nextError     error // Error to return on next operation
	persistentErr error // Error to return on all operations until cleared
}

// NewMockClient creates a new MockClient for testing
func NewMockClient() *MockClient {
	return &MockClient{
		volumes:   make(map[string]*VolumeInfo),
		address:   "mock-rds-server",
		connected: true, // Default to connected
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

// SetError sets an error to be returned on the next operation (test helper)
func (m *MockClient) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextError = err
}

// SetPersistentError sets an error to be returned on ALL operations until cleared (test helper)
func (m *MockClient) SetPersistentError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistentErr = err
}

// ClearError clears any pending error (test helper)
func (m *MockClient) ClearError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextError = nil
	m.persistentErr = nil
}

// SetConnected sets the connection state (test helper)
func (m *MockClient) SetConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = connected
}

// checkError checks for and clears pending error
func (m *MockClient) checkError() error {
	// Check persistent error first
	if m.persistentErr != nil {
		return m.persistentErr
	}
	// Then check one-time error
	if m.nextError != nil {
		err := m.nextError
		m.nextError = nil
		return err
	}
	return nil
}

// Connect implements RDSClient
func (m *MockClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for persistent error (simulates connection failure)
	if m.persistentErr != nil {
		return m.persistentErr
	}

	// Mark as connected on successful connect
	m.connected = true
	return nil
}

// Close implements RDSClient
func (m *MockClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected implements RDSClient
func (m *MockClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
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

	// Check for pending error
	if err := m.checkError(); err != nil {
		return err
	}

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

	// Check for pending error
	if err := m.checkError(); err != nil {
		return err
	}

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

	// Check for pending error
	if err := m.checkError(); err != nil {
		return err
	}

	vol, exists := m.volumes[slot]
	if !exists {
		return &VolumeNotFoundError{Slot: slot}
	}

	vol.FileSizeBytes = newSizeBytes
	return nil
}

// GetVolume implements RDSClient
func (m *MockClient) GetVolume(slot string) (*VolumeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for pending error
	if err := m.checkError(); err != nil {
		return nil, err
	}

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
