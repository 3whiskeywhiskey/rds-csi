package utils

import (
	"strings"
	"testing"
)

func TestGenerateVolumeID(t *testing.T) {
	id := GenerateVolumeID()

	if !strings.HasPrefix(id, VolumeIDPrefix) {
		t.Errorf("Generated volume ID does not have expected prefix: %s", id)
	}

	if err := ValidateVolumeID(id); err != nil {
		t.Errorf("Generated volume ID is invalid: %v", err)
	}

	// Generate another and ensure uniqueness
	id2 := GenerateVolumeID()
	if id == id2 {
		t.Error("Generated volume IDs are not unique")
	}
}

func TestValidateVolumeID(t *testing.T) {
	tests := []struct {
		name      string
		volumeID  string
		expectErr bool
	}{
		{
			name:      "valid volume ID",
			volumeID:  "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectErr: false,
		},
		{
			name:      "empty volume ID",
			volumeID:  "",
			expectErr: true,
		},
		{
			name:      "missing prefix",
			volumeID:  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectErr: true,
		},
		{
			name:      "invalid format",
			volumeID:  "pvc-invalid",
			expectErr: true,
		},
		{
			name:      "uppercase UUID",
			volumeID:  "pvc-A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
			expectErr: true,
		},
		{
			name:      "special characters",
			volumeID:  "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890; rm -rf /",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVolumeID(tt.volumeID)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSlotName(t *testing.T) {
	tests := []struct {
		name      string
		slot      string
		expectErr bool
	}{
		{
			name:      "valid slot",
			slot:      "pvc-abc123",
			expectErr: false,
		},
		{
			name:      "valid slot with hyphens",
			slot:      "pvc-a1b2c3d4-e5f6-7890",
			expectErr: false,
		},
		{
			name:      "empty slot",
			slot:      "",
			expectErr: true,
		},
		{
			name:      "slot with spaces",
			slot:      "pvc abc123",
			expectErr: true,
		},
		{
			name:      "slot with semicolon",
			slot:      "pvc-abc123; rm -rf /",
			expectErr: true,
		},
		{
			name:      "slot with pipe",
			slot:      "pvc-abc|evil",
			expectErr: true,
		},
		{
			name:      "slot with dollar sign",
			slot:      "pvc-$var",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlotName(tt.slot)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestVolumeIDToNQN(t *testing.T) {
	tests := []struct {
		name        string
		volumeID    string
		expectedNQN string
		expectErr   bool
	}{
		{
			name:        "valid volume ID",
			volumeID:    "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectedNQN: "nqn.2000-02.com.mikrotik:pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectErr:   false,
		},
		{
			name:      "invalid volume ID",
			volumeID:  "invalid",
			expectErr: true,
		},
		{
			name:      "empty volume ID",
			volumeID:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nqn, err := VolumeIDToNQN(tt.volumeID)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if nqn != tt.expectedNQN {
					t.Errorf("Expected NQN %s, got %s", tt.expectedNQN, nqn)
				}
			}
		})
	}
}

func TestVolumeIDToFilePath(t *testing.T) {
	tests := []struct {
		name         string
		volumeID     string
		basePath     string
		expectedPath string
		expectErr    bool
	}{
		{
			name:         "valid with custom base path",
			volumeID:     "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			basePath:     "/storage-pool/test",
			expectedPath: "/storage-pool/test/pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890.img",
			expectErr:    false,
		},
		{
			name:         "valid with default base path",
			volumeID:     "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			basePath:     "",
			expectedPath: "/storage-pool/kubernetes-volumes/pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890.img",
			expectErr:    false,
		},
		{
			name:      "invalid volume ID",
			volumeID:  "invalid",
			basePath:  "/storage-pool/test",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := VolumeIDToFilePath(tt.volumeID, tt.basePath)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if path != tt.expectedPath {
					t.Errorf("Expected path %s, got %s", tt.expectedPath, path)
				}
			}
		})
	}
}

func TestExtractVolumeIDFromNQN(t *testing.T) {
	tests := []struct {
		name             string
		nqn              string
		expectedVolumeID string
		expectErr        bool
	}{
		{
			name:             "valid NQN",
			nqn:              "nqn.2000-02.com.mikrotik:pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectedVolumeID: "pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectErr:        false,
		},
		{
			name:      "invalid prefix",
			nqn:       "nqn.other:pvc-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expectErr: true,
		},
		{
			name:      "missing volume ID",
			nqn:       "nqn.2000-02.com.mikrotik:",
			expectErr: true,
		},
		{
			name:      "invalid volume ID in NQN",
			nqn:       "nqn.2000-02.com.mikrotik:invalid",
			expectErr: true,
		},
		{
			name:      "empty NQN",
			nqn:       "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumeID, err := ExtractVolumeIDFromNQN(tt.nqn)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if volumeID != tt.expectedVolumeID {
					t.Errorf("Expected volume ID %s, got %s", tt.expectedVolumeID, volumeID)
				}
			}
		})
	}
}
