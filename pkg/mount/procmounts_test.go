package mount

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetMounts_ParsesBasicMountinfo tests parsing of valid mountinfo content
func TestGetMounts_ParsesBasicMountinfo(t *testing.T) {
	// Create temp file with simulated mountinfo content
	tmpDir := t.TempDir()
	mountinfoPath := filepath.Join(tmpDir, "mountinfo")

	// Typical mountinfo format from /proc/self/mountinfo
	mountinfoContent := `36 35 0:34 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,memory
48 27 8:1 / /boot rw,relatime - ext4 /dev/sda1 rw
55 30 253:0 / / rw,relatime - ext4 /dev/mapper/vg-root rw
120 55 259:1 / /var/lib/nvme rw,relatime - ext4 /dev/nvme0n1 rw
`

	if err := os.WriteFile(mountinfoPath, []byte(mountinfoContent), 0644); err != nil {
		t.Fatalf("Failed to write mountinfo file: %v", err)
	}

	// Temporarily override /proc/self/mountinfo for testing
	// This is not directly supported, so we'll test parseMountInfoLine directly
	// and test GetMounts with the real system separately

	// Test parsing each line
	lines := []string{
		"36 35 0:34 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,memory",
		"48 27 8:1 / /boot rw,relatime - ext4 /dev/sda1 rw",
		"55 30 253:0 / / rw,relatime - ext4 /dev/mapper/vg-root rw",
		"120 55 259:1 / /var/lib/nvme rw,relatime - ext4 /dev/nvme0n1 rw",
	}

	expected := []MountInfo{
		{Source: "cgroup", Target: "/sys/fs/cgroup/memory", FSType: "cgroup", Options: "rw,nosuid,nodev,noexec,relatime"},
		{Source: "/dev/sda1", Target: "/boot", FSType: "ext4", Options: "rw,relatime"},
		{Source: "/dev/mapper/vg-root", Target: "/", FSType: "ext4", Options: "rw,relatime"},
		{Source: "/dev/nvme0n1", Target: "/var/lib/nvme", FSType: "ext4", Options: "rw,relatime"},
	}

	for i, line := range lines {
		mount, err := parseMountInfoLine(line)
		if err != nil {
			t.Errorf("Line %d: unexpected error: %v", i, err)
			continue
		}

		if mount.Source != expected[i].Source {
			t.Errorf("Line %d: expected source %q, got %q", i, expected[i].Source, mount.Source)
		}
		if mount.Target != expected[i].Target {
			t.Errorf("Line %d: expected target %q, got %q", i, expected[i].Target, mount.Target)
		}
		if mount.FSType != expected[i].FSType {
			t.Errorf("Line %d: expected fstype %q, got %q", i, expected[i].FSType, mount.FSType)
		}
		if mount.Options != expected[i].Options {
			t.Errorf("Line %d: expected options %q, got %q", i, expected[i].Options, mount.Options)
		}
	}
}

// TestGetMounts_HandlesEscapedPaths tests parsing of paths with escaped characters
func TestGetMounts_HandlesEscapedPaths(t *testing.T) {
	testCases := []struct {
		name     string
		line     string
		expected MountInfo
	}{
		{
			name: "space in mount path",
			line: `100 50 8:1 / /mnt/my\040data rw,relatime - ext4 /dev/sdb1 rw`,
			expected: MountInfo{
				Source:  "/dev/sdb1",
				Target:  "/mnt/my data",
				FSType:  "ext4",
				Options: "rw,relatime",
			},
		},
		{
			name: "tab in source",
			line: `101 50 8:2 / /mnt/normal rw,relatime - ext4 /dev/sd\011b2 rw`,
			expected: MountInfo{
				Source:  "/dev/sd\tb2",
				Target:  "/mnt/normal",
				FSType:  "ext4",
				Options: "rw,relatime",
			},
		},
		{
			name: "backslash in path",
			line: `102 50 8:3 / /mnt/test\134path rw,relatime - ext4 /dev/sdb3 rw`,
			expected: MountInfo{
				Source:  "/dev/sdb3",
				Target:  "/mnt/test\\path",
				FSType:  "ext4",
				Options: "rw,relatime",
			},
		},
		{
			name: "multiple spaces",
			line: `103 50 8:4 / /mnt/my\040multiple\040spaces rw,relatime - ext4 /dev/sdb4 rw`,
			expected: MountInfo{
				Source:  "/dev/sdb4",
				Target:  "/mnt/my multiple spaces",
				FSType:  "ext4",
				Options: "rw,relatime",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mount, err := parseMountInfoLine(tc.line)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if mount.Source != tc.expected.Source {
				t.Errorf("Expected source %q, got %q", tc.expected.Source, mount.Source)
			}
			if mount.Target != tc.expected.Target {
				t.Errorf("Expected target %q, got %q", tc.expected.Target, mount.Target)
			}
			if mount.FSType != tc.expected.FSType {
				t.Errorf("Expected fstype %q, got %q", tc.expected.FSType, mount.FSType)
			}
			if mount.Options != tc.expected.Options {
				t.Errorf("Expected options %q, got %q", tc.expected.Options, mount.Options)
			}
		})
	}
}

// TestGetMounts_HandlesOptionalFields tests parsing with varying numbers of optional fields
func TestGetMounts_HandlesOptionalFields(t *testing.T) {
	testCases := []struct {
		name     string
		line     string
		expected MountInfo
	}{
		{
			name: "no optional fields",
			line: `36 35 0:34 / /sys/fs/cgroup rw,relatime - cgroup cgroup rw`,
			expected: MountInfo{
				Source:  "cgroup",
				Target:  "/sys/fs/cgroup",
				FSType:  "cgroup",
				Options: "rw,relatime",
			},
		},
		{
			name: "one optional field",
			line: `37 35 0:35 / /sys/fs/cgroup/cpu rw,relatime shared:1 - cgroup cgroup rw`,
			expected: MountInfo{
				Source:  "cgroup",
				Target:  "/sys/fs/cgroup/cpu",
				FSType:  "cgroup",
				Options: "rw,relatime",
			},
		},
		{
			name: "multiple optional fields",
			line: `38 35 0:36 / /sys/fs/cgroup/memory rw,relatime shared:2 master:3 - cgroup cgroup rw`,
			expected: MountInfo{
				Source:  "cgroup",
				Target:  "/sys/fs/cgroup/memory",
				FSType:  "cgroup",
				Options: "rw,relatime",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mount, err := parseMountInfoLine(tc.line)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if mount.Source != tc.expected.Source {
				t.Errorf("Expected source %q, got %q", tc.expected.Source, mount.Source)
			}
			if mount.Target != tc.expected.Target {
				t.Errorf("Expected target %q, got %q", tc.expected.Target, mount.Target)
			}
			if mount.FSType != tc.expected.FSType {
				t.Errorf("Expected fstype %q, got %q", tc.expected.FSType, mount.FSType)
			}
		})
	}
}

// TestGetMountDevice_FindsExistingMount tests finding an existing mount
func TestGetMountDevice_FindsExistingMount(t *testing.T) {
	// We can't easily mock GetMounts for this test without refactoring
	// So we'll test against the real system and check for root mount
	// This is an integration-style test

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Root mount should always exist
	device, err := GetMountDevice("/")
	if err != nil {
		t.Fatalf("GetMountDevice failed for /: %v", err)
	}

	if device == "" {
		t.Error("Expected non-empty device for root mount")
	}

	t.Logf("Root mount device: %s", device)
}

// TestGetMountDevice_MountNotFound tests behavior when mount doesn't exist
func TestGetMountDevice_MountNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Try to get device for a path that definitely doesn't exist as a mount
	nonexistentPath := "/this/path/should/never/be/a/mount/point"

	_, err := GetMountDevice(nonexistentPath)
	if err == nil {
		t.Error("Expected error for non-existent mount, got nil")
	}

	expectedErrSubstr := "mount point not found"
	if err != nil && !contains(err.Error(), expectedErrSubstr) {
		t.Errorf("Expected error containing %q, got %q", expectedErrSubstr, err.Error())
	}
}

// TestGetMounts_RealSystem tests GetMounts against the real system
func TestGetMounts_RealSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mounts, err := GetMounts()
	if err != nil {
		t.Fatalf("GetMounts failed: %v", err)
	}

	// Should have at least root mount
	if len(mounts) == 0 {
		t.Fatal("Expected at least one mount, got none")
	}

	// Find root mount
	foundRoot := false
	for _, m := range mounts {
		if m.Target == "/" {
			foundRoot = true
			t.Logf("Root mount: source=%s, fstype=%s, options=%s", m.Source, m.FSType, m.Options)

			// Verify fields are populated
			if m.Source == "" {
				t.Error("Root mount has empty source")
			}
			if m.FSType == "" {
				t.Error("Root mount has empty fstype")
			}
			break
		}
	}

	if !foundRoot {
		t.Error("Root mount not found in mount list")
	}

	// Log a few mounts for visibility
	t.Logf("Total mounts found: %d", len(mounts))
	for i, m := range mounts {
		if i >= 5 {
			break
		}
		t.Logf("Mount %d: %s -> %s (%s)", i, m.Source, m.Target, m.FSType)
	}
}

// TestParseMountInfoLine_InvalidInput tests error handling
func TestParseMountInfoLine_InvalidInput(t *testing.T) {
	testCases := []struct {
		name string
		line string
	}{
		{
			name: "too few fields",
			line: "36 35 0:34 /",
		},
		{
			name: "missing separator",
			line: "36 35 0:34 / /sys/fs/cgroup rw,relatime cgroup cgroup rw",
		},
		{
			name: "empty line",
			line: "",
		},
		{
			name: "only whitespace",
			line: "   ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMountInfoLine(tc.line)
			if err == nil {
				t.Error("Expected error for invalid input, got nil")
			}
		})
	}
}

// TestGetMountInfo_FindsExistingMount tests GetMountInfo for existing mount
func TestGetMountInfo_FindsExistingMount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test with root mount
	info, err := GetMountInfo("/")
	if err != nil {
		t.Fatalf("GetMountInfo failed for /: %v", err)
	}

	if info == nil {
		t.Fatal("Expected non-nil MountInfo")
	}

	if info.Target != "/" {
		t.Errorf("Expected target /, got %s", info.Target)
	}

	if info.Source == "" {
		t.Error("Expected non-empty source")
	}

	if info.FSType == "" {
		t.Error("Expected non-empty fstype")
	}

	t.Logf("Root mount info: source=%s, fstype=%s, options=%s", info.Source, info.FSType, info.Options)
}

// TestGetMountInfo_MountNotFound tests GetMountInfo for non-existent mount
func TestGetMountInfo_MountNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nonexistentPath := "/this/path/should/never/be/a/mount/point"

	_, err := GetMountInfo(nonexistentPath)
	if err == nil {
		t.Error("Expected error for non-existent mount, got nil")
	}

	expectedErrSubstr := "mount point not found"
	if err != nil && !contains(err.Error(), expectedErrSubstr) {
		t.Errorf("Expected error containing %q, got %q", expectedErrSubstr, err.Error())
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
