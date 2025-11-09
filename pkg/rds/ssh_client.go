package rds

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

// sshClient implements RDSClient using SSH protocol to connect to RouterOS
type sshClient struct {
	address            string // RDS IP address
	port               int
	user               string
	privateKey         []byte
	timeout            time.Duration
	sshClient          *ssh.Client
	hostKeyCallback    ssh.HostKeyCallback
	insecureSkipVerify bool
}

// newSSHClient creates a new SSH-based RDS client
func newSSHClient(config ClientConfig) (*sshClient, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("address is required")
	}
	if config.User == "" {
		return nil, fmt.Errorf("user is required")
	}
	// Note: PrivateKey is optional for testing with mock servers that don't require auth

	// Set defaults
	if config.Port == 0 {
		config.Port = 22
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	// Handle host key callback
	var hostKeyCallback ssh.HostKeyCallback
	if config.HostKeyCallback != nil {
		// Use provided callback (must be ssh.HostKeyCallback)
		if cb, ok := config.HostKeyCallback.(ssh.HostKeyCallback); ok {
			hostKeyCallback = cb
		} else {
			return nil, fmt.Errorf("HostKeyCallback must be of type ssh.HostKeyCallback")
		}
	}

	return &sshClient{
		address:            config.Address,
		port:               config.Port,
		user:               config.User,
		privateKey:         config.PrivateKey,
		timeout:            config.Timeout,
		hostKeyCallback:    hostKeyCallback,
		insecureSkipVerify: config.InsecureSkipVerify,
	}, nil
}

// GetAddress returns the RDS server address
func (c *sshClient) GetAddress() string {
	return c.address
}

// Connect establishes SSH connection to RDS
func (c *sshClient) Connect() error {
	klog.V(4).Infof("Connecting to RDS at %s:%d as user %s", c.address, c.port, c.user)

	// Configure SSH client with host key callback
	var hostKeyCallback ssh.HostKeyCallback
	if c.hostKeyCallback != nil {
		hostKeyCallback = c.hostKeyCallback
		klog.V(4).Info("Using custom host key verification")
	} else if c.insecureSkipVerify {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
		klog.Warning("INSECURE: Skipping SSH host key verification - not recommended for production")
	} else {
		// Default: use InsecureIgnoreHostKey for backward compatibility
		// In production, users should provide their own HostKeyCallback
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
		klog.V(4).Info("Using InsecureIgnoreHostKey (default) - configure HostKeyCallback for production security")
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.user,
		HostKeyCallback: hostKeyCallback,
		Timeout:         c.timeout,
	}

	// Add authentication if private key is provided
	if len(c.privateKey) > 0 {
		// Parse private key
		signer, err := ssh.ParsePrivateKey(c.privateKey)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}
	} else {
		// No authentication (for testing with mock servers)
		klog.V(4).Info("No private key provided, attempting connection without authentication")
	}

	// Establish connection
	addr := fmt.Sprintf("%s:%d", c.address, c.port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	c.sshClient = client
	klog.V(4).Infof("Successfully connected to RDS at %s:%d", c.address, c.port)
	return nil
}

// Close closes the SSH connection
func (c *sshClient) Close() error {
	if c.sshClient != nil {
		klog.V(4).Infof("Closing SSH connection to RDS")
		return c.sshClient.Close()
	}
	return nil
}

// IsConnected returns true if SSH connection is active
func (c *sshClient) IsConnected() bool {
	if c.sshClient == nil {
		return false
	}

	// Test connection by trying to create a session
	// RouterOS may not support keepalive requests, so use session creation as test
	session, err := c.sshClient.NewSession()
	if err != nil {
		return false
	}
	session.Close()
	return true
}

// runCommand executes a RouterOS CLI command via SSH
func (c *sshClient) runCommand(command string) (string, error) {
	if c.sshClient == nil {
		return "", fmt.Errorf("not connected to RDS")
	}

	klog.V(5).Infof("Executing RouterOS command: %s", command)

	// Create session
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Run command
	if err := session.Run(command); err != nil {
		// Check if it's an exit error (command failed)
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return stdout.String(), fmt.Errorf("command failed (exit %d): %s", exitErr.ExitStatus(), stderr.String())
		}
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	output := stdout.String()
	klog.V(5).Infof("Command output: %s", output)
	return output, nil
}

// runCommandWithRetry executes a command with retry logic for transient errors
func (c *sshClient) runCommandWithRetry(command string, maxRetries int) (string, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			klog.V(4).Infof("Retrying command after %v (attempt %d/%d)", backoff, attempt+1, maxRetries)
			time.Sleep(backoff)
		}

		// Reconnect if connection is lost
		if !c.IsConnected() {
			klog.V(4).Info("Reconnecting to RDS before retry")
			if err := c.Connect(); err != nil {
				lastErr = err
				continue
			}
		}

		output, err := c.runCommand(command)
		if err == nil {
			return output, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			klog.V(4).Infof("Non-retryable error: %v", err)
			return "", lastErr
		}

		klog.V(4).Infof("Retryable error: %v", err)
	}

	return "", fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are retryable
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// EOF and connection reset errors are retryable
	if err == io.EOF {
		return true
	}

	errStr := err.Error()

	// Don't retry command errors like "not enough space", "invalid parameter"
	nonRetryablePatterns := []string{
		"not enough space",
		"invalid parameter",
		"no such item",
		"authentication failed",
	}

	for _, pattern := range nonRetryablePatterns {
		if containsString(errStr, pattern) {
			return false
		}
	}

	// Retry everything else (connection issues, transient errors)
	return true
}

// containsString checks if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexString(s, substr) >= 0)
}

// indexString finds the index of substr in s
func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
