package rds

import (
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
)

// SNMP OIDs for MikroTik RouterOS hardware monitoring
const (
	// MIKROTIK-MIB::mtxrHealth (1.3.6.1.4.1.14988.1.1.3)
	oidCPUTemperature   = "1.3.6.1.4.1.14988.1.1.3.10"
	oidBoardTemperature = "1.3.6.1.4.1.14988.1.1.3.11"
	oidFan1Speed        = "1.3.6.1.4.1.14988.1.1.3.17"
	oidFan2Speed        = "1.3.6.1.4.1.14988.1.1.3.18"
	oidPSU1Voltage      = "1.3.6.1.4.1.14988.1.1.3.8"
	oidPSU2Voltage      = "1.3.6.1.4.1.14988.1.1.3.9"
	oidPSU1Temperature  = "1.3.6.1.4.1.14988.1.1.3.12"
	oidPSU2Temperature  = "1.3.6.1.4.1.14988.1.1.3.13"

	// HOST-RESOURCES-MIB::hrStorageTable (1.3.6.1.2.1.25.2.3)
	// These OIDs need to be walked to find the storage entry for the RAID6 pool
	// For now, we'll use placeholder indices that need hardware validation
	oidStorageIndex = "1.3.6.1.2.1.25.2.3.1.1" // hrStorageIndex
	oidStorageSize  = "1.3.6.1.2.1.25.2.3.1.5" // hrStorageSize
	oidStorageUsed  = "1.3.6.1.2.1.25.2.3.1.6" // hrStorageUsed
	oidStorageUnits = "1.3.6.1.2.1.25.2.3.1.4" // hrStorageAllocationUnits
)

// GetHardwareHealth retrieves hardware health metrics via SNMP
func (c *sshClient) GetHardwareHealth(snmpHost string, snmpCommunity string) (*HardwareHealthMetrics, error) {
	metrics := &HardwareHealthMetrics{}

	// Configure SNMP client
	snmpClient := &gosnmp.GoSNMP{
		Target:    snmpHost,
		Port:      161,
		Community: snmpCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(5) * time.Second,
		Retries:   2,
	}

	err := snmpClient.Connect()
	if err != nil {
		return nil, fmt.Errorf("SNMP connect failed: %w", err)
	}
	defer snmpClient.Conn.Close()

	// Query temperature and fan OIDs
	healthOIDs := []string{
		oidCPUTemperature,
		oidBoardTemperature,
		oidFan1Speed,
		oidFan2Speed,
		oidPSU1Voltage,
		oidPSU2Voltage,
		oidPSU1Temperature,
		oidPSU2Temperature,
	}

	result, err := snmpClient.Get(healthOIDs)
	if err != nil {
		return nil, fmt.Errorf("SNMP get failed: %w", err)
	}

	// Parse results (convert from SNMP PDU variables to metrics)
	if len(result.Variables) >= 8 {
		metrics.CPUTemperature = parseFloat64(result.Variables[0])
		metrics.BoardTemperature = parseFloat64(result.Variables[1])
		metrics.Fan1Speed = parseFloat64(result.Variables[2])
		metrics.Fan2Speed = parseFloat64(result.Variables[3])
		// Convert voltage to power estimate (voltage * typical current)
		// Note: Real power monitoring requires amperage readings
		metrics.PSU1Power = parseFloat64(result.Variables[4]) * 10 // Rough estimate
		metrics.PSU2Power = parseFloat64(result.Variables[5]) * 10
		metrics.PSU1Temperature = parseFloat64(result.Variables[6])
		metrics.PSU2Temperature = parseFloat64(result.Variables[7])
	}

	// For disk capacity, we would need to walk the hrStorageTable to find the right index
	// This requires hardware validation to determine the correct storage entry
	// For now, leave disk metrics at 0 (requires enhancement in future phase)
	metrics.DiskPoolSizeBytes = 0
	metrics.DiskPoolUsedBytes = 0

	return metrics, nil
}

// parseFloat64 converts gosnmp.SnmpPDU value to float64
func parseFloat64(pdu gosnmp.SnmpPDU) float64 {
	switch pdu.Type {
	case gosnmp.Integer:
		return float64(pdu.Value.(int))
	case gosnmp.Gauge32, gosnmp.Counter32, gosnmp.Counter64:
		switch v := pdu.Value.(type) {
		case int:
			return float64(v)
		case uint:
			return float64(v)
		case uint64:
			return float64(v)
		default:
			return 0
		}
	default:
		return 0
	}
}
