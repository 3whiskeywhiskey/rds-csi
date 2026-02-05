package mock

import (
	"math/rand"
	"time"

	"k8s.io/klog/v2"
)

// TimingSimulator adds realistic timing delays to mock RDS operations
type TimingSimulator struct {
	enabled              bool
	sshLatency           time.Duration
	sshLatencyJitter     time.Duration
	diskAddDelay         time.Duration
	diskRemoveDelay      time.Duration
	rng                  *rand.Rand
}

// NewTimingSimulator creates a new timing simulator from configuration
func NewTimingSimulator(config MockRDSConfig) *TimingSimulator {
	return &TimingSimulator{
		enabled:          config.RealisticTiming,
		sshLatency:       time.Duration(config.SSHLatencyMs) * time.Millisecond,
		sshLatencyJitter: time.Duration(config.SSHLatencyJitterMs) * time.Millisecond,
		diskAddDelay:     time.Duration(config.DiskAddDelayMs) * time.Millisecond,
		diskRemoveDelay:  time.Duration(config.DiskRemoveDelayMs) * time.Millisecond,
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SimulateSSHLatency simulates SSH connection latency with jitter
// Called at session start after SSH handshake completes
func (t *TimingSimulator) SimulateSSHLatency() {
	if !t.enabled || t.sshLatency == 0 {
		return
	}

	// Add jitter: base latency ± jitter (e.g., 200ms ± 50ms = 150-250ms range)
	jitter := time.Duration(0)
	if t.sshLatencyJitter > 0 {
		// Random jitter in range [-jitter, +jitter]
		jitter = time.Duration(t.rng.Int63n(int64(t.sshLatencyJitter*2))) - t.sshLatencyJitter
	}

	delay := t.sshLatency + jitter
	if delay < 0 {
		delay = 0
	}

	klog.V(4).Infof("Mock RDS timing: SSH latency simulation %dms", delay.Milliseconds())
	time.Sleep(delay)
}

// SimulateDiskOperation simulates disk operation delays (add/remove)
// Called before state modification to match real RDS behavior
func (t *TimingSimulator) SimulateDiskOperation(opType string) {
	if !t.enabled {
		return
	}

	var delay time.Duration
	switch opType {
	case "add":
		delay = t.diskAddDelay
	case "remove":
		delay = t.diskRemoveDelay
	default:
		return
	}

	if delay == 0 {
		return
	}

	klog.V(4).Infof("Mock RDS timing: Disk %s operation simulation %dms", opType, delay.Milliseconds())
	time.Sleep(delay)
}
