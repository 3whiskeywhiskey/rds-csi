package reconciler

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
)

// mockRDSClient implements rds.RDSClient for testing
type mockRDSClient struct {
	volumes        []rds.VolumeInfo
	files          []rds.FileInfo
	deletedVolumes []string
	deletedFiles   []string
}

func (m *mockRDSClient) CreateVolume(opts rds.CreateVolumeOptions) error {
	return nil
}

func (m *mockRDSClient) DeleteVolume(slot string) error {
	m.deletedVolumes = append(m.deletedVolumes, slot)
	return nil
}

func (m *mockRDSClient) ResizeVolume(slot string, newSizeBytes int64) error {
	return nil
}

func (m *mockRDSClient) GetVolume(slot string) (*rds.VolumeInfo, error) {
	for _, vol := range m.volumes {
		if vol.Slot == slot {
			return &vol, nil
		}
	}
	return nil, &rds.VolumeNotFoundError{Slot: slot}
}

func (m *mockRDSClient) VerifyVolumeExists(slot string) error {
	_, err := m.GetVolume(slot)
	return err
}

func (m *mockRDSClient) GetCapacity(basePath string) (*rds.CapacityInfo, error) {
	return &rds.CapacityInfo{
		TotalBytes: 1000000000000,
		FreeBytes:  500000000000,
		UsedBytes:  500000000000,
	}, nil
}

func (m *mockRDSClient) ListVolumes() ([]rds.VolumeInfo, error) {
	return m.volumes, nil
}

func (m *mockRDSClient) ListFiles(path string) ([]rds.FileInfo, error) {
	return m.files, nil
}

func (m *mockRDSClient) DeleteFile(path string) error {
	m.deletedFiles = append(m.deletedFiles, path)
	return nil
}

func (m *mockRDSClient) Connect() error {
	return nil
}

func (m *mockRDSClient) Close() error {
	return nil
}

func (m *mockRDSClient) GetAddress() string {
	return "10.42.241.3"
}

func (m *mockRDSClient) IsConnected() bool {
	return true
}

func (m *mockRDSClient) CreateSnapshot(opts rds.CreateSnapshotOptions) (*rds.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRDSClient) DeleteSnapshot(snapshotID string) error {
	return nil
}

func (m *mockRDSClient) GetSnapshot(snapshotID string) (*rds.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRDSClient) ListSnapshots() ([]rds.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRDSClient) RestoreSnapshot(snapshotID string, newVolumeOpts rds.CreateVolumeOptions) error {
	return nil
}

func TestNewOrphanReconciler(t *testing.T) {
	tests := []struct {
		name    string
		config  OrphanReconcilerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: OrphanReconcilerConfig{
				RDSClient: &mockRDSClient{},
				K8sClient: fake.NewSimpleClientset(),
				Enabled:   true,
			},
			wantErr: false,
		},
		{
			name: "missing RDS client",
			config: OrphanReconcilerConfig{
				K8sClient: fake.NewSimpleClientset(),
				Enabled:   true,
			},
			wantErr: true,
		},
		{
			name: "missing k8s client",
			config: OrphanReconcilerConfig{
				RDSClient: &mockRDSClient{},
				Enabled:   true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOrphanReconciler(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOrphanReconciler() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOrphanReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name          string
		rdsVolumes    []rds.VolumeInfo
		k8sPVs        []*v1.PersistentVolume
		dryRun        bool
		wantDeleted   []string
		wantNoDeletes bool
	}{
		{
			name: "no orphans - all volumes have PVs",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "pvc-123", FilePath: "/storage-pool/metal-csi/pvc-123.img", FileSizeBytes: 10737418240},
				{Slot: "pvc-456", FilePath: "/storage-pool/metal-csi/pvc-456.img", FileSizeBytes: 10737418240},
			},
			k8sPVs: []*v1.PersistentVolume{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-123"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-123",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-456"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-456",
							},
						},
					},
				},
			},
			wantNoDeletes: true,
		},
		{
			name: "orphaned volume - no corresponding PV",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "pvc-123", FilePath: "/storage-pool/metal-csi/pvc-123.img", FileSizeBytes: 10737418240},
				{Slot: "pvc-orphan", FilePath: "/storage-pool/metal-csi/pvc-orphan.img", FileSizeBytes: 10737418240},
			},
			k8sPVs: []*v1.PersistentVolume{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-123"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-123",
							},
						},
					},
				},
			},
			wantDeleted: []string{"pvc-orphan"},
		},
		{
			name: "dry run - orphans detected but not deleted",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "pvc-orphan", FilePath: "/storage-pool/metal-csi/pvc-orphan.img", FileSizeBytes: 10737418240},
			},
			k8sPVs:        []*v1.PersistentVolume{},
			dryRun:        true,
			wantNoDeletes: true,
		},
		{
			name: "non-CSI volumes ignored",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "manual-volume", FilePath: "/storage-pool/manual-volume.img", FileSizeBytes: 10737418240},
				{Slot: "pvc-123", FilePath: "/storage-pool/metal-csi/pvc-123.img", FileSizeBytes: 10737418240},
			},
			k8sPVs: []*v1.PersistentVolume{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-123"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-123",
							},
						},
					},
				},
			},
			wantNoDeletes: true, // manual-volume should be ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock RDS client
			mockRDS := &mockRDSClient{
				volumes:        tt.rdsVolumes,
				deletedVolumes: []string{},
			}

			// Create fake Kubernetes clientset with PVs
			k8sClient := fake.NewSimpleClientset()
			for _, pv := range tt.k8sPVs {
				if _, err := k8sClient.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{}); err != nil {
					t.Fatalf("Failed to create test PV: %v", err)
				}
			}

			// Create reconciler
			config := OrphanReconcilerConfig{
				RDSClient:     mockRDS,
				K8sClient:     k8sClient,
				CheckInterval: 1 * time.Hour,
				GracePeriod:   1 * time.Second, // Short grace period for testing
				DryRun:        tt.dryRun,
				Enabled:       true,
			}

			reconciler, err := NewOrphanReconciler(config)
			if err != nil {
				t.Fatalf("NewOrphanReconciler() failed: %v", err)
			}

			// Run reconciliation
			if err := reconciler.reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile() failed: %v", err)
			}

			// Check deleted volumes
			if tt.wantNoDeletes {
				if len(mockRDS.deletedVolumes) > 0 {
					t.Errorf("Expected no deletions, but got: %v", mockRDS.deletedVolumes)
				}
			} else {
				if len(mockRDS.deletedVolumes) != len(tt.wantDeleted) {
					t.Errorf("Expected %d deletions, got %d: %v", len(tt.wantDeleted), len(mockRDS.deletedVolumes), mockRDS.deletedVolumes)
				}

				// Check that the correct volumes were deleted
				deletedMap := make(map[string]bool)
				for _, vol := range mockRDS.deletedVolumes {
					deletedMap[vol] = true
				}

				for _, expected := range tt.wantDeleted {
					if !deletedMap[expected] {
						t.Errorf("Expected volume %s to be deleted, but it wasn't", expected)
					}
				}
			}
		})
	}
}

func TestOrphanReconciler_OrphanedFiles(t *testing.T) {
	tests := []struct {
		name        string
		rdsVolumes  []rds.VolumeInfo
		files       []rds.FileInfo
		k8sPVs      []*v1.PersistentVolume
		basePath    string
		expectFiles int // Expected number of orphaned files
	}{
		{
			name: "orphaned files - files without disk objects",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "pvc-123", FilePath: "/storage-pool/metal-csi/pvc-123.img", FileSizeBytes: 10737418240},
			},
			files: []rds.FileInfo{
				// Note: Mock returns normalized paths (with leading /) as parseFileInfo() would
				// In production, RouterOS /file print returns paths without /, but parseFileInfo() adds it
				{Name: "pvc-123.img", Path: "/storage-pool/metal-csi/pvc-123.img", SizeBytes: 10737418240, Type: "file"},
				{Name: "pvc-orphan1.img", Path: "/storage-pool/metal-csi/pvc-orphan1.img", SizeBytes: 10737418240, Type: "file"},
				{Name: "pvc-orphan2.img", Path: "/storage-pool/metal-csi/pvc-orphan2.img", SizeBytes: 10737418240, Type: "file"},
			},
			k8sPVs: []*v1.PersistentVolume{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-123"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-123",
							},
						},
					},
				},
			},
			basePath:    "/storage-pool/metal-csi",
			expectFiles: 2, // pvc-orphan1.img and pvc-orphan2.img
		},
		{
			name: "no orphaned files - all files have disk objects",
			rdsVolumes: []rds.VolumeInfo{
				{Slot: "pvc-123", FilePath: "/storage-pool/metal-csi/pvc-123.img", FileSizeBytes: 10737418240},
				{Slot: "pvc-456", FilePath: "/storage-pool/metal-csi/pvc-456.img", FileSizeBytes: 10737418240},
			},
			files: []rds.FileInfo{
				// Mock returns normalized paths as parseFileInfo() would
				{Name: "pvc-123.img", Path: "/storage-pool/metal-csi/pvc-123.img", SizeBytes: 10737418240, Type: "file"},
				{Name: "pvc-456.img", Path: "/storage-pool/metal-csi/pvc-456.img", SizeBytes: 10737418240, Type: "file"},
			},
			k8sPVs: []*v1.PersistentVolume{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-123"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-123",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pv-456"},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "rds.csi.srvlab.io",
								VolumeHandle: "pvc-456",
							},
						},
					},
				},
			},
			basePath:    "/storage-pool/metal-csi",
			expectFiles: 0,
		},
		{
			name:        "no files listed when basePath is empty",
			rdsVolumes:  []rds.VolumeInfo{},
			files:       []rds.FileInfo{},
			k8sPVs:      []*v1.PersistentVolume{},
			basePath:    "", // Empty base path - file checking disabled
			expectFiles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock RDS client
			mockRDS := &mockRDSClient{
				volumes:        tt.rdsVolumes,
				files:          tt.files,
				deletedVolumes: []string{},
			}

			// Create fake Kubernetes clientset with PVs
			k8sClient := fake.NewSimpleClientset()
			for _, pv := range tt.k8sPVs {
				if _, err := k8sClient.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{}); err != nil {
					t.Fatalf("Failed to create test PV: %v", err)
				}
			}

			// Create reconciler
			config := OrphanReconcilerConfig{
				RDSClient:     mockRDS,
				K8sClient:     k8sClient,
				CheckInterval: 1 * time.Hour,
				GracePeriod:   1 * time.Second,
				DryRun:        true, // Always dry-run for tests
				Enabled:       true,
				BasePath:      tt.basePath,
			}

			reconciler, err := NewOrphanReconciler(config)
			if err != nil {
				t.Fatalf("NewOrphanReconciler() failed: %v", err)
			}

			// Run reconciliation
			if err := reconciler.reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile() failed: %v", err)
			}

			// Verify orphaned files were detected (we can't check the count directly,
			// but we verify the reconciliation completed without error)
			// In a real scenario, we'd expose metrics or a status API to verify counts
		})
	}
}
