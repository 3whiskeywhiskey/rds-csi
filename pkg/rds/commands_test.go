package rds

import (
	"testing"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/utils"
)

// setupTestBasePaths configures allowed base paths for testing
func setupTestBasePaths(t *testing.T) {
	t.Helper()
	utils.ResetAllowedBasePaths()
	if err := utils.SetAllowedBasePath("/storage-pool/metal-csi"); err != nil {
		t.Fatalf("failed to set test base path: %v", err)
	}
	t.Cleanup(utils.ResetAllowedBasePaths)
}

func TestParseVolumeInfo(t *testing.T) {
	// Real RouterOS output format (multi-line)
	output := `type=file slot="pvc-test-123" slot-default="" parent="" fs=-
               model="/storage-pool/test.img"
               size=53 687 091 200 mount-filesystem=yes mount-read-only=no
               compress=no sector-size=512 raid-master=none
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-123"
               nvme-tcp-server-allow-host-name="" iscsi-export=no
               nfs-sharing=no smb-sharing=no media-sharing=no
               media-interface=none swap=no
               file-path=/storage-pool/test.img
               file-size=50.0GiB file-offset=0`

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

	// file-size=50.0GiB = 50 * 1024^3 bytes = 53687091200
	expectedSize := int64(50 * 1024 * 1024 * 1024)
	if volume.FileSizeBytes != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, volume.FileSizeBytes)
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

	// Status should be "ready" for file-type volumes with nvme-tcp-export=yes
	if volume.Status != "ready" {
		t.Errorf("Expected status ready, got %s", volume.Status)
	}
}

func TestParseVolumeList(t *testing.T) {
	// Real RouterOS /disk print output with multiple volumes (multi-line format)
	output := ` 0  type=file slot="pvc-test-1" size=53 687 091 200
               file-path=/storage-pool/test1.img file-size=50.0GiB
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-1"

 1  type=file slot="pvc-test-2" size=107 374 182 400
               file-path=/storage-pool/test2.img file-size=100.0GiB
               nvme-tcp-export=yes nvme-tcp-server-port=4420
               nvme-tcp-server-nqn="nqn.2000-02.com.mikrotik:pvc-test-2"`

	volumes, err := parseVolumeList(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(volumes) != 2 {
		t.Errorf("Expected 2 volumes, got %d", len(volumes))
	}

	if len(volumes) > 0 && volumes[0].Slot != "pvc-test-1" {
		t.Errorf("Expected first volume slot pvc-test-1, got %s", volumes[0].Slot)
	}

	if len(volumes) > 1 && volumes[1].Slot != "pvc-test-2" {
		t.Errorf("Expected second volume slot pvc-test-2, got %s", volumes[1].Slot)
	}

	// Verify first volume details
	if len(volumes) > 0 {
		expectedSize := int64(50 * 1024 * 1024 * 1024) // 50 GiB
		if volumes[0].FileSizeBytes != expectedSize {
			t.Errorf("Expected first volume size %d, got %d", expectedSize, volumes[0].FileSizeBytes)
		}
		if !volumes[0].NVMETCPExport {
			t.Error("Expected first volume NVMETCPExport to be true")
		}
	}
}

func TestParseCapacityInfo(t *testing.T) {
	// Real RouterOS /file print detail output format with space-separated numbers
	output := `name=/storage-pool type=directory size=7 681 574 174 720
               free=5 632 440 000 000 use=27%
               creation-time=jan/01/2025 00:00:00`

	capacity, err := parseCapacityInfo(output)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Expected values from space-separated numbers
	expectedTotal := int64(7681574174720) // size=7 681 574 174 720
	if capacity.TotalBytes != expectedTotal {
		t.Errorf("Expected total bytes %d, got %d", expectedTotal, capacity.TotalBytes)
	}

	expectedFree := int64(5632440000000) // free=5 632 440 000 000
	if capacity.FreeBytes != expectedFree {
		t.Errorf("Expected free bytes %d, got %d", expectedFree, capacity.FreeBytes)
	}

	// Used should be calculated as Total - Free
	expectedUsed := expectedTotal - expectedFree
	if capacity.UsedBytes != expectedUsed {
		t.Errorf("Expected used bytes %d, got %d", expectedUsed, capacity.UsedBytes)
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

func TestParseFileInfo(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		expectedPath string
		expectedName string
		expectError  bool
	}{
		{
			name: "file path with leading slash",
			output: `name=/storage-pool/metal-csi/test.img type=file
                    size=53 687 091 200`,
			expectedPath: "/storage-pool/metal-csi/test.img",
			expectedName: "test.img",
			expectError:  false,
		},
		{
			name: "file path without leading slash (real RouterOS /file print format)",
			output: `name=storage-pool/metal-csi/test.img type=file
                    size=53 687 091 200`,
			expectedPath: "/storage-pool/metal-csi/test.img", // Should be normalized to absolute path
			expectedName: "test.img",
			expectError:  false,
		},
		{
			name: "file path with quoted name without leading slash",
			output: `name="storage-pool/metal-csi/test-volume.img" type=file
                    size=107 374 182 400`,
			expectedPath: "/storage-pool/metal-csi/test-volume.img", // Should be normalized
			expectedName: "test-volume.img",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parseFileInfo(tt.output)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if file.Path != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, file.Path)
			}

			if file.Name != tt.expectedName {
				t.Errorf("Expected name %s, got %s", tt.expectedName, file.Name)
			}

			// Verify path always has leading slash (critical for path comparison)
			if file.Path != "" && file.Path[0] != '/' {
				t.Errorf("Path should be normalized to absolute format with leading /, got: %s", file.Path)
			}
		})
	}
}

func TestValidateCreateVolumeOptions(t *testing.T) {
	setupTestBasePaths(t)
	tests := []struct {
		name      string
		opts      CreateVolumeOptions
		expectErr bool
	}{
		{
			name: "valid options",
			opts: CreateVolumeOptions{
				Slot:          "pvc-test-123",
				FilePath:      "/storage-pool/metal-csi/volumes/test.img",
				FileSizeBytes: 50 * 1024 * 1024 * 1024,
				NVMETCPPort:   4420,
				NVMETCPNQN:    "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: false,
		},
		{
			name: "missing slot",
			opts: CreateVolumeOptions{
				FilePath:      "/storage-pool/metal-csi/volumes/test.img",
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
				FilePath:   "/storage-pool/metal-csi/volumes/test.img",
				NVMETCPNQN: "nqn.2000-02.com.mikrotik:pvc-test-123",
			},
			expectErr: true,
		},
		{
			name: "missing NQN",
			opts: CreateVolumeOptions{
				Slot:          "pvc-test-123",
				FilePath:      "/storage-pool/metal-csi/volumes/test.img",
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

func TestParseFileInfo_FileSizeFormats(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		expectedSize int64
		expectError  bool
	}{
		{
			name: "file size in GiB",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=10.0GiB last-modified=2025-11-11 14:32:41`,
			expectedSize: 10 * 1024 * 1024 * 1024,
			expectError:  false,
		},
		{
			name: "file size in MiB",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=1024.0MiB last-modified=2025-11-11 14:32:41`,
			expectedSize: 1024 * 1024 * 1024,
			expectError:  false,
		},
		{
			name: "file size in TiB",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=5.5TiB last-modified=2025-11-11 14:32:41`,
			expectedSize: int64(5.5 * 1024 * 1024 * 1024 * 1024),
			expectError:  false,
		},
		{
			name: "file size in KiB",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=512.0KiB last-modified=2025-11-11 14:32:41`,
			expectedSize: 512 * 1024,
			expectError:  false,
		},
		{
			name: "raw size with spaces (fallback)",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    size=10 737 418 240 last-modified=2025-11-11 14:32:41`,
			expectedSize: 10737418240,
			expectError:  false,
		},
		{
			name: "file size with GB (not GiB)",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=50GB last-modified=2025-11-11 14:32:41`,
			expectedSize: 50 * 1024 * 1024 * 1024, // parseSize treats GB as GiB
			expectError:  false,
		},
		{
			name: "small file size",
			output: `name=storage-pool/metal-csi/test.img type=.img
                    file size=100.0MiB last-modified=2025-11-11 14:32:41`,
			expectedSize: 100 * 1024 * 1024,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parseFileInfo(tt.output)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Allow 1 MB tolerance for floating point calculations
			diff := file.SizeBytes - tt.expectedSize
			if diff < 0 {
				diff = -diff
			}
			if diff > 1024*1024 {
				t.Errorf("Expected size %d bytes, got %d bytes (diff: %d)",
					tt.expectedSize, file.SizeBytes, diff)
			}
		})
	}
}

func TestParseFileList_MultipleFiles(t *testing.T) {
	output := ` 0   name=storage-pool/metal-csi type=directory
     last-modified=2025-11-11 16:47:07

 1   name=storage-pool/metal-csi/pvc-ccdecfad-a8bf-572e-9120-464c4d99f12f.img
     type=.img file size=10.0GiB last-modified=2025-11-11 14:32:41

 2   name=storage-pool/metal-csi/pvc-0f923194-922a-5dd8-b376-c2c6ccb56dd8.img
     type=.img file size=1024.0MiB last-modified=2025-11-11 16:47:07`

	files, err := parseFileList(output)
	if err != nil {
		t.Fatalf("parseFileList failed: %v", err)
	}

	// Should find 2 .img files (directory is also parsed but that's ok)
	imgFiles := 0
	for _, file := range files {
		if file.Type == ".img" {
			imgFiles++

			// Verify sizes are parsed correctly
			switch file.Name {
			case "pvc-ccdecfad-a8bf-572e-9120-464c4d99f12f.img":
				expectedSize := int64(10 * 1024 * 1024 * 1024)
				if file.SizeBytes != expectedSize {
					t.Errorf("File 1: expected size %d, got %d", expectedSize, file.SizeBytes)
				}
			case "pvc-0f923194-922a-5dd8-b376-c2c6ccb56dd8.img":
				expectedSize := int64(1024 * 1024 * 1024)
				if file.SizeBytes != expectedSize {
					t.Errorf("File 2: expected size %d, got %d", expectedSize, file.SizeBytes)
				}
			}
		}
	}

	if imgFiles != 2 {
		t.Errorf("Expected 2 .img files, found %d", imgFiles)
	}
}

func TestParseRouterOSTime(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedYear int
		expectedDay  int
		expectZero   bool
	}{
		{
			name:         "ISO format with last-modified",
			input:        `name=test.img type=file last-modified=2025-11-12 00:36:13`,
			expectedYear: 2025,
			expectedDay:  12,
			expectZero:   false,
		},
		{
			name:         "ISO format with creation-time",
			input:        `name=test.img type=file creation-time=2025-12-25 10:30:45`,
			expectedYear: 2025,
			expectedDay:  25,
			expectZero:   false,
		},
		{
			name:         "RouterOS month format with creation-time",
			input:        `name=test.img type=file creation-time=jan/02/2025 00:00:00`,
			expectedYear: 2025,
			expectedDay:  2,
			expectZero:   false,
		},
		{
			name:         "RouterOS month format with last-modified",
			input:        `name=test.img type=file last-modified=nov/15/2025 14:32:41`,
			expectedYear: 2025,
			expectedDay:  15,
			expectZero:   false,
		},
		{
			name:       "no time field",
			input:      `name=test.img type=file size=1024`,
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRouterOSTime(tt.input)

			if tt.expectZero {
				if !result.IsZero() {
					t.Errorf("Expected zero time, got %v", result)
				}
				return
			}

			if result.IsZero() {
				t.Errorf("Expected non-zero time, got zero")
				return
			}

			if result.Year() != tt.expectedYear {
				t.Errorf("Expected year %d, got %d", tt.expectedYear, result.Year())
			}

			if result.Day() != tt.expectedDay {
				t.Errorf("Expected day %d, got %d", tt.expectedDay, result.Day())
			}
		})
	}
}

func TestParseFileInfo_CreatedAtParsing(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		expectedYear int
		expectZero   bool
	}{
		{
			name: "ISO format last-modified",
			output: `name=storage-pool/metal-csi/pvc-test.img type=.img
                    file size=10.0GiB last-modified=2025-11-12 00:36:13`,
			expectedYear: 2025,
			expectZero:   false,
		},
		{
			name: "RouterOS format creation-time",
			output: `name=storage-pool/metal-csi/pvc-test.img type=.img
                    file size=10.0GiB creation-time=jan/01/2025 00:00:00`,
			expectedYear: 2025,
			expectZero:   false,
		},
		{
			name: "no time field",
			output: `name=storage-pool/metal-csi/pvc-test.img type=.img
                    file size=10.0GiB`,
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parseFileInfo(tt.output)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectZero {
				if !file.CreatedAt.IsZero() {
					t.Errorf("Expected zero CreatedAt, got %v", file.CreatedAt)
				}
				return
			}

			if file.CreatedAt.IsZero() {
				t.Errorf("Expected non-zero CreatedAt, got zero time")
				return
			}

			if file.CreatedAt.Year() != tt.expectedYear {
				t.Errorf("Expected year %d, got %d", tt.expectedYear, file.CreatedAt.Year())
			}
		})
	}
}
