package mock

import (
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
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		klog.Errorf("Failed to handshake: %v", err)
		return
	}
	defer sshConn.Close()

	klog.V(4).Infof("New SSH connection from %s", sshConn.RemoteAddr())

	// Discard all global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
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
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "exec":
			if len(req.Payload) > 4 {
				// Parse command from payload
				cmdLen := int(req.Payload[3])
				if len(req.Payload) >= 4+cmdLen {
					command := string(req.Payload[4 : 4+cmdLen])
					klog.V(4).Infof("Executing command: %s", command)

					// Execute the command and get response
					response, exitStatus := s.executeCommand(command)

					// Send response
					if response != "" {
						channel.Write([]byte(response))
					}

					// Send exit status
					req.Reply(true, nil)
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: uint32(exitStatus)}))
					return
				}
			}
			req.Reply(false, nil)

		case "shell":
			// Not supported for now
			req.Reply(false, nil)

		default:
			req.Reply(false, nil)
		}
	}
}

func (s *MockRDSServer) executeCommand(command string) (string, int) {
	command = strings.TrimSpace(command)

	// Parse /disk add command
	if strings.HasPrefix(command, "/disk add") {
		return s.handleDiskAdd(command)
	}

	// Parse /disk remove command
	if strings.HasPrefix(command, "/disk remove") {
		return s.handleDiskRemove(command)
	}

	// Parse /disk print detail command
	if strings.HasPrefix(command, "/disk print detail") {
		return s.handleDiskPrintDetail(command)
	}

	// Parse /file print detail command
	if strings.HasPrefix(command, "/file print detail") {
		return s.handleFilePrintDetail(command)
	}

	return fmt.Sprintf("bad command name %s\n", command), 1
}

func (s *MockRDSServer) handleDiskAdd(command string) (string, int) {
	// Parse parameters from command
	// Example: /disk add type=file file-path=/storage/vol.img file-size=1073741824 slot=pvc-123 nvme-tcp-export=yes nvme-tcp-server-port=4420 nvme-tcp-server-nqn=nqn.2025-01.io.srvlab.rds:pvc-123

	slot := extractParam(command, "slot")
	filePath := extractParam(command, "file-path")
	fileSizeStr := extractParam(command, "file-size")
	nvmePortStr := extractParam(command, "nvme-tcp-server-port")
	nqn := extractParam(command, "nvme-tcp-server-nqn")

	if slot == "" || filePath == "" || fileSizeStr == "" {
		return "failure: missing required parameters\n", 1
	}

	// Parse file size
	var fileSize int64
	fmt.Sscanf(fileSizeStr, "%d", &fileSize)

	var nvmePort int
	if nvmePortStr != "" {
		fmt.Sscanf(nvmePortStr, "%d", &nvmePort)
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

	// Return all volumes
	var output strings.Builder
	for _, vol := range s.volumes {
		output.WriteString(s.formatDiskDetail(vol))
		output.WriteString("\n")
	}
	return output.String(), 0
}

func (s *MockRDSServer) formatDiskDetail(vol *MockVolume) string {
	exported := "no"
	if vol.Exported {
		exported = "yes"
	}

	return fmt.Sprintf(`Slot: %s
Type: file
File Path: %s
File Size: %d
NVMe TCP Export: %s
NVMe TCP Server Port: %d
NVMe TCP Server NQN: %s
`, vol.Slot, vol.FilePath, vol.FileSizeBytes, exported, vol.NVMETCPPort, vol.NVMETCPNQN)
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

func generateHostKey() (ssh.Signer, error) {
	// For testing, we'll use a simplified approach
	// In production tests, you might want to use a proper key
	key := []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBqPZq7H5Oy3m8NQFO4fYMQB+k9T4PGxH7gGKCJHuG5PAAAAJh0T3JxdE9y
cQAAAAtzc2gtZWQyNTUxOQAAACBqPZq7H5Oy3m8NQFO4fYMQB+k9T4PGxH7gGKCJHuG5PA
AAAEDnEE7m4RqTVE7jRxYLbNMD8Px+qD0I5qXXGcH5+8v3G2o9mrsfk7LebwVAU7h9gxAH
6T1Pg8bEfuAYoIke4bk8AAAAEHRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`)

	return ssh.ParsePrivateKey(key)
}
