// Package observability provides Prometheus metrics for the RDS CSI driver.
package observability

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}
	if m.registry == nil {
		t.Error("registry is nil")
	}
}

func TestHandler(t *testing.T) {
	m := NewMetrics()
	handler := m.Handler()
	if handler == nil {
		t.Fatal("Handler returned nil")
	}

	// Test that handler serves metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "rds_csi_") {
		t.Error("metrics response should contain rds_csi_ namespace")
	}
}

func TestRecordVolumeOp(t *testing.T) {
	m := NewMetrics()

	// Test success
	m.RecordVolumeOp("stage", nil, 100*time.Millisecond)

	// Test failure
	m.RecordVolumeOp("stage", errors.New("test error"), 50*time.Millisecond)

	// Verify via handler output
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_volume_operations_total") {
		t.Error("expected volume_operations_total metric")
	}
	if !strings.Contains(body, "rds_csi_volume_operation_duration_seconds") {
		t.Error("expected volume_operation_duration_seconds metric")
	}
}

func TestRecordVolumeOp_AllOperations(t *testing.T) {
	operations := []string{"create", "delete", "stage", "unstage", "publish", "unpublish"}

	for _, op := range operations {
		t.Run(op, func(t *testing.T) {
			m := NewMetrics()

			// Record success and failure for each operation
			m.RecordVolumeOp(op, nil, 100*time.Millisecond)
			m.RecordVolumeOp(op, errors.New("test error"), 50*time.Millisecond)

			handler := m.Handler()
			req := httptest.NewRequest("GET", "/metrics", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			body := rec.Body.String()

			// Check for operation label
			if !strings.Contains(body, `operation="`+op+`"`) {
				t.Errorf("expected operation label %q in metrics", op)
			}

			// Check both status labels
			if !strings.Contains(body, `status="success"`) {
				t.Error("expected status=success label in metrics")
			}
			if !strings.Contains(body, `status="failure"`) {
				t.Error("expected status=failure label in metrics")
			}
		})
	}
}

func TestRecordNVMeConnect(t *testing.T) {
	m := NewMetrics()

	// Test success
	m.RecordNVMeConnect(nil, 500*time.Millisecond)

	// Test failure
	m.RecordNVMeConnect(errors.New("connection failed"), 0)

	// Verify active connections gauge increased on success
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_nvme_connects_total") {
		t.Error("expected nvme_connects_total metric")
	}
	if !strings.Contains(body, "rds_csi_nvme_connections_active") {
		t.Error("expected nvme_connections_active metric")
	}
	if !strings.Contains(body, "rds_csi_nvme_connect_duration_seconds") {
		t.Error("expected nvme_connect_duration_seconds metric")
	}
}

func TestRecordNVMeConnect_ActiveConnectionsGauge(t *testing.T) {
	m := NewMetrics()

	// Connect successfully 3 times
	m.RecordNVMeConnect(nil, 100*time.Millisecond)
	m.RecordNVMeConnect(nil, 100*time.Millisecond)
	m.RecordNVMeConnect(nil, 100*time.Millisecond)

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_nvme_connections_active 3") {
		t.Errorf("expected nvme_connections_active to be 3, got:\n%s", body)
	}
}

func TestRecordNVMeDisconnect(t *testing.T) {
	m := NewMetrics()

	// Connect first
	m.RecordNVMeConnect(nil, 100*time.Millisecond)

	// Disconnect
	m.RecordNVMeDisconnect()

	// Active connections should be 0
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_nvme_connections_active 0") {
		t.Errorf("expected nvme_connections_active to be 0, got:\n%s", body)
	}
}

func TestRecordNVMeDisconnect_MultipleConnections(t *testing.T) {
	m := NewMetrics()

	// Connect 3 times
	m.RecordNVMeConnect(nil, 100*time.Millisecond)
	m.RecordNVMeConnect(nil, 100*time.Millisecond)
	m.RecordNVMeConnect(nil, 100*time.Millisecond)

	// Disconnect once
	m.RecordNVMeDisconnect()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_nvme_connections_active 2") {
		t.Errorf("expected nvme_connections_active to be 2, got:\n%s", body)
	}
}

func TestRecordMountOp(t *testing.T) {
	m := NewMetrics()

	m.RecordMountOp("mount", nil)
	m.RecordMountOp("unmount", errors.New("unmount failed"))

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_mount_operations_total") {
		t.Error("expected mount_operations_total metric")
	}
}

func TestRecordMountOp_BothOperations(t *testing.T) {
	m := NewMetrics()

	// Record mount success
	m.RecordMountOp("mount", nil)

	// Record unmount failure
	m.RecordMountOp("unmount", errors.New("device busy"))

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Check for operation labels
	if !strings.Contains(body, `operation="mount"`) {
		t.Error("expected operation=mount label")
	}
	if !strings.Contains(body, `operation="unmount"`) {
		t.Error("expected operation=unmount label")
	}
}

func TestRecordStaleMountDetected(t *testing.T) {
	m := NewMetrics()

	m.RecordStaleMountDetected()
	m.RecordStaleMountDetected()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_stale_mounts_detected_total 2") {
		t.Errorf("expected stale_mounts_detected_total to be 2, got:\n%s", body)
	}
}

func TestRecordStaleRecovery(t *testing.T) {
	m := NewMetrics()

	m.RecordStaleRecovery(nil)
	m.RecordStaleRecovery(errors.New("recovery failed"))

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_stale_recoveries_total") {
		t.Error("expected stale_recoveries_total metric")
	}
}

func TestRecordStaleRecovery_StatusLabels(t *testing.T) {
	m := NewMetrics()

	// Record success
	m.RecordStaleRecovery(nil)

	// Record failure
	m.RecordStaleRecovery(errors.New("recovery failed"))

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Check both status labels exist
	if !strings.Contains(body, `status="success"`) {
		t.Error("expected status=success label")
	}
	if !strings.Contains(body, `status="failure"`) {
		t.Error("expected status=failure label")
	}
}

func TestRecordOrphanCleaned(t *testing.T) {
	m := NewMetrics()

	m.RecordOrphanCleaned()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_orphans_cleaned_total 1") {
		t.Errorf("expected orphans_cleaned_total to be 1, got:\n%s", body)
	}
}

func TestRecordOrphanCleaned_Multiple(t *testing.T) {
	m := NewMetrics()

	m.RecordOrphanCleaned()
	m.RecordOrphanCleaned()
	m.RecordOrphanCleaned()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_orphans_cleaned_total 3") {
		t.Errorf("expected orphans_cleaned_total to be 3, got:\n%s", body)
	}
}

func TestRecordEventPosted(t *testing.T) {
	m := NewMetrics()

	m.RecordEventPosted("MountFailure")
	m.RecordEventPosted("MountFailure")
	m.RecordEventPosted("ConnectionFailure")

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "rds_csi_events_posted_total") {
		t.Error("expected events_posted_total metric")
	}
}

func TestRecordEventPosted_ReasonLabels(t *testing.T) {
	m := NewMetrics()

	// Record various event reasons
	m.RecordEventPosted("MountFailure")
	m.RecordEventPosted("RecoveryFailed")
	m.RecordEventPosted("StaleMountDetected")

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Check for reason labels
	if !strings.Contains(body, `reason="MountFailure"`) {
		t.Error("expected reason=MountFailure label")
	}
	if !strings.Contains(body, `reason="RecoveryFailed"`) {
		t.Error("expected reason=RecoveryFailed label")
	}
	if !strings.Contains(body, `reason="StaleMountDetected"`) {
		t.Error("expected reason=StaleMountDetected label")
	}
}

func TestMetricsNamespace(t *testing.T) {
	m := NewMetrics()

	// Record something to populate metrics
	m.RecordStaleMountDetected()
	m.RecordVolumeOp("stage", nil, 100*time.Millisecond)
	m.RecordNVMeConnect(nil, 100*time.Millisecond)
	m.RecordMountOp("mount", nil)
	m.RecordOrphanCleaned()
	m.RecordEventPosted("TestReason")

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// All RDS CSI metrics should use rds_csi_ namespace
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// Skip standard Go runtime metrics (these are from the custom registry)
		if strings.HasPrefix(line, "go_") || strings.HasPrefix(line, "process_") {
			continue
		}
		if !strings.HasPrefix(line, "rds_csi_") {
			t.Errorf("metric line should start with rds_csi_: %s", line)
		}
	}
}

func TestMetricsIsolation(t *testing.T) {
	// Create two separate Metrics instances to verify isolation
	m1 := NewMetrics()
	m2 := NewMetrics()

	// Record metrics only on m1
	m1.RecordStaleMountDetected()
	m1.RecordStaleMountDetected()

	// Record different metric on m2
	m2.RecordOrphanCleaned()

	// Verify m1 has stale mounts
	handler1 := m1.Handler()
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	rec1 := httptest.NewRecorder()
	handler1.ServeHTTP(rec1, req1)
	body1 := rec1.Body.String()

	if !strings.Contains(body1, "rds_csi_stale_mounts_detected_total 2") {
		t.Error("m1 should have stale_mounts_detected_total 2")
	}

	// Verify m2 has orphans cleaned
	handler2 := m2.Handler()
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	body2 := rec2.Body.String()

	if !strings.Contains(body2, "rds_csi_orphans_cleaned_total 1") {
		t.Error("m2 should have orphans_cleaned_total 1")
	}

	// m2 should not have stale mounts count from m1
	// The metric line might not exist at all or be 0
	if strings.Contains(body2, "rds_csi_stale_mounts_detected_total 2") {
		t.Error("m2 should not have m1's stale mounts count")
	}
}

func TestCustomRegistryDoesNotPanic(t *testing.T) {
	// Verify that creating multiple Metrics instances doesn't cause
	// duplicate registration panics (since we use custom registry)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Creating multiple Metrics instances caused panic: %v", r)
		}
	}()

	for i := 0; i < 10; i++ {
		m := NewMetrics()
		m.RecordVolumeOp("stage", nil, 100*time.Millisecond)
	}
}

func TestHistogramBuckets(t *testing.T) {
	m := NewMetrics()

	// Record various durations to populate histogram buckets
	durations := []time.Duration{
		50 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		3 * time.Second,
		10 * time.Second,
	}

	for _, d := range durations {
		m.RecordVolumeOp("stage", nil, d)
	}

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Check that histogram has bucket data
	if !strings.Contains(body, "rds_csi_volume_operation_duration_seconds_bucket") {
		t.Error("expected histogram bucket data")
	}
	if !strings.Contains(body, "rds_csi_volume_operation_duration_seconds_sum") {
		t.Error("expected histogram sum")
	}
	if !strings.Contains(body, "rds_csi_volume_operation_duration_seconds_count") {
		t.Error("expected histogram count")
	}
}
