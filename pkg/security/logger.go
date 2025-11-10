package security

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Logger provides centralized security event logging
type Logger struct {
	metrics *SecurityMetrics
}

// globalLogger is the global security logger instance
var (
	globalLogger *Logger
	loggerOnce   sync.Once
)

// GetLogger returns the global security logger instance
func GetLogger() *Logger {
	loggerOnce.Do(func() {
		globalLogger = &Logger{
			metrics: GetMetrics(),
		}
	})
	return globalLogger
}

// NewLogger creates a new security logger
func NewLogger() *Logger {
	return &Logger{
		metrics: GetMetrics(),
	}
}

// LogEvent logs a security event with structured logging
func (l *Logger) LogEvent(event *SecurityEvent) {
	// Record in metrics
	l.metrics.RecordEvent(event)

	// Determine klog verbosity and logging function based on severity
	var logFunc func(args ...interface{})
	var verbosity klog.Level

	switch event.Severity {
	case SeverityInfo:
		verbosity = 2
		logFunc = func(args ...interface{}) {
			klog.V(verbosity).Info(args...)
		}
	case SeverityWarning:
		verbosity = 1
		logFunc = klog.Warning
	case SeverityError:
		verbosity = 0
		logFunc = klog.Error
	case SeverityCritical:
		verbosity = 0
		logFunc = klog.Error
	default:
		verbosity = 2
		logFunc = func(args ...interface{}) {
			klog.V(verbosity).Info(args...)
		}
	}

	// Build structured log message
	logMsg := l.formatLogMessage(event)
	logFunc(logMsg)

	// For critical events, also log as JSON for easy parsing
	if event.Severity == SeverityCritical {
		if jsonBytes, err := json.Marshal(event); err == nil {
			klog.Errorf("CRITICAL_SECURITY_EVENT: %s", string(jsonBytes))
		}
	}
}

// formatLogMessage formats a security event as a structured log message
func (l *Logger) formatLogMessage(event *SecurityEvent) string {
	msg := fmt.Sprintf("[SECURITY] category=%s type=%s severity=%s outcome=%s msg=\"%s\"",
		event.Category, event.EventType, event.Severity, event.Outcome, event.Message)

	// Add identity fields
	if event.Username != "" {
		msg += fmt.Sprintf(" username=%s", event.Username)
	}
	if event.SourceIP != "" {
		msg += fmt.Sprintf(" source_ip=%s", event.SourceIP)
	}
	if event.TargetIP != "" {
		msg += fmt.Sprintf(" target_ip=%s", event.TargetIP)
	}
	if event.NodeID != "" {
		msg += fmt.Sprintf(" node_id=%s", event.NodeID)
	}

	// Add Kubernetes context
	if event.Namespace != "" {
		msg += fmt.Sprintf(" namespace=%s", event.Namespace)
	}
	if event.PodName != "" {
		msg += fmt.Sprintf(" pod_name=%s", event.PodName)
	}
	if event.PVCName != "" {
		msg += fmt.Sprintf(" pvc_name=%s", event.PVCName)
	}

	// Add resource fields
	if event.VolumeID != "" {
		msg += fmt.Sprintf(" volume_id=%s", event.VolumeID)
	}
	if event.VolumeName != "" {
		msg += fmt.Sprintf(" volume_name=%s", event.VolumeName)
	}
	if event.NQN != "" {
		msg += fmt.Sprintf(" nqn=%s", event.NQN)
	}
	if event.DevicePath != "" {
		msg += fmt.Sprintf(" device_path=%s", event.DevicePath)
	}
	if event.MountPath != "" {
		msg += fmt.Sprintf(" mount_path=%s", event.MountPath)
	}

	// Add operation details
	if event.Operation != "" {
		msg += fmt.Sprintf(" operation=%s", event.Operation)
	}
	if event.Duration > 0 {
		msg += fmt.Sprintf(" duration_ms=%d", event.Duration.Milliseconds())
	}
	if event.Error != "" {
		msg += fmt.Sprintf(" error=\"%s\"", event.Error)
	}

	// Add custom details
	for key, value := range event.Details {
		msg += fmt.Sprintf(" %s=\"%s\"", key, value)
	}

	// Add timestamp
	msg += fmt.Sprintf(" timestamp=%s", event.Timestamp.Format("2006-01-02T15:04:05.000Z"))

	return msg
}

// Helper methods for common security events

// LogSSHConnectionAttempt logs an SSH connection attempt
func (l *Logger) LogSSHConnectionAttempt(username, address string) {
	event := NewSecurityEvent(
		EventSSHConnectionAttempt,
		CategoryAuthentication,
		SeverityInfo,
		"SSH connection attempt",
	).WithIdentity(username, "", "").
		WithTarget(address, "")
	l.LogEvent(event)
}

// LogSSHConnectionSuccess logs a successful SSH connection
func (l *Logger) LogSSHConnectionSuccess(username, address string) {
	event := NewSecurityEvent(
		EventSSHConnectionSuccess,
		CategoryAuthentication,
		SeverityInfo,
		"SSH connection established",
	).WithIdentity(username, "", "").
		WithTarget(address, "").
		WithOutcome(OutcomeSuccess)
	l.LogEvent(event)
}

// LogSSHConnectionFailure logs a failed SSH connection
func (l *Logger) LogSSHConnectionFailure(username, address string, err error) {
	event := NewSecurityEvent(
		EventSSHConnectionFailure,
		CategoryAuthentication,
		SeverityError,
		"SSH connection failed",
	).WithIdentity(username, "", "").
		WithTarget(address, "").
		WithOutcome(OutcomeFailure).
		WithError(err)
	l.LogEvent(event)
}

// LogSSHHostKeyVerified logs successful SSH host key verification
func (l *Logger) LogSSHHostKeyVerified(address, fingerprint string) {
	event := NewSecurityEvent(
		EventSSHHostKeyVerified,
		CategoryAuthentication,
		SeverityInfo,
		"SSH host key verified",
	).WithTarget(address, "").
		WithDetail("fingerprint", fingerprint).
		WithOutcome(OutcomeSuccess)
	l.LogEvent(event)
}

// LogSSHHostKeyMismatch logs an SSH host key mismatch (critical security event)
func (l *Logger) LogSSHHostKeyMismatch(address, expectedFingerprint, actualFingerprint string) {
	event := NewSecurityEvent(
		EventSSHHostKeyMismatch,
		CategorySecurityViolation,
		SeverityCritical,
		"SSH host key verification failed - possible MITM attack",
	).WithTarget(address, "").
		WithDetail("expected_fingerprint", expectedFingerprint).
		WithDetail("actual_fingerprint", actualFingerprint).
		WithOutcome(OutcomeDenied)
	l.LogEvent(event)
}

// LogVolumeCreate logs volume creation events
func (l *Logger) LogVolumeCreate(volumeID, volumeName string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumeCreateSuccess
		severity = SeverityInfo
		message = "Volume created successfully"
	case OutcomeFailure:
		eventType = EventVolumeCreateFailure
		severity = SeverityError
		message = "Volume creation failed"
	default:
		eventType = EventVolumeCreateRequest
		severity = SeverityInfo
		message = "Volume creation requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, volumeName).
		WithOutcome(outcome).
		WithOperation("CreateVolume", duration)

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogVolumeDelete logs volume deletion events
func (l *Logger) LogVolumeDelete(volumeID, volumeName string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumeDeleteSuccess
		severity = SeverityInfo
		message = "Volume deleted successfully"
	case OutcomeFailure:
		eventType = EventVolumeDeleteFailure
		severity = SeverityWarning
		message = "Volume deletion failed"
	default:
		eventType = EventVolumeDeleteRequest
		severity = SeverityInfo
		message = "Volume deletion requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, volumeName).
		WithOutcome(outcome).
		WithOperation("DeleteVolume", duration)

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogVolumeStage logs volume staging events
func (l *Logger) LogVolumeStage(volumeID, nodeID, nqn, targetIP string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumeStageSuccess
		severity = SeverityInfo
		message = "Volume staged successfully"
	case OutcomeFailure:
		eventType = EventVolumeStageFailure
		severity = SeverityError
		message = "Volume staging failed"
	default:
		eventType = EventVolumeStageRequest
		severity = SeverityInfo
		message = "Volume staging requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, "").
		WithIdentity("", "", nodeID).
		WithTarget(targetIP, nqn).
		WithOutcome(outcome).
		WithOperation("NodeStageVolume", duration)

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogVolumeUnstage logs volume unstaging events
func (l *Logger) LogVolumeUnstage(volumeID, nodeID, nqn string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumeUnstageSuccess
		severity = SeverityInfo
		message = "Volume unstaged successfully"
	case OutcomeFailure:
		eventType = EventVolumeUnstageFailure
		severity = SeverityWarning
		message = "Volume unstaging failed"
	default:
		eventType = EventVolumeUnstageRequest
		severity = SeverityInfo
		message = "Volume unstaging requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, "").
		WithIdentity("", "", nodeID).
		WithTarget("", nqn).
		WithOutcome(outcome).
		WithOperation("NodeUnstageVolume", duration)

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogVolumePublish logs volume publish events
func (l *Logger) LogVolumePublish(volumeID, nodeID, mountPath string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumePublishSuccess
		severity = SeverityInfo
		message = "Volume published successfully"
	case OutcomeFailure:
		eventType = EventVolumePublishFailure
		severity = SeverityError
		message = "Volume publish failed"
	default:
		eventType = EventVolumePublishRequest
		severity = SeverityInfo
		message = "Volume publish requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, "").
		WithIdentity("", "", nodeID).
		WithOutcome(outcome).
		WithOperation("NodePublishVolume", duration)

	event.MountPath = mountPath

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogVolumeUnpublish logs volume unpublish events
func (l *Logger) LogVolumeUnpublish(volumeID, nodeID, mountPath string, outcome EventOutcome, err error, duration time.Duration) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventVolumeUnpublishSuccess
		severity = SeverityInfo
		message = "Volume unpublished successfully"
	case OutcomeFailure:
		eventType = EventVolumeUnpublishFailure
		severity = SeverityWarning
		message = "Volume unpublish failed"
	default:
		eventType = EventVolumeUnpublishRequest
		severity = SeverityInfo
		message = "Volume unpublish requested"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryVolumeOperation,
		severity,
		message,
	).WithVolume(volumeID, "").
		WithIdentity("", "", nodeID).
		WithOutcome(outcome).
		WithOperation("NodeUnpublishVolume", duration)

	event.MountPath = mountPath

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogNVMEConnect logs NVMe connection attempts
func (l *Logger) LogNVMEConnect(nqn, address, nodeID string, outcome EventOutcome, err error) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = EventNVMEConnectSuccess
		severity = SeverityInfo
		message = "NVMe connection established"
	case OutcomeFailure:
		eventType = EventNVMEConnectFailure
		severity = SeverityError
		message = "NVMe connection failed"
	default:
		eventType = EventNVMEConnectAttempt
		severity = SeverityInfo
		message = "NVMe connection attempt"
	}

	event := NewSecurityEvent(
		eventType,
		CategoryNetworkAccess,
		severity,
		message,
	).WithTarget(address, nqn).
		WithIdentity("", "", nodeID).
		WithOutcome(outcome)

	if err != nil {
		event.WithError(err)
	}

	l.LogEvent(event)
}

// LogNVMEDisconnect logs NVMe disconnections
func (l *Logger) LogNVMEDisconnect(nqn, nodeID string, err error) {
	event := NewSecurityEvent(
		EventNVMEDisconnect,
		CategoryNetworkAccess,
		SeverityInfo,
		"NVMe disconnected",
	).WithTarget("", nqn).
		WithIdentity("", "", nodeID).
		WithOutcome(OutcomeSuccess)

	if err != nil {
		event.WithError(err)
		event.Outcome = OutcomeFailure
	}

	l.LogEvent(event)
}

// LogSecurityViolation logs security violations
func (l *Logger) LogSecurityViolation(eventType EventType, message string, details map[string]string) {
	event := NewSecurityEvent(
		eventType,
		CategorySecurityViolation,
		SeverityCritical,
		message,
	).WithOutcome(OutcomeDenied)

	for key, value := range details {
		event.WithDetail(key, value)
	}

	l.LogEvent(event)
}

// LogValidationFailure logs validation failures
func (l *Logger) LogValidationFailure(parameter, value, reason string) {
	l.LogSecurityViolation(
		EventValidationFailure,
		"Validation failure",
		map[string]string{
			"parameter": parameter,
			"value":     value,
			"reason":    reason,
		},
	)
}

// GetMetrics returns the current security metrics
func (l *Logger) GetMetrics() *SecurityMetrics {
	return l.metrics
}
