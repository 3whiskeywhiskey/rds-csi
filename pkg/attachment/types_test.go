package attachment

import (
	"testing"
	"time"
)

func TestIsMigrating(t *testing.T) {
	tests := []struct {
		name     string
		state    *AttachmentState
		expected bool
	}{
		{
			name: "not migrating - no migration timestamp",
			state: &AttachmentState{
				Nodes: []NodeAttachment{{NodeID: "node-1"}},
			},
			expected: false,
		},
		{
			name: "not migrating - single node with timestamp",
			state: &AttachmentState{
				Nodes:              []NodeAttachment{{NodeID: "node-1"}},
				MigrationStartedAt: timePtr(time.Now()),
			},
			expected: false, // Need 2 nodes to be migrating
		},
		{
			name: "migrating - two nodes with timestamp",
			state: &AttachmentState{
				Nodes: []NodeAttachment{
					{NodeID: "node-1"},
					{NodeID: "node-2"},
				},
				MigrationStartedAt: timePtr(time.Now()),
			},
			expected: true,
		},
		{
			name: "not migrating - two nodes but no timestamp",
			state: &AttachmentState{
				Nodes: []NodeAttachment{
					{NodeID: "node-1"},
					{NodeID: "node-2"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.IsMigrating()
			if result != tt.expected {
				t.Errorf("IsMigrating() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsMigrationTimedOut(t *testing.T) {
	tests := []struct {
		name     string
		state    *AttachmentState
		expected bool
	}{
		{
			name: "not timed out - no migration",
			state: &AttachmentState{
				MigrationStartedAt: nil,
				MigrationTimeout:   5 * time.Minute,
			},
			expected: false,
		},
		{
			name: "not timed out - zero timeout (disabled)",
			state: &AttachmentState{
				MigrationStartedAt: timePtr(time.Now().Add(-10 * time.Minute)),
				MigrationTimeout:   0,
			},
			expected: false,
		},
		{
			name: "not timed out - within timeout",
			state: &AttachmentState{
				MigrationStartedAt: timePtr(time.Now().Add(-1 * time.Minute)),
				MigrationTimeout:   5 * time.Minute,
			},
			expected: false,
		},
		{
			name: "timed out - exceeded timeout",
			state: &AttachmentState{
				MigrationStartedAt: timePtr(time.Now().Add(-10 * time.Minute)),
				MigrationTimeout:   5 * time.Minute,
			},
			expected: true,
		},
		{
			name: "timed out - exactly at boundary",
			state: &AttachmentState{
				MigrationStartedAt: timePtr(time.Now().Add(-5*time.Minute - 1*time.Second)),
				MigrationTimeout:   5 * time.Minute,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.IsMigrationTimedOut()
			if result != tt.expected {
				t.Errorf("IsMigrationTimedOut() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}
