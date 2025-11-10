package security

import (
	"strings"
	"testing"
	"time"
)

func TestGetMetrics_Singleton(t *testing.T) {
	metrics1 := GetMetrics()
	metrics2 := GetMetrics()

	if metrics1 != metrics2 {
		t.Error("GetMetrics() should return the same instance")
	}
}

func TestSecurityMetrics_RecordEvent(t *testing.T) {
	metrics := &SecurityMetrics{}

	tests := []struct {
		name        string
		event       *SecurityEvent
		checkMetric func(*SecurityMetrics) int64
		expected    int64
	}{
		{
			name:        "SSH Connection Attempt",
			event:       NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.SSHConnectionAttempts },
			expected:    1,
		},
		{
			name:        "SSH Connection Success",
			event:       NewSecurityEvent(EventSSHConnectionSuccess, CategoryAuthentication, SeverityInfo, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.SSHConnectionSuccesses },
			expected:    1,
		},
		{
			name:        "SSH Connection Failure",
			event:       NewSecurityEvent(EventSSHConnectionFailure, CategoryAuthentication, SeverityError, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.SSHConnectionFailures },
			expected:    1,
		},
		{
			name:        "SSH Host Key Mismatch",
			event:       NewSecurityEvent(EventSSHHostKeyMismatch, CategorySecurityViolation, SeverityCritical, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.SSHHostKeyMismatches },
			expected:    1,
		},
		{
			name:        "Volume Create Request",
			event:       NewSecurityEvent(EventVolumeCreateRequest, CategoryVolumeOperation, SeverityInfo, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.VolumeCreateRequests },
			expected:    1,
		},
		{
			name:        "Volume Create Success",
			event:       NewSecurityEvent(EventVolumeCreateSuccess, CategoryVolumeOperation, SeverityInfo, "Test").WithOperation("Create", 100*time.Millisecond),
			checkMetric: func(m *SecurityMetrics) int64 { return m.VolumeCreateSuccesses },
			expected:    1,
		},
		{
			name:        "Volume Create Failure",
			event:       NewSecurityEvent(EventVolumeCreateFailure, CategoryVolumeOperation, SeverityError, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.VolumeCreateFailures },
			expected:    1,
		},
		{
			name:        "NVMe Connect Success",
			event:       NewSecurityEvent(EventNVMEConnectSuccess, CategoryNetworkAccess, SeverityInfo, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.NVMEConnectSuccesses },
			expected:    1,
		},
		{
			name:        "Validation Failure",
			event:       NewSecurityEvent(EventValidationFailure, CategorySecurityViolation, SeverityCritical, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.ValidationFailures },
			expected:    1,
		},
		{
			name:        "Command Injection Attempt",
			event:       NewSecurityEvent(EventCommandInjectionAttempt, CategorySecurityViolation, SeverityCritical, "Test"),
			checkMetric: func(m *SecurityMetrics) int64 { return m.CommandInjectionAttempts },
			expected:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics.Reset()
			metrics.RecordEvent(tt.event)

			actual := tt.checkMetric(metrics)
			if actual != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, actual)
			}
		})
	}
}

func TestSecurityMetrics_SeverityCounters(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Log events with different severities
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionFailure, CategoryAuthentication, SeverityWarning, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionFailure, CategoryAuthentication, SeverityError, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventSSHHostKeyMismatch, CategorySecurityViolation, SeverityCritical, "Test"))

	if metrics.InfoEvents != 2 {
		t.Errorf("Expected 2 info events, got %d", metrics.InfoEvents)
	}
	if metrics.WarningEvents != 1 {
		t.Errorf("Expected 1 warning event, got %d", metrics.WarningEvents)
	}
	if metrics.ErrorEvents != 1 {
		t.Errorf("Expected 1 error event, got %d", metrics.ErrorEvents)
	}
	if metrics.CriticalEvents != 1 {
		t.Errorf("Expected 1 critical event, got %d", metrics.CriticalEvents)
	}
}

func TestSecurityMetrics_AverageOperationDuration(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Record operations with different durations
	metrics.RecordEvent(NewSecurityEvent(EventVolumeCreateSuccess, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOperation("Create", 100*time.Millisecond))
	metrics.RecordEvent(NewSecurityEvent(EventVolumeDeleteSuccess, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOperation("Delete", 200*time.Millisecond))
	metrics.RecordEvent(NewSecurityEvent(EventVolumeStageSuccess, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOperation("Stage", 300*time.Millisecond))

	// Average should be (100 + 200 + 300) / 3 = 200ms
	expected := 200 * time.Millisecond
	if metrics.AverageOperationDuration != expected {
		t.Errorf("Expected average duration %v, got %v", expected, metrics.AverageOperationDuration)
	}
}

func TestSecurityMetrics_Reset(t *testing.T) {
	metrics := &SecurityMetrics{}

	// Set some values
	metrics.SSHConnectionAttempts = 10
	metrics.VolumeCreateRequests = 5
	metrics.InfoEvents = 15
	metrics.AverageOperationDuration = 100 * time.Millisecond

	// Reset
	metrics.Reset()

	// All values should be zero
	if metrics.SSHConnectionAttempts != 0 {
		t.Errorf("Expected 0 SSH connection attempts after reset, got %d", metrics.SSHConnectionAttempts)
	}
	if metrics.VolumeCreateRequests != 0 {
		t.Errorf("Expected 0 volume create requests after reset, got %d", metrics.VolumeCreateRequests)
	}
	if metrics.InfoEvents != 0 {
		t.Errorf("Expected 0 info events after reset, got %d", metrics.InfoEvents)
	}
	if metrics.AverageOperationDuration != 0 {
		t.Errorf("Expected 0 average duration after reset, got %v", metrics.AverageOperationDuration)
	}
}

func TestSecurityMetrics_String(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Record some events
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionSuccess, CategoryAuthentication, SeverityInfo, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventVolumeCreateRequest, CategoryVolumeOperation, SeverityInfo, "Test"))
	metrics.RecordEvent(NewSecurityEvent(EventVolumeCreateSuccess, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOperation("Create", 100*time.Millisecond))

	str := metrics.String()

	// Check that key metrics are present in string
	expectedSubstrings := []string{
		"SecurityMetrics{",
		"SSH(attempts=1",
		"success=1",
		"VolumeCreate(requests=1",
		"success=1",
		"AvgOpDuration=100ms",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(str, substr) {
			t.Errorf("String() output missing expected substring: %s\nGot: %s", substr, str)
		}
	}
}

func TestSecurityMetrics_Snapshot(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Set some values
	metrics.SSHConnectionAttempts = 10
	metrics.VolumeCreateRequests = 5

	// Take snapshot
	snapshot := metrics.Snapshot()

	// Snapshot should have the same values
	if snapshot.SSHConnectionAttempts != 10 {
		t.Errorf("Expected snapshot SSH attempts 10, got %d", snapshot.SSHConnectionAttempts)
	}
	if snapshot.VolumeCreateRequests != 5 {
		t.Errorf("Expected snapshot volume requests 5, got %d", snapshot.VolumeCreateRequests)
	}

	// Modify original
	metrics.SSHConnectionAttempts = 20

	// Snapshot should be unchanged
	if snapshot.SSHConnectionAttempts != 10 {
		t.Error("Snapshot should be independent of original metrics")
	}
}

func TestSecurityMetrics_LastTimestamps(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Record SSH connection
	sshEvent := NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test")
	metrics.RecordEvent(sshEvent)

	if metrics.LastSSHConnection.IsZero() {
		t.Error("LastSSHConnection should be set")
	}
	if !metrics.LastSSHConnection.Equal(sshEvent.Timestamp) {
		t.Errorf("LastSSHConnection mismatch: expected %v, got %v",
			sshEvent.Timestamp, metrics.LastSSHConnection)
	}

	// Record volume operation
	volEvent := NewSecurityEvent(EventVolumeCreateRequest, CategoryVolumeOperation, SeverityInfo, "Test")
	metrics.RecordEvent(volEvent)

	if metrics.LastVolumeOperation.IsZero() {
		t.Error("LastVolumeOperation should be set")
	}

	// Record security violation
	violationEvent := NewSecurityEvent(EventValidationFailure, CategorySecurityViolation, SeverityCritical, "Test")
	metrics.RecordEvent(violationEvent)

	if metrics.LastSecurityViolation.IsZero() {
		t.Error("LastSecurityViolation should be set")
	}
}

func TestSecurityMetrics_AllVolumeOperations(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Test all volume operation types
	operations := []struct {
		requestEvent EventType
		successEvent EventType
		failureEvent EventType
		requestCheck func(*SecurityMetrics) int64
		successCheck func(*SecurityMetrics) int64
		failureCheck func(*SecurityMetrics) int64
	}{
		{
			EventVolumeCreateRequest, EventVolumeCreateSuccess, EventVolumeCreateFailure,
			func(m *SecurityMetrics) int64 { return m.VolumeCreateRequests },
			func(m *SecurityMetrics) int64 { return m.VolumeCreateSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumeCreateFailures },
		},
		{
			EventVolumeDeleteRequest, EventVolumeDeleteSuccess, EventVolumeDeleteFailure,
			func(m *SecurityMetrics) int64 { return m.VolumeDeleteRequests },
			func(m *SecurityMetrics) int64 { return m.VolumeDeleteSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumeDeleteFailures },
		},
		{
			EventVolumeStageRequest, EventVolumeStageSuccess, EventVolumeStageFailure,
			func(m *SecurityMetrics) int64 { return m.VolumeStageRequests },
			func(m *SecurityMetrics) int64 { return m.VolumeStageSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumeStageFailures },
		},
		{
			EventVolumeUnstageRequest, EventVolumeUnstageSuccess, EventVolumeUnstageFailure,
			func(m *SecurityMetrics) int64 { return m.VolumeUnstageRequests },
			func(m *SecurityMetrics) int64 { return m.VolumeUnstageSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumeUnstageFailures },
		},
		{
			EventVolumePublishRequest, EventVolumePublishSuccess, EventVolumePublishFailure,
			func(m *SecurityMetrics) int64 { return m.VolumePublishRequests },
			func(m *SecurityMetrics) int64 { return m.VolumePublishSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumePublishFailures },
		},
		{
			EventVolumeUnpublishRequest, EventVolumeUnpublishSuccess, EventVolumeUnpublishFailure,
			func(m *SecurityMetrics) int64 { return m.VolumeUnpublishRequests },
			func(m *SecurityMetrics) int64 { return m.VolumeUnpublishSuccesses },
			func(m *SecurityMetrics) int64 { return m.VolumeUnpublishFailures },
		},
	}

	for _, op := range operations {
		metrics.Reset()

		// Record request
		metrics.RecordEvent(NewSecurityEvent(op.requestEvent, CategoryVolumeOperation, SeverityInfo, "Test"))
		if op.requestCheck(metrics) != 1 {
			t.Errorf("Request count mismatch for %s", op.requestEvent)
		}

		// Record success
		metrics.RecordEvent(NewSecurityEvent(op.successEvent, CategoryVolumeOperation, SeverityInfo, "Test"))
		if op.successCheck(metrics) != 1 {
			t.Errorf("Success count mismatch for %s", op.successEvent)
		}

		// Record failure
		metrics.RecordEvent(NewSecurityEvent(op.failureEvent, CategoryVolumeOperation, SeverityError, "Test"))
		if op.failureCheck(metrics) != 1 {
			t.Errorf("Failure count mismatch for %s", op.failureEvent)
		}
	}
}

func TestSecurityMetrics_Concurrency(t *testing.T) {
	metrics := &SecurityMetrics{}
	metrics.Reset()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				metrics.RecordEvent(NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test"))
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 1000 total events
	if metrics.SSHConnectionAttempts != 1000 {
		t.Errorf("Expected 1000 SSH connection attempts, got %d", metrics.SSHConnectionAttempts)
	}
}
