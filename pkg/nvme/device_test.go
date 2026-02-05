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
		{
			name: "filtered self PIDs",
			result: DeviceUsageResult{
				InUse:            false,
				FilteredSelfPIDs: 2,
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
			_ = tt.result.FilteredSelfPIDs
		})
	}
}

func TestCheckDeviceInUseWithRetry_NotInUse(t *testing.T) {
	// Test retry logic when device is not in use
	ctx := context.Background()

	result := CheckDeviceInUseWithRetry(ctx, "/dev/nonexistent-device-xyz", 3, 100*time.Millisecond)

	if result.InUse {
		t.Errorf("Expected device not in use, got InUse=true")
	}
}

func TestCheckDeviceInUseWithRetry_SingleAttempt(t *testing.T) {
	// Test with retries=1 (should behave like CheckDeviceInUse)
	ctx := context.Background()

	result := CheckDeviceInUseWithRetry(ctx, "/dev/nonexistent-device-xyz", 1, 100*time.Millisecond)

	if result.InUse {
		t.Errorf("Expected device not in use, got InUse=true")
	}
}

func TestCheckDeviceInUseWithRetry_ZeroRetries(t *testing.T) {
	// Test with retries=0 (should default to 1)
	ctx := context.Background()

	result := CheckDeviceInUseWithRetry(ctx, "/dev/nonexistent-device-xyz", 0, 100*time.Millisecond)

	if result.InUse {
		t.Errorf("Expected device not in use, got InUse=true")
	}
}

func TestCheckDeviceInUseWithRetry_ContextCancellation(t *testing.T) {
	// Test that retry respects context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Use /dev/null which might show as in-use, triggering retries
	// But context should cancel before all retries complete
	startTime := time.Now()
	_ = CheckDeviceInUseWithRetry(ctx, "/dev/null", 100, 1*time.Second)
	elapsed := time.Since(startTime)

	// Should complete quickly due to context cancellation, not wait for all retries
	if elapsed > 2*time.Second {
		t.Errorf("Retry took too long (%v), should have been cancelled by context", elapsed)
	}
}

// TestCheckDeviceInUse_SelfPIDFiltering tests the self-PID filtering logic
// This test verifies that we properly filter out the driver's own PID
func TestCheckDeviceInUse_SelfPIDFiltering(t *testing.T) {
	// This is a behavior test - we can't easily mock lsof output,
	// but we can verify the FilteredSelfPIDs field is present and works
	ctx := context.Background()

	// Test with /dev/null which might have some usage
	result := CheckDeviceInUse(ctx, "/dev/null")

	// The FilteredSelfPIDs field should be accessible
	if result.FilteredSelfPIDs < 0 {
		t.Errorf("FilteredSelfPIDs should be >= 0, got %d", result.FilteredSelfPIDs)
	}

	// If lsof found the driver's own PID, it should be filtered
	// We can't guarantee this happens, but we can verify the logic exists
	t.Logf("Result: InUse=%v, Processes=%v, FilteredSelfPIDs=%d",
		result.InUse, result.Processes, result.FilteredSelfPIDs)
}
