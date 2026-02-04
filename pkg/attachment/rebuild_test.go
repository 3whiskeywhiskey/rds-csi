package attachment

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Helper functions for creating test objects

// createFakeVolumeAttachment creates a test VolumeAttachment with specified parameters.
func createFakeVolumeAttachment(name, attacher, volumeID, nodeID string, attached bool) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Now(),
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: attacher,
			NodeName: nodeID,
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &volumeID,
			},
		},
		Status: storagev1.VolumeAttachmentStatus{
			Attached: attached,
		},
	}
}

// createFakeVolumeAttachmentWithTime creates a VA with a specific creation timestamp.
func createFakeVolumeAttachmentWithTime(name, attacher, volumeID, nodeID string, attached bool, creationTime time.Time) *storagev1.VolumeAttachment {
	va := createFakeVolumeAttachment(name, attacher, volumeID, nodeID, attached)
	va.CreationTimestamp = metav1.NewTime(creationTime)
	return va
}

// createFakePV creates a test PersistentVolume with specified access modes.
func createFakePV(volumeID string, accessModes []corev1.PersistentVolumeAccessMode) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeID,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: accessModes,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       driverName,
					VolumeHandle: volumeID,
				},
			},
		},
	}
}

// createFakePVWithAnnotations creates a PV with attachment annotations.
func createFakePVWithAnnotations(volumeID, nodeID, attachedAt string) *corev1.PersistentVolume {
	pv := createFakePV(volumeID, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})
	pv.Annotations = map[string]string{
		AnnotationAttachedNode: nodeID,
		AnnotationAttachedAt:   attachedAt,
	}
	return pv
}

// Task 1: Test basic VA-based rebuild scenarios

func TestRebuildStateFromVolumeAttachments_SingleAttachment(t *testing.T) {
	volumeID := "pvc-vol1"
	nodeID := "node-1"

	// Create fake VA and PV
	va := createFakeVolumeAttachment("va1", driverName, volumeID, nodeID, true)
	pv := createFakePV(volumeID, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})

	// Create fake clientset
	client := fake.NewSimpleClientset(va, pv)
	am := NewAttachmentManager(client)

	// Rebuild state
	err := am.RebuildStateFromVolumeAttachments(context.Background())
	if err != nil {
		t.Fatalf("RebuildStateFromVolumeAttachments failed: %v", err)
	}

	// Verify attachment exists
	state, exists := am.GetAttachment(volumeID)
	if !exists {
		t.Fatal("Expected attachment to exist after rebuild")
	}

	// Verify correct volumeID
	if state.VolumeID != volumeID {
		t.Errorf("Expected volumeID %s, got %s", volumeID, state.VolumeID)
	}

	// Verify correct nodeID
	if state.NodeID != nodeID {
		t.Errorf("Expected nodeID %s, got %s", nodeID, state.NodeID)
	}

	// Verify Nodes slice
	if len(state.Nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(state.Nodes))
	}
	if state.Nodes[0].NodeID != nodeID {
		t.Errorf("Expected node[0].NodeID %s, got %s", nodeID, state.Nodes[0].NodeID)
	}

	// Verify access mode is RWO
	if state.AccessMode != "RWO" {
		t.Errorf("Expected AccessMode RWO, got %s", state.AccessMode)
	}

	// Verify not in migration
	if state.MigrationStartedAt != nil {
		t.Errorf("Expected no migration state, got MigrationStartedAt=%v", state.MigrationStartedAt)
	}
}

func TestRebuildStateFromVolumeAttachments_MultipleVolumes(t *testing.T) {
	vol1 := "pvc-vol1"
	vol2 := "pvc-vol2"
	vol3 := "pvc-vol3"

	// Create 3 VAs for different volumes
	va1 := createFakeVolumeAttachment("va1", driverName, vol1, "node-1", true)
	va2 := createFakeVolumeAttachment("va2", driverName, vol2, "node-2", true)
	va3 := createFakeVolumeAttachment("va3", driverName, vol3, "node-3", true)

	// Create corresponding PVs
	pv1 := createFakePV(vol1, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})
	pv2 := createFakePV(vol2, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})
	pv3 := createFakePV(vol3, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})

	client := fake.NewSimpleClientset(va1, va2, va3, pv1, pv2, pv3)
	am := NewAttachmentManager(client)

	// Rebuild state
	err := am.RebuildStateFromVolumeAttachments(context.Background())
	if err != nil {
		t.Fatalf("RebuildStateFromVolumeAttachments failed: %v", err)
	}

	// Verify all 3 attachments exist
	attachments := am.ListAttachments()
	if len(attachments) != 3 {
		t.Fatalf("Expected 3 attachments, got %d", len(attachments))
	}

	// Verify each volume
	for volumeID, expectedNode := range map[string]string{
		vol1: "node-1",
		vol2: "node-2",
		vol3: "node-3",
	} {
		state, exists := am.GetAttachment(volumeID)
		if !exists {
			t.Errorf("Expected attachment for volume %s to exist", volumeID)
			continue
		}
		if state.NodeID != expectedNode {
			t.Errorf("Volume %s: expected node %s, got %s", volumeID, expectedNode, state.NodeID)
		}
	}
}

func TestRebuildStateFromVolumeAttachments_NoAttachments(t *testing.T) {
	// Create fake clientset with no VolumeAttachments
	client := fake.NewSimpleClientset()
	am := NewAttachmentManager(client)

	// Rebuild state
	err := am.RebuildStateFromVolumeAttachments(context.Background())
	if err != nil {
		t.Fatalf("RebuildStateFromVolumeAttachments failed: %v", err)
	}

	// Verify no attachments
	attachments := am.ListAttachments()
	if len(attachments) != 0 {
		t.Errorf("Expected 0 attachments, got %d", len(attachments))
	}
}

func TestRebuildStateFromVolumeAttachments_DetachedVA(t *testing.T) {
	volumeID := "pvc-vol1"

	// Create detached VA (attached=false)
	va := createFakeVolumeAttachment("va1", driverName, volumeID, "node-1", false)
	pv := createFakePV(volumeID, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})

	client := fake.NewSimpleClientset(va, pv)
	am := NewAttachmentManager(client)

	// Rebuild state
	err := am.RebuildStateFromVolumeAttachments(context.Background())
	if err != nil {
		t.Fatalf("RebuildStateFromVolumeAttachments failed: %v", err)
	}

	// Verify detached VA is NOT included
	state, exists := am.GetAttachment(volumeID)
	if exists {
		t.Errorf("Expected detached VA to NOT be in state, but found: %+v", state)
	}
}

func TestRebuildStateFromVolumeAttachments_OtherDriverVA(t *testing.T) {
	volumeID := "pvc-vol1"
	otherDriver := "ebs.csi.aws.com"

	// Create VA for different driver
	va := createFakeVolumeAttachment("va1", otherDriver, volumeID, "node-1", true)
	pv := createFakePV(volumeID, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce})

	client := fake.NewSimpleClientset(va, pv)
	am := NewAttachmentManager(client)

	// Rebuild state
	err := am.RebuildStateFromVolumeAttachments(context.Background())
	if err != nil {
		t.Fatalf("RebuildStateFromVolumeAttachments failed: %v", err)
	}

	// Verify other driver VA is NOT included
	state, exists := am.GetAttachment(volumeID)
	if exists {
		t.Errorf("Expected other driver VA to NOT be in state, but found: %+v", state)
	}
}
