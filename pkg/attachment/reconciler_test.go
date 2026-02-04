package attachment

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

// createTestListers creates test node and PV listers from a fake clientset
func createTestListers(clientset *fake.Clientset, objects ...runtime.Object) (nodeLister corev1listers.NodeLister, pvLister corev1listers.PersistentVolumeLister) {
	for _, obj := range objects {
		switch v := obj.(type) {
		case *corev1.Node:
			clientset.CoreV1().Nodes().Create(context.Background(), v, metav1.CreateOptions{})
		case *corev1.PersistentVolume:
			clientset.CoreV1().PersistentVolumes().Create(context.Background(), v, metav1.CreateOptions{})
		}
	}
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	nodeLister = informerFactory.Core().V1().Nodes().Lister()
	pvLister = informerFactory.Core().V1().PersistentVolumes().Lister()
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)
	return nodeLister, pvLister
}

func TestNewAttachmentReconciler_RequiresManager(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	_, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:    nil,
		K8sClient:  k8sClient,
		NodeLister: nodeLister,
		PVLister:   pvLister,
	})

	if err == nil {
		t.Error("Expected error when manager is nil")
	}
}

func TestNewAttachmentReconciler_RequiresK8sClient(t *testing.T) {
	am := NewAttachmentManager(nil)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	_, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:    am,
		K8sClient:  nil,
		NodeLister: nodeLister,
		PVLister:   pvLister,
	})

	if err == nil {
		t.Error("Expected error when k8sClient is nil")
	}
}

func TestNewAttachmentReconciler_DefaultValues(t *testing.T) {
	am := NewAttachmentManager(nil)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:    am,
		K8sClient:  k8sClient,
		NodeLister: nodeLister,
		PVLister:   pvLister,
		// No interval or grace period specified
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check defaults
	if r.interval != 5*time.Minute {
		t.Errorf("Expected default interval of 5 minutes, got %v", r.interval)
	}
	if r.gracePeriod != 30*time.Second {
		t.Errorf("Expected default grace period of 30 seconds, got %v", r.gracePeriod)
	}
}

func TestReconciler_StartStop(t *testing.T) {
	am := NewAttachmentManager(nil)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    100 * time.Millisecond,
		GracePeriod: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	ctx := context.Background()

	// Start should succeed
	err = r.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Start again should fail (already running)
	err = r.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already-running reconciler")
	}

	// Stop should succeed
	r.Stop()

	// Stop again should be safe (no-op)
	r.Stop()
}

func TestReconciler_ContextCancellation(t *testing.T) {
	am := NewAttachmentManager(nil)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    1 * time.Hour, // Long interval so we control timing
		GracePeriod: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	err = r.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Get doneCh before cancelling
	r.mu.Lock()
	doneCh := r.doneCh
	r.mu.Unlock()

	// Cancel context - this should cause the goroutine to exit
	cancel()

	// Wait for goroutine to exit (with timeout)
	select {
	case <-doneCh:
		// Successfully stopped
	case <-time.After(1 * time.Second):
		t.Fatal("Reconciler did not stop within 1 second after context cancellation")
	}
}

func TestReconciler_ClearsStaleAttachment_NodeDeleted(t *testing.T) {
	// Create fake k8s client with NO nodes
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	// Create attachment manager and track a volume
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-stale"
	nodeID := "deleted-node"

	// Track attachment to a node that doesn't exist in k8s
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Verify attachment exists
	_, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected attachment to exist before reconciliation")
	}

	// Create reconciler with very short grace period (already expired)
	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    100 * time.Millisecond,
		GracePeriod: 1 * time.Nanosecond, // Effectively expired immediately
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	// Run a single reconciliation
	r.reconcile(ctx)

	// Verify attachment was cleared
	_, exists = am.GetAttachment(volumeID)
	if exists {
		t.Error("Expected stale attachment to be cleared after reconciliation")
	}
}

func TestReconciler_PreservesValidAttachment_NodeExists(t *testing.T) {
	// Create fake k8s client with a node
	existingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-node",
		},
	}
	k8sClient := fake.NewSimpleClientset(existingNode)
	nodeLister, pvLister := createTestListers(k8sClient)

	// Create attachment manager and track a volume
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-valid"
	nodeID := "existing-node"

	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Create reconciler
	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    100 * time.Millisecond,
		GracePeriod: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	// Run reconciliation
	r.reconcile(ctx)

	// Verify attachment still exists (node exists, so attachment is valid)
	_, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Error("Expected valid attachment to be preserved after reconciliation")
	}
}

func TestReconciler_RespectsGracePeriod(t *testing.T) {
	// Create fake k8s client with NO nodes
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	// Create attachment manager
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-grace"
	nodeID := "deleted-node"

	// Track and then untrack to create a recent detach timestamp
	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}
	err = am.UntrackAttachment(ctx, volumeID)
	if err != nil {
		t.Fatalf("UntrackAttachment failed: %v", err)
	}

	// Re-track (simulating the state when reconciler runs)
	err = am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("Re-track failed: %v", err)
	}

	// Create reconciler with long grace period
	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    100 * time.Millisecond,
		GracePeriod: 1 * time.Hour, // Very long grace period
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	// Run reconciliation
	r.reconcile(ctx)

	// Attachment should still exist because we're within grace period
	// (detach timestamp was set just before)
	_, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Error("Expected attachment to be preserved during grace period")
	}
}

func TestReconciler_HandlesAPIErrors(t *testing.T) {
	// NOTE: With informer-based caching, API errors during reconciliation don't occur
	// because we use cached listers. API errors would only happen during initial cache sync.
	// This test now verifies correct behavior with cached data: node not in cache = deleted

	// Create fake k8s client with NO nodes (simulating deleted node)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	// Create attachment manager and track a volume to non-existent node
	am := NewAttachmentManager(nil)
	ctx := context.Background()
	volumeID := "pvc-test-cache-miss"
	nodeID := "deleted-node"

	err := am.TrackAttachment(ctx, volumeID, nodeID)
	if err != nil {
		t.Fatalf("TrackAttachment failed: %v", err)
	}

	// Create reconciler
	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		Interval:    100 * time.Millisecond,
		GracePeriod: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	// Run reconciliation - should clear attachment (node not in cache = deleted)
	r.reconcile(ctx)

	// Attachment should be cleared (node doesn't exist in cache)
	_, exists := am.GetAttachment(volumeID)
	if exists {
		t.Error("Expected attachment to be cleared when node not in informer cache")
	}
}

func TestReconciler_GetGracePeriod(t *testing.T) {
	am := NewAttachmentManager(nil)
	k8sClient := fake.NewSimpleClientset()
	nodeLister, pvLister := createTestListers(k8sClient)

	gracePeriod := 45 * time.Second
	r, err := NewAttachmentReconciler(ReconcilerConfig{
		Manager:     am,
		K8sClient:   k8sClient,
		NodeLister:  nodeLister,
		PVLister:    pvLister,
		GracePeriod: gracePeriod,
	})
	if err != nil {
		t.Fatalf("Failed to create reconciler: %v", err)
	}

	if r.GetGracePeriod() != gracePeriod {
		t.Errorf("Expected grace period %v, got %v", gracePeriod, r.GetGracePeriod())
	}
}
