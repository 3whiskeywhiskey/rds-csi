package mock

import (
	"sync"

	"k8s.io/klog/v2"
)

// ErrorMode defines the type of error to inject
type ErrorMode int

const (
	// ErrorModeNone indicates no error injection
	ErrorModeNone ErrorMode = iota
	// ErrorModeDiskFull simulates "failure: not enough space" errors
	ErrorModeDiskFull
	// ErrorModeSSHTimeout simulates SSH connection timeout
	ErrorModeSSHTimeout
	// ErrorModeCommandFail simulates command execution failure
	ErrorModeCommandFail
)

// ErrorInjector manages error injection for testing
type ErrorInjector struct {
	mode         ErrorMode
	operationNum int
	triggerAfter int
	mu           sync.Mutex // Protect operation counter
}

// NewErrorInjector creates a new error injector from configuration
func NewErrorInjector(config MockRDSConfig) *ErrorInjector {
	mode := ParseErrorMode(config.ErrorMode)
	return &ErrorInjector{
		mode:         mode,
		triggerAfter: config.ErrorAfterN,
	}
}

// ParseErrorMode converts string error mode to ErrorMode constant
func ParseErrorMode(s string) ErrorMode {
	switch s {
	case "disk_full":
		return ErrorModeDiskFull
	case "ssh_timeout":
		return ErrorModeSSHTimeout
	case "command_fail":
		return ErrorModeCommandFail
	case "none", "":
		return ErrorModeNone
	default:
		klog.Warningf("Unknown error mode %q, using none", s)
		return ErrorModeNone
	}
}

// ShouldFailSSHConnect returns true if SSH connection should timeout
func (e *ErrorInjector) ShouldFailSSHConnect() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode != ErrorModeSSHTimeout {
		return false
	}

	e.operationNum++
	return e.operationNum > e.triggerAfter
}

// ShouldFailDiskAdd returns whether disk add should fail and the error message
func (e *ErrorInjector) ShouldFailDiskAdd() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode != ErrorModeDiskFull && e.mode != ErrorModeCommandFail {
		return false, ""
	}

	e.operationNum++
	if e.operationNum <= e.triggerAfter {
		return false, ""
	}

	// Return appropriate error message based on mode
	switch e.mode {
	case ErrorModeDiskFull:
		return true, "failure: not enough space\n"
	case ErrorModeCommandFail:
		return true, "failure: execution error\n"
	default:
		return false, ""
	}
}

// ShouldFailDiskRemove returns whether disk remove should fail and the error message
func (e *ErrorInjector) ShouldFailDiskRemove() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode != ErrorModeCommandFail {
		return false, ""
	}

	e.operationNum++
	if e.operationNum <= e.triggerAfter {
		return false, ""
	}

	return true, "failure: execution error\n"
}

// Reset resets the operation counter for test isolation
func (e *ErrorInjector) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.operationNum = 0
}
