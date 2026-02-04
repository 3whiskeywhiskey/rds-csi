package circuitbreaker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVolumeCircuitBreaker_Success(t *testing.T) {
	vcb := NewVolumeCircuitBreaker()
	ctx := context.Background()

	err := vcb.Execute(ctx, "vol-1", func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected nil error for success, got: %v", err)
	}
}

func TestVolumeCircuitBreaker_OpensAfterFailures(t *testing.T) {
	vcb := NewVolumeCircuitBreaker()
	ctx := context.Background()
	testErr := errors.New("test failure")

	// Fail 3 times to open circuit
	for i := 0; i < DefaultConsecutiveFailures; i++ {
		err := vcb.Execute(ctx, "vol-fail", func() error {
			return testErr
		})
		if err != testErr {
			t.Errorf("Iteration %d: expected test error, got: %v", i, err)
		}
	}

	// Next call should get circuit open error
	err := vcb.Execute(ctx, "vol-fail", func() error {
		return nil
	})

	if err == nil {
		t.Error("Expected error when circuit is open")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}

	if st.Code() != codes.Unavailable {
		t.Errorf("Expected Unavailable code, got: %v", st.Code())
	}

	if !strings.Contains(st.Message(), "OPEN") {
		t.Errorf("Error message should mention OPEN state: %s", st.Message())
	}
}

func TestVolumeCircuitBreaker_Reset(t *testing.T) {
	vcb := NewVolumeCircuitBreaker()
	ctx := context.Background()
	testErr := errors.New("test failure")

	// Open the circuit
	for i := 0; i < DefaultConsecutiveFailures; i++ {
		_ = vcb.Execute(ctx, "vol-reset", func() error {
			return testErr
		})
	}

	// Verify circuit is open
	if vcb.State("vol-reset") != "open" {
		t.Errorf("Expected open state, got: %s", vcb.State("vol-reset"))
	}

	// Reset via annotation
	annotations := map[string]string{ResetAnnotation: "true"}
	if !vcb.CheckReset("vol-reset", annotations) {
		t.Error("Expected reset to return true")
	}

	// Verify circuit is gone (defaults to closed)
	if vcb.State("vol-reset") != "closed" {
		t.Errorf("Expected closed state after reset, got: %s", vcb.State("vol-reset"))
	}
}

func TestVolumeCircuitBreaker_IsolatedVolumes(t *testing.T) {
	vcb := NewVolumeCircuitBreaker()
	ctx := context.Background()
	testErr := errors.New("test failure")

	// Fail vol-a
	for i := 0; i < DefaultConsecutiveFailures; i++ {
		_ = vcb.Execute(ctx, "vol-a", func() error {
			return testErr
		})
	}

	// vol-b should still work
	err := vcb.Execute(ctx, "vol-b", func() error {
		return nil
	})

	if err != nil {
		t.Errorf("vol-b should not be affected by vol-a failures: %v", err)
	}
}
