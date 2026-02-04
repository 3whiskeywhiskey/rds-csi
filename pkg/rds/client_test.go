package rds

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		config    ClientConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "empty protocol defaults to ssh",
			config: ClientConfig{
				Protocol:           "",
				Address:            "10.42.68.1",
				User:               "admin",
				InsecureSkipVerify: true,
			},
			expectErr: false,
		},
		{
			name: "explicit ssh protocol creates SSH client",
			config: ClientConfig{
				Protocol:           "ssh",
				Address:            "10.42.68.1",
				User:               "admin",
				InsecureSkipVerify: true,
			},
			expectErr: false,
		},
		{
			name: "api protocol returns not yet implemented error",
			config: ClientConfig{
				Protocol: "api",
				Address:  "10.42.68.1",
				User:     "admin",
			},
			expectErr: true,
			errMsg:    "not yet implemented",
		},
		{
			name: "unknown protocol returns unsupported protocol error",
			config: ClientConfig{
				Protocol: "telnet",
				Address:  "10.42.68.1",
				User:     "admin",
			},
			expectErr: true,
			errMsg:    "unsupported protocol",
		},
		{
			name: "invalid SSH config missing address returns error",
			config: ClientConfig{
				Protocol: "ssh",
				User:     "admin",
			},
			expectErr: true,
			errMsg:    "address is required",
		},
		{
			name: "invalid SSH config missing user returns error",
			config: ClientConfig{
				Protocol: "ssh",
				Address:  "10.42.68.1",
			},
			expectErr: true,
			errMsg:    "user is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)

			// Verify it's an SSH client by checking the address
			assert.Equal(t, tt.config.Address, client.GetAddress())
		})
	}
}

func TestNewClient_DefaultValues(t *testing.T) {
	// Test that NewClient properly passes through to newSSHClient with defaults
	client, err := NewClient(ClientConfig{
		Address:            "10.42.68.1",
		User:               "admin",
		InsecureSkipVerify: true,
	})

	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify it's an SSH client
	sshClient, ok := client.(*sshClient)
	require.True(t, ok, "client should be *sshClient")

	// Verify defaults are applied
	assert.Equal(t, 22, sshClient.port, "default port should be 22")
	assert.Equal(t, 10*time.Second, sshClient.timeout, "default timeout should be 10s")
}

func TestNewClient_CustomValues(t *testing.T) {
	// Test that custom values are preserved
	customTimeout := 5 * time.Second
	customPort := 2222

	client, err := NewClient(ClientConfig{
		Address:            "10.42.68.1",
		Port:               customPort,
		User:               "admin",
		Timeout:            customTimeout,
		InsecureSkipVerify: true,
	})

	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify it's an SSH client
	sshClient, ok := client.(*sshClient)
	require.True(t, ok, "client should be *sshClient")

	// Verify custom values are applied
	assert.Equal(t, customPort, sshClient.port, "custom port should be preserved")
	assert.Equal(t, customTimeout, sshClient.timeout, "custom timeout should be preserved")
}
