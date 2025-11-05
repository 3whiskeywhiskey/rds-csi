package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestGetPluginInfo(t *testing.T) {
	driver := &Driver{
		name:    "test.csi.driver",
		version: "v1.0.0",
	}

	ids := NewIdentityServer(driver)

	req := &csi.GetPluginInfoRequest{}
	resp, err := ids.GetPluginInfo(context.Background(), req)

	if err != nil {
		t.Fatalf("GetPluginInfo failed: %v", err)
	}

	if resp.Name != "test.csi.driver" {
		t.Errorf("Expected name test.csi.driver, got %s", resp.Name)
	}

	if resp.VendorVersion != "v1.0.0" {
		t.Errorf("Expected version v1.0.0, got %s", resp.VendorVersion)
	}
}

func TestGetPluginInfoNoName(t *testing.T) {
	driver := &Driver{
		name:    "",
		version: "v1.0.0",
	}

	ids := NewIdentityServer(driver)

	req := &csi.GetPluginInfoRequest{}
	_, err := ids.GetPluginInfo(context.Background(), req)

	if err == nil {
		t.Error("Expected error when driver name is empty, got nil")
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	driver := &Driver{
		name:    "test.csi.driver",
		version: "v1.0.0",
	}

	ids := NewIdentityServer(driver)

	req := &csi.GetPluginCapabilitiesRequest{}
	resp, err := ids.GetPluginCapabilities(context.Background(), req)

	if err != nil {
		t.Fatalf("GetPluginCapabilities failed: %v", err)
	}

	if len(resp.Capabilities) == 0 {
		t.Error("Expected capabilities but got none")
	}

	// Check that CONTROLLER_SERVICE capability is present
	hasControllerService := false
	for _, cap := range resp.Capabilities {
		if cap.GetService() != nil {
			if cap.GetService().Type == csi.PluginCapability_Service_CONTROLLER_SERVICE {
				hasControllerService = true
				break
			}
		}
	}

	if !hasControllerService {
		t.Error("Expected CONTROLLER_SERVICE capability but not found")
	}
}

func TestProbeHealthy(t *testing.T) {
	driver := &Driver{
		name:    "test.csi.driver",
		version: "v1.0.0",
		// No RDS client, should still be healthy
	}

	ids := NewIdentityServer(driver)

	req := &csi.ProbeRequest{}
	resp, err := ids.Probe(context.Background(), req)

	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	if resp.Ready == nil || !resp.Ready.GetValue() {
		t.Error("Expected driver to be ready")
	}
}
