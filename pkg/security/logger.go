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

// severityMapping defines how a severity level maps to klog behavior
type severityMapping struct {
	verbosity klog.Level
	logFunc   func(args ...interface{})
}

// severityMap maps EventSeverity to klog verbosity and logging function
// This is a table-driven replacement for the switch in LogEvent()
var severityMap = map[EventSeverity]severityMapping{
	SeverityInfo:     {verbosity: 2, logFunc: func(args ...interface{}) { klog.V(2).Info(args...) }},
	SeverityWarning:  {verbosity: 1, logFunc: klog.Warning},
	SeverityError:    {verbosity: 0, logFunc: klog.Error},
	SeverityCritical: {verbosity: 0, logFunc: klog.Error},
}

// LogEvent logs a security event with structured logging
func (l *Logger) LogEvent(event *SecurityEvent) {
	// Record in metrics
	l.metrics.RecordEvent(event)

	// Look up severity mapping (default to Info if unknown)
	mapping, ok := severityMap[event.Severity]
	if !ok {
		mapping = severityMap[SeverityInfo]
	}
	logFunc := mapping.logFunc

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

// OperationLogConfig defines the configuration for a logging operation
type OperationLogConfig struct {
	Operation    string
	Category     EventCategory
	SuccessType  EventType
	FailureType  EventType
	RequestType  EventType
	SuccessSev   EventSeverity
	FailureSev   EventSeverity
	SuccessMsg   string
	FailureMsg   string
	RequestMsg   string
}

// operationConfigs defines the logging configuration for all operations
var operationConfigs = map[string]OperationLogConfig{
	"VolumeCreate":    {Operation: "CreateVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumeCreateSuccess, FailureType: EventVolumeCreateFailure, RequestType: EventVolumeCreateRequest, SuccessSev: SeverityInfo, FailureSev: SeverityError, SuccessMsg: "Volume created successfully", FailureMsg: "Volume creation failed", RequestMsg: "Volume creation requested"},
	"VolumeDelete":    {Operation: "DeleteVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumeDeleteSuccess, FailureType: EventVolumeDeleteFailure, RequestType: EventVolumeDeleteRequest, SuccessSev: SeverityInfo, FailureSev: SeverityWarning, SuccessMsg: "Volume deleted successfully", FailureMsg: "Volume deletion failed", RequestMsg: "Volume deletion requested"},
	"VolumeStage":     {Operation: "NodeStageVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumeStageSuccess, FailureType: EventVolumeStageFailure, RequestType: EventVolumeStageRequest, SuccessSev: SeverityInfo, FailureSev: SeverityError, SuccessMsg: "Volume staged successfully", FailureMsg: "Volume staging failed", RequestMsg: "Volume staging requested"},
	"VolumeUnstage":   {Operation: "NodeUnstageVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumeUnstageSuccess, FailureType: EventVolumeUnstageFailure, RequestType: EventVolumeUnstageRequest, SuccessSev: SeverityInfo, FailureSev: SeverityWarning, SuccessMsg: "Volume unstaged successfully", FailureMsg: "Volume unstaging failed", RequestMsg: "Volume unstaging requested"},
	"VolumePublish":   {Operation: "NodePublishVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumePublishSuccess, FailureType: EventVolumePublishFailure, RequestType: EventVolumePublishRequest, SuccessSev: SeverityInfo, FailureSev: SeverityError, SuccessMsg: "Volume published successfully", FailureMsg: "Volume publish failed", RequestMsg: "Volume publish requested"},
	"VolumeUnpublish": {Operation: "NodeUnpublishVolume", Category: CategoryVolumeOperation, SuccessType: EventVolumeUnpublishSuccess, FailureType: EventVolumeUnpublishFailure, RequestType: EventVolumeUnpublishRequest, SuccessSev: SeverityInfo, FailureSev: SeverityWarning, SuccessMsg: "Volume unpublished successfully", FailureMsg: "Volume unpublish failed", RequestMsg: "Volume unpublish requested"},
	"NVMEConnect":     {Operation: "NVMEConnect", Category: CategoryNetworkAccess, SuccessType: EventNVMEConnectSuccess, FailureType: EventNVMEConnectFailure, RequestType: EventNVMEConnectAttempt, SuccessSev: SeverityInfo, FailureSev: SeverityError, SuccessMsg: "NVMe connection established", FailureMsg: "NVMe connection failed", RequestMsg: "NVMe connection attempt"},
}

// EventField is a functional option for configuring SecurityEvent fields
type EventField func(*SecurityEvent)

// WithVolume sets volume information
func WithVolume(volumeID, volumeName string) EventField {
	return func(e *SecurityEvent) {
		e.VolumeID = volumeID
		e.VolumeName = volumeName
	}
}

// WithNode sets node information
func WithNode(nodeID string) EventField {
	return func(e *SecurityEvent) {
		e.NodeID = nodeID
	}
}

// WithTarget sets target information
func WithTarget(ip, nqn string) EventField {
	return func(e *SecurityEvent) {
		e.TargetIP = ip
		e.NQN = nqn
	}
}

// WithDuration sets operation duration
func WithDuration(d time.Duration) EventField {
	return func(e *SecurityEvent) {
		e.Duration = d
	}
}

// WithMountPath sets mount path
func WithMountPath(path string) EventField {
	return func(e *SecurityEvent) {
		e.MountPath = path
	}
}

// WithError sets error information
func WithError(err error) EventField {
	return func(e *SecurityEvent) {
		if err != nil {
			e.Error = err.Error()
		}
	}
}

// LogOperation logs an operation using the table-driven configuration
func (l *Logger) LogOperation(config OperationLogConfig, outcome EventOutcome, fields ...EventField) {
	var eventType EventType
	var severity EventSeverity
	var message string

	switch outcome {
	case OutcomeSuccess:
		eventType = config.SuccessType
		severity = config.SuccessSev
		message = config.SuccessMsg
	case OutcomeFailure:
		eventType = config.FailureType
		severity = config.FailureSev
		message = config.FailureMsg
	default:
		eventType = config.RequestType
		severity = SeverityInfo
		message = config.RequestMsg
	}

	event := NewSecurityEvent(eventType, config.Category, severity, message)
	event.Operation = config.Operation
	event.Outcome = outcome

	for _, field := range fields {
		field(event)
	}

	l.LogEvent(event)
}

// LogVolumeCreate logs volume creation events
func (l *Logger) LogVolumeCreate(volumeID, volumeName string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumeCreate"], outcome,
		WithVolume(volumeID, volumeName),
		WithDuration(duration),
		WithError(err))
}

// LogVolumeDelete logs volume deletion events
func (l *Logger) LogVolumeDelete(volumeID, volumeName string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumeDelete"], outcome,
		WithVolume(volumeID, volumeName),
		WithDuration(duration),
		WithError(err))
}

// LogVolumeStage logs volume staging events
func (l *Logger) LogVolumeStage(volumeID, nodeID, nqn, targetIP string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumeStage"], outcome,
		WithVolume(volumeID, ""),
		WithNode(nodeID),
		WithTarget(targetIP, nqn),
		WithDuration(duration),
		WithError(err))
}

// LogVolumeUnstage logs volume unstaging events
func (l *Logger) LogVolumeUnstage(volumeID, nodeID, nqn string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumeUnstage"], outcome,
		WithVolume(volumeID, ""),
		WithNode(nodeID),
		WithTarget("", nqn),
		WithDuration(duration),
		WithError(err))
}

// LogVolumePublish logs volume publish events
func (l *Logger) LogVolumePublish(volumeID, nodeID, mountPath string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumePublish"], outcome,
		WithVolume(volumeID, ""),
		WithNode(nodeID),
		WithMountPath(mountPath),
		WithDuration(duration),
		WithError(err))
}

// LogVolumeUnpublish logs volume unpublish events
func (l *Logger) LogVolumeUnpublish(volumeID, nodeID, mountPath string, outcome EventOutcome, err error, duration time.Duration) {
	l.LogOperation(operationConfigs["VolumeUnpublish"], outcome,
		WithVolume(volumeID, ""),
		WithNode(nodeID),
		WithMountPath(mountPath),
		WithDuration(duration),
		WithError(err))
}

// LogNVMEConnect logs NVMe connection attempts
func (l *Logger) LogNVMEConnect(nqn, address, nodeID string, outcome EventOutcome, err error) {
	l.LogOperation(operationConfigs["NVMEConnect"], outcome,
		WithTarget(address, nqn),
		WithNode(nodeID),
		WithError(err))
}

// LogNVMEDisconnect logs NVMe disconnections
func (l *Logger) LogNVMEDisconnect(nqn, nodeID string, err error) {
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeFailure
	}

	event := NewSecurityEvent(
		EventNVMEDisconnect,
		CategoryNetworkAccess,
		SeverityInfo,
		"NVMe disconnected",
	).WithTarget("", nqn).
		WithIdentity("", "", nodeID).
		WithOutcome(outcome)

	if err != nil {
		event.WithError(err)
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
