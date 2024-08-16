package grpcpool

import (
	"errors"
	"fmt"
	"google.golang.org/grpc/connectivity"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
)

const (
	waitGetDefault     = 2 * time.Second
	idleTimeoutDefault = time.Minute
)

// Error variables for common error scenarios
var (
	ErrOptionMinOrMax  = errors.New("invalid MinConn or MaxConn")
	ErrConnection      = errors.New("Connection is not ready")
	ErrClosed          = errors.New("client pool is closed")
	ErrNoAvailableConn = errors.New("no available connections")
	ErrTimeout         = errors.New("client pool timeout exceeded")
	ErrAlreadyClosed   = errors.New("Connection was already closed")
	ErrFullPool        = errors.New("attempt to destroy ClientConn in a full pool")
)

// Logger - interface for logger
type Logger interface {
	Debug(msg ...interface{})
	Error(msg ...interface{})
}

// Connection represents a single gRPC connection in the pool
type Connection struct {
	conn              *grpc.ClientConn
	isWork            bool
	isUse             bool
	lastUsedTimestamp time.Time
	pool              *Pool
}

// Free marks the connection as not in use and updates the last used timestamp
func (c *Connection) Free() {
	c.pool.mu.Lock()
	defer c.pool.mu.Unlock()

	c.isUse = false
	c.lastUsedTimestamp = time.Now()
	if c.pool.logger != nil {
		c.pool.logger.Debug("Connection freed")
	}
}

// IsReady checks if the connection is ready for use
func (c *Connection) IsReady() bool {
	if c.conn == nil {
		return false
	}
	defer func() {
		if r := recover(); r != nil {
			c.isWork = false
			if c.pool.logger != nil {
				c.pool.logger.Error("Panic in IsReady:", r)
			}
		}
	}()
	state := c.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// GetConn returns the underlying gRPC client connection
func (c *Connection) GetConn() *grpc.ClientConn {
	return c.conn
}

// destroy closes the connection if it's not already shut down
func (c *Connection) destroy() error {
	if c.conn.GetState() != connectivity.Shutdown {
		if c.pool.logger != nil {
			c.pool.logger.Debug("Destroying connection")
		}
		return c.conn.Close()
	}
	return nil
}

// connectionFactory is a function type for creating new gRPC client connections
type connectionFactory func() (*grpc.ClientConn, error)

// poolStats represents statistics about the connection pool
type poolStats struct {
	MinConnections     int
	MaxConnections     int
	CurrentConnections int
	WorkConnections    int
}

// PoolOptions contains configuration options for the connection pool
type PoolOptions struct {
	MinConn     int
	MaxConn     int
	WaitGetConn time.Duration
	IdleTimeout time.Duration
	Logger      Logger
}

// Pool represents a pool of gRPC client connections
type Pool struct {
	mu            sync.RWMutex
	connections   []*Connection
	factory       connectionFactory
	logger        Logger
	cleanupTicker *time.Ticker
	options       PoolOptions
}

// NewPool creates a new connection pool with the given factory and options
func NewPool(factory connectionFactory, options PoolOptions) (*Pool, error) {
	if options.MinConn < 0 || options.MaxConn < options.MinConn {
		return nil, ErrOptionMinOrMax
	}
	newOptions := options
	if newOptions.MinConn <= 0 {
		newOptions.MinConn = 1
	}
	if newOptions.MaxConn <= 0 {
		newOptions.MaxConn = 1
	}
	if newOptions.WaitGetConn <= 0 {
		newOptions.WaitGetConn = waitGetDefault
	}
	if newOptions.IdleTimeout <= 0 {
		newOptions.IdleTimeout = idleTimeoutDefault
	}

	p := &Pool{
		factory:     factory,
		connections: make([]*Connection, 0, newOptions.MaxConn),
		options:     newOptions,
		logger:      newOptions.Logger,
	}

	if p.logger != nil {
		p.logger.Debug("Creating new connection pool")
	}

	p.Init()
	p.cleanupTicker = time.NewTicker(time.Second)
	go p.cleanup()

	return p, nil
}

// Get retrieves an available connection from the pool or creates a new one if possible
func (p *Pool) Get() (*Connection, error) {
	startTime := time.Now()

	// Channel to signal availability of connection
	availableConn := make(chan *Connection, 1)
	timeout := make(chan struct{}, 1)

	go func() {
		for {
			p.mu.Lock()

			// Sort connections by last used timestamp (from newest to oldest)
			sort.Slice(p.connections, func(i, j int) bool {
				return p.connections[i].lastUsedTimestamp.After(p.connections[j].lastUsedTimestamp)
			})

			for _, conn := range p.connections {
				if !conn.isUse {
					conn.isUse = true
					conn.lastUsedTimestamp = time.Now()
					p.mu.Unlock()
					availableConn <- conn
					return
				}
			}

			// If we can't create a new connection, we have to wait
			if len(p.connections) < p.options.MaxConn {
				newConn, err := p.createConnection()
				if err != nil {
					if p.logger != nil {
						p.logger.Error("Failed to create new connection:", err)
					}
					p.mu.Unlock()
					availableConn <- nil
					return
				}
				p.connections = append(p.connections, newConn)
				p.mu.Unlock()
				availableConn <- newConn
				return
			}

			// Unlock the mutex and let the main routine handle the wait
			p.mu.Unlock()
			time.Sleep(100 * time.Millisecond)

			if time.Since(startTime) >= p.options.WaitGetConn {
				timeout <- struct{}{}
				return
			}
		}
	}()

	select {
	case conn := <-availableConn:
		if conn == nil {
			return nil, ErrConnection
		}
		if p.logger != nil {
			p.logger.Debug("Reusing existing or created connection")
		}
		return conn, nil
	case <-timeout:
		if p.logger != nil {
			p.logger.Error("Timeout waiting for available connection")
		}
		return nil, ErrNoAvailableConn
	}
}

// GetStats returns the current statistics of the connection pool
func (p *Pool) GetStats() poolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	workConnections := 0
	for _, conn := range p.connections {
		if conn.isWork {
			workConnections += 1
		}
	}
	stats := poolStats{
		MinConnections:     p.options.MinConn,
		MaxConnections:     p.options.MaxConn,
		CurrentConnections: len(p.connections),
		WorkConnections:    workConnections,
	}
	if p.logger != nil {
		p.logger.Debug(fmt.Sprintf("Pool stats: %+v", stats))
	}
	return stats
}

// createConnection creates a new connection using the factory
func (p *Pool) createConnection() (*Connection, error) {
	cc, err := p.factory()
	if err != nil {
		if p.logger != nil {
			p.logger.Error("Failed to create connection:", err)
		}
		return nil, err
	}
	conn := &Connection{conn: cc, pool: p, isWork: true, lastUsedTimestamp: time.Now()}
	if !conn.IsReady() {
		if p.logger != nil {
			p.logger.Error("New connection is not ready")
		}
		return nil, ErrConnection
	}
	if p.logger != nil {
		p.logger.Debug("Created new connection")
	}
	return conn, nil
}

// Init initializes the pool with the minimum number of connections
func (p *Pool) Init() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.connections) < p.options.MinConn {
		for i := 0; i < p.options.MinConn; i++ {
			newConn, err := p.createConnection()
			if err != nil {
				if p.logger != nil {
					p.logger.Error("Failed to initialize connection:", err)
				}
				return
			}
			p.connections = append(p.connections, newConn)
		}
		if p.logger != nil {
			p.logger.Debug(fmt.Sprintf("Initialized pool with %d connections", p.options.MinConn))
		}
	}
}

// cleanup periodically removes idle connections and ensures the minimum number of connections
func (p *Pool) cleanup() {
	for range p.cleanupTicker.C {
		p.mu.Lock()
		now := time.Now()
		activeCount := 0
		var expiredConns []*Connection

		// First pass: count active connections and collect expired ones
		for _, conn := range p.connections {
			if !conn.isUse && now.Sub(conn.lastUsedTimestamp) > p.options.IdleTimeout {
				expiredConns = append(expiredConns, conn)
			} else {
				activeCount++
			}
		}

		// Second pass: remove or refresh connections
		for _, conn := range expiredConns {
			if activeCount < p.options.MinConn {
				// Refresh the connection instead of removing it
				if conn.IsReady() {
					conn.lastUsedTimestamp = now
					activeCount++
					if p.logger != nil {
						p.logger.Debug("Refreshed idle connection")
					}
				} else {
					// If the connection is not ready, try to recreate it
					newConn, err := p.createConnection()
					if err == nil {
						*conn = *newConn
						activeCount++
						if p.logger != nil {
							p.logger.Debug("Recreated expired connection")
						}
					} else if p.logger != nil {
						p.logger.Error("Failed to recreate expired connection:", err)
					}
				}
			} else {
				// Remove the connection
				for i, c := range p.connections {
					if c == conn {
						p.connections = append(p.connections[:i], p.connections[i+1:]...)
						conn.destroy()
						if p.logger != nil {
							p.logger.Debug("Removed expired connection")
						}
						break
					}
				}
			}
		}

		p.mu.Unlock()

		if p.logger != nil {
			p.logger.Debug(fmt.Sprintf("Cleanup complete, active connections: %d", len(p.connections)))
		}
	}
}

// Close shuts down the pool and all its connections
func (p *Pool) Close() {
	p.cleanupTicker.Stop()
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		conn.destroy()
	}
	p.connections = nil
	if p.logger != nil {
		p.logger.Debug("Connection pool closed")
	}
}
