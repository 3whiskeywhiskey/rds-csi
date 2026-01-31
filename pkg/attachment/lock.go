package attachment

import "sync"

// VolumeLockManager provides per-volume mutex management for serializing
// operations on individual volumes while allowing concurrent operations
// on different volumes.
type VolumeLockManager struct {
	// mu protects the locks map itself
	mu sync.Mutex

	// locks maps volumeID to per-volume mutex
	locks map[string]*sync.Mutex
}

// NewVolumeLockManager creates a new VolumeLockManager
func NewVolumeLockManager() *VolumeLockManager {
	return &VolumeLockManager{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock acquires the per-volume lock for the specified volumeID.
// If no lock exists for this volume, one is created.
// This method blocks until the lock is acquired.
func (vlm *VolumeLockManager) Lock(volumeID string) {
	// Acquire manager lock to get/create the per-volume lock
	vlm.mu.Lock()
	lock, exists := vlm.locks[volumeID]
	if !exists {
		lock = &sync.Mutex{}
		vlm.locks[volumeID] = lock
	}
	// CRITICAL: Release manager lock BEFORE acquiring per-volume lock
	// This prevents holding the manager lock while waiting for a per-volume lock
	vlm.mu.Unlock()

	// Now acquire the per-volume lock (may block)
	lock.Lock()
}

// Unlock releases the per-volume lock for the specified volumeID.
// The lock must have been previously acquired with Lock().
func (vlm *VolumeLockManager) Unlock(volumeID string) {
	vlm.mu.Lock()
	lock, exists := vlm.locks[volumeID]
	vlm.mu.Unlock()

	if exists {
		lock.Unlock()
	}
}
