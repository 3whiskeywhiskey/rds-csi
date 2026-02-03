package mount

import (
	"context"
	"testing"
)

func TestCheckFilesystemHealth_UnsupportedFS(t *testing.T) {
	// Unsupported filesystem types should be skipped (return nil)
	ctx := context.Background()
	err := CheckFilesystemHealth(ctx, "/dev/null", "btrfs")
	if err != nil {
		t.Errorf("Expected nil for unsupported filesystem, got: %v", err)
	}
}

func TestCheckFilesystemHealth_EmptyFS(t *testing.T) {
	// Empty fsType should be skipped
	ctx := context.Background()
	err := CheckFilesystemHealth(ctx, "/dev/null", "")
	if err != nil {
		t.Errorf("Expected nil for empty filesystem type, got: %v", err)
	}
}

func TestCheckFilesystemHealth_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should respect context cancellation
	// Note: This test may pass or fail depending on timing
	// The important thing is it doesn't hang
	_ = CheckFilesystemHealth(ctx, "/dev/null", "ext4")
}

// Note: Testing actual filesystem checks requires root and real devices.
// These tests verify the function handles edge cases gracefully.
// Integration tests with real devices should be done in E2E tests.
