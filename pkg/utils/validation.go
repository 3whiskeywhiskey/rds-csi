package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Shell metacharacters that could be used for command injection
var dangerousCharacters = []string{
	";",    // Command separator
	"|",    // Pipe
	"&",    // Background/AND
	"$",    // Variable expansion
	"`",    // Command substitution
	"(",    // Subshell
	")",    // Subshell
	"<",    // Input redirection
	">",    // Output redirection
	"\n",   // Newline (command separator)
	"\r",   // Carriage return
	"*",    // Glob wildcard
	"?",    // Glob wildcard
	"[",    // Glob wildcard
	"]",    // Glob wildcard
	"'",    // String delimiter (can break out of quotes)
	"\"",   // String delimiter (can break out of quotes)
	"\\",   // Escape character
	"\t",   // Tab (can cause parsing issues)
	"\x00", // Null byte
}

// AllowedBasePaths defines the whitelist of allowed base paths for volumes.
// This starts empty and must be initialized via SetAllowedBasePath() during driver startup.
var AllowedBasePaths []string

// SetAllowedBasePath sets the allowed base path for volume validation.
// This replaces any existing paths and should be called during driver initialization.
func SetAllowedBasePath(path string) error {
	if path == "" {
		return fmt.Errorf("base path cannot be empty")
	}

	cleanPath, err := SanitizeBasePath(path)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}

	AllowedBasePaths = []string{cleanPath}
	return nil
}

// AddAllowedBasePath adds an additional path to the allowed base paths whitelist.
// Use SetAllowedBasePath() for initial configuration; this is for adding secondary paths.
func AddAllowedBasePath(path string) error {
	if path == "" {
		return nil // No path to add
	}

	cleanPath, err := SanitizeBasePath(path)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}

	// Check if already in list
	for _, existing := range AllowedBasePaths {
		if existing == cleanPath {
			return nil // Already exists
		}
	}

	AllowedBasePaths = append(AllowedBasePaths, cleanPath)
	return nil
}

// ResetAllowedBasePaths clears the allowed base paths list.
// This is primarily for testing to ensure test isolation.
func ResetAllowedBasePaths() {
	AllowedBasePaths = nil
}

// ValidateFilePath validates that a file path is safe for use in shell commands
// It checks for:
// - Shell metacharacters that could enable command injection
// - Path traversal attempts (../)
// - Absolute path requirements
// - Whitelist of allowed base paths
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Check for dangerous shell metacharacters
	for _, char := range dangerousCharacters {
		if strings.Contains(path, char) {
			return fmt.Errorf("file path contains dangerous character %q: %s", char, path)
		}
	}

	// Clean the path to resolve any ./ or ../ components
	cleanPath := filepath.Clean(path)

	// Check if cleaning changed the path (indicates traversal attempt)
	if cleanPath != path {
		return fmt.Errorf("file path contains traversal sequences or unnecessary components: %s (cleaned: %s)", path, cleanPath)
	}

	// Path must be absolute (start with /)
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("file path must be absolute: %s", path)
	}

	// Check if path starts with one of the allowed base paths
	allowed := false
	for _, basePath := range AllowedBasePaths {
		if strings.HasPrefix(cleanPath, basePath) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("file path not in allowed base paths: %s (allowed: %v)", cleanPath, AllowedBasePaths)
	}

	// Additional check: ensure no double slashes (can sometimes bypass filters)
	if strings.Contains(cleanPath, "//") {
		return fmt.Errorf("file path contains double slashes: %s", cleanPath)
	}

	return nil
}

// ValidateFilePathWithBase validates a file path and ensures it's within a specific base path
// This is useful when you know the exact base path that should be used
func ValidateFilePathWithBase(path, basePath string) error {
	if err := ValidateFilePath(path); err != nil {
		return err
	}

	if basePath == "" {
		return fmt.Errorf("base path cannot be empty")
	}

	cleanBase := filepath.Clean(basePath)
	cleanPath := filepath.Clean(path)

	if !strings.HasPrefix(cleanPath, cleanBase) {
		return fmt.Errorf("file path %s is not within base path %s", cleanPath, cleanBase)
	}

	return nil
}

// SanitizeBasePath validates and sanitizes a base path
// This should be called when setting up the base path from configuration
func SanitizeBasePath(basePath string) (string, error) {
	if basePath == "" {
		return "", fmt.Errorf("base path cannot be empty")
	}

	// Check for double slashes BEFORE cleaning (filepath.Clean normalizes them)
	if strings.Contains(basePath, "//") {
		return "", fmt.Errorf("base path contains double slashes: %s", basePath)
	}

	// Clean the path
	cleanPath := filepath.Clean(basePath)

	// Must be absolute
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("base path must be absolute: %s", basePath)
	}

	// Check for dangerous characters
	for _, char := range dangerousCharacters {
		if strings.Contains(cleanPath, char) {
			return "", fmt.Errorf("base path contains dangerous character %q: %s", char, cleanPath)
		}
	}

	return cleanPath, nil
}

// ValidateCreateVolumeOptions validates options for creating a volume
// This includes validation of file paths and other parameters
func ValidateCreateVolumeOptions(filePath string, sizeBytes int64, slot string) error {
	// Validate file path
	if err := ValidateFilePath(filePath); err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	// Validate size
	if sizeBytes <= 0 {
		return fmt.Errorf("volume size must be positive: %d", sizeBytes)
	}

	// Validate slot name (prevents command injection in slot parameter)
	if err := ValidateSlotName(slot); err != nil {
		return fmt.Errorf("invalid slot name: %w", err)
	}

	return nil
}

// IsPathSafe performs a quick safety check on a path
// Returns true if the path appears safe, false otherwise
func IsPathSafe(path string) bool {
	return ValidateFilePath(path) == nil
}
