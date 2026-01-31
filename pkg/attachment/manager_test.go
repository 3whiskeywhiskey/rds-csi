package attachment

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAttachmentManager_TrackAttachment(t *testing.T) {
	// Create AttachmentManager with nil k8sClient for in-memory only
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	volumeID := "vol-1"
	nodeID := "node-1"

	// Track attachment
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Get attachment and verify
	state, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected attachment to exist, but it doesn't")
	}

	if state.NodeID != nodeID {
		t.Errorf("Expected nodeID %s, got %s", nodeID, state.NodeID)
	}

	if state.VolumeID != volumeID {
		t.Errorf("Expected volumeID %s, got %s", volumeID, state.VolumeID)
	}

	// Verify AttachedAt is approximately now
	if time.Since(state.AttachedAt) > 5*time.Second {
		t.Errorf("Expected AttachedAt to be recent, but it's %v old", time.Since(state.AttachedAt))
	}
}

func TestAttachmentManager_TrackAttachment_Idempotent(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	volumeID := "vol-1"
	nodeID := "node-1"

	// Track attachment
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("First TrackAttachment failed: %v", err)
	}

	// Track same attachment again (idempotent)
	err = am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("Second TrackAttachment (idempotent) failed: %v", err)
	}

	// Verify still only one attachment
	attachments := am.ListAttachments()
	if len(attachments) != 1 {
		t.Errorf("Expected 1 attachment after idempotent track, got %d", len(attachments))
	}
}

func TestAttachmentManager_TrackAttachment_ConflictError(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	volumeID := "vol-1"
	nodeID1 := "node-1"
	nodeID2 := "node-2"

	// Track attachment to node-1
	err := am.TrackAttachment(ctx, volumeID, nodeID1)
	if err != nil {
		t.Fatalf("First TrackAttachment failed: %v", err)
	}

	// Try to track same volume to different node - should fail
	err = am.TrackAttachment(ctx, volumeID, nodeID2)
	if err == nil {
		t.Fatal("Expected error when tracking volume to different node, but got nil")
	}

	// Verify error message mentions the conflict
	expectedSubstring := "already attached to node node-1"
	if !contains(err.Error(), expectedSubstring) {
		t.Errorf("Expected error message to contain '%s', got: %s", expectedSubstring, err.Error())
	}

	// Verify original attachment is still intact
	state, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected original attachment to exist")
	}
	if state.NodeID != nodeID1 {
		t.Errorf("Expected attachment to still be on node-1, got %s", state.NodeID)
	}
}

func TestAttachmentManager_UntrackAttachment(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	volumeID := "vol-1"
	nodeID := "node-1"

	// Track attachment
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Verify it exists
	_, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected attachment to exist before untracking")
	}

	// Untrack attachment
	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("UntrackAttachment failed: %v", err)
	}

	// Verify it no longer exists
	_, exists = am.GetAttachment(volumeID)
	if exists {
		t.Fatal("Expected attachment to not exist after untracking")
	}
}

func TestAttachmentManager_UntrackAttachment_Idempotent(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	// Untrack non-existent volume - should not error
	err := am.UntrackAttachment(ctx, "vol-nonexistent")
	if err != nil {
		t.Fatalf("UntrackAttachment of non-existent volume failed: %v", err)
	}

	// Track and untrack
	volumeID := "vol-1"
	_ = am.TrackAttachment(ctx, volumeID, "node-1")
	_ = am.UntrackAttachment(ctx, volumeID)

	// Untrack again - should be idempotent
	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("Second UntrackAttachment (idempotent) failed: %v", err)
	}
}

func TestAttachmentManager_ListAttachments(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	// Track multiple volumes
	_ = am.TrackAttachment(ctx, "vol-1", "node-1")
	_ = am.TrackAttachment(ctx, "vol-2", "node-2")
	_ = am.TrackAttachment(ctx, "vol-3", "node-1")

	// List attachments
	attachments := am.ListAttachments()

	if len(attachments) != 3 {
		t.Errorf("Expected 3 attachments, got %d", len(attachments))
	}

	// Verify correct mappings
	if attachments["vol-1"].NodeID != "node-1" {
		t.Errorf("Expected vol-1 on node-1, got %s", attachments["vol-1"].NodeID)
	}
	if attachments["vol-2"].NodeID != "node-2" {
		t.Errorf("Expected vol-2 on node-2, got %s", attachments["vol-2"].NodeID)
	}
	if attachments["vol-3"].NodeID != "node-1" {
		t.Errorf("Expected vol-3 on node-1, got %s", attachments["vol-3"].NodeID)
	}

	// Delete from returned map - internal state should be unchanged
	delete(attachments, "vol-1")

	// Verify internal state still has vol-1
	freshState, exists := am.GetAttachment("vol-1")
	if !exists {
		t.Error("Expected vol-1 to still exist in internal state after deleting from returned map")
	}
	if freshState.NodeID != "node-1" {
		t.Errorf("Expected vol-1 to still be on node-1, but got %s", freshState.NodeID)
	}

	// Verify returned map copy is actually separate
	if len(attachments) != 2 {
		t.Errorf("Expected returned map to have 2 entries after delete, got %d", len(attachments))
	}
	if len(am.ListAttachments()) != 3 {
		t.Errorf("Expected internal state to still have 3 entries, got %d", len(am.ListAttachments()))
	}
}

func TestAttachmentManager_ConcurrentTrack(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	numGoroutines := 50
	errChan := make(chan error, numGoroutines)

	// Track different volumes concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			volumeID := fmt.Sprintf("vol-%d", n)
			nodeID := fmt.Sprintf("node-%d", n)
			errChan <- am.TrackAttachment(ctx, volumeID, nodeID)
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-errChan
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	// Verify all attachments tracked
	attachments := am.ListAttachments()
	if len(attachments) != numGoroutines {
		t.Errorf("Expected %d attachments, got %d", numGoroutines, len(attachments))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

// Helper to create test PV
func createTestPV(volumeID, nodeID string) *corev1.PersistentVolume {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeID,
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "rds.csi.srvlab.io",
					VolumeHandle: volumeID,
				},
			},
		},
	}
	if nodeID != "" {
		pv.Annotations = map[string]string{
			AnnotationAttachedNode: nodeID,
			AnnotationAttachedAt:   metav1.Now().Format(metav1.RFC3339Micro),
		}
	}
	return pv
}

func TestAttachmentManager_PersistAttachment(t *testing.T) {
	volumeID := "pv-vol-1"
	nodeID := "node-1"

	// Create fake clientset with a PV
	pv := createTestPV(volumeID, "")
	fakeClient := fake.NewSimpleClientset(pv)

	// Create AttachmentManager with fake client
	am := NewAttachmentManager(fakeClient)
	ctx := context.Background()

	// Track attachment
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Get PV from fake client and verify annotation
	updatedPV, err := fakeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PV: %v", err)
	}

	if updatedPV.Annotations == nil {
		t.Fatal("Expected PV to have annotations, but it's nil")
	}

	if updatedPV.Annotations[AnnotationAttachedNode] != nodeID {
		t.Errorf("Expected annotation %s=%s, got %s", AnnotationAttachedNode, nodeID, updatedPV.Annotations[AnnotationAttachedNode])
	}

	if updatedPV.Annotations[AnnotationAttachedAt] == "" {
		t.Errorf("Expected annotation %s to be set", AnnotationAttachedAt)
	}
}

func TestAttachmentManager_ClearAttachment(t *testing.T) {
	volumeID := "pv-vol-1"
	nodeID := "node-1"

	// Create PV with attachment annotations
	pv := createTestPV(volumeID, nodeID)
	fakeClient := fake.NewSimpleClientset(pv)

	// Create AttachmentManager
	am := NewAttachmentManager(fakeClient)
	ctx := context.Background()

	// Track then untrack
	_ = am.TrackAttachment(ctx, volumeID, nodeID)
	err := am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("UntrackAttachment failed: %v", err)
	}

	// Verify annotations removed from PV
	updatedPV, err := fakeClient.CoreV1().PersistentVolumes().Get(ctx, volumeID, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PV: %v", err)
	}

	if updatedPV.Annotations != nil {
		if _, hasNode := updatedPV.Annotations[AnnotationAttachedNode]; hasNode {
			t.Errorf("Expected %s annotation to be removed, but it's still present", AnnotationAttachedNode)
		}
		if _, hasAt := updatedPV.Annotations[AnnotationAttachedAt]; hasAt {
			t.Errorf("Expected %s annotation to be removed, but it's still present", AnnotationAttachedAt)
		}
	}
}

func TestAttachmentManager_RebuildState(t *testing.T) {
	ctx := context.Background()

	// Create fake clientset with 3 PVs
	pv1 := createTestPV("pv-1", "node-a")
	pv2 := createTestPV("pv-2", "node-b")
	pv3 := createTestPV("pv-3", "") // No annotations (not attached)

	fakeClient := fake.NewSimpleClientset(pv1, pv2, pv3)

	// Create AttachmentManager
	am := NewAttachmentManager(fakeClient)

	// Call Initialize (which calls RebuildState)
	err := am.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify ListAttachments returns 2 entries (pv-1 and pv-2)
	attachments := am.ListAttachments()
	if len(attachments) != 2 {
		t.Errorf("Expected 2 attachments after rebuild, got %d", len(attachments))
	}

	// Verify correct volume-to-node mappings
	if state, exists := attachments["pv-1"]; exists {
		if state.NodeID != "node-a" {
			t.Errorf("Expected pv-1 on node-a, got %s", state.NodeID)
		}
	} else {
		t.Error("Expected pv-1 to be in attachments")
	}

	if state, exists := attachments["pv-2"]; exists {
		if state.NodeID != "node-b" {
			t.Errorf("Expected pv-2 on node-b, got %s", state.NodeID)
		}
	} else {
		t.Error("Expected pv-2 to be in attachments")
	}

	// Verify pv-3 is not in attachments (no annotations)
	if _, exists := attachments["pv-3"]; exists {
		t.Error("Expected pv-3 to not be in attachments (no annotations)")
	}
}

func TestAttachmentManager_RebuildState_IgnoresOtherDrivers(t *testing.T) {
	ctx := context.Background()

	// Create PV with different driver
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv-other",
			Annotations: map[string]string{
				AnnotationAttachedNode: "node-1",
				AnnotationAttachedAt:   metav1.Now().Format(metav1.RFC3339Micro),
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "other.csi.io",
					VolumeHandle: "pv-other",
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(pv)
	am := NewAttachmentManager(fakeClient)

	// RebuildState should not include it
	err := am.RebuildState(ctx)
	if err != nil {
		t.Fatalf("RebuildState failed: %v", err)
	}

	attachments := am.ListAttachments()
	if len(attachments) != 0 {
		t.Errorf("Expected 0 attachments (different driver), got %d", len(attachments))
	}
}

func TestAttachmentManager_RebuildState_NoClient(t *testing.T) {
	// Create AttachmentManager with nil k8sClient
	am := NewAttachmentManager(nil)
	ctx := context.Background()

	// RebuildState should return nil (not error)
	err := am.RebuildState(ctx)
	if err != nil {
		t.Fatalf("RebuildState with nil client should not error, got: %v", err)
	}

	// Attachments should be empty
	attachments := am.ListAttachments()
	if len(attachments) != 0 {
		t.Errorf("Expected 0 attachments (nil client), got %d", len(attachments))
	}
}

func TestAttachmentManager_PersistRollback(t *testing.T) {
	volumeID := "pv-nonexistent"
	nodeID := "node-1"

	// Create fake client WITHOUT the PV (simulate PV not existing)
	fakeClient := fake.NewSimpleClientset()

	am := NewAttachmentManager(fakeClient)
	ctx := context.Background()

	// Track attachment - should succeed (persistence failure is non-fatal for missing PV)
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment should succeed even if PV doesn't exist yet, got: %v", err)
	}

	// Verify in-memory state is still set (persistence failure for missing PV is logged but not fatal)
	state, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Error("Expected attachment to exist in memory even if PV doesn't exist")
	}
	if state != nil && state.NodeID != nodeID {
		t.Errorf("Expected nodeID %s, got %s", nodeID, state.NodeID)
	}
}

func TestIsWithinGracePeriod_NoDetachTimestamp(t *testing.T) {
	am := NewAttachmentManager(nil)

	// Volume never tracked/detached should not be within grace period
	result := am.IsWithinGracePeriod("pvc-never-existed", 30*time.Second)
	if result {
		t.Error("Expected IsWithinGracePeriod to return false for non-existent volume")
	}
}

func TestIsWithinGracePeriod_WithinPeriod(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-grace-within"
	nodeID := "node-1"

	// Track and then untrack to create detach timestamp
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("UntrackAttachment failed: %v", err)
	}

	// Check immediately after detach (should be within grace period)
	result := am.IsWithinGracePeriod(volumeID, 30*time.Second)
	if !result {
		t.Error("Expected IsWithinGracePeriod to return true immediately after detach")
	}
}

func TestIsWithinGracePeriod_OutsidePeriod(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-grace-outside"
	nodeID := "node-1"

	// Track and then untrack
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("UntrackAttachment failed: %v", err)
	}

	// Use a very short grace period that we've already exceeded
	// Sleep a tiny bit to ensure we're past the grace period
	time.Sleep(1 * time.Millisecond)

	result := am.IsWithinGracePeriod(volumeID, 1*time.Nanosecond)
	if result {
		t.Error("Expected IsWithinGracePeriod to return false after grace period expired")
	}
}

func TestGetDetachTimestamp(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-detach-time"
	nodeID := "node-1"

	// Before any tracking, should return zero time
	ts := am.GetDetachTimestamp(volumeID)
	if !ts.IsZero() {
		t.Error("Expected zero timestamp for never-tracked volume")
	}

	// Track and untrack
	_ = am.TrackAttachment(ctx, volumeID, nodeID)
	_ = am.UntrackAttachment(ctx, volumeID)

	// Should have a non-zero timestamp now
	ts = am.GetDetachTimestamp(volumeID)
	if ts.IsZero() {
		t.Error("Expected non-zero timestamp after untrack")
	}

	// Timestamp should be recent
	if time.Since(ts) > 5*time.Second {
		t.Error("Detach timestamp is too old")
	}
}

func TestClearDetachTimestamp(t *testing.T) {
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-clear-time"
	nodeID := "node-1"

	// Track and untrack to create timestamp
	_ = am.TrackAttachment(ctx, volumeID, nodeID)
	_ = am.UntrackAttachment(ctx, volumeID)

	// Verify timestamp exists
	ts := am.GetDetachTimestamp(volumeID)
	if ts.IsZero() {
		t.Fatal("Expected timestamp to exist before clear")
	}

	// Clear it
	am.ClearDetachTimestamp(volumeID)

	// Should no longer be within grace period
	result := am.IsWithinGracePeriod(volumeID, 30*time.Second)
	if result {
		t.Error("Expected IsWithinGracePeriod to return false after clearing timestamp")
	}
}

func TestGracePeriod_LiveMigrationScenario(t *testing.T) {
	// Simulates KubeVirt live migration: detach from node-1, attach to node-2
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-kubevirt-vm-disk"
	node1 := "worker-node-1"
	node2 := "worker-node-2"
	gracePeriod := 30 * time.Second

	// Initial attachment to node-1
	err := am.TrackAttachment(ctx, volumeID, node1)
	if err != nil {
		t.Fatalf("Initial track failed: %v", err)
	}

	// Detach from node-1 (VM migrating)
	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("Untrack failed: %v", err)
	}

	// Immediately try to attach to node-2 (migration target)
	// Should be within grace period
	if !am.IsWithinGracePeriod(volumeID, gracePeriod) {
		t.Error("Expected to be within grace period immediately after detach")
	}

	// Clear timestamp as handoff completes
	am.ClearDetachTimestamp(volumeID)

	// Track to new node
	err = am.TrackAttachment(ctx, volumeID, node2)
	if err != nil {
		t.Fatalf("Reattachment failed: %v", err)
	}

	// Verify attached to new node
	state, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected attachment to exist")
	}
	if state.NodeID != node2 {
		t.Errorf("Expected attachment to node %s, got %s", node2, state.NodeID)
	}
}
