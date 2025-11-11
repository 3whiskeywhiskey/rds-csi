package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestShowRawRouterOSOutput shows the actual raw output from RouterOS commands
func TestShowRawRouterOSOutput(t *testing.T) {
	// Check if hardware test is enabled
	address := os.Getenv("RDS_ADDRESS")
	if address == "" {
		t.Skip("Skipping: RDS_ADDRESS not set")
	}

	user := os.Getenv("RDS_USER")
	if user == "" {
		t.Skip("Skipping: RDS_USER not set")
	}

	privateKeyPath := os.Getenv("RDS_PRIVATE_KEY_PATH")
	if privateKeyPath == "" {
		t.Skip("Skipping: RDS_PRIVATE_KEY_PATH not set")
	}

	// Read private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		t.Fatalf("Failed to read private key: %v", err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(privateKeyData)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	// Configure SSH
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Connect
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", address), config)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Test command 1: List disks
	t.Log("\n=== Raw output of: /disk print detail ===")
	runAndShowOutput(t, client, "/disk print detail")

	// Test command 2: Show a specific disk (use one that exists)
	t.Log("\n=== Raw output of: /disk print detail where slot=test-hardware-volume-001 ===")
	runAndShowOutput(t, client, "/disk print detail where slot=test-hardware-volume-001")

	// Test command 3: Show file info
	volumeBasePath := os.Getenv("RDS_VOLUME_BASE_PATH")
	if volumeBasePath == "" {
		volumeBasePath = "/storage-pool/kubernetes-volumes"
	}
	t.Logf("\n=== Raw output of: /file print detail where name=\"%s\" ===", volumeBasePath)
	runAndShowOutput(t, client, fmt.Sprintf(`/file print detail where name="%s"`, volumeBasePath))
}

func runAndShowOutput(t *testing.T, client *ssh.Client, command string) {
	session, err := client.NewSession()
	if err != nil {
		t.Errorf("Failed to create session: %v", err)
		return
	}
	defer func() { _ = session.Close() }()

	output, err := session.CombinedOutput(command)
	if err != nil {
		t.Logf("Command error: %v", err)
	}

	t.Logf("Output length: %d bytes", len(output))
	t.Logf("Output:\n%s", string(output))
	t.Logf("Output (with escape codes visible):\n%q", string(output))
}
