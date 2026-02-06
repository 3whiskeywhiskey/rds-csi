package mock

import (
	"os"
	"testing"
	"time"
)

// TestLoadConfigFromEnv_Defaults validates default configuration values when no env vars are set
func TestLoadConfigFromEnv_Defaults(t *testing.T) {
	// Clear all mock RDS env vars to ensure clean defaults
	os.Unsetenv("MOCK_RDS_REALISTIC_TIMING")
	os.Unsetenv("MOCK_RDS_SSH_LATENCY_MS")
	os.Unsetenv("MOCK_RDS_SSH_LATENCY_JITTER_MS")
	os.Unsetenv("MOCK_RDS_DISK_ADD_DELAY_MS")
	os.Unsetenv("MOCK_RDS_DISK_REMOVE_DELAY_MS")
	os.Unsetenv("MOCK_RDS_ERROR_MODE")
	os.Unsetenv("MOCK_RDS_ERROR_AFTER_N")
	os.Unsetenv("MOCK_RDS_ENABLE_HISTORY")
	os.Unsetenv("MOCK_RDS_HISTORY_DEPTH")
	os.Unsetenv("MOCK_RDS_ROUTEROS_VERSION")

	config := LoadConfigFromEnv()

	// Validate defaults
	if config.RealisticTiming != false {
		t.Errorf("expected RealisticTiming=false, got %v", config.RealisticTiming)
	}
	if config.SSHLatencyMs != 200 {
		t.Errorf("expected SSHLatencyMs=200, got %d", config.SSHLatencyMs)
	}
	if config.SSHLatencyJitterMs != 50 {
		t.Errorf("expected SSHLatencyJitterMs=50, got %d", config.SSHLatencyJitterMs)
	}
	if config.DiskAddDelayMs != 500 {
		t.Errorf("expected DiskAddDelayMs=500, got %d", config.DiskAddDelayMs)
	}
	if config.DiskRemoveDelayMs != 300 {
		t.Errorf("expected DiskRemoveDelayMs=300, got %d", config.DiskRemoveDelayMs)
	}
	if config.ErrorMode != "none" {
		t.Errorf("expected ErrorMode=none, got %s", config.ErrorMode)
	}
	if config.ErrorAfterN != 0 {
		t.Errorf("expected ErrorAfterN=0, got %d", config.ErrorAfterN)
	}
	if config.EnableHistory != true {
		t.Errorf("expected EnableHistory=true, got %v", config.EnableHistory)
	}
	if config.HistoryDepth != 100 {
		t.Errorf("expected HistoryDepth=100, got %d", config.HistoryDepth)
	}
	if config.RouterOSVersion != "7.16" {
		t.Errorf("expected RouterOSVersion=7.16, got %s", config.RouterOSVersion)
	}
}

// TestLoadConfigFromEnv_RealisticTiming tests realistic timing configuration
func TestLoadConfigFromEnv_RealisticTiming(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true string", "true", true},
		{"1 as true", "1", true},
		{"yes as true", "yes", true},
		{"false string", "false", false},
		{"0 as false", "0", false},
		{"empty as false", "", false},
		{"invalid as false", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				os.Unsetenv("MOCK_RDS_REALISTIC_TIMING")
			} else {
				t.Setenv("MOCK_RDS_REALISTIC_TIMING", tt.value)
			}

			config := LoadConfigFromEnv()

			if config.RealisticTiming != tt.expected {
				t.Errorf("expected RealisticTiming=%v, got %v", tt.expected, config.RealisticTiming)
			}
		})
	}
}

// TestLoadConfigFromEnv_ErrorMode tests error mode configuration
func TestLoadConfigFromEnv_ErrorMode(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"none", "none", "none"},
		{"disk_full", "disk_full", "disk_full"},
		{"ssh_timeout", "ssh_timeout", "ssh_timeout"},
		{"command_fail", "command_fail", "command_fail"},
		{"empty defaults to none", "", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				os.Unsetenv("MOCK_RDS_ERROR_MODE")
			} else {
				t.Setenv("MOCK_RDS_ERROR_MODE", tt.value)
			}

			config := LoadConfigFromEnv()

			if config.ErrorMode != tt.expected {
				t.Errorf("expected ErrorMode=%s, got %s", tt.expected, config.ErrorMode)
			}
		})
	}
}

// TestLoadConfigFromEnv_IntegerParsing tests integer environment variable parsing
func TestLoadConfigFromEnv_IntegerParsing(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		value       string
		expectedVal int
		defaultVal  int
	}{
		{"valid SSH latency", "MOCK_RDS_SSH_LATENCY_MS", "300", 300, 200},
		{"invalid SSH latency uses default", "MOCK_RDS_SSH_LATENCY_MS", "invalid", 200, 200},
		{"empty SSH latency uses default", "MOCK_RDS_SSH_LATENCY_MS", "", 200, 200},
		{"valid error after N", "MOCK_RDS_ERROR_AFTER_N", "5", 5, 0},
		{"invalid error after N uses default", "MOCK_RDS_ERROR_AFTER_N", "abc", 0, 0},
		{"valid history depth", "MOCK_RDS_HISTORY_DEPTH", "50", 50, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				os.Unsetenv(tt.envVar)
			} else {
				t.Setenv(tt.envVar, tt.value)
			}

			config := LoadConfigFromEnv()

			var actualVal int
			switch tt.envVar {
			case "MOCK_RDS_SSH_LATENCY_MS":
				actualVal = config.SSHLatencyMs
			case "MOCK_RDS_ERROR_AFTER_N":
				actualVal = config.ErrorAfterN
			case "MOCK_RDS_HISTORY_DEPTH":
				actualVal = config.HistoryDepth
			}

			if actualVal != tt.expectedVal {
				t.Errorf("expected %s=%d, got %d", tt.envVar, tt.expectedVal, actualVal)
			}
		})
	}
}

// TestTimingSimulator_Disabled validates timing is instant when disabled
func TestTimingSimulator_Disabled(t *testing.T) {
	config := MockRDSConfig{RealisticTiming: false}
	timing := NewTimingSimulator(config)

	// Should return immediately
	start := time.Now()
	timing.SimulateSSHLatency()
	elapsed := time.Since(start)

	// Allow 10ms for overhead (shouldn't sleep at all)
	if elapsed > 10*time.Millisecond {
		t.Errorf("timing should be disabled, but took %v", elapsed)
	}
}

// TestTimingSimulator_Enabled validates realistic timing delays
func TestTimingSimulator_Enabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}

	config := MockRDSConfig{
		RealisticTiming:    true,
		SSHLatencyMs:       200,
		SSHLatencyJitterMs: 50,
	}
	timing := NewTimingSimulator(config)

	start := time.Now()
	timing.SimulateSSHLatency()
	elapsed := time.Since(start)

	// Should be between 150-250ms (200ms ± 50ms)
	if elapsed < 150*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Errorf("expected latency 150-250ms, got %v", elapsed)
	}
}

// TestTimingSimulator_DiskOperations validates disk operation delays
func TestTimingSimulator_DiskOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}

	config := MockRDSConfig{
		RealisticTiming:   true,
		DiskAddDelayMs:    100,
		DiskRemoveDelayMs: 50,
	}
	timing := NewTimingSimulator(config)

	// Test disk add delay
	start := time.Now()
	timing.SimulateDiskOperation("add")
	elapsed := time.Since(start)

	// Should be approximately 100ms (allow ±10ms for overhead)
	if elapsed < 90*time.Millisecond || elapsed > 110*time.Millisecond {
		t.Errorf("expected disk add delay ~100ms, got %v", elapsed)
	}

	// Test disk remove delay
	start = time.Now()
	timing.SimulateDiskOperation("remove")
	elapsed = time.Since(start)

	// Should be approximately 50ms (allow ±10ms for overhead)
	if elapsed < 40*time.Millisecond || elapsed > 60*time.Millisecond {
		t.Errorf("expected disk remove delay ~50ms, got %v", elapsed)
	}

	// Test unknown operation (should be instant)
	start = time.Now()
	timing.SimulateDiskOperation("unknown")
	elapsed = time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("unknown operation should be instant, but took %v", elapsed)
	}
}

// TestErrorInjector_None validates no error injection in none mode
func TestErrorInjector_None(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "none"}
	injector := NewErrorInjector(config)

	shouldFail, _ := injector.ShouldFailDiskAdd()
	if shouldFail {
		t.Error("expected no error injection in none mode")
	}

	shouldFail, _ = injector.ShouldFailDiskRemove()
	if shouldFail {
		t.Error("expected no error injection in none mode")
	}
}

// TestErrorInjector_DiskFull validates disk full error injection
func TestErrorInjector_DiskFull(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "disk_full", ErrorAfterN: 0}
	injector := NewErrorInjector(config)

	shouldFail, errMsg := injector.ShouldFailDiskAdd()
	if !shouldFail {
		t.Error("expected disk full error")
	}
	if errMsg != "failure: not enough space\n" {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

// TestErrorInjector_CommandFail validates command failure error injection
func TestErrorInjector_CommandFail(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "command_fail", ErrorAfterN: 0}
	injector := NewErrorInjector(config)

	// Test disk add failure
	shouldFail, errMsg := injector.ShouldFailDiskAdd()
	if !shouldFail {
		t.Error("expected command fail error on disk add")
	}
	if errMsg != "failure: execution error\n" {
		t.Errorf("unexpected error message: %s", errMsg)
	}

	// Test disk remove failure
	shouldFail, errMsg = injector.ShouldFailDiskRemove()
	if !shouldFail {
		t.Error("expected command fail error on disk remove")
	}
	if errMsg != "failure: execution error\n" {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

// TestErrorInjector_AfterN validates trigger after N operations
func TestErrorInjector_AfterN(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "disk_full", ErrorAfterN: 2}
	injector := NewErrorInjector(config)

	// First call - should not fail
	shouldFail, _ := injector.ShouldFailDiskAdd()
	if shouldFail {
		t.Error("first call should not fail (ErrorAfterN=2)")
	}

	// Second call - should not fail
	shouldFail, _ = injector.ShouldFailDiskAdd()
	if shouldFail {
		t.Error("second call should not fail (ErrorAfterN=2)")
	}

	// Third call - should fail
	shouldFail, _ = injector.ShouldFailDiskAdd()
	if !shouldFail {
		t.Error("third call should fail (ErrorAfterN=2)")
	}
}

// TestErrorInjector_Reset validates operation counter reset
func TestErrorInjector_Reset(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "disk_full", ErrorAfterN: 1}
	injector := NewErrorInjector(config)

	// First call - should not fail
	shouldFail, _ := injector.ShouldFailDiskAdd()
	if shouldFail {
		t.Error("first call should not fail")
	}

	// Second call - should fail
	shouldFail, _ = injector.ShouldFailDiskAdd()
	if !shouldFail {
		t.Error("second call should fail")
	}

	// Reset
	injector.Reset()

	// After reset - should not fail again (counter reset)
	shouldFail, _ = injector.ShouldFailDiskAdd()
	if shouldFail {
		t.Error("after reset, first call should not fail")
	}
}

// TestParseErrorMode validates error mode string parsing
func TestParseErrorMode(t *testing.T) {
	tests := []struct {
		input    string
		expected ErrorMode
	}{
		{"none", ErrorModeNone},
		{"", ErrorModeNone},
		{"disk_full", ErrorModeDiskFull},
		{"ssh_timeout", ErrorModeSSHTimeout},
		{"command_fail", ErrorModeCommandFail},
		{"invalid", ErrorModeNone}, // Unknown defaults to none
		{"INVALID", ErrorModeNone}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseErrorMode(tt.input)
			if got != tt.expected {
				t.Errorf("ParseErrorMode(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// TestErrorInjector_SSHTimeout validates SSH timeout error injection
func TestErrorInjector_SSHTimeout(t *testing.T) {
	config := MockRDSConfig{ErrorMode: "ssh_timeout", ErrorAfterN: 1}
	injector := NewErrorInjector(config)

	// First call - should not fail
	shouldFail := injector.ShouldFailSSHConnect()
	if shouldFail {
		t.Error("first SSH connection should not fail")
	}

	// Second call - should fail
	shouldFail = injector.ShouldFailSSHConnect()
	if !shouldFail {
		t.Error("second SSH connection should fail")
	}
}
