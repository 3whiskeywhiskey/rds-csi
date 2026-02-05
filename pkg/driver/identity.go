package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/klog/v2"
)

// IdentityServer implements the CSI Identity service
type IdentityServer struct {
	csi.UnimplementedIdentityServer
	driver *Driver
}

// NewIdentityServer creates a new Identity service
func NewIdentityServer(driver *Driver) *IdentityServer {
	return &IdentityServer{
		driver: driver,
	}
}

// GetPluginInfo returns metadata about the plugin
func (ids *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	klog.V(5).Info("GetPluginInfo called")

	if ids.driver.name == "" {
		return nil, status.Error(codes.Unavailable, "driver name not configured")
	}

	return &csi.GetPluginInfoResponse{
		Name:          ids.driver.name,
		VendorVersion: ids.driver.version,
	}, nil
}

// GetPluginCapabilities returns the capabilities of the plugin
func (ids *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	klog.V(5).Info("GetPluginCapabilities called")

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
		},
	}, nil
}

// Probe returns the health and readiness of the plugin
func (ids *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	klog.V(5).Info("Probe called")

	ready := true

	// Check RDS connection state (prefer connectionManager if available)
	if ids.driver.connectionManager != nil {
		if !ids.driver.connectionManager.IsConnected() {
			klog.Warning("RDS client is not connected - reporting not ready")
			ready = false
		}
	} else if ids.driver.rdsClient != nil {
		// Fallback to direct client check
		if !ids.driver.rdsClient.IsConnected() {
			klog.Warning("RDS client is not connected - reporting not ready")
			ready = false
		}
	}

	// Record connection state metric
	if ids.driver.metrics != nil && ids.driver.rdsClient != nil {
		ids.driver.metrics.RecordConnectionState(ids.driver.rdsClient.GetAddress(), ready)
	}

	return &csi.ProbeResponse{
		Ready: wrapperspb.Bool(ready),
	}, nil
}
