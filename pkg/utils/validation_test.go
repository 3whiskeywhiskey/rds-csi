package utils

import (
	"testing"
)

// setupTestBasePaths configures allowed base paths for testing
func setupTestBasePaths(t *testing.T) {
	t.Helper()
	ResetAllowedBasePaths()
	// Add test paths that match our test cases
	if err := SetAllowedBasePath("/storage-pool/metal-csi"); err != nil {
		t.Fatalf("failed to set test base path: %v", err)
	}
	if err := AddAllowedBasePath("/storage-pool/kubernetes-volumes"); err != nil {
		t.Fatalf("failed to add test base path: %v", err)
	}
	t.Cleanup(ResetAllowedBasePaths)
}

func TestValidateFilePath(t *testing.T) {
	setupTestBasePaths(t)
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{
			name:    "valid absolute path",
			path:    "/storage-pool/kubernetes-volumes/pvc-123.img",
			wantErr: false,
		},
		{
			name:    "valid path with metal-csi",
			path:    "/storage-pool/metal-csi/volumes/pvc-456.img",
			wantErr: false,
		},
		{
			name:    "valid path with hyphen",
			path:    "/storage-pool/metal-csi/pvc-test-volume.img",
			wantErr: false,
		},
		// Empty path
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		// Relative paths
		{
			name:    "relative path",
			path:    "storage-pool/volumes/pvc-123.img",
			wantErr: true,
		},
		// Path traversal attempts
		{
			name:    "path traversal with ../",
			path:    "/storage-pool/kubernetes-volumes/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal in middle",
			path:    "/storage-pool/../volumes/pvc-123.img",
			wantErr: true,
		},
		{
			name:    "path with ./",
			path:    "/storage-pool/./volumes/pvc-123.img",
			wantErr: true,
		},
		// Shell metacharacters - command injection attempts
		{
			name:    "semicolon injection",
			path:    "/storage-pool/volumes/pvc-123.img; rm -rf /",
			wantErr: true,
		},
		{
			name:    "pipe injection",
			path:    "/storage-pool/volumes/pvc-123.img | cat /etc/passwd",
			wantErr: true,
		},
		{
			name:    "ampersand injection",
			path:    "/storage-pool/volumes/pvc-123.img && rm -rf /",
			wantErr: true,
		},
		{
			name:    "dollar sign injection",
			path:    "/storage-pool/volumes/pvc-$USER.img",
			wantErr: true,
		},
		{
			name:    "backtick injection",
			path:    "/storage-pool/volumes/pvc-`whoami`.img",
			wantErr: true,
		},
		{
			name:    "parenthesis injection",
			path:    "/storage-pool/volumes/pvc-(whoami).img",
			wantErr: true,
		},
		{
			name:    "redirect injection",
			path:    "/storage-pool/volumes/pvc-123.img > /tmp/evil",
			wantErr: true,
		},
		{
			name:    "newline injection",
			path:    "/storage-pool/volumes/pvc-123.img\nrm -rf /",
			wantErr: true,
		},
		{
			name:    "carriage return injection",
			path:    "/storage-pool/volumes/pvc-123.img\rrm -rf /",
			wantErr: true,
		},
		{
			name:    "wildcard injection",
			path:    "/storage-pool/volumes/*.img",
			wantErr: true,
		},
		{
			name:    "quote injection",
			path:    "/storage-pool/volumes/pvc-'test'.img",
			wantErr: true,
		},
		{
			name:    "double quote injection",
			path:    "/storage-pool/volumes/pvc-\"test\".img",
			wantErr: true,
		},
		{
			name:    "backslash injection",
			path:    "/storage-pool/volumes/pvc-test\\nrm.img",
			wantErr: true,
		},
		// Not in whitelist
		{
			name:    "path not in whitelist",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path in /tmp",
			path:    "/tmp/evil.img",
			wantErr: true,
		},
		// Double slashes
		{
			name:    "double slash",
			path:    "/storage-pool//volumes/pvc-123.img",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFilePathWithBase(t *testing.T) {
	setupTestBasePaths(t)
	tests := []struct {
		name     string
		path     string
		basePath string
		wantErr  bool
	}{
		{
			name:     "valid path within base",
			path:     "/storage-pool/metal-csi/volumes/pvc-123.img",
			basePath: "/storage-pool/metal-csi",
			wantErr:  false,
		},
		{
			name:     "path not within base",
			path:     "/storage-pool/other/pvc-123.img",
			basePath: "/storage-pool/metal-csi",
			wantErr:  true,
		},
		{
			name:     "empty base path",
			path:     "/storage-pool/metal-csi/pvc-123.img",
			basePath: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePathWithBase(tt.path, tt.basePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePathWithBase() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeBasePath(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		want     string
		wantErr  bool
	}{
		{
			name:     "valid absolute path",
			basePath: "/storage-pool/volumes",
			want:     "/storage-pool/volumes",
			wantErr:  false,
		},
		{
			name:     "path with trailing slash",
			basePath: "/storage-pool/volumes/",
			want:     "/storage-pool/volumes",
			wantErr:  false,
		},
		{
			name:     "empty path",
			basePath: "",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "relative path",
			basePath: "storage-pool/volumes",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "path with dangerous characters",
			basePath: "/storage-pool/volumes;rm -rf /",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "path with double slashes",
			basePath: "/storage-pool//volumes",
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeBasePath(tt.basePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizeBasePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SanitizeBasePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetAllowedBasePath(t *testing.T) {
	t.Cleanup(ResetAllowedBasePaths)

	// Test setting a valid path
	ResetAllowedBasePaths()
	err := SetAllowedBasePath("/storage-pool/test")
	if err != nil {
		t.Errorf("SetAllowedBasePath() unexpected error: %v", err)
	}
	if len(AllowedBasePaths) != 1 || AllowedBasePaths[0] != "/storage-pool/test" {
		t.Errorf("SetAllowedBasePath() did not set path correctly: %v", AllowedBasePaths)
	}

	// Test that it replaces existing paths
	err = SetAllowedBasePath("/storage-pool/new")
	if err != nil {
		t.Errorf("SetAllowedBasePath() unexpected error: %v", err)
	}
	if len(AllowedBasePaths) != 1 || AllowedBasePaths[0] != "/storage-pool/new" {
		t.Errorf("SetAllowedBasePath() did not replace path: %v", AllowedBasePaths)
	}

	// Test empty path
	err = SetAllowedBasePath("")
	if err == nil {
		t.Error("SetAllowedBasePath() should error on empty path")
	}

	// Test invalid path
	err = SetAllowedBasePath("/storage-pool/test; rm -rf /")
	if err == nil {
		t.Error("SetAllowedBasePath() should error on dangerous path")
	}
}

func TestAddAllowedBasePath(t *testing.T) {
	t.Cleanup(ResetAllowedBasePaths)

	ResetAllowedBasePaths()
	// Set initial path
	_ = SetAllowedBasePath("/storage-pool/base")

	// Add another path
	err := AddAllowedBasePath("/storage-pool/extra")
	if err != nil {
		t.Errorf("AddAllowedBasePath() unexpected error: %v", err)
	}
	if len(AllowedBasePaths) != 2 {
		t.Errorf("AddAllowedBasePath() did not add path: %v", AllowedBasePaths)
	}

	// Adding duplicate should be no-op
	err = AddAllowedBasePath("/storage-pool/extra")
	if err != nil {
		t.Errorf("AddAllowedBasePath() unexpected error on duplicate: %v", err)
	}
	if len(AllowedBasePaths) != 2 {
		t.Errorf("AddAllowedBasePath() should not duplicate: %v", AllowedBasePaths)
	}

	// Empty path should be no-op
	err = AddAllowedBasePath("")
	if err != nil {
		t.Errorf("AddAllowedBasePath() unexpected error on empty: %v", err)
	}
}

func TestValidateCreateVolumeOptions(t *testing.T) {
	setupTestBasePaths(t)
	tests := []struct {
		name      string
		filePath  string
		sizeBytes int64
		slot      string
		wantErr   bool
	}{
		{
			name:      "valid options",
			filePath:  "/storage-pool/metal-csi/pvc-123.img",
			sizeBytes: 1024 * 1024 * 1024, // 1GB
			slot:      "pvc-123",
			wantErr:   false,
		},
		{
			name:      "invalid file path",
			filePath:  "/etc/passwd",
			sizeBytes: 1024 * 1024 * 1024,
			slot:      "pvc-123",
			wantErr:   true,
		},
		{
			name:      "zero size",
			filePath:  "/storage-pool/metal-csi/pvc-123.img",
			sizeBytes: 0,
			slot:      "pvc-123",
			wantErr:   true,
		},
		{
			name:      "negative size",
			filePath:  "/storage-pool/metal-csi/pvc-123.img",
			sizeBytes: -1,
			slot:      "pvc-123",
			wantErr:   true,
		},
		{
			name:      "invalid slot name",
			filePath:  "/storage-pool/metal-csi/pvc-123.img",
			sizeBytes: 1024 * 1024 * 1024,
			slot:      "pvc-123; rm -rf /",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateVolumeOptions(tt.filePath, tt.sizeBytes, tt.slot)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateVolumeOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsPathSafe(t *testing.T) {
	setupTestBasePaths(t)
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "safe path",
			path: "/storage-pool/metal-csi/pvc-123.img",
			want: true,
		},
		{
			name: "unsafe path with injection",
			path: "/storage-pool/metal-csi/pvc-123.img; rm -rf /",
			want: false,
		},
		{
			name: "unsafe path traversal",
			path: "/storage-pool/../etc/passwd",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPathSafe(tt.path); got != tt.want {
				t.Errorf("IsPathSafe() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkValidateFilePath(b *testing.B) {
	ResetAllowedBasePaths()
	_ = SetAllowedBasePath("/storage-pool/kubernetes-volumes")
	b.Cleanup(ResetAllowedBasePaths)

	path := "/storage-pool/kubernetes-volumes/pvc-12345678-1234-1234-1234-123456789abc.img"
	for i := 0; i < b.N; i++ {
		_ = ValidateFilePath(path)
	}
}

func BenchmarkValidateFilePathMalicious(b *testing.B) {
	ResetAllowedBasePaths()
	_ = SetAllowedBasePath("/storage-pool/kubernetes-volumes")
	b.Cleanup(ResetAllowedBasePaths)

	path := "/storage-pool/kubernetes-volumes/pvc-123; rm -rf /"
	for i := 0; i < b.N; i++ {
		_ = ValidateFilePath(path)
	}
}
