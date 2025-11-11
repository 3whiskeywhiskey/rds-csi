package driver

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

const (
	// Maximum message size for gRPC
	maxMsgSize = 16 * 1024 * 1024 // 16 MiB
)

// NonBlockingGRPCServer is a non-blocking gRPC server
type NonBlockingGRPCServer struct {
	server   *grpc.Server
	listener net.Listener
	endpoint string
}

// NewNonBlockingGRPCServer creates a new non-blocking gRPC server
func NewNonBlockingGRPCServer(endpoint string) *NonBlockingGRPCServer {
	return &NonBlockingGRPCServer{
		endpoint: endpoint,
	}
}

// Start starts the gRPC server
func (s *NonBlockingGRPCServer) Start(ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) error {
	// Parse endpoint
	proto, addr, err := parseEndpoint(s.endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	klog.V(4).Infof("Starting gRPC server on %s://%s", proto, addr)

	// Remove existing socket file if it exists (unix sockets only)
	if proto == "unix" {
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing socket: %w", err)
		}
	}

	// Create listener
	listener, err := net.Listen(proto, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s://%s: %w", proto, addr, err)
	}
	s.listener = listener

	// Configure gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	}

	// Create gRPC server
	s.server = grpc.NewServer(opts...)

	// Register services
	if ids != nil {
		csi.RegisterIdentityServer(s.server, ids)
		klog.V(4).Info("Registered Identity service")
	}

	if cs != nil {
		csi.RegisterControllerServer(s.server, cs)
		klog.V(4).Info("Registered Controller service")
	}

	if ns != nil {
		csi.RegisterNodeServer(s.server, ns)
		klog.V(4).Info("Registered Node service")
	}

	// Start serving in a goroutine
	klog.Infof("gRPC server listening on %s://%s", proto, addr)
	go func() {
		if err := s.server.Serve(listener); err != nil {
			klog.Fatalf("Failed to serve: %v", err)
		}
	}()

	return nil
}

// Stop stops the gRPC server
func (s *NonBlockingGRPCServer) Stop() {
	klog.Info("Stopping gRPC server")
	if s.server != nil {
		s.server.GracefulStop()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

// Wait blocks until the server stops
func (s *NonBlockingGRPCServer) Wait() {
	// This is a placeholder - in practice, the main thread will handle shutdown signals
}

// parseEndpoint parses the endpoint into protocol and address
func parseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse endpoint: %w", err)
	}

	var proto, addr string

	switch u.Scheme {
	case "unix":
		proto = "unix"
		addr = u.Path
		if addr == "" {
			addr = u.Host
		}
	case "tcp":
		proto = "tcp"
		addr = u.Host
		if addr == "" {
			return "", "", fmt.Errorf("tcp endpoint must specify host")
		}
	case "":
		// If no scheme, assume unix socket
		proto = "unix"
		addr = strings.TrimPrefix(endpoint, "unix://")
	default:
		return "", "", fmt.Errorf("unsupported endpoint scheme: %s", u.Scheme)
	}

	if addr == "" {
		return "", "", fmt.Errorf("endpoint address cannot be empty")
	}

	return proto, addr, nil
}
