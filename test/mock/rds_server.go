package mock

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

// MockRDSServer simulates a MikroTik RDS server for testing
type MockRDSServer struct {
	address  string
	port     int
	listener net.Listener
	config   *ssh.ServerConfig
	volumes  map[string]*MockVolume
	mu       sync.RWMutex
	shutdown chan struct{}
}

// MockVolume represents a simulated volume on the mock RDS
type MockVolume struct {
	Slot          string
	FilePath      string
	FileSizeBytes int64
	NVMETCPPort   int
	NVMETCPNQN    string
	Exported      bool
}

// NewMockRDSServer creates a new mock RDS server for testing
func NewMockRDSServer(port int) (*MockRDSServer, error) {
	// Create SSH server config
	config := &ssh.ServerConfig{
		NoClientAuth: true, // Simplified for testing
	}

	// Generate a temporary host key
	hostKey, err := generateHostKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate host key: %w", err)
	}
	config.AddHostKey(hostKey)

	server := &MockRDSServer{
		address:  "localhost",
		port:     port,
		config:   config,
		volumes:  make(map[string]*MockVolume),
		shutdown: make(chan struct{}),
	}

	return server, nil
}

// Start starts the mock RDS SSH server
func (s *MockRDSServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.address, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	klog.Infof("Mock RDS server listening on %s", addr)

	go s.acceptConnections()

	return nil
}

// Stop stops the mock RDS server
func (s *MockRDSServer) Stop() error {
	close(s.shutdown)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Address returns the server address
func (s *MockRDSServer) Address() string {
	return s.address
}

// Port returns the server port
func (s *MockRDSServer) Port() int {
	return s.port
}

// GetVolume returns a volume by slot ID
func (s *MockRDSServer) GetVolume(slot string) (*MockVolume, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vol, ok := s.volumes[slot]
	return vol, ok
}

// ListVolumes returns all volumes
func (s *MockRDSServer) ListVolumes() []*MockVolume {
	s.mu.RLock()
	defer s.mu.RUnlock()
	volumes := make([]*MockVolume, 0, len(s.volumes))
	for _, vol := range s.volumes {
		volumes = append(volumes, vol)
	}
	return volumes
}

func (s *MockRDSServer) acceptConnections() {
	for {
		select {
		case <-s.shutdown:
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.shutdown:
					return
				default:
					klog.Errorf("Failed to accept connection: %v", err)
					continue
				}
			}

			go s.handleConnection(conn)
		}
	}
}

func (s *MockRDSServer) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		klog.Errorf("Failed to handshake: %v", err)
		return
	}
	defer func() { _ = sshConn.Close() }()

	klog.V(4).Infof("New SSH connection from %s", sshConn.RemoteAddr())

	// Discard all global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			klog.Errorf("Could not accept channel: %v", err)
			continue
		}

		go s.handleSession(channel, requests)
	}
}

func (s *MockRDSServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer func() { _ = channel.Close() }()

	for req := range requests {
		klog.V(4).Infof("Mock RDS received request type: %s, payload len: %d", req.Type, len(req.Payload))

		switch req.Type {
		case "exec":
			if len(req.Payload) > 4 {
				// Parse command from payload (SSH exec request format)
				cmdLen := uint32(req.Payload[0])<<24 | uint32(req.Payload[1])<<16 |
					uint32(req.Payload[2])<<8 | uint32(req.Payload[3])

				klog.V(4).Infof("Mock RDS command length: %d, total payload: %d", cmdLen, len(req.Payload))

				if len(req.Payload) >= 4+int(cmdLen) {
					command := string(req.Payload[4 : 4+cmdLen])
					klog.Infof("Mock RDS executing command: %s", command)

					// Execute the command and get response
					response, exitStatus := s.executeCommand(command)

					// Send response
					if response != "" {
						klog.V(4).Infof("Mock RDS sending response (%d bytes)", len(response))
						_, _ = channel.Write([]byte(response))
					}

					// Send exit status
					_ = req.Reply(true, nil)
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: uint32(exitStatus)}))
					klog.V(4).Infof("Mock RDS sent exit status: %d", exitStatus)
					return
				}
			}
			klog.Warningf("Mock RDS: Invalid exec payload format")
			_ = req.Reply(false, nil)

		case "shell":
			// Not supported for now
			_ = req.Reply(false, nil)

		default:
			_ = req.Reply(false, nil)
		}
	}
}

func (s *MockRDSServer) executeCommand(command string) (string, int) {
	command = strings.TrimSpace(command)
	klog.V(3).Infof("Mock RDS executing command: %s", command)

	// Parse /disk add command
	if strings.HasPrefix(command, "/disk add") {
		output, code := s.handleDiskAdd(command)
		klog.V(3).Infof("Mock RDS /disk add returned code %d, output: %s", code, output)
		return output, code
	}

	// Parse /disk remove command
	if strings.HasPrefix(command, "/disk remove") {
		output, code := s.handleDiskRemove(command)
		klog.V(3).Infof("Mock RDS /disk remove returned code %d", code)
		return output, code
	}

	// Parse /disk print detail command
	if strings.HasPrefix(command, "/disk print detail") {
		output, code := s.handleDiskPrintDetail(command)
		klog.V(3).Infof("Mock RDS /disk print detail returned code %d", code)
		return output, code
	}

	// Parse /file print detail command
	if strings.HasPrefix(command, "/file print detail") {
		output, code := s.handleFilePrintDetail(command)
		klog.V(3).Infof("Mock RDS /file print detail returned code %d", code)
		return output, code
	}

	klog.Warningf("Mock RDS: Unrecognized command: %s", command)
	return fmt.Sprintf("bad command name %s\n", command), 1
}

func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
	// Parse parameters from command
	// Example: /disk add type=file file-path=/storage/vol.img file-size=1G slot=pvc-123 nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn=nqn.2025-01.io.srvlab.rds:pvc-123

	slot := extractParam(command, "slot")
	filePath := extractParam(command, "file-path")
	fileSizeStr := extractParam(command, "file-size")
	nvmePortStr := extractParam(command, "nvme-tcp-server-port")
	nqn := extractParam(command, "nvme-tcp-server-nqn")

	if slot == "" || filePath == "" || fileSizeStr == "" {
		return "failure: missing required parameters\n", 1
	}

	// Parse file size (supports formats like "1G", "50G", "1T", or raw bytes)
	fileSize, err := parseSize(fileSizeStr)
	if err != nil {
		return fmt.Sprintf("failure: invalid file size %s: %v\n", fileSizeStr, err), 1
	}

	var nvmePort int
	if nvmePortStr != "" {
		_, _ = fmt.Sscanf(nvmePortStr, "%d", &nvmePort)
	} else {
		nvmePort = 4420 // default
	}

	// Check if volume already exists
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.volumes[slot]; exists {
		return "failure: volume already exists\n", 1
	}

	// Create volume
	s.volumes[slot] = &MockVolume{
		Slot:          slot,
		FilePath:      filePath,
		FileSizeBytes: fileSize,
		NVMETCPPort:   nvmePort,
		NVMETCPNQN:    nqn,
		Exported:      true,
	}

	klog.V(2).Infof("Mock RDS: Created volume %s", slot)
	return "", 0
}

func (s *MockRDSServer) handleDiskRemove(command string) (string, int) {
	// Parse: /disk remove [find slot=pvc-123]
	re := regexp.MustCompile(`slot=([^\s\]]+)`)
	matches := re.FindStringSubmatch(command)

	if len(matches) < 2 {
		return "failure: invalid command format\n", 1
	}

	slot := matches[1]

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.volumes[slot]; !exists {
		// Idempotent - not an error if volume doesn't exist
		return "", 0
	}

	delete(s.volumes, slot)
	klog.V(2).Infof("Mock RDS: Deleted volume %s", slot)
	return "", 0
}

func (s *MockRDSServer) handleDiskPrintDetail(command string) (string, int) {
	// Parse: /disk print detail where slot=pvc-123
	slot := ""
	if strings.Contains(command, "slot=") {
		re := regexp.MustCompile(`slot=([^\s]+)`)
		matches := re.FindStringSubmatch(command)
		if len(matches) >= 2 {
			slot = matches[1]
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if slot != "" {
		// Return specific volume
		vol, exists := s.volumes[slot]
		if !exists {
			return "", 0 // No output if not found
		}
		return s.formatDiskDetail(vol), 0
	}

	// Return all volumes (with line numbers for proper RouterOS format)
	var output strings.Builder
	i := 0
	for _, vol := range s.volumes {
		// RouterOS formats list output with line numbers
		output.WriteString(fmt.Sprintf("%2d %s\n", i, s.formatDiskDetail(vol)))
		i++
	}
	return output.String(), 0
}

func (s *MockRDSServer) formatDiskDetail(vol *MockVolume) string {
	exported := "no"
	if vol.Exported {
		exported = "yes"
	}

	// Format as RouterOS key="value" pairs on a single line
	return fmt.Sprintf(`slot="%s" type="file" file-path="%s" file-size=%d nvme-tcp-export=%s nvme-tcp-server-port=%d nvme-tcp-server-nqn="%s" status="ready"`,
		vol.Slot, vol.FilePath, vol.FileSizeBytes, exported, vol.NVMETCPPort, vol.NVMETCPNQN)
}

func (s *MockRDSServer) handleFilePrintDetail(command string) (string, int) {
	// Simulate filesystem capacity
	// Parse: /file print detail where name="/storage-pool"

	output := `Name: /storage-pool
Type: directory
Size: 0
Total: 7.23TiB
Free: 5.42TiB
`
	return output, 0
}

func extractParam(command, param string) string {
	re := regexp.MustCompile(param + `=([^\s]+)`)
	matches := re.FindStringSubmatch(command)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseSize parses human-readable size strings like "1G", "50G", "1T" or raw bytes
func parseSize(sizeStr string) (int64, error) {
	// Parse with suffix (K, M, G, T) first
	re := regexp.MustCompile(`^(\d+)([KMGT])$`)
	matches := re.FindStringSubmatch(sizeStr)
	if len(matches) == 3 {
		// Parse the number part
		var num int64
		if _, err := fmt.Sscanf(matches[1], "%d", &num); err != nil {
			return 0, fmt.Errorf("invalid number: %s", matches[1])
		}

		// Multiply by the appropriate factor
		switch matches[2] {
		case "K":
			return num * 1024, nil
		case "M":
			return num * 1024 * 1024, nil
		case "G":
			return num * 1024 * 1024 * 1024, nil
		case "T":
			return num * 1024 * 1024 * 1024 * 1024, nil
		}

		return 0, fmt.Errorf("unknown size suffix: %s", matches[2])
	}

	// Try to parse as raw number (only if no suffix)
	var size int64
	if _, err := fmt.Sscanf(sizeStr, "%d", &size); err == nil {
		return size, nil
	}

	return 0, fmt.Errorf("invalid size format: %s", sizeStr)
}

func generateHostKey() (ssh.Signer, error) {
	// Generate a new RSA key for the mock SSH server
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode the private key to PEM format
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Parse it back to create an ssh.Signer
	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated key: %w", err)
	}

	return signer, nil
}
