package driver

import (
	"fmt"
	"strconv"
)

// NVMe connection parameter keys for StorageClass
const (
	// paramCtrlLossTmo is the controller loss timeout parameter key
	// Value: integer seconds, -1 for unlimited, 0 for kernel default
	paramCtrlLossTmo = "ctrlLossTmo"

	// paramReconnectDelay is the reconnect delay parameter key
	// Value: integer seconds, must be > 0
	paramReconnectDelay = "reconnectDelay"

	// paramKeepAliveTmo is the keep-alive timeout parameter key
	// Value: integer seconds, 0 for kernel default
	paramKeepAliveTmo = "keepAliveTmo"
)

// NVMEConnectionParams holds parsed NVMe connection parameters from StorageClass
type NVMEConnectionParams struct {
	// CtrlLossTmo is the controller loss timeout in seconds
	// -1 = unlimited (recommended), 0 = kernel default, >0 = specific timeout
	CtrlLossTmo int

	// ReconnectDelay is the delay between reconnect attempts in seconds
	ReconnectDelay int

	// KeepAliveTmo is the keep-alive timeout in seconds
	KeepAliveTmo int
}

// DefaultNVMEConnectionParams returns the default connection parameters
func DefaultNVMEConnectionParams() NVMEConnectionParams {
	return NVMEConnectionParams{
		CtrlLossTmo:    -1, // Unlimited reconnection
		ReconnectDelay: 5,  // 5 seconds between retries
		KeepAliveTmo:   0,  // Use kernel default
	}
}

// ParseNVMEConnectionParams parses NVMe connection parameters from StorageClass parameters
// Returns default values for missing parameters and errors for invalid values
func ParseNVMEConnectionParams(params map[string]string) (NVMEConnectionParams, error) {
	// Start with defaults
	config := DefaultNVMEConnectionParams()

	// Parse ctrl_loss_tmo if present
	if val, ok := params[paramCtrlLossTmo]; ok {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return config, fmt.Errorf("invalid %s value %q: %w", paramCtrlLossTmo, val, err)
		}
		// Validate: must be >= -1
		// -1 = unlimited, 0 = kernel default, >0 = specific timeout
		if parsed < -1 {
			return config, fmt.Errorf("%s must be -1 (unlimited), 0 (kernel default), or positive; got %d", paramCtrlLossTmo, parsed)
		}
		config.CtrlLossTmo = parsed
	}

	// Parse reconnect_delay if present
	if val, ok := params[paramReconnectDelay]; ok {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return config, fmt.Errorf("invalid %s value %q: %w", paramReconnectDelay, val, err)
		}
		// Validate: must be > 0
		if parsed < 1 {
			return config, fmt.Errorf("%s must be positive; got %d", paramReconnectDelay, parsed)
		}
		config.ReconnectDelay = parsed
	}

	// Parse keep_alive_tmo if present
	if val, ok := params[paramKeepAliveTmo]; ok {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return config, fmt.Errorf("invalid %s value %q: %w", paramKeepAliveTmo, val, err)
		}
		// Validate: must be >= 0
		if parsed < 0 {
			return config, fmt.Errorf("%s must be non-negative; got %d", paramKeepAliveTmo, parsed)
		}
		config.KeepAliveTmo = parsed
	}

	return config, nil
}

// ToVolumeContext converts NVMEConnectionParams to a string map for inclusion in VolumeContext
// This allows the parameters to be passed from Controller to Node via CSI VolumeContext
func ToVolumeContext(params NVMEConnectionParams) map[string]string {
	return map[string]string{
		paramCtrlLossTmo:    fmt.Sprintf("%d", params.CtrlLossTmo),
		paramReconnectDelay: fmt.Sprintf("%d", params.ReconnectDelay),
		paramKeepAliveTmo:   fmt.Sprintf("%d", params.KeepAliveTmo),
	}
}
