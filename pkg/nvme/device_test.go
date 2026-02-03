package nvme

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCheckDeviceInUse_NonexistentDevice(t *testing.T) {
	// Test with a device that doesn't exist
	// lsof should return exit code 1 (no processes found)
	ctx := context.Background()
	result := CheckDeviceInUse(ctx, "/dev/nonexistent-device-xyz")

	// Should return not in use (lsof exits 1 for no matching files)
	// or an error if lsof itself fails
	if result.InUse {
		t.Errorf("Expected device not in use, got InUse=true with processes: %v", result.Processes)
	}
}

func TestCheckDeviceInUse_CanceledContext(t *testing.T) {
	// Test with already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := CheckDeviceInUse(ctx, "/dev/null")

	// Should handle cancellation gracefully
	// Result depends on timing - may timeout or get context error
	if result.InUse && len(result.Processes) > 0 {
		t.Logf("Context was canceled but lsof still ran (timing): processes=%v", result.Processes)
	}
}

func TestCheckDeviceInUse_DevNull(t *testing.T) {
	// /dev/null might have some processes using it, but test should complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := CheckDeviceInUse(ctx, "/dev/null")

	// Should complete without error (timeout or result)
	if result.TimedOut {
		t.Log("Check timed out - lsof took too long")
	} else if result.Error != nil {
		t.Logf("Check returned error: %v", result.Error)
	} else if result.InUse {
		t.Logf("Device /dev/null in use by: %v", result.Processes)
	} else {
		t.Log("Device /dev/null not in use")
	}
	// All outcomes are valid - test verifies function doesn't panic/hang
}

func TestDeviceUsageResult_Fields(t *testing.T) {
	// Test DeviceUsageResult struct usage
	tests := []struct {
		name   string
		result DeviceUsageResult
	}{
		{
			name:   "not in use",
			result: DeviceUsageResult{InUse: false},
		},
		{
			name: "in use with processes",
			result: DeviceUsageResult{
				InUse:     true,
				Processes: []string{"qemu[PID:1234]", "kubelet[PID:5678]"},
			},
		},
		{
			name: "timed out",
			result: DeviceUsageResult{
				InUse:    false,
				TimedOut: true,
			},
		},
		{
			name: "error",
			result: DeviceUsageResult{
				InUse: false,
				Error: fmt.Errorf("lsof not found"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct is usable
			_ = tt.result.InUse
			_ = tt.result.Processes
			_ = tt.result.TimedOut
			_ = tt.result.Error
		})
	}
}
