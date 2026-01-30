package nvme

import (
	"fmt"
)

// ConnectionConfig holds NVMe/TCP connection resilience parameters
// These parameters control kernel-level reconnection behavior
type ConnectionConfig struct {
	// CtrlLossTmo is the controller loss timeout in seconds
	// -1 = unlimited (recommended for production, prevents filesystem read-only mount)
	// 0 = use kernel default (typically 600 seconds)
	// >0 = timeout in seconds
	CtrlLossTmo int

	// ReconnectDelay is the delay between reconnect attempts in seconds
	// Must be > 0
	// Default: 5 seconds
	ReconnectDelay int

	// KeepAliveTmo is the keep-alive timeout in seconds
	// 0 = use kernel default
	// >0 = timeout in seconds
	KeepAliveTmo int
}

// DefaultConnectionConfig returns the recommended connection configuration
// for production use with unlimited reconnection
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		CtrlLossTmo:    -1, // Unlimited reconnection - prevents filesystem read-only mount
		ReconnectDelay: 5,  // 5 second retry interval
		KeepAliveTmo:   0,  // Use kernel default
	}
}

// BuildConnectArgs builds the nvme connect command arguments with connection parameters
// The returned slice starts with "connect" and includes all necessary flags
func BuildConnectArgs(target Target, config ConnectionConfig) []string {
	args := []string{
		"connect",
		"-t", target.Transport,
		"-a", target.TargetAddress,
		"-s", fmt.Sprintf("%d", target.TargetPort),
		"-n", target.NQN,
	}

	// Add controller loss timeout if not using kernel default (0)
	// -1 = unlimited, >0 = specific timeout
	if config.CtrlLossTmo != 0 {
		args = append(args, "-l", fmt.Sprintf("%d", config.CtrlLossTmo))
	}

	// Add reconnect delay if specified (must be > 0)
	if config.ReconnectDelay > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", config.ReconnectDelay))
	}

	// Add keep-alive timeout if specified (must be > 0)
	if config.KeepAliveTmo > 0 {
		args = append(args, "-k", fmt.Sprintf("%d", config.KeepAliveTmo))
	}

	// Add host NQN if specified
	if target.HostNQN != "" {
		args = append(args, "-q", target.HostNQN)
	}

	return args
}
