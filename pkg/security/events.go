package security

import "time"

// EventCategory represents the category of a security event
type EventCategory string

const (
	// CategoryAuthentication represents authentication-related events
	CategoryAuthentication EventCategory = "authentication"

	// CategoryAuthorization represents authorization-related events
	CategoryAuthorization EventCategory = "authorization"

	// CategoryVolumeOperation represents volume lifecycle operations
	CategoryVolumeOperation EventCategory = "volume_operation"

	// CategoryNetworkAccess represents network connection events
	CategoryNetworkAccess EventCategory = "network_access"

	// CategoryDataAccess represents data access events
	CategoryDataAccess EventCategory = "data_access"

	// CategoryConfigChange represents configuration changes
	CategoryConfigChange EventCategory = "config_change"

	// CategorySecurityViolation represents potential security violations
	CategorySecurityViolation EventCategory = "security_violation"
)

// EventSeverity represents the severity level of a security event
type EventSeverity string

const (
	// SeverityInfo represents informational events
	SeverityInfo EventSeverity = "info"

	// SeverityWarning represents warning events
	SeverityWarning EventSeverity = "warning"

	// SeverityError represents error events
	SeverityError EventSeverity = "error"

	// SeverityCritical represents critical security events
	SeverityCritical EventSeverity = "critical"
)

// EventOutcome represents the outcome of a security event
type EventOutcome string

const (
	// OutcomeSuccess indicates the operation succeeded
	OutcomeSuccess EventOutcome = "success"

	// OutcomeFailure indicates the operation failed
	OutcomeFailure EventOutcome = "failure"

	// OutcomeDenied indicates the operation was denied
	OutcomeDenied EventOutcome = "denied"

	// OutcomeUnknown indicates the outcome is unknown
	OutcomeUnknown EventOutcome = "unknown"
)

// EventType represents specific types of security events
type EventType string

const (
	// Authentication events
	EventSSHConnectionAttempt   EventType = "ssh_connection_attempt"
	EventSSHConnectionSuccess   EventType = "ssh_connection_success"
	EventSSHConnectionFailure   EventType = "ssh_connection_failure"
	EventSSHHostKeyVerified     EventType = "ssh_host_key_verified"
	EventSSHHostKeyMismatch     EventType = "ssh_host_key_mismatch"
	EventSSHAuthSuccess         EventType = "ssh_auth_success"
	EventSSHAuthFailure         EventType = "ssh_auth_failure"

	// Volume operation events
	EventVolumeCreateRequest    EventType = "volume_create_request"
	EventVolumeCreateSuccess    EventType = "volume_create_success"
	EventVolumeCreateFailure    EventType = "volume_create_failure"
	EventVolumeDeleteRequest    EventType = "volume_delete_request"
	EventVolumeDeleteSuccess    EventType = "volume_delete_success"
	EventVolumeDeleteFailure    EventType = "volume_delete_failure"
	EventVolumeStageRequest     EventType = "volume_stage_request"
	EventVolumeStageSuccess     EventType = "volume_stage_success"
	EventVolumeStageFailure     EventType = "volume_stage_failure"
	EventVolumeUnstageRequest   EventType = "volume_unstage_request"
	EventVolumeUnstageSuccess   EventType = "volume_unstage_success"
	EventVolumeUnstageFailure   EventType = "volume_unstage_failure"
	EventVolumePublishRequest   EventType = "volume_publish_request"
	EventVolumePublishSuccess   EventType = "volume_publish_success"
	EventVolumePublishFailure   EventType = "volume_publish_failure"
	EventVolumeUnpublishRequest EventType = "volume_unpublish_request"
	EventVolumeUnpublishSuccess EventType = "volume_unpublish_success"
	EventVolumeUnpublishFailure EventType = "volume_unpublish_failure"

	// Network access events
	EventNVMEConnectAttempt EventType = "nvme_connect_attempt"
	EventNVMEConnectSuccess EventType = "nvme_connect_success"
	EventNVMEConnectFailure EventType = "nvme_connect_failure"
	EventNVMEDisconnect     EventType = "nvme_disconnect"

	// Data access events
	EventMountAttempt  EventType = "mount_attempt"
	EventMountSuccess  EventType = "mount_success"
	EventMountFailure  EventType = "mount_failure"
	EventUnmountAttempt EventType = "unmount_attempt"
	EventUnmountSuccess EventType = "unmount_success"
	EventUnmountFailure EventType = "unmount_failure"

	// Security violation events
	EventValidationFailure      EventType = "validation_failure"
	EventInvalidParameter       EventType = "invalid_parameter"
	EventCommandInjectionAttempt EventType = "command_injection_attempt"
	EventPathTraversalAttempt   EventType = "path_traversal_attempt"
	EventRateLimitExceeded      EventType = "rate_limit_exceeded"
	EventCircuitBreakerOpen     EventType = "circuit_breaker_open"
)

// SecurityEvent represents a security-relevant event in the system
type SecurityEvent struct {
	// Core event fields
	Timestamp time.Time     `json:"timestamp"`
	EventType EventType     `json:"event_type"`
	Category  EventCategory `json:"category"`
	Severity  EventSeverity `json:"severity"`
	Outcome   EventOutcome  `json:"outcome"`
	Message   string        `json:"message"`

	// Identity fields
	SourceIP   string `json:"source_ip,omitempty"`
	TargetIP   string `json:"target_ip,omitempty"`
	Username   string `json:"username,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	PodName    string `json:"pod_name,omitempty"`
	PVCName    string `json:"pvc_name,omitempty"`

	// Resource fields
	VolumeID   string `json:"volume_id,omitempty"`
	VolumeName string `json:"volume_name,omitempty"`
	DevicePath string `json:"device_path,omitempty"`
	MountPath  string `json:"mount_path,omitempty"`
	NQN        string `json:"nqn,omitempty"`

	// Operation details
	Operation string            `json:"operation,omitempty"`
	Duration  time.Duration     `json:"duration_ms,omitempty"`
	Error     string            `json:"error,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// NewSecurityEvent creates a new security event with timestamp
func NewSecurityEvent(eventType EventType, category EventCategory, severity EventSeverity, message string) *SecurityEvent {
	return &SecurityEvent{
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		Category:  category,
		Severity:  severity,
		Message:   message,
		Details:   make(map[string]string),
	}
}

// WithOutcome sets the outcome for the event
func (e *SecurityEvent) WithOutcome(outcome EventOutcome) *SecurityEvent {
	e.Outcome = outcome
	return e
}

// WithIdentity sets identity information for the event
func (e *SecurityEvent) WithIdentity(username, sourceIP, nodeID string) *SecurityEvent {
	e.Username = username
	e.SourceIP = sourceIP
	e.NodeID = nodeID
	return e
}

// WithVolume sets volume-related information for the event
func (e *SecurityEvent) WithVolume(volumeID, volumeName string) *SecurityEvent {
	e.VolumeID = volumeID
	e.VolumeName = volumeName
	return e
}

// WithK8sContext sets Kubernetes context information
func (e *SecurityEvent) WithK8sContext(namespace, podName, pvcName string) *SecurityEvent {
	e.Namespace = namespace
	e.PodName = podName
	e.PVCName = pvcName
	return e
}

// WithTarget sets target information
func (e *SecurityEvent) WithTarget(targetIP, nqn string) *SecurityEvent {
	e.TargetIP = targetIP
	e.NQN = nqn
	return e
}

// WithOperation sets operation details
func (e *SecurityEvent) WithOperation(operation string, duration time.Duration) *SecurityEvent {
	e.Operation = operation
	e.Duration = duration
	return e
}

// WithError sets error information
func (e *SecurityEvent) WithError(err error) *SecurityEvent {
	if err != nil {
		e.Error = err.Error()
	}
	return e
}

// WithDetail adds a custom detail field
func (e *SecurityEvent) WithDetail(key, value string) *SecurityEvent {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}
