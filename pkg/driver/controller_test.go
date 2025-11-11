package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	t.Run("ControllerPublishVolume", func(t *testing.T) {
		_, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{})
		if err == nil {
			t.Error("Expected unimplemented error")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.Unimplemented {
			t.Errorf("Expected Unimplemented code, got %v", st.Code())
		}
	})

	t.Run("ControllerUnpublishVolume", func(t *testing.T) {
		_, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{})
		if err == nil {
			t.Error("Expected unimplemented error")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.Unimplemented {
			t.Errorf("Expected Unimplemented code, got %v", st.Code())
		}
	})

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

	t.Run("ControllerExpandVolume", func(t *testing.T) {
		_, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{})
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
