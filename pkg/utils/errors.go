package utils

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/klog/v2"
)

// Sentinel errors for common conditions.
// Use errors.Is() to check for these rather than string matching.
var (
	// ErrVolumeNotFound indicates the requested volume does not exist
	ErrVolumeNotFound = errors.New("volume not found")

	// ErrVolumeExists indicates the volume already exists
	ErrVolumeExists = errors.New("volume already exists")

	// ErrNodeNotFound indicates the requested node does not exist
	ErrNodeNotFound = errors.New("node not found")

	// ErrInvalidParameter indicates an invalid parameter was provided
	ErrInvalidParameter = errors.New("invalid parameter")

	// ErrResourceExhausted indicates insufficient storage capacity
	ErrResourceExhausted = errors.New("resource exhausted")

	// ErrOperationTimeout indicates an operation timed out
	ErrOperationTimeout = errors.New("operation timeout")

	// ErrDeviceNotFound indicates NVMe device was not found
	ErrDeviceNotFound = errors.New("device not found")

	// ErrDeviceInUse indicates the device is currently in use
	ErrDeviceInUse = errors.New("device in use")

	// ErrMountFailed indicates a mount operation failed
	ErrMountFailed = errors.New("mount failed")

	// ErrUnmountFailed indicates an unmount operation failed
	ErrUnmountFailed = errors.New("unmount failed")
)

// ErrorType classifies errors for sanitization purposes
type ErrorType int

const (
	// ErrorTypeInternal indicates an internal error with implementation details
	ErrorTypeInternal ErrorType = iota

	// ErrorTypeUser indicates a user-facing error that should be sanitized
	ErrorTypeUser

	// ErrorTypeValidation indicates a validation error (safe to show to users)
	ErrorTypeValidation
)

// SanitizedError wraps an error with classification and sanitization
type SanitizedError struct {
	// Original error (kept for logging)
	originalErr error

	// Sanitized message (safe for user consumption)
	sanitizedMsg string

	// Error type classification
	errorType ErrorType

	// Additional context for logging only
	internalContext map[string]string
}

// Error implements the error interface, returning the sanitized message
func (e *SanitizedError) Error() string {
	return e.sanitizedMsg
}

// Unwrap returns the original error for error unwrapping
func (e *SanitizedError) Unwrap() error {
	return e.originalErr
}

// GetOriginal returns the original unsanitized error
func (e *SanitizedError) GetOriginal() error {
	return e.originalErr
}

// GetInternalContext returns internal context for logging
func (e *SanitizedError) GetInternalContext() map[string]string {
	return e.internalContext
}

// Log logs the full error details at the appropriate level
func (e *SanitizedError) Log() {
	msg := fmt.Sprintf("Error: %s", e.sanitizedMsg)
	if e.originalErr != nil {
		msg = fmt.Sprintf("%s (internal: %v)", msg, e.originalErr)
	}
	if len(e.internalContext) > 0 {
		msg = fmt.Sprintf("%s context=%v", msg, e.internalContext)
	}

	switch e.errorType {
	case ErrorTypeInternal:
		klog.Errorf("[INTERNAL ERROR] %s", msg)
	case ErrorTypeUser:
		klog.Warningf("[USER ERROR] %s", msg)
	case ErrorTypeValidation:
		klog.V(4).Infof("[VALIDATION ERROR] %s", msg)
	}
}

// NewInternalError creates an internal error with sensitive information removed
func NewInternalError(err error, userMsg string) *SanitizedError {
	if err == nil {
		err = fmt.Errorf("internal error")
	}

	sanitized := &SanitizedError{
		originalErr:     err,
		sanitizedMsg:    userMsg,
		errorType:       ErrorTypeInternal,
		internalContext: make(map[string]string),
	}

	// Log immediately for debugging
	sanitized.Log()

	return sanitized
}

// NewUserError creates a user-facing error with sanitization
func NewUserError(err error, operation string) *SanitizedError {
	if err == nil {
		err = fmt.Errorf("operation failed")
	}

	// Sanitize the error message
	sanitizedMsg := SanitizeErrorMessage(err.Error())

	// Create friendly message
	userMsg := fmt.Sprintf("%s failed: %s", operation, sanitizedMsg)

	sanitized := &SanitizedError{
		originalErr:     err,
		sanitizedMsg:    userMsg,
		errorType:       ErrorTypeUser,
		internalContext: make(map[string]string),
	}

	sanitized.internalContext["operation"] = operation

	// Log for debugging
	sanitized.Log()

	return sanitized
}

// NewValidationError creates a validation error (safe to show to users)
func NewValidationError(field, reason string) *SanitizedError {
	msg := fmt.Sprintf("validation failed for %s: %s", field, reason)

	sanitized := &SanitizedError{
		originalErr:     fmt.Errorf("validation failed for %s: %s", field, reason),
		sanitizedMsg:    msg,
		errorType:       ErrorTypeValidation,
		internalContext: make(map[string]string),
	}

	sanitized.internalContext["field"] = field
	sanitized.internalContext["reason"] = reason

	return sanitized
}

// WithContext adds internal context to the error (for logging only)
func (e *SanitizedError) WithContext(key, value string) *SanitizedError {
	if e.internalContext == nil {
		e.internalContext = make(map[string]string)
	}
	e.internalContext[key] = value
	return e
}

// Regular expressions for sanitization
var (
	// Match IPv4 addresses (e.g., 192.168.1.1, 10.0.0.1)
	ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

	// Match IPv6 addresses (basic pattern)
	ipv6Pattern = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`)

	// Match absolute file paths (Unix and Windows)
	// Unix: starts with / and contains at least one more path component
	unixPathPattern = regexp.MustCompile(`/[a-zA-Z0-9_\-]+(?:/[a-zA-Z0-9_.\-]+)*`)
	// Windows: starts with drive letter (C:\ etc) followed by path components
	// Matches: C:\Users\Admin\file.txt
	windowsPathPattern = regexp.MustCompile(`[A-Z]:\\[\w\\\-\.]+`)

	// Match hostnames and FQDNs
	hostnamePattern = regexp.MustCompile(`\b[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)*\.(com|net|org|io|local|lan)\b`)

	// Match SSH key fingerprints
	fingerprintPattern = regexp.MustCompile(`SHA256:[A-Za-z0-9+/=]+`)

	// Match error messages with stack traces
	stackTracePattern = regexp.MustCompile(`\n\s+at\s+.*|goroutine\s+\d+.*`)
)

// SanitizeErrorMessage removes sensitive information from error messages
func SanitizeErrorMessage(msg string) string {
	// Remove IPv4 addresses
	msg = ipv4Pattern.ReplaceAllString(msg, "[IP-ADDRESS]")

	// Remove IPv6 addresses
	msg = ipv6Pattern.ReplaceAllString(msg, "[IP-ADDRESS]")

	// Remove SSH fingerprints
	msg = fingerprintPattern.ReplaceAllString(msg, "[FINGERPRINT]")

	// Remove absolute paths (but preserve relative references)
	msg = sanitizePaths(msg)

	// Remove hostnames (but keep generic terms like "server", "host")
	msg = sanitizeHostnames(msg)

	// Remove stack traces
	msg = stackTracePattern.ReplaceAllString(msg, "")

	// Clean up multiple spaces
	msg = regexp.MustCompile(`\s+`).ReplaceAllString(msg, " ")

	// Trim whitespace
	msg = strings.TrimSpace(msg)

	return msg
}

// sanitizePaths removes file paths while preserving useful information
func sanitizePaths(msg string) string {
	// First, preserve relative paths by temporarily replacing them
	relativePaths := make(map[string]string)
	relativePathPattern := regexp.MustCompile(`\./[a-zA-Z0-9_.\-]+`)
	msg = relativePathPattern.ReplaceAllStringFunc(msg, func(path string) string {
		placeholder := fmt.Sprintf("__RELATIVE_PATH_%d__", len(relativePaths))
		relativePaths[placeholder] = path
		return placeholder
	})

	// Unix paths
	msg = unixPathPattern.ReplaceAllStringFunc(msg, func(path string) string {
		// Keep basename for context, but hide directory structure
		if strings.HasPrefix(path, "/dev/") {
			// Keep /dev/ paths for debugging NVMe issues
			return path
		}
		if strings.HasPrefix(path, "/sys/") {
			// Keep /sys/ paths for debugging
			return path
		}
		if strings.HasPrefix(path, "/proc/") {
			// Keep /proc/ paths for debugging
			return path
		}

		// For other paths, show only basename
		base := filepath.Base(path)
		if base != "." && base != "/" {
			return fmt.Sprintf("[PATH]/%s", base)
		}
		return "[PATH]"
	})

	// Restore relative paths
	for placeholder, originalPath := range relativePaths {
		msg = strings.ReplaceAll(msg, placeholder, originalPath)
	}

	// Windows paths
	msg = windowsPathPattern.ReplaceAllStringFunc(msg, func(path string) string {
		// Extract basename manually since filepath.Base doesn't work for Windows paths on Unix
		parts := strings.Split(path, "\\")
		base := parts[len(parts)-1]
		if base != "" && base != "." {
			return fmt.Sprintf("[PATH]\\%s", base)
		}
		return "[PATH]"
	})

	return msg
}

// sanitizeHostnames removes hostnames while keeping generic terms
func sanitizeHostnames(msg string) string {
	return hostnamePattern.ReplaceAllString(msg, "[HOSTNAME]")
}

// SanitizeError wraps an existing error with sanitization
// This is useful for wrapping errors from external libraries
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}

	// If it's already a SanitizedError, return as-is
	if _, ok := err.(*SanitizedError); ok {
		return err
	}

	// Create sanitized wrapper
	sanitizedMsg := SanitizeErrorMessage(err.Error())

	return &SanitizedError{
		originalErr:     err,
		sanitizedMsg:    sanitizedMsg,
		errorType:       ErrorTypeUser,
		internalContext: make(map[string]string),
	}
}

// SanitizeErrorf creates a sanitized error with formatting
func SanitizeErrorf(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	sanitizedMsg := SanitizeErrorMessage(msg)

	return &SanitizedError{
		originalErr:     fmt.Errorf("%s", msg),
		sanitizedMsg:    sanitizedMsg,
		errorType:       ErrorTypeUser,
		internalContext: make(map[string]string),
	}
}

// WrapError wraps an error with additional context and sanitization
func WrapError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	// Create context message
	contextMsg := fmt.Sprintf(format, args...)

	// Get original error
	originalErr := err
	if se, ok := err.(*SanitizedError); ok {
		originalErr = se.GetOriginal()
	}

	// Wrap with context
	wrappedErr := fmt.Errorf("%s: %w", contextMsg, originalErr)

	// Sanitize the complete message
	sanitizedMsg := SanitizeErrorMessage(wrappedErr.Error())

	sanitized := &SanitizedError{
		originalErr:     wrappedErr,
		sanitizedMsg:    sanitizedMsg,
		errorType:       ErrorTypeUser,
		internalContext: make(map[string]string),
	}

	sanitized.internalContext["context"] = contextMsg

	return sanitized
}

// IsInternalError checks if an error is an internal error
func IsInternalError(err error) bool {
	if se, ok := err.(*SanitizedError); ok {
		return se.errorType == ErrorTypeInternal
	}
	return false
}

// IsUserError checks if an error is a user-facing error
func IsUserError(err error) bool {
	if se, ok := err.(*SanitizedError); ok {
		return se.errorType == ErrorTypeUser
	}
	return false
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	if se, ok := err.(*SanitizedError); ok {
		return se.errorType == ErrorTypeValidation
	}
	return false
}

// LogErrorDetails logs the full error details for debugging
func LogErrorDetails(err error) {
	if err == nil {
		return
	}

	if se, ok := err.(*SanitizedError); ok {
		se.Log()
	} else {
		klog.Errorf("Error: %v", err)
	}
}

// GetSanitizedMessage extracts the sanitized message from any error
func GetSanitizedMessage(err error) string {
	if err == nil {
		return ""
	}

	if se, ok := err.(*SanitizedError); ok {
		return se.sanitizedMsg
	}

	// Sanitize unsanitized errors
	return SanitizeErrorMessage(err.Error())
}
