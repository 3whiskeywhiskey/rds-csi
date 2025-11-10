package utils

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldMatch []string   // Substrings that should be in output
		shouldntMatch []string // Substrings that should NOT be in output
	}{
		{
			name:          "IPv4 address sanitization",
			input:         "Failed to connect to 192.168.1.100:22",
			shouldMatch:   []string{"Failed to connect", "[IP-ADDRESS]"},
			shouldntMatch: []string{"192.168.1.100"},
		},
		{
			name:          "Multiple IPv4 addresses",
			input:         "Connection from 10.0.0.1 to 172.16.0.50 failed",
			shouldMatch:   []string{"Connection", "[IP-ADDRESS]", "failed"},
			shouldntMatch: []string{"10.0.0.1", "172.16.0.50"},
		},
		{
			name:          "IPv6 address sanitization",
			input:         "Failed to connect to 2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			shouldMatch:   []string{"Failed to connect", "[IP-ADDRESS]"},
			shouldntMatch: []string{"2001:0db8"},
		},
		{
			name:          "Unix absolute path sanitization",
			input:         "Failed to read /home/user/.ssh/id_rsa",
			shouldMatch:   []string{"Failed to read", "[PATH]/id_rsa"},
			shouldntMatch: []string{"/home/user/.ssh"},
		},
		{
			name:          "Windows absolute path sanitization",
			input:         "Cannot access C:\\Users\\Admin\\Documents\\secret.txt",
			shouldMatch:   []string{"Cannot access", "[PATH]", "secret.txt"},
			shouldntMatch: []string{"C:\\Users\\Admin"},
		},
		{
			name:          "Keep /dev/ paths",
			input:         "Device /dev/nvme0n1 not found",
			shouldMatch:   []string{"Device", "/dev/nvme0n1", "not found"},
			shouldntMatch: []string{},
		},
		{
			name:          "Keep /sys/ paths",
			input:         "Error reading /sys/class/nvme/nvme0/address",
			shouldMatch:   []string{"Error reading", "/sys/class/nvme/nvme0/address"},
			shouldntMatch: []string{},
		},
		{
			name:          "SSH fingerprint sanitization",
			input:         "Host key mismatch: expected SHA256:abc123def456, got SHA256:xyz789uvw",
			shouldMatch:   []string{"Host key mismatch", "expected", "[FINGERPRINT]", "got", "[FINGERPRINT]"},
			shouldntMatch: []string{"SHA256:abc123def456", "SHA256:xyz789uvw"},
		},
		{
			name:          "Hostname sanitization",
			input:         "Failed to resolve server.example.com",
			shouldMatch:   []string{"Failed to resolve", "[HOSTNAME]"},
			shouldntMatch: []string{"server.example.com"},
		},
		{
			name:          "Multiple hostnames",
			input:         "DNS lookup failed for db.internal.lan and cache.prod.io",
			shouldMatch:   []string{"DNS lookup failed", "[HOSTNAME]"},
			shouldntMatch: []string{"db.internal.lan", "cache.prod.io"},
		},
		{
			name:          "Mixed sensitive data",
			input:         "SSH connection to admin@192.168.1.1:22 at /home/admin/.ssh failed with SHA256:abc123",
			shouldMatch:   []string{"SSH connection", "[IP-ADDRESS]", "[PATH]/.ssh", "[FINGERPRINT]", "failed"},
			shouldntMatch: []string{"192.168.1.1", "/home/admin", "SHA256:abc123"},
		},
		{
			name:          "No sensitive data",
			input:         "Volume creation failed: insufficient storage space",
			shouldMatch:   []string{"Volume creation failed", "insufficient storage space"},
			shouldntMatch: []string{},
		},
		{
			name:          "Port numbers preserved",
			input:         "Connection to [IP-ADDRESS]:4420 refused",
			shouldMatch:   []string{"Connection", "4420", "refused"},
			shouldntMatch: []string{},
		},
		{
			name:          "Stack trace removal",
			input:         "Error occurred\n  at function1 (file.go:10)\n  at function2 (file.go:20)\ngoroutine 1 [running]",
			shouldMatch:   []string{"Error occurred"},
			shouldntMatch: []string{"at function1", "goroutine"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeErrorMessage(tt.input)

			// Check expected matches
			for _, match := range tt.shouldMatch {
				if !strings.Contains(result, match) {
					t.Errorf("Expected result to contain %q, got: %s", match, result)
				}
			}

			// Check unexpected matches
			for _, noMatch := range tt.shouldntMatch {
				if strings.Contains(result, noMatch) {
					t.Errorf("Expected result NOT to contain %q, got: %s", noMatch, result)
				}
			}
		})
	}
}

func TestNewInternalError(t *testing.T) {
	originalErr := errors.New("database connection failed at 10.0.0.1:5432")
	userMsg := "Internal error occurred"

	err := NewInternalError(originalErr, userMsg)

	if err.Error() != userMsg {
		t.Errorf("Expected sanitized message %q, got %q", userMsg, err.Error())
	}

	if err.GetOriginal() != originalErr {
		t.Error("Original error not preserved")
	}

	if err.errorType != ErrorTypeInternal {
		t.Errorf("Expected ErrorTypeInternal, got %v", err.errorType)
	}
}

func TestNewUserError(t *testing.T) {
	originalErr := errors.New("failed to connect to 192.168.1.1")
	operation := "Volume creation"

	err := NewUserError(originalErr, operation)

	// Should contain operation but not IP
	if !strings.Contains(err.Error(), operation) {
		t.Errorf("Expected error to contain operation %q, got: %s", operation, err.Error())
	}

	if strings.Contains(err.Error(), "192.168.1.1") {
		t.Errorf("Expected error NOT to contain IP address, got: %s", err.Error())
	}

	if err.errorType != ErrorTypeUser {
		t.Errorf("Expected ErrorTypeUser, got %v", err.errorType)
	}

	context := err.GetInternalContext()
	if context["operation"] != operation {
		t.Errorf("Expected internal context to contain operation %q", operation)
	}
}

func TestNewValidationError(t *testing.T) {
	field := "volumeID"
	reason := "must match pattern ^pvc-[a-f0-9-]+$"

	err := NewValidationError(field, reason)

	if !strings.Contains(err.Error(), field) {
		t.Errorf("Expected error to contain field %q, got: %s", field, err.Error())
	}

	if !strings.Contains(err.Error(), reason) {
		t.Errorf("Expected error to contain reason %q, got: %s", reason, err.Error())
	}

	if err.errorType != ErrorTypeValidation {
		t.Errorf("Expected ErrorTypeValidation, got %v", err.errorType)
	}
}

func TestSanitizedErrorWithContext(t *testing.T) {
	err := NewInternalError(errors.New("test"), "error occurred")
	err = err.WithContext("volumeID", "pvc-123")
	err = err.WithContext("nodeID", "node-1")

	context := err.GetInternalContext()
	if context["volumeID"] != "pvc-123" {
		t.Error("Expected volumeID in context")
	}
	if context["nodeID"] != "node-1" {
		t.Error("Expected nodeID in context")
	}
}

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name          string
		input         error
		shouldContain string
		shouldntContain string
	}{
		{
			name:            "Nil error",
			input:           nil,
			shouldContain:   "",
			shouldntContain: "",
		},
		{
			name:            "Already sanitized",
			input:           NewUserError(errors.New("test"), "operation"),
			shouldContain:   "operation",
			shouldntContain: "",
		},
		{
			name:            "Unsanitized error with IP",
			input:           errors.New("connection failed to 10.0.0.1"),
			shouldContain:   "[IP-ADDRESS]",
			shouldntContain: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeError(tt.input)

			if tt.input == nil {
				if result != nil {
					t.Error("Expected nil result for nil input")
				}
				return
			}

			resultStr := result.Error()

			if tt.shouldContain != "" && !strings.Contains(resultStr, tt.shouldContain) {
				t.Errorf("Expected result to contain %q, got: %s", tt.shouldContain, resultStr)
			}

			if tt.shouldntContain != "" && strings.Contains(resultStr, tt.shouldntContain) {
				t.Errorf("Expected result NOT to contain %q, got: %s", tt.shouldntContain, resultStr)
			}
		})
	}
}

func TestSanitizeErrorf(t *testing.T) {
	err := SanitizeErrorf("Failed to connect to %s:%d", "192.168.1.1", 22)

	if !strings.Contains(err.Error(), "[IP-ADDRESS]") {
		t.Errorf("Expected sanitized error, got: %s", err.Error())
	}

	if strings.Contains(err.Error(), "192.168.1.1") {
		t.Errorf("Expected IP to be sanitized, got: %s", err.Error())
	}
}

func TestWrapError(t *testing.T) {
	originalErr := errors.New("connection refused")
	wrapped := WrapError(originalErr, "failed to connect to %s", "192.168.1.1")

	if wrapped == nil {
		t.Fatal("Expected wrapped error, got nil")
	}

	// Should contain context message (sanitized)
	if !strings.Contains(wrapped.Error(), "failed to connect") {
		t.Errorf("Expected wrapped error to contain context, got: %s", wrapped.Error())
	}

	// Should NOT contain IP address
	if strings.Contains(wrapped.Error(), "192.168.1.1") {
		t.Errorf("Expected IP to be sanitized, got: %s", wrapped.Error())
	}

	// Should be able to unwrap to original
	if !errors.Is(wrapped, originalErr) {
		t.Error("Expected error chain to contain original error")
	}
}

func TestWrapErrorNil(t *testing.T) {
	wrapped := WrapError(nil, "context")

	if wrapped != nil {
		t.Error("Expected nil when wrapping nil error")
	}
}

func TestErrorTypeChecks(t *testing.T) {
	internalErr := NewInternalError(errors.New("test"), "internal")
	userErr := NewUserError(errors.New("test"), "operation")
	validationErr := NewValidationError("field", "reason")
	regularErr := errors.New("regular error")

	// Test IsInternalError
	if !IsInternalError(internalErr) {
		t.Error("Expected IsInternalError to return true for internal error")
	}
	if IsInternalError(userErr) {
		t.Error("Expected IsInternalError to return false for user error")
	}
	if IsInternalError(regularErr) {
		t.Error("Expected IsInternalError to return false for regular error")
	}

	// Test IsUserError
	if !IsUserError(userErr) {
		t.Error("Expected IsUserError to return true for user error")
	}
	if IsUserError(internalErr) {
		t.Error("Expected IsUserError to return false for internal error")
	}
	if IsUserError(regularErr) {
		t.Error("Expected IsUserError to return false for regular error")
	}

	// Test IsValidationError
	if !IsValidationError(validationErr) {
		t.Error("Expected IsValidationError to return true for validation error")
	}
	if IsValidationError(internalErr) {
		t.Error("Expected IsValidationError to return false for internal error")
	}
	if IsValidationError(regularErr) {
		t.Error("Expected IsValidationError to return false for regular error")
	}
}

func TestGetSanitizedMessage(t *testing.T) {
	tests := []struct {
		name          string
		input         error
		shouldContain string
		shouldntContain string
	}{
		{
			name:          "Nil error",
			input:         nil,
			shouldContain: "",
		},
		{
			name:          "Sanitized error",
			input:         NewUserError(errors.New("test at 10.0.0.1"), "op"),
			shouldContain: "[IP-ADDRESS]",
			shouldntContain: "10.0.0.1",
		},
		{
			name:          "Regular error with IP",
			input:         errors.New("connection to 192.168.1.1 failed"),
			shouldContain: "[IP-ADDRESS]",
			shouldntContain: "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSanitizedMessage(tt.input)

			if tt.input == nil {
				if result != "" {
					t.Error("Expected empty string for nil error")
				}
				return
			}

			if tt.shouldContain != "" && !strings.Contains(result, tt.shouldContain) {
				t.Errorf("Expected result to contain %q, got: %s", tt.shouldContain, result)
			}

			if tt.shouldntContain != "" && strings.Contains(result, tt.shouldntContain) {
				t.Errorf("Expected result NOT to contain %q, got: %s", tt.shouldntContain, result)
			}
		})
	}
}

func TestSanitizePathsComplexCases(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{
			name:  "Multiple paths in message",
			input: "Failed to copy /home/user/file.txt to /var/lib/data/output.txt",
			want:  "Failed to copy [PATH]/file.txt to [PATH]/output.txt",
		},
		{
			name:  "Mixed device and user paths",
			input: "Mount /dev/nvme0n1 to /mnt/volumes/vol1 failed",
			want:  "Mount /dev/nvme0n1 to [PATH]/vol1 failed",
		},
		{
			name:  "Relative path preserved",
			input:  "Cannot find ./config.yaml in current directory",
			want:   "Cannot find ./config.yaml in current directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeErrorMessage(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorUnwrapping(t *testing.T) {
	baseErr := errors.New("base error")
	wrappedErr := fmt.Errorf("wrapped: %w", baseErr)
	sanitizedErr := NewUserError(wrappedErr, "operation")

	// Should be able to unwrap to base error
	if !errors.Is(sanitizedErr, baseErr) {
		t.Error("Expected to unwrap to base error")
	}

	// Unwrap should return the original wrapped error
	unwrapped := errors.Unwrap(sanitizedErr)
	if unwrapped != wrappedErr {
		t.Error("Expected Unwrap to return original wrapped error")
	}
}

func TestComplexSanitization(t *testing.T) {
	// Simulate a real error message with multiple sensitive components
	input := `Failed to establish SSH connection to admin@192.168.1.100:22
  Private key: /home/user/.ssh/id_rsa
  Known hosts: /home/user/.ssh/known_hosts
  Server fingerprint: SHA256:abcd1234efgh5678
  Resolved hostname: server.example.com (10.0.0.50)
  Error: connection timeout after 30s
    at ssh.Dial (ssh.go:123)
    at main.connect (main.go:456)
  goroutine 1 [running]`

	result := SanitizeErrorMessage(input)

	// Should NOT contain sensitive data
	sensitiveData := []string{
		"192.168.1.100",
		"10.0.0.50",
		"/home/user",
		"SHA256:abcd1234efgh5678",
		"server.example.com",
		"at ssh.Dial",
		"goroutine",
	}

	for _, sensitive := range sensitiveData {
		if strings.Contains(result, sensitive) {
			t.Errorf("Result should NOT contain %q, but got: %s", sensitive, result)
		}
	}

	// Should contain safe information
	safeData := []string{
		"Failed to establish SSH connection",
		"[IP-ADDRESS]",
		"[PATH]/id_rsa",
		"[PATH]/known_hosts",
		"[FINGERPRINT]",
		"[HOSTNAME]",
		"connection timeout",
	}

	for _, safe := range safeData {
		if !strings.Contains(result, safe) {
			t.Errorf("Result should contain %q, but got: %s", safe, result)
		}
	}
}
