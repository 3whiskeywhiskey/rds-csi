package mount

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// mockExecCommand creates a mock exec.Cmd for testing
func mockExecCommand(stdout, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1",
			"STDOUT=" + stdout,
			"STDERR=" + stderr,
			"EXIT_CODE=" + fmt.Sprintf("%d", exitCode),
		}
		return cmd
	}
}

// TestHelperProcess is used by mockExecCommand to simulate command execution
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// Output mock data
	_, _ = os.Stdout.WriteString(os.Getenv("STDOUT"))
	_, _ = os.Stderr.WriteString(os.Getenv("STDERR"))

	// Exit with specified code
	exitCode, _ := strconv.Atoi(os.Getenv("EXIT_CODE"))
	os.Exit(exitCode)
}

func TestMount(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		target      string
		fsType      string
		options     []string
		expectError bool
	}{
		{
			name:        "basic mount",
			source:      "/dev/nvme0n1",
			target:      "/mnt/test",
			fsType:      "ext4",
			options:     []string{},
			expectError: false,
		},
		{
			name:        "mount with options",
			source:      "/dev/nvme0n1",
			target:      "/mnt/test",
			fsType:      "ext4",
			options:     []string{"ro", "noatime"},
			expectError: false,
		},
		{
			name:        "bind mount",
			source:      "/mnt/staging",
			target:      "/mnt/target",
			fsType:      "",
			options:     []string{"bind"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mounter{
				execCommand: mockExecCommand("", "", 0),
			}

			// Create temporary target directory
			tmpTarget := t.TempDir()

			err := m.Mount(tt.source, tmpTarget, tt.fsType, tt.options)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestUnmount(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		isMountPoint bool
		expectError  bool
	}{
		{
			name:         "unmount mounted path",
			target:       "/mnt/test",
			isMountPoint: true,
			expectError:  false,
		},
		{
			name:         "unmount not mounted path",
			target:       "/mnt/test",
			isMountPoint: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock findmnt output based on mount status
			findmntOutput := ""
			if tt.isMountPoint {
				findmntOutput = tt.target
			}

			m := &mounter{
				execCommand: mockExecCommand(findmntOutput, "", 0),
			}

			err := m.Unmount(tt.target)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestIsLikelyMountPoint(t *testing.T) {
	tests := []struct {
		name            string
		createPath      bool
		findmntOutput   string
		findmntExitCode int
		expectedResult  bool
		expectError     bool
	}{
		{
			name:            "is mount point",
			createPath:      true,
			findmntOutput:   "/mnt/test\n", // findmnt returns path with newline
			findmntExitCode: 0,
			expectedResult:  true,
			expectError:     false,
		},
		{
			name:            "not mount point",
			createPath:      true,
			findmntOutput:   "",
			findmntExitCode: 1,
			expectedResult:  false,
			expectError:     false,
		},
		{
			name:            "path does not exist",
			createPath:      false,
			findmntOutput:   "",
			findmntExitCode: 0,
			expectedResult:  false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary path for testing
			var testPath string
			if tt.createPath {
				testPath = t.TempDir()
			} else {
				testPath = "/nonexistent-path-for-testing"
			}

			m := &mounter{
				execCommand: mockExecCommand(tt.findmntOutput, "", tt.findmntExitCode),
			}

			result, err := m.IsLikelyMountPoint(testPath)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expectedResult {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name        string
		device      string
		fsType      string
		isFormatted bool
		expectError bool
	}{
		{
			name:        "skip already formatted",
			device:      "/dev/nvme0n1",
			fsType:      "ext4",
			isFormatted: true,
			expectError: false,
		},
		{
			name:        "unsupported filesystem",
			device:      "/dev/nvme0n1",
			fsType:      "btrfs",
			isFormatted: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock blkid output based on formatted status
			blkidOutput := ""
			blkidExitCode := 0
			if tt.isFormatted {
				blkidOutput = tt.fsType
			} else {
				blkidExitCode = 2 // blkid returns 2 when no filesystem found
			}

			m := &mounter{
				execCommand: mockExecCommand(blkidOutput, "", blkidExitCode),
			}

			err := m.Format(tt.device, tt.fsType)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}

	// Note: Testing actual format operations with ext4/xfs requires more complex mocking
	// where different commands (blkid vs mkfs) return different exit codes.
	// These cases are covered by integration tests.
}

func TestIsFormatted(t *testing.T) {
	tests := []struct {
		name           string
		device         string
		blkidOutput    string
		blkidExitCode  int
		expectedResult bool
		expectError    bool
	}{
		{
			name:           "formatted ext4",
			device:         "/dev/nvme0n1",
			blkidOutput:    "ext4",
			blkidExitCode:  0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "formatted xfs",
			device:         "/dev/nvme0n1",
			blkidOutput:    "xfs",
			blkidExitCode:  0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "not formatted",
			device:         "/dev/nvme0n1",
			blkidOutput:    "",
			blkidExitCode:  2,
			expectedResult: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mounter{
				execCommand: mockExecCommand(tt.blkidOutput, "", tt.blkidExitCode),
			}

			result, err := m.IsFormatted(tt.device)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expectedResult {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestGetDeviceStats(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		dfOutput      string
		expectError   bool
		expectedStats *DeviceStats
	}{
		{
			name:        "valid stats",
			path:        "/mnt/test",
			dfOutput:    "Size Used Avail Inodes IUsed IFree\n107374182400 21474836480 85899345920 6553600 131072 6422528",
			expectError: false,
			expectedStats: &DeviceStats{
				TotalBytes:      107374182400,
				UsedBytes:       21474836480,
				AvailableBytes:  85899345920,
				TotalInodes:     6553600,
				UsedInodes:      131072,
				AvailableInodes: 6422528,
			},
		},
		{
			name:          "invalid output",
			path:          "/mnt/test",
			dfOutput:      "invalid output",
			expectError:   true,
			expectedStats: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mounter{
				execCommand: mockExecCommand(tt.dfOutput, "", 0),
			}

			stats, err := m.GetDeviceStats(tt.path)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expectedStats != nil && stats != nil {
				if stats.TotalBytes != tt.expectedStats.TotalBytes {
					t.Errorf("Expected TotalBytes %d, got %d", tt.expectedStats.TotalBytes, stats.TotalBytes)
				}
				if stats.UsedBytes != tt.expectedStats.UsedBytes {
					t.Errorf("Expected UsedBytes %d, got %d", tt.expectedStats.UsedBytes, stats.UsedBytes)
				}
				if stats.AvailableBytes != tt.expectedStats.AvailableBytes {
					t.Errorf("Expected AvailableBytes %d, got %d", tt.expectedStats.AvailableBytes, stats.AvailableBytes)
				}
				if stats.TotalInodes != tt.expectedStats.TotalInodes {
					t.Errorf("Expected TotalInodes %d, got %d", tt.expectedStats.TotalInodes, stats.TotalInodes)
				}
				if stats.UsedInodes != tt.expectedStats.UsedInodes {
					t.Errorf("Expected UsedInodes %d, got %d", tt.expectedStats.UsedInodes, stats.UsedInodes)
				}
				if stats.AvailableInodes != tt.expectedStats.AvailableInodes {
					t.Errorf("Expected AvailableInodes %d, got %d", tt.expectedStats.AvailableInodes, stats.AvailableInodes)
				}
			}
		})
	}
}

func TestNewMounter(t *testing.T) {
	m := NewMounter()
	if m == nil {
		t.Fatal("NewMounter returned nil")
	}

	// Verify it implements the interface
	var _ = Mounter(m)
}

func TestMountCreateTargetDirectory(t *testing.T) {
	m := &mounter{
		execCommand: mockExecCommand("", "", 0),
	}

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	target := tmpDir + "/subdir/deep/path"

	err := m.Mount("/dev/test", target, "ext4", nil)
	if err != nil {
		t.Errorf("Mount failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Error("Target directory was not created")
	}
}

func TestFormatUnsupportedFilesystem(t *testing.T) {
	m := &mounter{
		execCommand: mockExecCommand("", "", 2),
	}

	err := m.Format("/dev/test", "unsupported-fs")
	if err == nil {
		t.Error("Expected error for unsupported filesystem")
	}
	if !strings.Contains(err.Error(), "unsupported filesystem type") {
		t.Errorf("Expected 'unsupported filesystem type' error, got: %v", err)
	}
}
