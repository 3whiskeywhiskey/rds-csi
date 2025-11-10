package security

import (
	"fmt"
	"sync"
	"time"
)

// SecurityMetrics tracks security-related metrics
type SecurityMetrics struct {
	mu sync.RWMutex

	// Authentication metrics
	SSHConnectionAttempts int64 `json:"ssh_connection_attempts"`
	SSHConnectionSuccesses int64 `json:"ssh_connection_successes"`
	SSHConnectionFailures int64 `json:"ssh_connection_failures"`
	SSHHostKeyMismatches  int64 `json:"ssh_host_key_mismatches"`
	SSHAuthFailures       int64 `json:"ssh_auth_failures"`

	// Volume operation metrics
	VolumeCreateRequests  int64 `json:"volume_create_requests"`
	VolumeCreateSuccesses int64 `json:"volume_create_successes"`
	VolumeCreateFailures  int64 `json:"volume_create_failures"`
	VolumeDeleteRequests  int64 `json:"volume_delete_requests"`
	VolumeDeleteSuccesses int64 `json:"volume_delete_successes"`
	VolumeDeleteFailures  int64 `json:"volume_delete_failures"`
	VolumeStageRequests   int64 `json:"volume_stage_requests"`
	VolumeStageSuccesses  int64 `json:"volume_stage_successes"`
	VolumeStageFailures   int64 `json:"volume_stage_failures"`
	VolumeUnstageRequests  int64 `json:"volume_unstage_requests"`
	VolumeUnstageSuccesses int64 `json:"volume_unstage_successes"`
	VolumeUnstageFailures  int64 `json:"volume_unstage_failures"`
	VolumePublishRequests  int64 `json:"volume_publish_requests"`
	VolumePublishSuccesses int64 `json:"volume_publish_successes"`
	VolumePublishFailures  int64 `json:"volume_publish_failures"`
	VolumeUnpublishRequests  int64 `json:"volume_unpublish_requests"`
	VolumeUnpublishSuccesses int64 `json:"volume_unpublish_successes"`
	VolumeUnpublishFailures  int64 `json:"volume_unpublish_failures"`

	// Network access metrics
	NVMEConnectAttempts int64 `json:"nvme_connect_attempts"`
	NVMEConnectSuccesses int64 `json:"nvme_connect_successes"`
	NVMEConnectFailures int64 `json:"nvme_connect_failures"`
	NVMEDisconnects     int64 `json:"nvme_disconnects"`

	// Data access metrics
	MountAttempts  int64 `json:"mount_attempts"`
	MountSuccesses int64 `json:"mount_successes"`
	MountFailures  int64 `json:"mount_failures"`
	UnmountAttempts  int64 `json:"unmount_attempts"`
	UnmountSuccesses int64 `json:"unmount_successes"`
	UnmountFailures  int64 `json:"unmount_failures"`

	// Security violation metrics
	ValidationFailures       int64 `json:"validation_failures"`
	InvalidParameters        int64 `json:"invalid_parameters"`
	CommandInjectionAttempts int64 `json:"command_injection_attempts"`
	PathTraversalAttempts    int64 `json:"path_traversal_attempts"`
	RateLimitExceeded        int64 `json:"rate_limit_exceeded"`
	CircuitBreakerOpens      int64 `json:"circuit_breaker_opens"`

	// Severity counters
	InfoEvents     int64 `json:"info_events"`
	WarningEvents  int64 `json:"warning_events"`
	ErrorEvents    int64 `json:"error_events"`
	CriticalEvents int64 `json:"critical_events"`

	// Timing metrics
	LastSSHConnection       time.Time     `json:"last_ssh_connection"`
	LastVolumeOperation     time.Time     `json:"last_volume_operation"`
	LastSecurityViolation   time.Time     `json:"last_security_violation"`
	AverageOperationDuration time.Duration `json:"average_operation_duration_ms"`
	totalOperationTime      time.Duration
	totalOperations         int64
}

// globalMetrics is the global security metrics instance
var (
	globalMetrics *SecurityMetrics
	metricsOnce   sync.Once
)

// GetMetrics returns the global security metrics instance
func GetMetrics() *SecurityMetrics {
	metricsOnce.Do(func() {
		globalMetrics = &SecurityMetrics{}
	})
	return globalMetrics
}

// RecordEvent records a security event in metrics
func (m *SecurityMetrics) RecordEvent(event *SecurityEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record by severity
	switch event.Severity {
	case SeverityInfo:
		m.InfoEvents++
	case SeverityWarning:
		m.WarningEvents++
	case SeverityError:
		m.ErrorEvents++
	case SeverityCritical:
		m.CriticalEvents++
	}

	// Record by event type
	switch event.EventType {
	// Authentication events
	case EventSSHConnectionAttempt:
		m.SSHConnectionAttempts++
		m.LastSSHConnection = event.Timestamp
	case EventSSHConnectionSuccess:
		m.SSHConnectionSuccesses++
	case EventSSHConnectionFailure:
		m.SSHConnectionFailures++
	case EventSSHHostKeyMismatch:
		m.SSHHostKeyMismatches++
		m.LastSecurityViolation = event.Timestamp
	case EventSSHAuthFailure:
		m.SSHAuthFailures++

	// Volume create events
	case EventVolumeCreateRequest:
		m.VolumeCreateRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumeCreateSuccess:
		m.VolumeCreateSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumeCreateFailure:
		m.VolumeCreateFailures++

	// Volume delete events
	case EventVolumeDeleteRequest:
		m.VolumeDeleteRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumeDeleteSuccess:
		m.VolumeDeleteSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumeDeleteFailure:
		m.VolumeDeleteFailures++

	// Volume stage events
	case EventVolumeStageRequest:
		m.VolumeStageRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumeStageSuccess:
		m.VolumeStageSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumeStageFailure:
		m.VolumeStageFailures++

	// Volume unstage events
	case EventVolumeUnstageRequest:
		m.VolumeUnstageRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumeUnstageSuccess:
		m.VolumeUnstageSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumeUnstageFailure:
		m.VolumeUnstageFailures++

	// Volume publish events
	case EventVolumePublishRequest:
		m.VolumePublishRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumePublishSuccess:
		m.VolumePublishSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumePublishFailure:
		m.VolumePublishFailures++

	// Volume unpublish events
	case EventVolumeUnpublishRequest:
		m.VolumeUnpublishRequests++
		m.LastVolumeOperation = event.Timestamp
	case EventVolumeUnpublishSuccess:
		m.VolumeUnpublishSuccesses++
		m.recordOperationDuration(event.Duration)
	case EventVolumeUnpublishFailure:
		m.VolumeUnpublishFailures++

	// NVMe events
	case EventNVMEConnectAttempt:
		m.NVMEConnectAttempts++
	case EventNVMEConnectSuccess:
		m.NVMEConnectSuccesses++
	case EventNVMEConnectFailure:
		m.NVMEConnectFailures++
	case EventNVMEDisconnect:
		m.NVMEDisconnects++

	// Mount events
	case EventMountAttempt:
		m.MountAttempts++
	case EventMountSuccess:
		m.MountSuccesses++
	case EventMountFailure:
		m.MountFailures++
	case EventUnmountAttempt:
		m.UnmountAttempts++
	case EventUnmountSuccess:
		m.UnmountSuccesses++
	case EventUnmountFailure:
		m.UnmountFailures++

	// Security violations
	case EventValidationFailure:
		m.ValidationFailures++
		m.LastSecurityViolation = event.Timestamp
	case EventInvalidParameter:
		m.InvalidParameters++
		m.LastSecurityViolation = event.Timestamp
	case EventCommandInjectionAttempt:
		m.CommandInjectionAttempts++
		m.LastSecurityViolation = event.Timestamp
	case EventPathTraversalAttempt:
		m.PathTraversalAttempts++
		m.LastSecurityViolation = event.Timestamp
	case EventRateLimitExceeded:
		m.RateLimitExceeded++
		m.LastSecurityViolation = event.Timestamp
	case EventCircuitBreakerOpen:
		m.CircuitBreakerOpens++
		m.LastSecurityViolation = event.Timestamp
	}
}

// recordOperationDuration records the duration of an operation for averaging
func (m *SecurityMetrics) recordOperationDuration(duration time.Duration) {
	if duration > 0 {
		m.totalOperationTime += duration
		m.totalOperations++
		if m.totalOperations > 0 {
			m.AverageOperationDuration = m.totalOperationTime / time.Duration(m.totalOperations)
		}
	}
}

// Reset resets all metrics to zero
func (m *SecurityMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reset all fields individually to preserve the mutex
	m.SSHConnectionAttempts = 0
	m.SSHConnectionSuccesses = 0
	m.SSHConnectionFailures = 0
	m.SSHHostKeyMismatches = 0
	m.SSHAuthFailures = 0

	m.VolumeCreateRequests = 0
	m.VolumeCreateSuccesses = 0
	m.VolumeCreateFailures = 0
	m.VolumeDeleteRequests = 0
	m.VolumeDeleteSuccesses = 0
	m.VolumeDeleteFailures = 0
	m.VolumeStageRequests = 0
	m.VolumeStageSuccesses = 0
	m.VolumeStageFailures = 0
	m.VolumeUnstageRequests = 0
	m.VolumeUnstageSuccesses = 0
	m.VolumeUnstageFailures = 0
	m.VolumePublishRequests = 0
	m.VolumePublishSuccesses = 0
	m.VolumePublishFailures = 0
	m.VolumeUnpublishRequests = 0
	m.VolumeUnpublishSuccesses = 0
	m.VolumeUnpublishFailures = 0

	m.NVMEConnectAttempts = 0
	m.NVMEConnectSuccesses = 0
	m.NVMEConnectFailures = 0
	m.NVMEDisconnects = 0

	m.MountAttempts = 0
	m.MountSuccesses = 0
	m.MountFailures = 0
	m.UnmountAttempts = 0
	m.UnmountSuccesses = 0
	m.UnmountFailures = 0

	m.ValidationFailures = 0
	m.InvalidParameters = 0
	m.CommandInjectionAttempts = 0
	m.PathTraversalAttempts = 0
	m.RateLimitExceeded = 0
	m.CircuitBreakerOpens = 0

	m.InfoEvents = 0
	m.WarningEvents = 0
	m.ErrorEvents = 0
	m.CriticalEvents = 0

	m.LastSSHConnection = time.Time{}
	m.LastVolumeOperation = time.Time{}
	m.LastSecurityViolation = time.Time{}
	m.AverageOperationDuration = 0
	m.totalOperationTime = 0
	m.totalOperations = 0
}

// String returns a human-readable representation of the metrics
func (m *SecurityMetrics) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return fmt.Sprintf("SecurityMetrics{"+
		"SSH(attempts=%d, success=%d, failures=%d, key_mismatches=%d, auth_failures=%d), "+
		"VolumeCreate(requests=%d, success=%d, failures=%d), "+
		"VolumeDelete(requests=%d, success=%d, failures=%d), "+
		"VolumeStage(requests=%d, success=%d, failures=%d), "+
		"VolumeUnstage(requests=%d, success=%d, failures=%d), "+
		"VolumePublish(requests=%d, success=%d, failures=%d), "+
		"VolumeUnpublish(requests=%d, success=%d, failures=%d), "+
		"NVMe(attempts=%d, success=%d, failures=%d, disconnects=%d), "+
		"Mount(attempts=%d, success=%d, failures=%d), "+
		"Unmount(attempts=%d, success=%d, failures=%d), "+
		"Violations(validation=%d, invalid_params=%d, cmd_injection=%d, path_traversal=%d, rate_limit=%d, circuit_breaker=%d), "+
		"Severity(info=%d, warning=%d, error=%d, critical=%d), "+
		"AvgOpDuration=%dms}",
		m.SSHConnectionAttempts, m.SSHConnectionSuccesses, m.SSHConnectionFailures, m.SSHHostKeyMismatches, m.SSHAuthFailures,
		m.VolumeCreateRequests, m.VolumeCreateSuccesses, m.VolumeCreateFailures,
		m.VolumeDeleteRequests, m.VolumeDeleteSuccesses, m.VolumeDeleteFailures,
		m.VolumeStageRequests, m.VolumeStageSuccesses, m.VolumeStageFailures,
		m.VolumeUnstageRequests, m.VolumeUnstageSuccesses, m.VolumeUnstageFailures,
		m.VolumePublishRequests, m.VolumePublishSuccesses, m.VolumePublishFailures,
		m.VolumeUnpublishRequests, m.VolumeUnpublishSuccesses, m.VolumeUnpublishFailures,
		m.NVMEConnectAttempts, m.NVMEConnectSuccesses, m.NVMEConnectFailures, m.NVMEDisconnects,
		m.MountAttempts, m.MountSuccesses, m.MountFailures,
		m.UnmountAttempts, m.UnmountSuccesses, m.UnmountFailures,
		m.ValidationFailures, m.InvalidParameters, m.CommandInjectionAttempts, m.PathTraversalAttempts, m.RateLimitExceeded, m.CircuitBreakerOpens,
		m.InfoEvents, m.WarningEvents, m.ErrorEvents, m.CriticalEvents,
		m.AverageOperationDuration.Milliseconds())
}

// Snapshot returns a copy of the current metrics
func (m *SecurityMetrics) Snapshot() SecurityMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := *m
	return snapshot
}
