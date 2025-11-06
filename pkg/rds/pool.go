package rds

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/klog/v2"
)

var (
	// ErrPoolClosed is returned when attempting to use a closed pool
	ErrPoolClosed = errors.New("connection pool is closed")

	// ErrPoolExhausted is returned when the pool has reached max connections
	ErrPoolExhausted = errors.New("connection pool exhausted")

	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// ConnectionPool manages a pool of RDS client connections with rate limiting
type ConnectionPool struct {
	factory    func() (RDSClient, error)
	maxSize    int
	maxIdle    int
	idleTime   time.Duration
	limiter    *rate.Limiter
	breaker    *CircuitBreaker
	mu         sync.Mutex
	idle       []pooledConnection
	active     int
	closed     bool
	metrics    *PoolMetrics
	waitQueue  chan struct{}
}

// pooledConnection wraps an RDSClient with metadata
type pooledConnection struct {
	client     RDSClient
	lastUsed   time.Time
	inUse      bool
}

// PoolConfig configures the connection pool
type PoolConfig struct {
	// Factory creates new RDS clients
	Factory func() (RDSClient, error)

	// MaxSize is the maximum number of connections (active + idle)
	MaxSize int

	// MaxIdle is the maximum number of idle connections
	MaxIdle int

	// IdleTimeout is how long a connection can be idle before closing
	IdleTimeout time.Duration

	// RateLimit is the maximum number of connections per second
	RateLimit float64

	// RateBurst is the burst size for rate limiting
	RateBurst int

	// CircuitBreakerThreshold is the failure count to open circuit
	CircuitBreakerThreshold int

	// CircuitBreakerTimeout is how long circuit stays open
	CircuitBreakerTimeout time.Duration
}

// PoolMetrics tracks connection pool statistics
type PoolMetrics struct {
	mu                sync.RWMutex
	totalConnections  int64
	activeConnections int
	idleConnections   int
	connectionErrors  int64
	circuitBreaks     int64
	rateLimitHits     int64
	waitTimeTotal     time.Duration
	waitCount         int64
}

// CircuitBreaker implements circuit breaker pattern for connection failures
type CircuitBreaker struct {
	mu            sync.Mutex
	threshold     int
	timeout       time.Duration
	failures      int
	lastFailTime  time.Time
	state         CircuitState
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed means normal operation
	CircuitClosed CircuitState = iota
	// CircuitOpen means too many failures, rejecting requests
	CircuitOpen
	// CircuitHalfOpen means testing if service recovered
	CircuitHalfOpen
)

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config PoolConfig) (*ConnectionPool, error) {
	if config.Factory == nil {
		return nil, fmt.Errorf("factory function is required")
	}
	if config.MaxSize <= 0 {
		config.MaxSize = 10
	}
	if config.MaxIdle <= 0 {
		config.MaxIdle = 5
	}
	if config.MaxIdle > config.MaxSize {
		config.MaxIdle = config.MaxSize
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 5 * time.Minute
	}
	if config.RateLimit <= 0 {
		config.RateLimit = 10.0 // 10 connections per second default
	}
	if config.RateBurst <= 0 {
		config.RateBurst = int(config.RateLimit) * 2
	}
	if config.CircuitBreakerThreshold <= 0 {
		config.CircuitBreakerThreshold = 5
	}
	if config.CircuitBreakerTimeout == 0 {
		config.CircuitBreakerTimeout = 30 * time.Second
	}

	pool := &ConnectionPool{
		factory:   config.Factory,
		maxSize:   config.MaxSize,
		maxIdle:   config.MaxIdle,
		idleTime:  config.IdleTimeout,
		limiter:   rate.NewLimiter(rate.Limit(config.RateLimit), config.RateBurst),
		breaker:   NewCircuitBreaker(config.CircuitBreakerThreshold, config.CircuitBreakerTimeout),
		idle:      make([]pooledConnection, 0, config.MaxIdle),
		metrics:   &PoolMetrics{},
		waitQueue: make(chan struct{}, config.MaxSize),
	}

	klog.V(4).Infof("Created connection pool: maxSize=%d, maxIdle=%d, rateLimit=%.1f/s",
		config.MaxSize, config.MaxIdle, config.RateLimit)

	return pool, nil
}

// Get acquires a connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (RDSClient, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	p.mu.Unlock()

	// Check circuit breaker
	if !p.breaker.Allow() {
		p.metrics.incrementCircuitBreaks()
		return nil, ErrCircuitOpen
	}

	// Apply rate limiting
	startWait := time.Now()
	if err := p.limiter.Wait(ctx); err != nil {
		p.metrics.incrementRateLimitHits()
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}
	p.metrics.recordWait(time.Since(startWait))

	// Try to get an idle connection
	p.mu.Lock()
	for len(p.idle) > 0 {
		conn := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		// Check if connection is still valid and not too old
		if time.Since(conn.lastUsed) > p.idleTime || !conn.client.IsConnected() {
			p.mu.Unlock()
			klog.V(5).Info("Closing stale idle connection")
			_ = conn.client.Close()
			p.mu.Lock()
			continue
		}

		// Reuse this connection
		p.active++
		p.updateMetrics()
		p.mu.Unlock()
		klog.V(5).Info("Reusing idle connection from pool")
		return conn.client, nil
	}

	// No idle connections, try to create new one
	if p.active >= p.maxSize {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}

	p.active++
	p.mu.Unlock()

	// Create new connection
	klog.V(5).Info("Creating new connection")
	client, err := p.factory()
	if err != nil {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		p.metrics.incrementErrors()
		p.breaker.RecordFailure()
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	p.metrics.incrementTotal()
	p.breaker.RecordSuccess()
	p.mu.Lock()
	p.updateMetrics()
	p.mu.Unlock()

	return client, nil
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(client RDSClient) error {
	if client == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return client.Close()
	}

	p.active--

	// Check if we should keep this connection in idle pool
	if len(p.idle) >= p.maxIdle || !client.IsConnected() {
		klog.V(5).Info("Closing connection (pool full or disconnected)")
		p.updateMetrics()
		return client.Close()
	}

	// Add to idle pool
	p.idle = append(p.idle, pooledConnection{
		client:   client,
		lastUsed: time.Now(),
		inUse:    false,
	})

	klog.V(5).Infof("Returned connection to pool (idle: %d, active: %d)", len(p.idle), p.active)
	p.updateMetrics()
	return nil
}

// Close closes all connections and shuts down the pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Close all idle connections
	for _, conn := range p.idle {
		if err := conn.client.Close(); err != nil {
			klog.Warningf("Error closing idle connection: %v", err)
		}
	}
	p.idle = nil

	klog.V(4).Info("Connection pool closed")
	return nil
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() PoolMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return *p.metrics
}

// updateMetrics updates the metrics (must be called with lock held)
func (p *ConnectionPool) updateMetrics() {
	p.metrics.mu.Lock()
	p.metrics.activeConnections = p.active
	p.metrics.idleConnections = len(p.idle)
	p.metrics.mu.Unlock()
}

// PoolMetrics methods
func (m *PoolMetrics) incrementTotal() {
	m.mu.Lock()
	m.totalConnections++
	m.mu.Unlock()
}

func (m *PoolMetrics) incrementErrors() {
	m.mu.Lock()
	m.connectionErrors++
	m.mu.Unlock()
}

func (m *PoolMetrics) incrementCircuitBreaks() {
	m.mu.Lock()
	m.circuitBreaks++
	m.mu.Unlock()
}

func (m *PoolMetrics) incrementRateLimitHits() {
	m.mu.Lock()
	m.rateLimitHits++
	m.mu.Unlock()
}

func (m *PoolMetrics) recordWait(duration time.Duration) {
	m.mu.Lock()
	m.waitTimeTotal += duration
	m.waitCount++
	m.mu.Unlock()
}

// String returns a human-readable representation of metrics
func (m *PoolMetrics) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgWait := time.Duration(0)
	if m.waitCount > 0 {
		avgWait = m.waitTimeTotal / time.Duration(m.waitCount)
	}

	return fmt.Sprintf("Connections(total=%d, active=%d, idle=%d) Errors=%d CircuitBreaks=%d RateLimitHits=%d AvgWait=%v",
		m.totalConnections, m.activeConnections, m.idleConnections,
		m.connectionErrors, m.circuitBreaks, m.rateLimitHits, avgWait)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
		state:     CircuitClosed,
	}
}

// Allow checks if a request is allowed through the circuit breaker
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has elapsed
		if now.Sub(cb.lastFailTime) > cb.timeout {
			klog.V(4).Info("Circuit breaker: transitioning to half-open")
			cb.state = CircuitHalfOpen
			cb.failures = 0
			return true
		}
		klog.V(5).Info("Circuit breaker: open, rejecting request")
		return false

	case CircuitHalfOpen:
		// Allow one request through to test
		return true
	}

	return false
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		klog.V(4).Info("Circuit breaker: closing after successful test")
		cb.state = CircuitClosed
	}

	cb.failures = 0
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.failures >= cb.threshold {
		if cb.state != CircuitOpen {
			klog.Warningf("Circuit breaker: opening after %d failures", cb.failures)
			cb.state = CircuitOpen
		}
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
