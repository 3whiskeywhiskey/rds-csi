package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestDebugCapacityQuery shows the actual output from capacity query
func TestDebugCapacityQuery(t *testing.T) {
	// Check if hardware test is enabled
	address := os.Getenv("RDS_ADDRESS")
	if address == "" {
		t.Skip("Skipping: RDS_ADDRESS not set")
	}

	user := os.Getenv("RDS_USER")
	privateKeyPath := os.Getenv("RDS_PRIVATE_KEY_PATH")
	volumeBasePath := os.Getenv("RDS_VOLUME_BASE_PATH")
	if volumeBasePath == "" {
		volumeBasePath = "/storage-pool/kubernetes-volumes"
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

	// Test the exact query used by GetCapacity
	t.Logf("\n=== Testing capacity query for: %s ===", volumeBasePath)

	// Try the exact command used in GetCapacity
	cmd := fmt.Sprintf(`/file print detail where name="%s"`, volumeBasePath)
	t.Logf("Running command: %s", cmd)
	runCapacityOutput(t, client, cmd)

	// Also try querying the parent directory
	t.Logf("\n=== Testing parent directory: /storage-pool ===")
	cmd2 := `/file print detail where name="/storage-pool"`
	t.Logf("Running command: %s", cmd2)
	runCapacityOutput(t, client, cmd2)

	// Also try listing all files to see available paths
	t.Logf("\n=== Listing all mount points ===")
	cmd3 := `/file print where type="directory"`
	t.Logf("Running command: %s", cmd3)
	runCapacityOutput(t, client, cmd3)
}

func runCapacityOutput(t *testing.T, client *ssh.Client, command string) {
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
	if len(output) > 0 {
		t.Logf("Output:\n%s", string(output))
	} else {
		t.Log("Output: (empty)")
	}
}
