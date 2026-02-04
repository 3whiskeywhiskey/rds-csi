package nvme

import (
	"fmt"
	"os"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	// DefaultResolverTTL is the default cache TTL for resolved device paths
	DefaultResolverTTL = 10 * time.Second
)

// cacheEntry holds a cached device path resolution
type cacheEntry struct {
	devicePath string
	resolvedAt time.Time
}

// DeviceResolver resolves NQN to device paths with caching
type DeviceResolver struct {
	scanner       *SysfsScanner
	cache         map[string]*cacheEntry
	mu            sync.RWMutex
	ttl           time.Duration
	isConnectedFn func(nqn string) (bool, error) // Injected for testing and connector integration
}

// ResolverConfig holds resolver configuration
type ResolverConfig struct {
	SysfsRoot string        // Default: "/sys"
	TTL       time.Duration // Default: 10 * time.Second
}

// DefaultResolverConfig returns sensible defaults
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		SysfsRoot: DefaultSysfsRoot,
		TTL:       DefaultResolverTTL,
	}
}

// NewDeviceResolver creates resolver with default config
func NewDeviceResolver() *DeviceResolver {
	return NewDeviceResolverWithConfig(DefaultResolverConfig())
}

// NewDeviceResolverWithConfig creates resolver with custom config
func NewDeviceResolverWithConfig(cfg ResolverConfig) *DeviceResolver {
	// Apply defaults for zero values
	if cfg.SysfsRoot == "" {
		cfg.SysfsRoot = DefaultSysfsRoot
	}
	if cfg.TTL == 0 {
		cfg.TTL = DefaultResolverTTL
	}

	return &DeviceResolver{
		scanner: NewSysfsScannerWithRoot(cfg.SysfsRoot),
		cache:   make(map[string]*cacheEntry),
		ttl:     cfg.TTL,
	}
}

// ResolveDevicePath resolves NQN to device path, using cache if valid
func (r *DeviceResolver) ResolveDevicePath(nqn string) (string, error) {
	// Check cache under read lock
	r.mu.RLock()
	entry, exists := r.cache[nqn]
	r.mu.RUnlock()

	if exists {
		// Validate cache entry: TTL not expired AND device still exists
		if time.Since(entry.resolvedAt) < r.ttl {
			if _, err := os.Stat(entry.devicePath); err == nil {
				klog.V(4).Infof("DeviceResolver: cache hit for NQN %s -> %s", nqn, entry.devicePath)
				return entry.devicePath, nil
			}
			klog.V(4).Infof("DeviceResolver: cache entry for NQN %s invalid (device %s not found), rescanning", nqn, entry.devicePath)
		} else {
			klog.V(4).Infof("DeviceResolver: cache entry for NQN %s expired (age %v > TTL %v), rescanning", nqn, time.Since(entry.resolvedAt), r.ttl)
		}
	} else {
		klog.V(4).Infof("DeviceResolver: cache miss for NQN %s, scanning sysfs", nqn)
	}

	// Scan sysfs for matching NQN
	devicePath, err := r.scanner.FindDeviceByNQN(nqn)
	if err != nil {
		return "", err
	}

	// Update cache under write lock
	r.mu.Lock()
	r.cache[nqn] = &cacheEntry{
		devicePath: devicePath,
		resolvedAt: time.Now(),
	}
	r.mu.Unlock()

	klog.V(2).Infof("DeviceResolver: resolved NQN %s -> %s", nqn, devicePath)
	return devicePath, nil
}

// Invalidate removes an NQN from the cache (call on disconnect)
func (r *DeviceResolver) Invalidate(nqn string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.cache[nqn]; exists {
		delete(r.cache, nqn)
		klog.V(4).Infof("DeviceResolver: invalidated cache for NQN %s", nqn)
	}
}

// InvalidateAll clears the entire cache
func (r *DeviceResolver) InvalidateAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := len(r.cache)
	r.cache = make(map[string]*cacheEntry)
	klog.V(4).Infof("DeviceResolver: invalidated entire cache (%d entries)", count)
}

// CacheStats returns statistics about the cache for debugging
type CacheStats struct {
	Entries    int
	OldestAge  time.Duration
	NewestAge  time.Duration
	ExpiredNum int
}

// GetCacheStats returns current cache statistics
func (r *DeviceResolver) GetCacheStats() CacheStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := CacheStats{
		Entries: len(r.cache),
	}

	if stats.Entries == 0 {
		return stats
	}

	now := time.Now()
	var oldest, newest time.Duration

	for _, entry := range r.cache {
		age := now.Sub(entry.resolvedAt)
		if oldest == 0 || age > oldest {
			oldest = age
		}
		if newest == 0 || age < newest {
			newest = age
		}
		if age >= r.ttl {
			stats.ExpiredNum++
		}
	}

	stats.OldestAge = oldest
	stats.NewestAge = newest
	return stats
}

// GetTTL returns the configured TTL for this resolver
func (r *DeviceResolver) GetTTL() time.Duration {
	return r.ttl
}

// IsCached returns whether an NQN has a valid (non-expired) cache entry
func (r *DeviceResolver) IsCached(nqn string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.cache[nqn]
	if !exists {
		return false
	}

	return time.Since(entry.resolvedAt) < r.ttl
}

// GetCachedPath returns the cached path for an NQN without validation
// Returns empty string if not cached. This is useful for debugging.
func (r *DeviceResolver) GetCachedPath(nqn string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.cache[nqn]; exists {
		return entry.devicePath
	}
	return ""
}

// String returns a human-readable representation of the resolver state
func (r *DeviceResolver) String() string {
	stats := r.GetCacheStats()
	return fmt.Sprintf("DeviceResolver(ttl=%v, entries=%d, expired=%d)",
		r.ttl, stats.Entries, stats.ExpiredNum)
}

// SetIsConnectedFn sets the connection check function for integration with connector.
// This enables orphan detection by allowing the resolver to check if a subsystem
// appears connected in nvme list-subsys output.
func (r *DeviceResolver) SetIsConnectedFn(fn func(nqn string) (bool, error)) {
	r.isConnectedFn = fn
}

// ListConnectedSubsystems returns all NQNs that have subsystem entries in sysfs.
// This scans /sys/class/nvme-subsystem/*/subsysnqn for connected subsystems.
func (r *DeviceResolver) ListConnectedSubsystems() ([]string, error) {
	return r.scanner.ListSubsystemNQNs()
}

// IsOrphanedSubsystem detects orphaned subsystems - appear connected but have no device.
// An orphaned subsystem occurs when the controller loses connection but the
// subsystem entry persists in nvme list-subsys output.
//
// Returns:
// - (true, nil): Subsystem appears connected but no device found - orphaned
// - (false, nil): Not connected OR connected with valid device - not orphaned
// - (false, err): Error checking connection state
func (r *DeviceResolver) IsOrphanedSubsystem(nqn string) (bool, error) {
	// Can't check without connector integration
	if r.isConnectedFn == nil {
		klog.V(4).Infof("IsOrphanedSubsystem: no isConnectedFn set, cannot check NQN %s", nqn)
		return false, nil
	}

	// Check if subsystem appears connected
	connected, err := r.isConnectedFn(nqn)
	if err != nil {
		return false, fmt.Errorf("failed to check connection state for NQN %s: %w", nqn, err)
	}

	if !connected {
		klog.V(4).Infof("IsOrphanedSubsystem: NQN %s is not connected, not orphaned", nqn)
		return false, nil
	}

	// Connected - try to find device via fresh sysfs scan (bypass cache)
	devicePath, err := r.scanner.FindDeviceByNQN(nqn)
	if err == nil && devicePath != "" {
		klog.V(4).Infof("IsOrphanedSubsystem: NQN %s connected with device %s, not orphaned", nqn, devicePath)
		return false, nil
	}

	// Appears connected but no device found - this is an orphaned subsystem
	klog.Warningf("Orphaned subsystem detected: NQN %s appears connected but has no device", nqn)
	return true, nil
}
