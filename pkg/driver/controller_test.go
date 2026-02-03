package driver

import (
	"context"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"git.srvlab.io/whiskey/rds-csi-driver/pkg/attachment"
	"git.srvlab.io/whiskey/rds-csi-driver/pkg/rds"
)

func TestValidateVolumeCapabilities(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{
			vcaps: []*csi.VolumeCapability_AccessMode{
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
			},
		},
	}

	tests := []struct {
		name      string
		caps      []*csi.VolumeCapability
		expectErr bool
	}{
		{
			name: "valid single node writer with mount",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid single node reader with block",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "unsupported multi node access",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "missing access type",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cs.validateVolumeCapabilities(tt.caps)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{
			cscaps: []*csi.ControllerServiceCapability{
				{
					Type: &csi.ControllerServiceCapability_Rpc{
						Rpc: &csi.ControllerServiceCapability_RPC{
							Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
						},
					},
				},
			},
		},
	}

	req := &csi.ControllerGetCapabilitiesRequest{}
	resp, err := cs.ControllerGetCapabilities(context.Background(), req)

	if err != nil {
		t.Fatalf("ControllerGetCapabilities failed: %v", err)
	}

	if len(resp.Capabilities) == 0 {
		t.Error("Expected capabilities but got none")
	}

	hasCreateDelete := false
	for _, cap := range resp.Capabilities {
		if cap.GetRpc() != nil {
			if cap.GetRpc().Type == csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME {
				hasCreateDelete = true
				break
			}
		}
	}

	if !hasCreateDelete {
		t.Error("Expected CREATE_DELETE_VOLUME capability but not found")
	}
}

func TestCreateVolumeValidation(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{
			vcaps: []*csi.VolumeCapability_AccessMode{
				{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			},
		},
	}

	tests := []struct {
		name      string
		req       *csi.CreateVolumeRequest
		expectErr bool
		errCode   codes.Code
	}{
		{
			name: "missing volume name",
			req: &csi.CreateVolumeRequest{
				Name: "",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
		{
			name: "missing volume capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               "test-volume",
				VolumeCapabilities: nil,
			},
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
		{
			name: "invalid volume capabilities",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cs.CreateVolume(context.Background(), tt.req)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected gRPC status error, got: %v", err)
					return
				}
				if st.Code() != tt.errCode {
					t.Errorf("Expected error code %v, got %v", tt.errCode, st.Code())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteVolumeValidation(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{},
	}

	tests := []struct {
		name      string
		volumeID  string
		expectErr bool
		errCode   codes.Code
	}{
		{
			name:      "missing volume ID",
			volumeID:  "",
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
		{
			name:      "invalid volume ID format",
			volumeID:  "invalid-format",
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
		{
			name:      "injection attempt",
			volumeID:  "pvc-test; rm -rf /",
			expectErr: true,
			errCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &csi.DeleteVolumeRequest{
				VolumeId: tt.volumeID,
			}

			_, err := cs.DeleteVolume(context.Background(), req)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected gRPC status error, got: %v", err)
					return
				}
				if st.Code() != tt.errCode {
					t.Errorf("Expected error code %v, got %v", tt.errCode, st.Code())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestUnimplementedMethods(t *testing.T) {
	cs := &ControllerServer{
		driver: &Driver{},
	}

	// Test all unimplemented methods return Unimplemented error
	// Note: ControllerPublishVolume and ControllerUnpublishVolume are now implemented
	// Note: ControllerExpandVolume is now implemented, so it's not in this test

	t.Run("CreateSnapshot", func(t *testing.T) {
		_, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{})
		if err == nil {
			t.Error("Expected unimplemented error")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.Unimplemented {
			t.Errorf("Expected Unimplemented code, got %v", st.Code())
		}
	})
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		expectProto string
		expectAddr  string
		expectErr   bool
	}{
		{
			name:        "unix socket with scheme",
			endpoint:    "unix:///tmp/csi.sock",
			expectProto: "unix",
			expectAddr:  "/tmp/csi.sock",
			expectErr:   false,
		},
		{
			name:        "unix socket without scheme",
			endpoint:    "/tmp/csi.sock",
			expectProto: "unix",
			expectAddr:  "/tmp/csi.sock",
			expectErr:   false,
		},
		{
			name:        "tcp endpoint",
			endpoint:    "tcp://0.0.0.0:10000",
			expectProto: "tcp",
			expectAddr:  "0.0.0.0:10000",
			expectErr:   false,
		},
		{
			name:      "invalid scheme",
			endpoint:  "http://localhost:80",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, addr, err := parseEndpoint(tt.endpoint)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if proto != tt.expectProto {
					t.Errorf("Expected proto %s, got %s", tt.expectProto, proto)
				}
				if addr != tt.expectAddr {
					t.Errorf("Expected addr %s, got %s", tt.expectAddr, addr)
				}
			}
		})
	}
}

// testControllerServer creates a ControllerServer with mock RDS client and fake k8s client
func testControllerServer(t *testing.T, nodes ...*corev1.Node) (*ControllerServer, *rds.MockClient) {
	t.Helper()

	// Create fake k8s client with provided nodes
	var objects []runtime.Object
	for _, n := range nodes {
		objects = append(objects, n)
	}
	k8sClient := fake.NewSimpleClientset(objects...)

	// Create mock RDS client
	mockRDS := rds.NewMockClient()
	mockRDS.SetAddress("10.0.0.1")

	// Create driver with attachment manager
	driver := &Driver{
		name:              DriverName,
		version:           "test",
		rdsClient:         mockRDS,
		k8sClient:         k8sClient,
		attachmentManager: attachment.NewAttachmentManager(k8sClient),
	}
	driver.addVolumeCapabilities()
	driver.addControllerServiceCapabilities()

	return NewControllerServer(driver), mockRDS
}

// testNode creates a test Node object
func testNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// ========================================
// ControllerPublishVolume Tests
// ========================================

// Test volume IDs must be valid UUIDs (pvc-<uuid> format)
const (
	testVolumeID1     = "pvc-11111111-1111-1111-1111-111111111111"
	testVolumeID2     = "pvc-22222222-2222-2222-2222-222222222222"
	testVolumeID3     = "pvc-33333333-3333-3333-3333-333333333333"
	testVolumeID4     = "pvc-44444444-4444-4444-4444-444444444444"
	testVolumeID5     = "pvc-55555555-5555-5555-5555-555555555555"
	testVolumeID6     = "pvc-66666666-6666-6666-6666-666666666666"
	testVolumeID7     = "pvc-77777777-7777-7777-7777-777777777777"
	testVolumeID8     = "pvc-88888888-8888-8888-8888-888888888888"
	testVolumeIDStale = "pvc-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
)

func TestControllerPublishVolume_Success(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, mockRDS := testControllerServer(t, node1)

	// Add test volume to mock
	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:          testVolumeID1,
		FilePath:      "/storage-pool/metal-csi/" + testVolumeID1 + ".img",
		FileSizeBytes: 1073741824,
		NVMETCPPort:   4420,
		NVMETCPNQN:    "nqn.2000-02.com.mikrotik:" + testVolumeID1,
	})

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID1,
		NodeId:   "node-1",
		VolumeContext: map[string]string{
			"nvmeAddress": "10.0.0.1",
		},
	}

	resp, err := cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("ControllerPublishVolume failed: %v", err)
	}

	// CSI-05: Verify publish_context
	if resp.PublishContext == nil {
		t.Fatal("PublishContext is nil")
	}
	if resp.PublishContext["nvme_address"] == "" {
		t.Error("nvme_address missing from PublishContext")
	}
	if resp.PublishContext["nvme_port"] != "4420" {
		t.Errorf("nvme_port = %s, want 4420", resp.PublishContext["nvme_port"])
	}
	expectedNQN := "nqn.2000-02.com.mikrotik:" + testVolumeID1
	if resp.PublishContext["nvme_nqn"] != expectedNQN {
		t.Errorf("nvme_nqn = %s, want %s", resp.PublishContext["nvme_nqn"], expectedNQN)
	}
	if resp.PublishContext["fs_type"] == "" {
		t.Error("fs_type missing from PublishContext")
	}
}

func TestControllerPublishVolume_Idempotent(t *testing.T) {
	// CSI-01: Same volume, same node = success
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, mockRDS := testControllerServer(t, node1)

	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        testVolumeID2,
		NVMETCPPort: 4420,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + testVolumeID2,
	})

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID2,
		NodeId:   "node-1",
	}

	// First publish
	_, err := cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("First publish failed: %v", err)
	}

	// Second publish (same node) - should succeed
	resp, err := cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("Second publish (idempotent) failed: %v", err)
	}
	if resp.PublishContext == nil {
		t.Error("Idempotent publish should return PublishContext")
	}
}

func TestControllerPublishVolume_RWOConflict(t *testing.T) {
	// CSI-02: Same volume, different node = FAILED_PRECONDITION
	ctx := context.Background()
	node1 := testNode("node-1")
	node2 := testNode("node-2")
	cs, mockRDS := testControllerServer(t, node1, node2)

	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        testVolumeID3,
		NVMETCPPort: 4420,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + testVolumeID3,
	})

	// Attach to node-1
	req1 := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID3,
		NodeId:   "node-1",
	}
	_, err := cs.ControllerPublishVolume(ctx, req1)
	if err != nil {
		t.Fatalf("First publish failed: %v", err)
	}

	// Try to attach to node-2 - should fail
	req2 := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID3,
		NodeId:   "node-2",
	}
	_, err = cs.ControllerPublishVolume(ctx, req2)
	if err == nil {
		t.Fatal("Expected FAILED_PRECONDITION error for RWO conflict")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("Expected code FailedPrecondition (9), got %v", st.Code())
	}
	if !strings.Contains(st.Message(), "node-1") {
		t.Errorf("Error message should mention blocking node, got: %s", st.Message())
	}
}

func TestControllerPublishVolume_StaleAttachmentSelfHealing(t *testing.T) {
	// CSI-06: Volume attached to deleted node = auto-clear and allow new attach
	ctx := context.Background()
	node1 := testNode("node-1")
	// "deleted-node" does NOT exist in k8s (simulates deleted node)
	cs, mockRDS := testControllerServer(t, node1)

	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        testVolumeIDStale,
		NVMETCPPort: 4420,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + testVolumeIDStale,
	})

	// Manually track attachment to non-existent node (simulates stale state after node deletion)
	am := cs.driver.GetAttachmentManager()
	// Use TrackAttachment directly on the manager to bypass validation
	// (in reality, this could happen if controller restarts and loads stale state)
	_ = am.TrackAttachment(ctx, testVolumeIDStale, "deleted-node")

	// Try to attach to node-1 - should succeed (self-healing)
	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeIDStale,
		NodeId:   "node-1",
	}
	resp, err := cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("Expected self-healing success, got error: %v", err)
	}
	if resp.PublishContext == nil {
		t.Error("Expected PublishContext after self-healing")
	}

	// Verify attachment is now to node-1
	state, exists := am.GetAttachment(testVolumeIDStale)
	if !exists {
		t.Fatal("Attachment should exist after self-healing")
	}
	if state.NodeID != "node-1" {
		t.Errorf("Attachment should be to node-1, got %s", state.NodeID)
	}
}

func TestControllerPublishVolume_VolumeNotFound(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, _ := testControllerServer(t, node1)

	// Don't add volume to mock - it won't exist
	// Use a valid UUID format but non-existent volume
	nonExistentVolumeID := "pvc-99999999-9999-9999-9999-999999999999"

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: nonExistentVolumeID,
		NodeId:   "node-1",
	}

	_, err := cs.ControllerPublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected NOT_FOUND error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("Expected code NotFound (5), got %v", st.Code())
	}
}

func TestControllerPublishVolume_InvalidVolumeID(t *testing.T) {
	ctx := context.Background()
	cs, _ := testControllerServer(t)

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: "", // Invalid
		NodeId:   "node-1",
	}

	_, err := cs.ControllerPublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected INVALID_ARGUMENT error")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected code InvalidArgument (3), got %v", st.Code())
	}
}

func TestControllerPublishVolume_InvalidNodeID(t *testing.T) {
	ctx := context.Background()
	cs, mockRDS := testControllerServer(t)

	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        testVolumeID4,
		NVMETCPPort: 4420,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + testVolumeID4,
	})

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID4,
		NodeId:   "", // Invalid
	}

	_, err := cs.ControllerPublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected INVALID_ARGUMENT error")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected code InvalidArgument (3), got %v", st.Code())
	}
}

func TestControllerPublishVolume_InvalidVolumeIDFormat(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, _ := testControllerServer(t, node1)

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: "invalid-format; rm -rf /", // Injection attempt
		NodeId:   "node-1",
	}

	_, err := cs.ControllerPublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected INVALID_ARGUMENT error for invalid volume ID format")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected code InvalidArgument (3), got %v", st.Code())
	}
}

// ========================================
// ControllerUnpublishVolume Tests
// ========================================

func TestControllerUnpublishVolume_Success(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, mockRDS := testControllerServer(t, node1)

	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot: testVolumeID5,
	})

	// First publish
	am := cs.driver.GetAttachmentManager()
	_ = am.TrackAttachment(ctx, testVolumeID5, "node-1")

	// Unpublish
	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: testVolumeID5,
		NodeId:   "node-1",
	}

	_, err := cs.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("ControllerUnpublishVolume failed: %v", err)
	}

	// Verify attachment removed
	_, exists := am.GetAttachment(testVolumeID5)
	if exists {
		t.Error("Attachment should be removed after unpublish")
	}
}

func TestControllerUnpublishVolume_Idempotent(t *testing.T) {
	// CSI-03: Unpublish on non-attached volume = success
	ctx := context.Background()
	cs, _ := testControllerServer(t)

	// Don't attach first - just try to unpublish
	// Use a valid UUID format
	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: testVolumeID6,
		NodeId:   "node-1",
	}

	// Should succeed (idempotent)
	_, err := cs.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("Idempotent unpublish should succeed, got: %v", err)
	}
}

func TestControllerUnpublishVolume_InvalidVolumeID(t *testing.T) {
	ctx := context.Background()
	cs, _ := testControllerServer(t)

	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "", // Invalid
	}

	_, err := cs.ControllerUnpublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected INVALID_ARGUMENT error")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected code InvalidArgument (3), got %v", st.Code())
	}
}

func TestControllerUnpublishVolume_InvalidVolumeIDFormat(t *testing.T) {
	ctx := context.Background()
	cs, _ := testControllerServer(t)

	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "injection; drop table", // Injection attempt
		NodeId:   "node-1",
	}

	_, err := cs.ControllerUnpublishVolume(ctx, req)
	if err == nil {
		t.Fatal("Expected INVALID_ARGUMENT error for invalid volume ID format")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected code InvalidArgument (3), got %v", st.Code())
	}
}

func TestControllerUnpublishVolume_EmptyNodeID(t *testing.T) {
	// CSI spec allows empty nodeID for force-detach scenarios
	ctx := context.Background()
	cs, _ := testControllerServer(t)

	// Track an attachment first
	am := cs.driver.GetAttachmentManager()
	_ = am.TrackAttachment(ctx, testVolumeID7, "some-node")

	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: testVolumeID7,
		NodeId:   "", // Empty - force detach
	}

	// Should succeed - empty nodeID is allowed per CSI spec
	_, err := cs.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("Unpublish with empty nodeID should succeed, got: %v", err)
	}
}

// ========================================
// RWX Capability Tests (Phase 08-03)
// ========================================

func TestValidateVolumeCapabilities_RWX(t *testing.T) {
	tests := []struct {
		name          string
		caps          []*csi.VolumeCapability
		expectError   bool
		errorContains string
	}{
		{
			name: "RWX block - should succeed",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
				},
			},
			expectError: false,
		},
		{
			name: "RWX filesystem - should fail with actionable error",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			},
			expectError:   true,
			errorContains: "volumeMode: Block",
		},
		{
			name: "RWO block - should succeed",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
				},
			},
			expectError: false,
		},
		{
			name: "RWO filesystem - should succeed",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create controller with driver that has RWX capability
			cs, _ := testControllerServer(t)

			err := cs.validateVolumeCapabilities(tc.caps)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCreateVolume_RWXFilesystemRejected(t *testing.T) {
	cs, _ := testControllerServer(t)

	req := &csi.CreateVolumeRequest{
		Name: "test-vol",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
				},
			},
		},
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1 * 1024 * 1024 * 1024,
		},
	}

	_, err := cs.CreateVolume(context.Background(), req)

	if err == nil {
		t.Fatal("expected error for RWX filesystem, got nil")
	}

	// Check it's an InvalidArgument error
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
	if !strings.Contains(st.Message(), "volumeMode: Block") {
		t.Errorf("expected error to mention volumeMode: Block, got %q", st.Message())
	}
}

func TestDriverVolumeCapabilities_IncludesRWX(t *testing.T) {
	cs, _ := testControllerServer(t)
	driver := cs.driver

	found := false
	for _, vcap := range driver.vcaps {
		if vcap.GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected MULTI_NODE_MULTI_WRITER in vcaps, not found")
	}
}

func TestControllerPublishVolume_RWXDualAttach(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	node2 := testNode("node-2")
	node3 := testNode("node-3")
	cs, mockRDS := testControllerServer(t, node1, node2, node3)

	volumeID := testVolumeID1
	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        volumeID,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + volumeID,
		NVMETCPPort: 4420,
	})

	rwxCap := &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}

	// First attach should succeed
	req1 := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-1",
		VolumeCapability: rwxCap,
	}
	_, err := cs.ControllerPublishVolume(ctx, req1)
	if err != nil {
		t.Fatalf("first attach failed: %v", err)
	}

	// Second attach (migration target) should succeed
	req2 := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-2",
		VolumeCapability: rwxCap,
	}
	_, err = cs.ControllerPublishVolume(ctx, req2)
	if err != nil {
		t.Fatalf("second attach failed: %v", err)
	}

	// Third attach should fail with migration limit
	req3 := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-3",
		VolumeCapability: rwxCap,
	}
	_, err = cs.ControllerPublishVolume(ctx, req3)
	if err == nil {
		t.Fatal("expected error for 3rd attach, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", st.Code())
	}
	if !strings.Contains(st.Message(), "migration limit") {
		t.Errorf("expected 'migration limit' in error, got %q", st.Message())
	}
}

func TestControllerPublishVolume_RWOConflictHintsRWX(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	node2 := testNode("node-2")
	cs, mockRDS := testControllerServer(t, node1, node2)

	volumeID := testVolumeID2
	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        volumeID,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + volumeID,
		NVMETCPPort: 4420,
	})

	rwoCap := &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}

	// First attach should succeed
	req1 := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-1",
		VolumeCapability: rwoCap,
	}
	_, err := cs.ControllerPublishVolume(ctx, req1)
	if err != nil {
		t.Fatalf("first attach failed: %v", err)
	}

	// Second attach should fail with RWX hint
	req2 := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-2",
		VolumeCapability: rwoCap,
	}
	_, err = cs.ControllerPublishVolume(ctx, req2)
	if err == nil {
		t.Fatal("expected error for RWO conflict, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", st.Code())
	}
	if !strings.Contains(st.Message(), "RWX") {
		t.Errorf("expected RWX hint in error message, got %q", st.Message())
	}
}

func TestControllerPublishVolume_RWXIdempotent(t *testing.T) {
	ctx := context.Background()
	node1 := testNode("node-1")
	cs, mockRDS := testControllerServer(t, node1)

	volumeID := testVolumeID3
	mockRDS.AddVolume(&rds.VolumeInfo{
		Slot:        volumeID,
		NVMETCPNQN:  "nqn.2000-02.com.mikrotik:" + volumeID,
		NVMETCPPort: 4420,
	})

	rwxCap := &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}

	req := &csi.ControllerPublishVolumeRequest{
		VolumeId:         volumeID,
		NodeId:           "node-1",
		VolumeCapability: rwxCap,
	}

	// First call
	_, err := cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("first attach failed: %v", err)
	}

	// Second call (idempotent) - should succeed
	_, err = cs.ControllerPublishVolume(ctx, req)
	if err != nil {
		t.Fatalf("idempotent attach failed: %v", err)
	}
}
