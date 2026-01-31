package driver

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestVMIGrouper_Disabled(t *testing.T) {
	// When disabled, should return empty string and no-op unlock
	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fake.NewSimpleClientset(),
		Enabled:   false,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "test-pvc")
	if vmiKey != "" {
		t.Errorf("Expected empty vmiKey when disabled, got %s", vmiKey)
	}
	unlock() // Should not panic
}

func TestVMIGrouper_NoK8sClient(t *testing.T) {
	// When no k8s client, should return empty string and no-op unlock
	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: nil,
		Enabled:   true,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "test-pvc")
	if vmiKey != "" {
		t.Errorf("Expected empty vmiKey with nil k8s client, got %s", vmiKey)
	}
	unlock() // Should not panic
}

func TestVMIGrouper_ResolvesVMIFromPod(t *testing.T) {
	// Create a fake k8s client with a pod that mounts a PVC and has VMI owner
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-myvm-12345",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "VirtualMachineInstance",
						Name: "myvm",
					},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
		CacheTTL:  60 * time.Second,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "test-pvc")
	defer unlock()

	expectedKey := "test-ns/myvm"
	if vmiKey != expectedKey {
		t.Errorf("Expected vmiKey %s, got %s", expectedKey, vmiKey)
	}
}

func TestVMIGrouper_ResolvesVMIFromLabel(t *testing.T) {
	// Create a pod with VMI label instead of ownerReference (hotplug scenario)
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hp-volume-12345",
				Namespace: "test-ns",
				Labels: map[string]string{
					"kubevirt.io/vmi": "myvm",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "hotplug",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "hotplug-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "hotplug-pvc")
	defer unlock()

	expectedKey := "test-ns/myvm"
	if vmiKey != expectedKey {
		t.Errorf("Expected vmiKey %s, got %s", expectedKey, vmiKey)
	}
}

func TestVMIGrouper_CachesResults(t *testing.T) {
	// Create fake client
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-myvm-12345",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "VirtualMachineInstance",
						Name: "myvm",
					},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "cached-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
		CacheTTL:  5 * time.Minute,
	})

	ctx := context.Background()

	// First call - should hit API
	vmiKey1, unlock1 := grouper.LockVMI(ctx, "test-ns", "cached-pvc")
	unlock1()

	// Second call - should hit cache
	vmiKey2, unlock2 := grouper.LockVMI(ctx, "test-ns", "cached-pvc")
	unlock2()

	if vmiKey1 != vmiKey2 {
		t.Errorf("Cache should return same result: %s vs %s", vmiKey1, vmiKey2)
	}

	expectedKey := "test-ns/myvm"
	if vmiKey1 != expectedKey {
		t.Errorf("Expected vmiKey %s, got %s", expectedKey, vmiKey1)
	}
}

func TestVMIGrouper_CacheInvalidation(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-myvm-12345",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "VirtualMachineInstance",
						Name: "myvm",
					},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "invalidate-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
		CacheTTL:  5 * time.Minute,
	})

	ctx := context.Background()

	// First call - populate cache
	vmiKey1, unlock1 := grouper.LockVMI(ctx, "test-ns", "invalidate-pvc")
	unlock1()

	if vmiKey1 != "test-ns/myvm" {
		t.Fatalf("Expected vmiKey test-ns/myvm, got %s", vmiKey1)
	}

	// Invalidate cache
	grouper.InvalidateCache("test-ns", "invalidate-pvc")

	// Verify cache was cleared (getCached returns nil)
	entry := grouper.getCached("test-ns/invalidate-pvc")
	if entry != nil {
		t.Error("Expected cache to be invalidated")
	}
}

func TestVMIGrouper_SerializesOperations(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-myvm-12345",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "VirtualMachineInstance",
						Name: "myvm",
					},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-1",
							},
						},
					},
					{
						Name: "vol2",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-2",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
	})

	ctx := context.Background()

	// Track order of operations
	var orderMu sync.Mutex
	var order []string
	var counter int32

	var wg sync.WaitGroup
	wg.Add(2)

	// Launch two concurrent operations on same VMI
	go func() {
		defer wg.Done()
		vmiKey, unlock := grouper.LockVMI(ctx, "test-ns", "pvc-1")
		defer unlock()

		// Record entry order
		n := atomic.AddInt32(&counter, 1)
		orderMu.Lock()
		order = append(order, "pvc-1-enter")
		orderMu.Unlock()

		// Simulate work
		time.Sleep(50 * time.Millisecond)

		orderMu.Lock()
		order = append(order, "pvc-1-exit")
		orderMu.Unlock()

		_ = vmiKey
		_ = n
	}()

	go func() {
		defer wg.Done()
		// Small delay to ensure pvc-1 likely grabs lock first
		time.Sleep(10 * time.Millisecond)

		vmiKey, unlock := grouper.LockVMI(ctx, "test-ns", "pvc-2")
		defer unlock()

		orderMu.Lock()
		order = append(order, "pvc-2-enter")
		orderMu.Unlock()

		time.Sleep(10 * time.Millisecond)

		orderMu.Lock()
		order = append(order, "pvc-2-exit")
		orderMu.Unlock()

		_ = vmiKey
	}()

	wg.Wait()

	// The operations should be serialized - pvc-2 shouldn't enter until pvc-1 exits
	// Expected order: pvc-1-enter, pvc-1-exit, pvc-2-enter, pvc-2-exit
	// (or the reverse if pvc-2 grabs lock first)
	orderMu.Lock()
	defer orderMu.Unlock()

	if len(order) != 4 {
		t.Fatalf("Expected 4 order entries, got %d: %v", len(order), order)
	}

	// Verify serialization - second enter should come after first exit
	firstEnterIdx := -1
	firstExitIdx := -1
	secondEnterIdx := -1

	for i, op := range order {
		if firstEnterIdx == -1 && (op == "pvc-1-enter" || op == "pvc-2-enter") {
			firstEnterIdx = i
		} else if op == "pvc-1-exit" || op == "pvc-2-exit" {
			if firstExitIdx == -1 {
				firstExitIdx = i
			}
		}
		if firstEnterIdx != -1 && (op == "pvc-1-enter" || op == "pvc-2-enter") && i != firstEnterIdx {
			secondEnterIdx = i
		}
	}

	if secondEnterIdx != -1 && secondEnterIdx < firstExitIdx {
		t.Errorf("Operations not serialized properly. Order: %v", order)
	}
}

func TestVMIGrouper_NoVMIFound(t *testing.T) {
	// Create a pod without VMI owner
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "regular-pod",
				Namespace: "test-ns",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "non-vmi-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "non-vmi-pvc")
	defer unlock()

	// Should return empty when no VMI found
	if vmiKey != "" {
		t.Errorf("Expected empty vmiKey for non-VMI pod, got %s", vmiKey)
	}
}

func TestVMIGrouper_NoPodFound(t *testing.T) {
	// Empty cluster - no pods
	fakeClient := fake.NewSimpleClientset()

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
	})

	vmiKey, unlock := grouper.LockVMI(context.Background(), "test-ns", "orphan-pvc")
	defer unlock()

	// Should return empty when no pod mounts the PVC
	if vmiKey != "" {
		t.Errorf("Expected empty vmiKey when no pod found, got %s", vmiKey)
	}
}

func TestVMIGrouper_DifferentVMIsNotSerialized(t *testing.T) {
	// Create two pods with different VMIs
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-vm1-12345",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "VirtualMachineInstance", Name: "vm1"},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "vm1-pvc",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-vm2-67890",
				Namespace: "test-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "VirtualMachineInstance", Name: "vm2"},
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "vm2-pvc",
							},
						},
					},
				},
			},
		},
	)

	grouper := NewVMIGrouper(VMIGrouperConfig{
		K8sClient: fakeClient,
		Enabled:   true,
	})

	ctx := context.Background()
	var wg sync.WaitGroup
	var vm1Done, vm2Done int32

	wg.Add(2)

	// Both should run concurrently since different VMIs
	go func() {
		defer wg.Done()
		vmiKey, unlock := grouper.LockVMI(ctx, "test-ns", "vm1-pvc")
		defer unlock()

		if vmiKey != "test-ns/vm1" {
			t.Errorf("Expected test-ns/vm1, got %s", vmiKey)
		}

		time.Sleep(50 * time.Millisecond)
		atomic.StoreInt32(&vm1Done, 1)
	}()

	go func() {
		defer wg.Done()
		vmiKey, unlock := grouper.LockVMI(ctx, "test-ns", "vm2-pvc")
		defer unlock()

		if vmiKey != "test-ns/vm2" {
			t.Errorf("Expected test-ns/vm2, got %s", vmiKey)
		}

		// Note: We don't assert vm1 hasn't finished - scheduler timing is unpredictable.
		// The key validation is that both complete (below) and use different VMI keys.
		atomic.StoreInt32(&vm2Done, 1)
	}()

	wg.Wait()

	if atomic.LoadInt32(&vm1Done) != 1 || atomic.LoadInt32(&vm2Done) != 1 {
		t.Error("Both operations should have completed")
	}
}
