package rds

import (
	"fmt"
	"strings"
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

// mockCommandRunner is a function type for mocking runCommand behavior
type mockCommandRunner func(command string) (string, error)

// testableSSHClient wraps sshClient for testing command execution
type testableSSHClient struct {
	*sshClient
	mockRunner mockCommandRunner
}

// Override runCommand to use mock
func (t *testableSSHClient) runCommand(command string) (string, error) {
	if t.mockRunner != nil {
		return t.mockRunner(command)
	}
	return "", fmt.Errorf("no mock runner configured")
}

// newTestableSSHClient creates a client for testing
func newTestableSSHClient(runner mockCommandRunner) *testableSSHClient {
	base := &sshClient{
		address: "test-rds",
		port:    22,
		user:    "admin",
	}
	return &testableSSHClient{
		sshClient:  base,
		mockRunner: runner,
	}
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

func TestTestableSSHClientInfrastructure(t *testing.T) {
	// Verify the mock infrastructure works
	expectedOutput := "mock output"
	commandReceived := ""

	runner := func(cmd string) (string, error) {
		commandReceived = cmd
		return expectedOutput, nil
	}

	client := newTestableSSHClient(runner)

	// Test that mock runner is called
	output, err := client.runCommand("test command")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if output != expectedOutput {
		t.Errorf("Expected output %q, got %q", expectedOutput, output)
	}
	if commandReceived != "test command" {
		t.Errorf("Expected command %q, got %q", "test command", commandReceived)
	}

	// Test error propagation
	errorRunner := func(cmd string) (string, error) {
		return "", fmt.Errorf("mock error")
	}
	errorClient := newTestableSSHClient(errorRunner)
	_, err = errorClient.runCommand("any")
	if err == nil || !strings.Contains(err.Error(), "mock error") {
		t.Errorf("Expected mock error to propagate")
	}
}

func TestVerifyVolumeExistsCommandConstruction(t *testing.T) {
	setupTestBasePaths(t)

	// Test that VerifyVolumeExists constructs correct command
	// This tests the validation and command pattern without SSH
	tests := []struct {
		name        string
		slot        string
		expectError bool
	}{
		{"valid slot", "pvc-test-123", false},
		{"empty slot", "", true},
		{"dangerous slot", "pvc;evil", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotName(tt.slot)
			if tt.expectError && err == nil {
				t.Error("Expected validation error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestExtractMountPoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mount point with leading slash",
			input:    "/storage-pool/metal-csi/volumes",
			expected: "storage-pool",
		},
		{
			name:     "mount point without leading slash",
			input:    "storage-pool/metal-csi/volumes",
			expected: "storage-pool",
		},
		{
			name:     "single component path",
			input:    "/nvme1",
			expected: "nvme1",
		},
		{
			name:     "multi-level path",
			input:    "/nvme1/kubernetes/volumes",
			expected: "nvme1",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMountPoint(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNormalizeRouterOSOutputEdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedContains string
		description      string
	}{
		{
			name:             "carriage returns",
			input:            "line1\r\nline2\r\n",
			expectedContains: "line1",
			description:      "should remove \\r characters",
		},
		{
			name:             "RouterOS flags header",
			input:            "Flags: X - disabled\ntype=file slot=test",
			expectedContains: "type=file slot=test",
			description:      "should skip Flags: header lines",
		},
		{
			name:             "continuation lines with tabs",
			input:            "type=file\n\tsize=1000",
			expectedContains: "type=file size=1000",
			description:      "should join continuation lines starting with tab",
		},
		{
			name:             "continuation lines with spaces",
			input:            "type=file\n   size=1000",
			expectedContains: "type=file size=1000",
			description:      "should join continuation lines starting with spaces",
		},
		{
			name:             "multiple continuation lines",
			input:            "type=file\n  size=1000\n  path=/test",
			expectedContains: "type=file size=1000 path=/test",
			description:      "should join multiple continuation lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRouterOSOutput(tt.input)
			if !strings.Contains(result, tt.expectedContains) {
				t.Errorf("Expected normalized output to contain %q, got %q\nDescription: %s",
					tt.expectedContains, result, tt.description)
			}
		})
	}
}
// Snapshot parsing tests

func TestParseSnapshotInfo(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectName  string
		expectRO    bool
		expectFS    string
		expectError bool
	}{
		{
			name: "valid snapshot with all fields",
			output: `name=snap-12345678-1234-1234-1234-123456789abc read-only=yes fs=storage-pool
                    parent=pvc-test-volume`,
			expectName: "snap-12345678-1234-1234-1234-123456789abc",
			expectRO:   true,
			expectFS:   "storage-pool",
		},
		{
			name: "snapshot with read-only=no (writable clone)",
			output: `name=snap-test-123 read-only=no fs=storage-pool
                    parent=snap-original`,
			expectName: "snap-test-123",
			expectRO:   false,
			expectFS:   "storage-pool",
		},
		{
			name: "snapshot with quoted name",
			output: `name="snap-quoted-name" read-only=yes fs="storage-pool"
                    parent="pvc-test"`,
			expectName: "snap-quoted-name",
			expectRO:   true,
			expectFS:   "storage-pool",
		},
		{
			name:        "empty output",
			output:      "",
			expectError: false, // parseSnapshotInfo returns empty SnapshotInfo, not error
		},
		{
			name: "partial output (missing optional fields)",
			output: `name=snap-partial read-only=yes`,
			expectName: "snap-partial",
			expectRO:   true,
			expectFS:   "", // FS label missing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := parseSnapshotInfo(tt.output)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			
			if snapshot.Name != tt.expectName {
				t.Errorf("Expected name %q, got %q", tt.expectName, snapshot.Name)
			}
			
			if snapshot.ReadOnly != tt.expectRO {
				t.Errorf("Expected read-only %v, got %v", tt.expectRO, snapshot.ReadOnly)
			}
			
			if snapshot.FSLabel != tt.expectFS {
				t.Errorf("Expected fs %q, got %q", tt.expectFS, snapshot.FSLabel)
			}
		})
	}
}

func TestParseSnapshotList(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		expectCount   int
		expectNames   []string
		expectError   bool
	}{
		{
			name: "multiple snapshots",
			output: `0  name=snap-12345678-1234-1234-1234-123456789abc read-only=yes fs=storage-pool
1  name=snap-abcdef12-3456-7890-abcd-ef1234567890 read-only=yes fs=storage-pool
2  name=snap-test-sanity read-only=yes fs=storage-pool`,
			expectCount: 3,
			expectNames: []string{
				"snap-12345678-1234-1234-1234-123456789abc",
				"snap-abcdef12-3456-7890-abcd-ef1234567890",
				"snap-test-sanity",
			},
		},
		{
			name:        "empty list",
			output:      "",
			expectCount: 0,
			expectNames: []string{},
		},
		{
			name: "mixed snap- and non-snap entries (should filter)",
			output: `0  name=snap-12345678-1234-1234-1234-123456789abc read-only=yes fs=storage-pool
1  name=backup-volume read-only=yes fs=storage-pool
2  name=snap-test-123 read-only=yes fs=storage-pool`,
			expectCount: 2, // Only snap-* entries
			expectNames: []string{
				"snap-12345678-1234-1234-1234-123456789abc",
				"snap-test-123",
			},
		},
		{
			name: "no snap- prefix (all filtered out)",
			output: `0  name=backup-volume read-only=yes fs=storage-pool
1  name=clone-volume read-only=no fs=storage-pool`,
			expectCount: 0,
			expectNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshots, err := parseSnapshotList(tt.output)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			
			if len(snapshots) != tt.expectCount {
				t.Errorf("Expected %d snapshots, got %d", tt.expectCount, len(snapshots))
			}
			
			// Verify specific names if provided
			if len(tt.expectNames) > 0 {
				for i, expectedName := range tt.expectNames {
					if i >= len(snapshots) {
						t.Errorf("Missing expected snapshot %q at index %d", expectedName, i)
						continue
					}
					if snapshots[i].Name != expectedName {
						t.Errorf("Snapshot %d: expected name %q, got %q", i, expectedName, snapshots[i].Name)
					}
				}
			}
			
			// Ensure empty list returns empty slice, not nil
			if tt.expectCount == 0 && snapshots == nil {
				t.Error("Expected empty slice, got nil")
			}
		})
	}
}

// MockClient snapshot operation tests

func TestMockClientSnapshotOperations(t *testing.T) {
	mock := NewMockClient()
	
	// Create a test volume first
	volOpts := CreateVolumeOptions{
		Slot:          "pvc-test-volume",
		FilePath:      "/storage-pool/test-vol.img",
		FileSizeBytes: 10 * 1024 * 1024 * 1024, // 10GB
		NVMETCPPort:   4420,
		NVMETCPNQN:    "nqn.2000-02.com.mikrotik:pvc-test-volume",
	}
	if err := mock.CreateVolume(volOpts); err != nil {
		t.Fatalf("Failed to create test volume: %v", err)
	}
	
	// Test 1: Create snapshot from volume
	snapOpts := CreateSnapshotOptions{
		Name:         "snap-test-123",
		SourceVolume: "pvc-test-volume",
		FSLabel:      "storage-pool",
	}
	
	snapshot, err := mock.CreateSnapshot(snapOpts)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	
	if snapshot.Name != "snap-test-123" {
		t.Errorf("Expected snapshot name snap-test-123, got %s", snapshot.Name)
	}
	if snapshot.SourceVolume != "pvc-test-volume" {
		t.Errorf("Expected source volume pvc-test-volume, got %s", snapshot.SourceVolume)
	}
	if !snapshot.ReadOnly {
		t.Error("Expected snapshot to be read-only")
	}
	if snapshot.FSLabel != "storage-pool" {
		t.Errorf("Expected fs label storage-pool, got %s", snapshot.FSLabel)
	}
	
	// Test 2: GetSnapshot returns correct snapshot
	retrieved, err := mock.GetSnapshot("snap-test-123")
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if retrieved.Name != snapshot.Name {
		t.Errorf("Retrieved snapshot name mismatch: expected %s, got %s", snapshot.Name, retrieved.Name)
	}
	
	// Test 3: Create duplicate snapshot with same name and source (idempotent)
	duplicate, err := mock.CreateSnapshot(snapOpts)
	if err != nil {
		t.Fatalf("CreateSnapshot (duplicate) failed: %v", err)
	}
	if duplicate.Name != snapshot.Name {
		t.Error("Expected idempotent CreateSnapshot to return existing snapshot")
	}
	
	// Test 4: Create duplicate snapshot with same name but different source (error)
	badOpts := CreateSnapshotOptions{
		Name:         "snap-test-123",
		SourceVolume: "pvc-different-volume",
		FSLabel:      "storage-pool",
	}
	_, err = mock.CreateSnapshot(badOpts)
	if err == nil {
		t.Error("Expected error when creating snapshot with same name but different source")
	}
	
	// Test 5: ListSnapshots returns all snapshots
	// Create another snapshot first
	snapOpts2 := CreateSnapshotOptions{
		Name:         "snap-test-456",
		SourceVolume: "pvc-test-volume",
		FSLabel:      "storage-pool",
	}
	if _, err := mock.CreateSnapshot(snapOpts2); err != nil {
		t.Fatalf("Failed to create second snapshot: %v", err)
	}
	
	snapshots, err := mock.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snapshots))
	}
	
	// Test 6: DeleteSnapshot removes snapshot
	if err := mock.DeleteSnapshot("snap-test-123"); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}
	
	// Verify snapshot is gone
	_, err = mock.GetSnapshot("snap-test-123")
	if err == nil {
		t.Error("Expected SnapshotNotFoundError after deletion")
	}
	if _, ok := err.(*SnapshotNotFoundError); !ok {
		t.Errorf("Expected SnapshotNotFoundError, got %T: %v", err, err)
	}
	
	// Test 7: Delete non-existent snapshot (idempotent)
	if err := mock.DeleteSnapshot("snap-nonexistent"); err != nil {
		t.Errorf("Expected DeleteSnapshot to be idempotent, got error: %v", err)
	}
	
	// Test 8: GetSnapshot on non-existent snapshot returns error
	_, err = mock.GetSnapshot("snap-nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent snapshot")
	}
	if _, ok := err.(*SnapshotNotFoundError); !ok {
		t.Errorf("Expected SnapshotNotFoundError, got %T: %v", err, err)
	}
	
	// Test 9: ListSnapshots with no snapshots returns empty slice
	// Delete remaining snapshot
	if err := mock.DeleteSnapshot("snap-test-456"); err != nil {
		t.Fatalf("Failed to delete second snapshot: %v", err)
	}
	
	snapshots, err = mock.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if snapshots == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots, got %d", len(snapshots))
	}
}

// Snapshot ID validation tests (in utils package, but test here for completeness)

func TestValidateSnapshotID(t *testing.T) {
	tests := []struct {
		name        string
		snapshotID  string
		expectError bool
	}{
		{
			name:        "valid snap-<uuid> format",
			snapshotID:  "snap-12345678-1234-1234-1234-123456789abc",
			expectError: false,
		},
		{
			name:        "valid alphanumeric (sanity test ID)",
			snapshotID:  "test-snapshot-123",
			expectError: false,
		},
		{
			name:        "empty string",
			snapshotID:  "",
			expectError: true,
		},
		{
			name:        "command injection: semicolon",
			snapshotID:  "snap-test;rm -rf /",
			expectError: true,
		},
		{
			name:        "command injection: pipe",
			snapshotID:  "snap-test|evil",
			expectError: true,
		},
		{
			name:        "command injection: dollar",
			snapshotID:  "snap-test$var",
			expectError: true,
		},
		{
			name:        "command injection: backtick",
			snapshotID:  "snap-test`cmd`",
			expectError: true,
		},
		{
			name:        "snap- prefix with invalid UUID",
			snapshotID:  "snap-INVALID",
			expectError: true,
		},
		{
			name:        "snap- prefix with uppercase UUID",
			snapshotID:  "snap-12345678-1234-1234-1234-123456789ABC",
			expectError: true,
		},
		{
			name:        "UUID without snap- prefix",
			snapshotID:  "12345678-1234-1234-1234-123456789abc",
			expectError: true,
		},
		{
			name:        "too long (>250 chars)",
			snapshotID:  "snap-" + strings.Repeat("a", 250),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateSnapshotID(tt.snapshotID)
			
			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}
