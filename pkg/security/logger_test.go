package security

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewSecurityEvent(t *testing.T) {
	event := NewSecurityEvent(
		EventSSHConnectionAttempt,
		CategoryAuthentication,
		SeverityInfo,
		"Test message",
	)

	if event.EventType != EventSSHConnectionAttempt {
		t.Errorf("Expected EventType %s, got %s", EventSSHConnectionAttempt, event.EventType)
	}
	if event.Category != CategoryAuthentication {
		t.Errorf("Expected Category %s, got %s", CategoryAuthentication, event.Category)
	}
	if event.Severity != SeverityInfo {
		t.Errorf("Expected Severity %s, got %s", SeverityInfo, event.Severity)
	}
	if event.Message != "Test message" {
		t.Errorf("Expected Message 'Test message', got '%s'", event.Message)
	}
	if event.Timestamp.IsZero() {
		t.Error("Expected Timestamp to be set, got zero time")
	}
	if event.Details == nil {
		t.Error("Expected Details map to be initialized")
	}
}

func TestSecurityEvent_WithMethods(t *testing.T) {
	event := NewSecurityEvent(
		EventVolumeCreateRequest,
		CategoryVolumeOperation,
		SeverityInfo,
		"Test",
	)

	// Test WithOutcome
	event.WithOutcome(OutcomeSuccess)
	if event.Outcome != OutcomeSuccess {
		t.Errorf("Expected Outcome %s, got %s", OutcomeSuccess, event.Outcome)
	}

	// Test WithIdentity
	event.WithIdentity("testuser", "10.0.0.1", "node1")
	if event.Username != "testuser" || event.SourceIP != "10.0.0.1" || event.NodeID != "node1" {
		t.Errorf("WithIdentity failed: got username=%s, sourceIP=%s, nodeID=%s",
			event.Username, event.SourceIP, event.NodeID)
	}

	// Test WithVolume
	event.WithVolume("vol-123", "pvc-test")
	if event.VolumeID != "vol-123" || event.VolumeName != "pvc-test" {
		t.Errorf("WithVolume failed: got volumeID=%s, volumeName=%s",
			event.VolumeID, event.VolumeName)
	}

	// Test WithK8sContext
	event.WithK8sContext("default", "pod-1", "pvc-1")
	if event.Namespace != "default" || event.PodName != "pod-1" || event.PVCName != "pvc-1" {
		t.Errorf("WithK8sContext failed: got namespace=%s, podName=%s, pvcName=%s",
			event.Namespace, event.PodName, event.PVCName)
	}

	// Test WithTarget
	event.WithTarget("10.0.0.2", "nqn.test")
	if event.TargetIP != "10.0.0.2" || event.NQN != "nqn.test" {
		t.Errorf("WithTarget failed: got targetIP=%s, nqn=%s",
			event.TargetIP, event.NQN)
	}

	// Test WithOperation
	duration := 100 * time.Millisecond
	event.WithOperation("TestOp", duration)
	if event.Operation != "TestOp" || event.Duration != duration {
		t.Errorf("WithOperation failed: got operation=%s, duration=%v",
			event.Operation, event.Duration)
	}

	// Test WithError
	testErr := errors.New("test error")
	event.WithError(testErr)
	if event.Error != "test error" {
		t.Errorf("WithError failed: got error=%s", event.Error)
	}

	// Test WithDetail
	event.WithDetail("key1", "value1")
	if event.Details["key1"] != "value1" {
		t.Errorf("WithDetail failed: got %s", event.Details["key1"])
	}
}

func TestLogger_FormatLogMessage(t *testing.T) {
	logger := NewLogger()

	event := NewSecurityEvent(
		EventSSHConnectionSuccess,
		CategoryAuthentication,
		SeverityInfo,
		"Connection successful",
	).WithIdentity("admin", "10.0.0.1", "node1").
		WithTarget("10.0.0.2", "nqn.test").
		WithVolume("vol-123", "pvc-test").
		WithK8sContext("default", "pod-1", "pvc-1").
		WithOperation("Connect", 50*time.Millisecond).
		WithOutcome(OutcomeSuccess).
		WithDetail("custom", "value")

	msg := logger.formatLogMessage(event)

	// Check that all important fields are present in the log message
	expectedFields := []string{
		"[SECURITY]",
		"category=authentication",
		"type=ssh_connection_success",
		"severity=info",
		"outcome=success",
		"username=admin",
		"source_ip=10.0.0.1",
		"target_ip=10.0.0.2",
		"node_id=node1",
		"namespace=default",
		"pod_name=pod-1",
		"pvc_name=pvc-1",
		"volume_id=vol-123",
		"volume_name=pvc-test",
		"nqn=nqn.test",
		"operation=Connect",
		"duration_ms=50",
		"custom=\"value\"",
	}

	for _, field := range expectedFields {
		if !strings.Contains(msg, field) {
			t.Errorf("Log message missing field: %s\nGot: %s", field, msg)
		}
	}
}

func TestLogger_LogEvent(t *testing.T) {
	logger := NewLogger()

	// Reset metrics before test
	logger.metrics.Reset()

	// Log various events
	event1 := NewSecurityEvent(EventSSHConnectionAttempt, CategoryAuthentication, SeverityInfo, "Test")
	logger.LogEvent(event1)

	event2 := NewSecurityEvent(EventVolumeCreateSuccess, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOperation("Create", 100*time.Millisecond)
	logger.LogEvent(event2)

	event3 := NewSecurityEvent(EventSSHHostKeyMismatch, CategorySecurityViolation, SeverityCritical, "Test")
	logger.LogEvent(event3)

	// Check metrics were updated
	metrics := logger.metrics.Snapshot()

	if metrics.SSHConnectionAttempts != 1 {
		t.Errorf("Expected 1 SSH connection attempt, got %d", metrics.SSHConnectionAttempts)
	}
	if metrics.VolumeCreateSuccesses != 1 {
		t.Errorf("Expected 1 volume create success, got %d", metrics.VolumeCreateSuccesses)
	}
	if metrics.SSHHostKeyMismatches != 1 {
		t.Errorf("Expected 1 host key mismatch, got %d", metrics.SSHHostKeyMismatches)
	}
	if metrics.InfoEvents != 2 {
		t.Errorf("Expected 2 info events, got %d", metrics.InfoEvents)
	}
	if metrics.CriticalEvents != 1 {
		t.Errorf("Expected 1 critical event, got %d", metrics.CriticalEvents)
	}
	if metrics.AverageOperationDuration != 100*time.Millisecond {
		t.Errorf("Expected average duration 100ms, got %v", metrics.AverageOperationDuration)
	}
}

func TestLogger_HelperMethods(t *testing.T) {
	logger := NewLogger()
	logger.metrics.Reset()

	tests := []struct {
		name         string
		logFunc      func()
		expectedType EventType
		checkMetric  func(*SecurityMetrics) int64
	}{
		{
			name:         "SSH Connection Attempt",
			logFunc:      func() { logger.LogSSHConnectionAttempt("admin", "10.0.0.1") },
			expectedType: EventSSHConnectionAttempt,
			checkMetric:  func(m *SecurityMetrics) int64 { return m.SSHConnectionAttempts },
		},
		{
			name:         "SSH Connection Success",
			logFunc:      func() { logger.LogSSHConnectionSuccess("admin", "10.0.0.1") },
			expectedType: EventSSHConnectionSuccess,
			checkMetric:  func(m *SecurityMetrics) int64 { return m.SSHConnectionSuccesses },
		},
		{
			name:         "SSH Connection Failure",
			logFunc:      func() { logger.LogSSHConnectionFailure("admin", "10.0.0.1", errors.New("test")) },
			expectedType: EventSSHConnectionFailure,
			checkMetric:  func(m *SecurityMetrics) int64 { return m.SSHConnectionFailures },
		},
		{
			name:         "SSH Host Key Verified",
			logFunc:      func() { logger.LogSSHHostKeyVerified("10.0.0.1", "fingerprint") },
			expectedType: EventSSHHostKeyVerified,
			checkMetric:  func(m *SecurityMetrics) int64 { return m.InfoEvents }, // No specific metric for this
		},
		{
			name:         "SSH Host Key Mismatch",
			logFunc:      func() { logger.LogSSHHostKeyMismatch("10.0.0.1", "expected", "actual") },
			expectedType: EventSSHHostKeyMismatch,
			checkMetric:  func(m *SecurityMetrics) int64 { return m.SSHHostKeyMismatches },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.metrics.Reset()
			before := logger.metrics.Snapshot()
			tt.logFunc()
			after := logger.metrics.Snapshot()

			beforeCount := tt.checkMetric(&before)
			afterCount := tt.checkMetric(&after)

			if afterCount != beforeCount+1 {
				t.Errorf("Expected metric to increase by 1, got before=%d after=%d", beforeCount, afterCount)
			}
		})
	}
}

func TestLogger_VolumeOperations(t *testing.T) {
	logger := NewLogger()
	logger.metrics.Reset()

	duration := 50 * time.Millisecond

	// Test volume create
	logger.LogVolumeCreate("vol-1", "pvc-1", OutcomeSuccess, nil, duration)
	metrics := logger.metrics.Snapshot()
	if metrics.VolumeCreateSuccesses != 1 {
		t.Errorf("Expected 1 create success, got %d", metrics.VolumeCreateSuccesses)
	}

	// Test volume delete
	logger.LogVolumeDelete("vol-1", "pvc-1", OutcomeFailure, errors.New("test"), duration)
	metrics = logger.metrics.Snapshot()
	if metrics.VolumeDeleteFailures != 1 {
		t.Errorf("Expected 1 delete failure, got %d", metrics.VolumeDeleteFailures)
	}

	// Test volume stage
	logger.LogVolumeStage("vol-1", "node-1", "nqn.test", "10.0.0.1", OutcomeSuccess, nil, duration)
	metrics = logger.metrics.Snapshot()
	if metrics.VolumeStageSuccesses != 1 {
		t.Errorf("Expected 1 stage success, got %d", metrics.VolumeStageSuccesses)
	}

	// Test volume unstage
	logger.LogVolumeUnstage("vol-1", "node-1", "nqn.test", OutcomeSuccess, nil, duration)
	metrics = logger.metrics.Snapshot()
	if metrics.VolumeUnstageSuccesses != 1 {
		t.Errorf("Expected 1 unstage success, got %d", metrics.VolumeUnstageSuccesses)
	}

	// Test volume publish
	logger.LogVolumePublish("vol-1", "node-1", "/mnt/test", OutcomeSuccess, nil, duration)
	metrics = logger.metrics.Snapshot()
	if metrics.VolumePublishSuccesses != 1 {
		t.Errorf("Expected 1 publish success, got %d", metrics.VolumePublishSuccesses)
	}

	// Test volume unpublish
	logger.LogVolumeUnpublish("vol-1", "node-1", "/mnt/test", OutcomeSuccess, nil, duration)
	metrics = logger.metrics.Snapshot()
	if metrics.VolumeUnpublishSuccesses != 1 {
		t.Errorf("Expected 1 unpublish success, got %d", metrics.VolumeUnpublishSuccesses)
	}
}

func TestLogger_NVMEOperations(t *testing.T) {
	logger := NewLogger()
	logger.metrics.Reset()

	// Test NVMe connect
	logger.LogNVMEConnect("nqn.test", "10.0.0.1", "node-1", OutcomeSuccess, nil)
	metrics := logger.metrics.Snapshot()
	if metrics.NVMEConnectSuccesses != 1 {
		t.Errorf("Expected 1 NVMe connect success, got %d", metrics.NVMEConnectSuccesses)
	}

	// Test NVMe disconnect
	logger.LogNVMEDisconnect("nqn.test", "node-1", nil)
	metrics = logger.metrics.Snapshot()
	if metrics.NVMEDisconnects != 1 {
		t.Errorf("Expected 1 NVMe disconnect, got %d", metrics.NVMEDisconnects)
	}
}

func TestLogger_SecurityViolations(t *testing.T) {
	logger := NewLogger()
	logger.metrics.Reset()

	// Test security violation
	details := map[string]string{
		"param": "test",
		"value": "invalid",
	}
	logger.LogSecurityViolation(EventCommandInjectionAttempt, "Test violation", details)

	metrics := logger.metrics.Snapshot()
	if metrics.CommandInjectionAttempts != 1 {
		t.Errorf("Expected 1 command injection attempt, got %d", metrics.CommandInjectionAttempts)
	}
	if metrics.CriticalEvents != 1 {
		t.Errorf("Expected 1 critical event, got %d", metrics.CriticalEvents)
	}

	// Test validation failure
	logger.LogValidationFailure("volumeID", "bad-value", "invalid format")
	metrics = logger.metrics.Snapshot()
	if metrics.ValidationFailures != 1 {
		t.Errorf("Expected 1 validation failure, got %d", metrics.ValidationFailures)
	}
}

func TestGetLogger_Singleton(t *testing.T) {
	logger1 := GetLogger()
	logger2 := GetLogger()

	if logger1 != logger2 {
		t.Error("GetLogger() should return the same instance")
	}
}

func TestLogger_WithError_Nil(t *testing.T) {
	event := NewSecurityEvent(EventVolumeCreateSuccess, CategoryVolumeOperation, SeverityInfo, "Test")
	event.WithError(nil)

	if event.Error != "" {
		t.Errorf("Expected empty error string for nil error, got: %s", event.Error)
	}
}

func TestLogger_FormatLogMessage_MinimalEvent(t *testing.T) {
	logger := NewLogger()

	event := NewSecurityEvent(
		EventSSHConnectionAttempt,
		CategoryAuthentication,
		SeverityInfo,
		"Minimal event",
	)

	msg := logger.formatLogMessage(event)

	// Should have basic fields
	if !strings.Contains(msg, "[SECURITY]") {
		t.Error("Missing [SECURITY] prefix")
	}
	if !strings.Contains(msg, "category=authentication") {
		t.Error("Missing category")
	}
	if !strings.Contains(msg, "type=ssh_connection_attempt") {
		t.Error("Missing event type")
	}
	if !strings.Contains(msg, "msg=\"Minimal event\"") {
		t.Error("Missing message")
	}

	// Should not have optional fields
	if strings.Contains(msg, "username=") {
		t.Error("Should not contain username field")
	}
}

func TestSecurityEvent_Chaining(t *testing.T) {
	// Test method chaining
	event := NewSecurityEvent(EventVolumeCreateRequest, CategoryVolumeOperation, SeverityInfo, "Test").
		WithOutcome(OutcomeSuccess).
		WithIdentity("user1", "10.0.0.1", "node1").
		WithVolume("vol1", "pvc1").
		WithK8sContext("default", "pod1", "pvc1").
		WithTarget("10.0.0.2", "nqn.test").
		WithOperation("Create", 100*time.Millisecond).
		WithError(errors.New("test error")).
		WithDetail("key1", "val1")

	// Verify all fields were set
	if event.Outcome != OutcomeSuccess {
		t.Error("Outcome not set")
	}
	if event.Username != "user1" {
		t.Error("Username not set")
	}
	if event.VolumeID != "vol1" {
		t.Error("VolumeID not set")
	}
	if event.Namespace != "default" {
		t.Error("Namespace not set")
	}
	if event.TargetIP != "10.0.0.2" {
		t.Error("TargetIP not set")
	}
	if event.Operation != "Create" {
		t.Error("Operation not set")
	}
	if event.Error != "test error" {
		t.Error("Error not set")
	}
	if event.Details["key1"] != "val1" {
		t.Error("Detail not set")
	}
}
