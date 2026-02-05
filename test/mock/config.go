// Package mock provides environment-configurable mock RDS server for testing.
//
// Environment Variables:
//
// Timing Control:
//   - MOCK_RDS_REALISTIC_TIMING: Enable realistic timing simulation (default: false)
//   - MOCK_RDS_SSH_LATENCY_MS: SSH connection latency in ms (default: 200)
//   - MOCK_RDS_SSH_LATENCY_JITTER_MS: Latency jitter range in ms (default: 50)
//   - MOCK_RDS_DISK_ADD_DELAY_MS: Disk add operation delay in ms (default: 500)
//   - MOCK_RDS_DISK_REMOVE_DELAY_MS: Disk remove operation delay in ms (default: 300)
//
// Error Injection:
//   - MOCK_RDS_ERROR_MODE: Error injection mode (none|disk_full|ssh_timeout|command_fail)
//   - MOCK_RDS_ERROR_AFTER_N: Fail after N operations (default: 0 = immediate)
//
// Observability:
//   - MOCK_RDS_ENABLE_HISTORY: Enable command history tracking (default: true)
//   - MOCK_RDS_HISTORY_DEPTH: Maximum history entries (default: 100)
//   - MOCK_RDS_ROUTEROS_VERSION: RouterOS version to simulate (default: "7.16")
package mock

import (
	"os"
	"strconv"
)

// MockRDSConfig holds configuration for mock RDS server behavior
type MockRDSConfig struct {
	// Timing control
	RealisticTiming        bool   // MOCK_RDS_REALISTIC_TIMING (default: false)
	SSHLatencyMs           int    // MOCK_RDS_SSH_LATENCY_MS (default: 200)
	SSHLatencyJitterMs     int    // MOCK_RDS_SSH_LATENCY_JITTER_MS (default: 50, gives 150-250ms range)
	DiskAddDelayMs         int    // MOCK_RDS_DISK_ADD_DELAY_MS (default: 500)
	DiskRemoveDelayMs      int    // MOCK_RDS_DISK_REMOVE_DELAY_MS (default: 300)

	// Error injection
	ErrorMode              string // MOCK_RDS_ERROR_MODE (none|disk_full|ssh_timeout|command_fail)
	ErrorAfterN            int    // MOCK_RDS_ERROR_AFTER_N (fail after N operations, default: 0 = immediate)

	// Observability
	EnableHistory          bool   // MOCK_RDS_ENABLE_HISTORY (default: true for backward compat)
	HistoryDepth           int    // MOCK_RDS_HISTORY_DEPTH (default: 100)
	RouterOSVersion        string // MOCK_RDS_ROUTEROS_VERSION (default: "7.16")
}

// LoadConfigFromEnv loads mock RDS configuration from environment variables
func LoadConfigFromEnv() MockRDSConfig {
	return MockRDSConfig{
		RealisticTiming:    getEnvBool("MOCK_RDS_REALISTIC_TIMING", false),
		SSHLatencyMs:       getEnvInt("MOCK_RDS_SSH_LATENCY_MS", 200),
		SSHLatencyJitterMs: getEnvInt("MOCK_RDS_SSH_LATENCY_JITTER_MS", 50),
		DiskAddDelayMs:     getEnvInt("MOCK_RDS_DISK_ADD_DELAY_MS", 500),
		DiskRemoveDelayMs:  getEnvInt("MOCK_RDS_DISK_REMOVE_DELAY_MS", 300),
		ErrorMode:          getEnvString("MOCK_RDS_ERROR_MODE", "none"),
		ErrorAfterN:        getEnvInt("MOCK_RDS_ERROR_AFTER_N", 0),
		EnableHistory:      getEnvBool("MOCK_RDS_ENABLE_HISTORY", true),
		HistoryDepth:       getEnvInt("MOCK_RDS_HISTORY_DEPTH", 100),
		RouterOSVersion:    getEnvString("MOCK_RDS_ROUTEROS_VERSION", "7.16"),
	}
}

// getEnvBool reads a boolean environment variable with a default value
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1" || val == "yes"
}

// getEnvInt reads an integer environment variable with a default value
func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// getEnvString reads a string environment variable with a default value
func getEnvString(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
