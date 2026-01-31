package attachment

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVolumeLockManager_LockUnlock(t *testing.T) {
	vlm := NewVolumeLockManager()
	volumeID := "vol-1"

	// Lock the volume
	vlm.Lock(volumeID)

	// Try to lock from a goroutine - should block
	locked := make(chan bool, 1)
	go func() {
		vlm.Lock(volumeID)
		locked <- true
		vlm.Unlock(volumeID)
	}()

	// Give goroutine time to attempt lock
	select {
	case <-locked:
		t.Fatal("Expected lock to block, but it didn't")
	case <-time.After(100 * time.Millisecond):
		// Good - lock is blocked
	}

	// Unlock the volume
	vlm.Unlock(volumeID)

	// Now the goroutine should be able to acquire the lock
	select {
	case <-locked:
		// Good - lock was acquired
	case <-time.After(1 * time.Second):
		t.Fatal("Expected lock to be acquired after unlock, but it timed out")
	}
}

func TestVolumeLockManager_DifferentVolumes(t *testing.T) {
	vlm := NewVolumeLockManager()

	// Lock vol-1
	vlm.Lock("vol-1")
	defer vlm.Unlock("vol-1")

	// Try to lock vol-2 from a goroutine - should NOT block
	locked := make(chan bool, 1)
	go func() {
		vlm.Lock("vol-2")
		locked <- true
		vlm.Unlock("vol-2")
	}()

	// Should be able to acquire lock immediately since different volume
	select {
	case <-locked:
		// Good - lock was acquired
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Expected lock on vol-2 to be acquired immediately, but it blocked")
	}
}

func TestVolumeLockManager_ConcurrentSameVolume(t *testing.T) {
	vlm := NewVolumeLockManager()
	volumeID := "vol-concurrent"
	numGoroutines := 100
	var counter int64
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			vlm.Lock(volumeID)
			// Critical section - increment counter
			current := atomic.LoadInt64(&counter)
			// Simulate some work
			time.Sleep(1 * time.Millisecond)
			atomic.StoreInt64(&counter, current+1)
			vlm.Unlock(volumeID)
		}()
	}

	wg.Wait()

	if counter != int64(numGoroutines) {
		t.Fatalf("Expected counter to be %d, got %d - lock serialization failed", numGoroutines, counter)
	}
}

func TestVolumeLockManager_UnlockNonExistent(t *testing.T) {
	vlm := NewVolumeLockManager()

	// Unlock a volume that was never locked - should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Unlock of non-existent volume panicked: %v", r)
		}
	}()

	vlm.Unlock("vol-nonexistent")

	// Should complete without panic
}

func TestVolumeLockManager_MultipleLockUnlock(t *testing.T) {
	vlm := NewVolumeLockManager()
	volumeID := "vol-multi"

	// Lock and unlock multiple times
	for i := 0; i < 10; i++ {
		vlm.Lock(volumeID)
		vlm.Unlock(volumeID)
	}

	// Should still be able to lock
	vlm.Lock(volumeID)
	vlm.Unlock(volumeID)
}

func TestVolumeLockManager_ConcurrentDifferentVolumes(t *testing.T) {
	vlm := NewVolumeLockManager()
	numVolumes := 50
	var wg sync.WaitGroup

	wg.Add(numVolumes)
	for i := 0; i < numVolumes; i++ {
		volumeID := fmt.Sprintf("vol-%d", i)
		go func(vid string) {
			defer wg.Done()
			vlm.Lock(vid)
			time.Sleep(10 * time.Millisecond)
			vlm.Unlock(vid)
		}(volumeID)
	}

	// All goroutines should complete quickly since they're different volumes
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Good - all completed
	case <-time.After(5 * time.Second):
		t.Fatal("Expected all goroutines to complete quickly for different volumes, but timed out")
	}
}
