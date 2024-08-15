package grpcpool

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"sync"
	"sync/atomic"
	"time"
)

// Error variables for common error scenarios
var (
	ErrClosed        = errors.New("grpc connector: client pool is closed")
	ErrTimeout       = errors.New("grpc connector: client pool timeout exceeded")
	ErrAlreadyClosed = errors.New("grpc connector: connection was already closed")
	ErrFullPool      = errors.New("grpc connector: attempt to close ClientConn in a full pool")
)

// Logger - interface for logger
type Logger interface {
	Debug(msg ...interface{})
	Error(msg ...interface{})
}

// Factory - a function type that creates a new gRPC client connection
type Factory func(ctx context.Context) (*grpc.ClientConn, error)

// Pool represents a pool of gRPC client connections
type Pool struct {
	factory         Factory
	clients         chan *ClientConn
	idleTimeout     time.Duration
	maxLifeDuration time.Duration
	capacity        int
	initialClients  int
	logger          Logger
	mu              sync.RWMutex
	cleanupTicker   *time.Ticker
	done            chan struct{}
	availableCount  int32
	totalCount      int32
}

// ClientConn wraps gRPC ClientConn with additional metadata
type ClientConn struct {
	*grpc.ClientConn
	pool          *Pool
	timeUsed      time.Time
	timeInitiated time.Time
	unhealthy     bool
}

// Close returns the connection to the pool or closes it if it's unhealthy or expired
func (cc *ClientConn) Close() error {
	if cc.ClientConn == nil {
		return ErrAlreadyClosed
	}

	cc.pool.mu.RLock()
	defer cc.pool.mu.RUnlock()

	if cc.pool.clients == nil {
		return ErrClosed
	}

	if cc.unhealthy || cc.pool.isExpired(cc) {
		cc.ClientConn.Close()
		cc.ClientConn = nil
		atomic.AddInt32(&cc.pool.totalCount, -1)
		cc.pool.logf("Closed unhealthy or expired connection. Available: %d, Total: %d", cc.pool.availableCount, cc.pool.totalCount)
		return nil
	}

	select {
	case cc.pool.clients <- cc:
		cc.timeUsed = time.Now()
		atomic.AddInt32(&cc.pool.availableCount, 1)
		cc.pool.logf("Returned connection to pool. Available: %d, Total: %d", cc.pool.availableCount, cc.pool.totalCount)
		return nil
	default:
		cc.ClientConn.Close()
		atomic.AddInt32(&cc.pool.totalCount, -1)
		cc.pool.logf("Closed excess connection. Available: %d, Total: %d", cc.pool.availableCount, cc.pool.totalCount)
		return ErrFullPool
	}
}

// Unhealthy marks the connection as unhealthy
func (cc *ClientConn) Unhealthy() {
	cc.unhealthy = true
}

// NewPool creates a new connection pool with specified parameters
func NewPool(factory Factory, initialClients, capacity int, idleTimeout, maxLifeDuration time.Duration, logger Logger) (*Pool, error) {
	if capacity <= 0 {
		capacity = 1
	}
	if initialClients < 0 {
		initialClients = 0
	}
	if initialClients > capacity {
		initialClients = capacity
	}

	p := &Pool{
		factory:         factory,
		clients:         make(chan *ClientConn, capacity),
		idleTimeout:     idleTimeout,
		maxLifeDuration: maxLifeDuration,
		capacity:        capacity,
		initialClients:  initialClients,
		logger:          logger,
		cleanupTicker:   time.NewTicker(idleTimeout / 2),
		done:            make(chan struct{}),
	}

	for i := 0; i < initialClients; i++ {
		if err := p.createConn(); err != nil {
			p.Close()
			return nil, err
		}
	}

	go p.cleanupLoop()

	return p, nil
}

func (p *Pool) createConn() error {
	cc, err := p.factory(context.Background())
	if err != nil {
		return err
	}
	p.clients <- &ClientConn{
		ClientConn:    cc,
		pool:          p,
		timeUsed:      time.Now(),
		timeInitiated: time.Now(),
	}
	atomic.AddInt32(&p.availableCount, 1)
	atomic.AddInt32(&p.totalCount, 1)
	p.logf("Created new connection. Available: %d, Total: %d", p.availableCount, p.totalCount)
	return nil
}

// Get retrieves a connection from the pool or creates a new one if necessary
func (p *Pool) Get(ctx context.Context) (*ClientConn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.clients == nil {
		return nil, ErrClosed
	}

	for {
		select {
		case cc := <-p.clients:
			if cc == nil {
				return nil, ErrClosed
			}
			if cc.ClientConn == nil || p.isExpired(cc) {
				p.logf("Connection expired or nil. Recreating...")
				cc.ClientConn.Close()
				newCC, err := p.factory(ctx)
				if err != nil {
					atomic.AddInt32(&p.availableCount, -1)
					atomic.AddInt32(&p.totalCount, -1)
					p.logf("Failed to recreate connection. Available: %d, Total: %d", p.availableCount, p.totalCount)
					return nil, err
				}
				cc.ClientConn = newCC
				cc.timeInitiated = time.Now()
			}
			cc.timeUsed = time.Now()
			atomic.AddInt32(&p.availableCount, -1)
			p.logf("Got connection from pool. Available: %d, Total: %d", p.availableCount, p.totalCount)
			return cc, nil
		default:
			if atomic.LoadInt32(&p.totalCount) < int32(p.capacity) {
				p.logf("Creating new connection as pool is not at capacity")
				if err := p.createConn(); err != nil {
					return nil, err
				}
				continue
			}
			p.logf("Waiting for available connection")
			select {
			case cc := <-p.clients:
				if cc == nil {
					return nil, ErrClosed
				}
				if cc.ClientConn == nil || p.isExpired(cc) {
					newCC, err := p.factory(ctx)
					if err != nil {
						atomic.AddInt32(&p.availableCount, -1)
						return nil, err
					}
					cc.ClientConn = newCC
					cc.timeInitiated = time.Now()
				}
				cc.timeUsed = time.Now()
				atomic.AddInt32(&p.availableCount, -1)
				return cc, nil
			case <-ctx.Done():
				return nil, ErrTimeout
			}
		}
	}
}

// isExpired checks if the connection has exceeded its idle time or maximum lifetime
func (p *Pool) isExpired(cc *ClientConn) bool {
	if p.idleTimeout > 0 && cc.timeUsed.Add(p.idleTimeout).Before(time.Now()) {
		return true
	}
	if p.maxLifeDuration > 0 && cc.timeInitiated.Add(p.maxLifeDuration).Before(time.Now()) {
		return true
	}
	return false
}

// Close closes all connections in the pool and prevents further use
func (p *Pool) Close() {
	p.mu.Lock()
	if p.clients == nil {
		p.mu.Unlock()
		return
	}
	clients := p.clients
	p.clients = nil
	p.mu.Unlock()

	close(p.done)
	p.cleanupTicker.Stop()

	close(clients)
	for cc := range clients {
		if cc.ClientConn != nil {
			cc.ClientConn.Close()
		}
	}
}

// IsClosed checks if the pool has been closed
func (p *Pool) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients == nil
}

// Capacity returns the maximum number of connections the pool can hold
func (p *Pool) Capacity() int {
	return p.capacity
}

// AvailableConn returns the number of connections currently available in the pool
func (p *Pool) AvailableConn() int {
	return len(p.clients)
}

// cleanupLoop starts periodic cleanup of unused connections
func (p *Pool) cleanupLoop() {
	for {
		select {
		case <-p.cleanupTicker.C:
			p.cleanupIdleConnections()
		case <-p.done:
			return
		}
	}
}

// cleanupIdleConnections closes unused connections
func (p *Pool) cleanupIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.clients == nil {
		return
	}

	var activeConns []*ClientConn
	for {
		select {
		case cc := <-p.clients:
			if cc == nil {
				return
			}
			if p.isExpired(cc) && atomic.LoadInt32(&p.totalCount) > int32(p.initialClients) {
				cc.ClientConn.Close()
				atomic.AddInt32(&p.availableCount, -1)
				atomic.AddInt32(&p.totalCount, -1)
				p.logf("Cleaned up idle connection. Available: %d, Total: %d", p.availableCount, p.totalCount)
			} else {
				activeConns = append(activeConns, cc)
			}
		default:
			for _, cc := range activeConns {
				p.clients <- cc
			}
			return
		}
	}
}

func (p *Pool) logf(format string, args ...interface{}) {
	if p.logger != nil {
		p.logger.Debug(fmt.Sprintf(format, args...))
	}
}
