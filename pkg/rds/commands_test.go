package rds

import (
	"testing"
)

func TestParseVolumeInfo(t *testing.T) {
	output := `slot="pvc-test-123" type="file" file-path="/storage-pool/test.img" file-size=53687091200 nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-123" status="ready"`

	volume, err := parseVolumeInfo(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if volume.Slot != "pvc-test-123" {
		t.Errorf("Expected slot pvc-test-123, got %s", volume.Slot)
	}

	if volume.Type != "file" {
		t.Errorf("Expected type file, got %s", volume.Type)
	}

	if volume.FilePath != "/storage-pool/test.img" {
		t.Errorf("Expected path /storage-pool/test.img, got %s", volume.FilePath)
	}

	if volume.FileSizeBytes != 53687091200 {
		t.Errorf("Expected size 53687091200, got %d", volume.FileSizeBytes)
	}

	if !volume.NVMETCPExport {
		t.Error("Expected NVMETCPExport to be true")
	}

	if volume.NVMETCPPort != 4420 {
		t.Errorf("Expected port 4420, got %d", volume.NVMETCPPort)
	}

	if volume.NVMETCPNQN != "nqn.2000-02.com.mikrotik:pvc-test-123" {
		t.Errorf("Expected NQN nqn.2000-02.com.mikrotik:pvc-test-123, got %s", volume.NVMETCPNQN)
	}

	if volume.Status != "ready" {
		t.Errorf("Expected status ready, got %s", volume.Status)
	}
}

func TestParseVolumeList(t *testing.T) {
	output := ` 0  slot="pvc-test-1" type="file" file-path="/storage-pool/test1.img" file-size=53687091200 nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-1" status="ready"

 1  slot="pvc-test-2" type="file" file-path="/storage-pool/test2.img" file-size=107374182400 nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-2" status="ready"`

	volumes, err := parseVolumeList(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(volumes) != 2 {
		t.Errorf("Expected 2 volumes, got %d", len(volumes))
	}

	if volumes[0].Slot != "pvc-test-1" {
		t.Errorf("Expected first volume slot pvc-test-1, got %s", volumes[0].Slot)
	}

	if volumes[1].Slot != "pvc-test-2" {
		t.Errorf("Expected second volume slot pvc-test-2, got %s", volumes[1].Slot)
	}
}

func TestParseCapacityInfo(t *testing.T) {
	output := `name: /storage-pool
type: directory
size: 0
creation-time: jan/01/2025 00:00:00

Total: 7.23TiB
Free: 5.12TiB
Used: 2.11TiB`

	capacity, err := parseCapacityInfo(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 7.23 TiB = 7.23 * 1024^4 bytes
	tib := int64(1024 * 1024 * 1024 * 1024)
	expectedTotal := int64(7.23 * float64(tib))
	if capacity.TotalBytes < expectedTotal-1024*1024 || capacity.TotalBytes > expectedTotal+1024*1024 {
		t.Errorf("Expected total bytes around %d, got %d", expectedTotal, capacity.TotalBytes)
	}

	// 5.12 TiB
	expectedFree := int64(5.12 * float64(tib))
	if capacity.FreeBytes < expectedFree-1024*1024 || capacity.FreeBytes > expectedFree+1024*1024 {
		t.Errorf("Expected free bytes around %d, got %d", expectedFree, capacity.FreeBytes)
	}
}

func TestValidateSlotName(t *testing.T) {
	tests := []struct {
		name      string
		slot      string
		expectErr bool
	}{
		{"valid alphanumeric", "pvc-abc123", false},
		{"valid with hyphens", "pvc-a1b2-c3d4", false},
		{"empty slot", "", true},
		{"with space", "pvc abc", true},
		{"with semicolon", "pvc-abc; rm -rf", true},
		{"with pipe", "pvc|evil", true},
		{"with dollar", "pvc$var", true},
		{"with backtick", "pvc`cmd`", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotName(tt.slot)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{1024, "1K"},
		{1024 * 1024, "1M"},
		{1024 * 1024 * 1024, "1G"},
		{50 * 1024 * 1024 * 1024, "50G"},
		{1024 * 1024 * 1024 * 1024, "1T"},
		{512, "512"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, expected %s", tt.bytes, result, tt.expected)
		}
	}
}

func TestParseSize(t *testing.T) {
	tib := int64(1024 * 1024 * 1024 * 1024)
	tests := []struct {
		value    string
		unit     string
		expected int64
	}{
		{"50", "G", 50 * 1024 * 1024 * 1024},
		{"100", "GB", 100 * 1024 * 1024 * 1024},
		{"7.23", "TiB", int64(7.23 * float64(tib))},
		{"1024", "M", 1024 * 1024 * 1024},
		{"1", "K", 1024},
	}

	for _, tt := range tests {
		result, err := parseSize(tt.value, tt.unit)
		if err != nil {
			t.Errorf("parseSize(%s, %s) returned error: %v", tt.value, tt.unit, err)
			continue
		}

		// Allow small margin for floating point
		diff := result - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 1024*1024 { // 1 MB tolerance
			t.Errorf("parseSize(%s, %s) = %d, expected %d", tt.value, tt.unit, result, tt.expected)
		}
	}
}

func TestValidateCreateVolumeOptions(t *testing.T) {
	tests := []struct {
		name      string
		opts      CreateVolumeOptions
		expectErr bool
	}{
		{
			name: "valid options",
			opts: CreateVolumeOptions{
				Slot:          "pvc-test-123",
				FilePath:      "/storage-pool/test.img",
				FileSizeBytes: 50 * 1024 * 1024 * 1024,
				NVMETCPPort:   4420,
				NVMETCPNQN:    "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: false,
		},
		{
			name: "missing slot",
			opts: CreateVolumeOptions{
				FilePath:      "/storage-pool/test.img",
				FileSizeBytes: 50 * 1024 * 1024 * 1024,
				NVMETCPNQN:    "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: true,
		},
		{
			name: "missing file path",
			opts: CreateVolumeOptions{
				Slot:          "pvc-test-123",
				FileSizeBytes: 50 * 1024 * 1024 * 1024,
				NVMETCPNQN:    "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: true,
		},
		{
			name: "zero size",
			opts: CreateVolumeOptions{
				Slot:       "pvc-test-123",
				FilePath:   "/storage-pool/test.img",
				NVMETCPNQN: "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: true,
		},
		{
			name: "missing NQN",
			opts: CreateVolumeOptions{
				Slot:          "pvc-test-123",
				FilePath:      "/storage-pool/test.img",
				FileSizeBytes: 50 * 1024 * 1024 * 1024,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateVolumeOptions(tt.opts)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
