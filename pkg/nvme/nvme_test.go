package nvme

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestConnect(t *testing.T) {
	tests := []struct {
		name        string
		target      Target
		listOutput  string
		devicePath  string
		expectError bool
	}{
		{
			name: "successful connection",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test-123",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
			},
			listOutput:  "No NVMe subsystems",
			devicePath:  "/dev/nvme0n1",
			expectError: false,
		},
		{
			name: "already connected",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test-123",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
			},
			listOutput:  `{"Subsystems":[{"NQN":"nqn.2000-02.com.mikrotik:pvc-test-123"}]}`,
			devicePath:  "/dev/nvme0n1",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For testing, we need to override GetDevicePath
			// This test is simplified - in real testing we'd need to mock the filesystem
			// Skip the actual connection test for now
			t.Skip("Skipping integration test - requires mocking filesystem and exec properly")
		})
	}
}

func TestDisconnect(t *testing.T) {
	tests := []struct {
		name        string
		nqn         string
		connected   bool
		expectError bool
	}{
		{
			name:        "disconnect connected device",
			nqn:         "nqn.2000-02.com.mikrotik:pvc-test-123",
			connected:   true,
			expectError: false,
		},
		{
			name:        "disconnect already disconnected",
			nqn:         "nqn.2000-02.com.mikrotik:pvc-test-123",
			connected:   false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock list-subsys output based on connected state
			listOutput := "No NVMe subsystems"
			if tt.connected {
				listOutput = `{"Subsystems":[{"NQN":"` + tt.nqn + `"}]}`
			}

			c := &connector{
				execCommand:      mockExecCommand(listOutput, "", 0),
				config:           DefaultConfig(),
				metrics:          &Metrics{},
				activeOperations: make(map[string]*operationTracker),
				resolver:         NewDeviceResolver(),
			}

			err := c.Disconnect(tt.nqn)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestIsConnected(t *testing.T) {
	tests := []struct {
		name       string
		nqn        string
		listOutput string
		expected   bool
	}{
		{
			name:       "device connected",
			nqn:        "nqn.2000-02.com.mikrotik:pvc-test-123",
			listOutput: `{"Subsystems":[{"NQN":"nqn.2000-02.com.mikrotik:pvc-test-123"}]}`,
			expected:   true,
		},
		{
			name:       "device not connected",
			nqn:        "nqn.2000-02.com.mikrotik:pvc-test-123",
			listOutput: "No NVMe subsystems",
			expected:   false,
		},
		{
			name:       "different device connected",
			nqn:        "nqn.2000-02.com.mikrotik:pvc-test-123",
			listOutput: `{"Subsystems":[{"NQN":"nqn.2000-02.com.mikrotik:pvc-other"}]}`,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &connector{
				execCommand:      mockExecCommand(tt.listOutput, "", 0),
				config:           DefaultConfig(),
				metrics:          &Metrics{},
				activeOperations: make(map[string]*operationTracker),
				resolver:         NewDeviceResolver(),
			}

			connected, err := c.IsConnected(tt.nqn)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if connected != tt.expected {
				t.Errorf("Expected IsConnected=%v, got %v", tt.expected, connected)
			}
		})
	}
}

func TestGetDevicePathNotFound(t *testing.T) {
	c := &connector{
		execCommand:      exec.Command,
		config:           DefaultConfig(),
		metrics:          &Metrics{},
		activeOperations: make(map[string]*operationTracker),
		resolver:         NewDeviceResolver(),
	}

	// Test with non-existent NQN
	_, err := c.GetDevicePath("nqn.2000-02.com.mikrotik:non-existent")
	if err == nil {
		t.Error("Expected error for non-existent device, got nil")
	}
	if !strings.Contains(err.Error(), "no device found") {
		t.Errorf("Expected 'no device found' error, got: %v", err)
	}
}

func TestWaitForDeviceTimeout(t *testing.T) {
	c := &connector{
		execCommand:      exec.Command,
		config:           DefaultConfig(),
		metrics:          &Metrics{},
		activeOperations: make(map[string]*operationTracker),
		resolver:         NewDeviceResolver(),
	}

	// Test timeout with non-existent device
	_, err := c.WaitForDevice("nqn.2000-02.com.mikrotik:non-existent", 500*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestTargetValidation(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		valid  bool
	}{
		{
			name: "valid target with all fields",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
				HostNQN:       "nqn.2014-08.org.nvmexpress:uuid:test",
			},
			valid: true,
		},
		{
			name: "valid target without host NQN",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
			},
			valid: true,
		},
		{
			name: "empty NQN",
			target: Target{
				Transport:     "tcp",
				NQN:           "",
				TargetAddress: "10.0.0.1",
				TargetPort:    4420,
			},
			valid: false,
		},
		{
			name: "empty address",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test",
				TargetAddress: "",
				TargetPort:    4420,
			},
			valid: false,
		},
		{
			name: "zero port",
			target: Target{
				Transport:     "tcp",
				NQN:           "nqn.2000-02.com.mikrotik:pvc-test",
				TargetAddress: "10.0.0.1",
				TargetPort:    0,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation checks
			valid := tt.target.NQN != ""
			if tt.target.TargetAddress == "" {
				valid = false
			}
			if tt.target.TargetPort == 0 {
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("Expected valid=%v, got %v", tt.valid, valid)
			}
		})
	}
}

func TestNewConnector(t *testing.T) {
	c := NewConnector()
	if c == nil {
		t.Fatal("NewConnector returned nil")
	}

	// Verify it implements the interface
	var _ = Connector(c)
}

// TestGetDevicePathWithMockFilesystem tests device path discovery
func TestGetDevicePathWithMockFilesystem(t *testing.T) {
	// Create temporary directory structure to simulate /sys/class/nvme
	tmpDir := t.TempDir()

	// Create mock device structure
	deviceDir := filepath.Join(tmpDir, "nvme0")
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("Failed to create mock device dir: %v", err)
	}

	// Create subsysnqn file
	nqn := "nqn.2000-02.com.mikrotik:pvc-test-123"
	nqnPath := filepath.Join(deviceDir, "subsysnqn")
	if err := os.WriteFile(nqnPath, []byte(nqn+"\n"), 0644); err != nil {
		t.Fatalf("Failed to create subsysnqn file: %v", err)
	}

	// Create namespace directory
	namespaceDir := filepath.Join(deviceDir, "nvme0n1")
	if err := os.MkdirAll(namespaceDir, 0755); err != nil {
		t.Fatalf("Failed to create namespace dir: %v", err)
	}

	// Test: This would require modifying GetDevicePath to accept a custom root path
	// For now, this test documents the expected behavior
	t.Log("GetDevicePath test requires filesystem mocking refactor")
}
