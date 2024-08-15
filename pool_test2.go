package grpcpool

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestNewPool(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, err := NewPool(factory, 2, 5, time.Second, 5*time.Minute, nil)
	if err != nil {
		t.Fatalf("NewPool returned an error: %v", err)
	}

	if p.Capacity() != 5 {
		t.Errorf("Expected capacity 5, got %d", p.Capacity())
	}

	if p.AvailableConn() != 2 {
		t.Errorf("Expected 2 available connections, got %d", p.AvailableConn())
	}
}

func TestGet(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, _ := NewPool(factory, 1, 3, time.Second, 5*time.Minute, nil)

	conn1, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned an error: %v", err)
	}
	if p.AvailableConn() != 0 {
		t.Errorf("Expected 0 available connections, got %d", p.AvailableConn())
	}

	conn2, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned an error: %v", err)
	}
	if p.AvailableConn() != 0 {
		t.Errorf("Expected 0 available connections, got %d", p.AvailableConn())
	}

	conn1.Close()
	if p.AvailableConn() != 1 {
		t.Errorf("Expected 1 available connection, got %d", p.AvailableConn())
	}

	conn2.Close()
	if p.AvailableConn() != 2 {
		t.Errorf("Expected 2 available connections, got %d", p.AvailableConn())
	}
}

func TestTimeout(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, _ := NewPool(factory, 1, 1, time.Second, 5*time.Minute, nil)

	_, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned an error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = p.Get(ctx)
	if err != ErrTimeout {
		t.Errorf("Expected ErrTimeout, got %v", err)
	}
}

func TestUnhealthy(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, _ := NewPool(factory, 1, 1, time.Second, 5*time.Minute, nil)

	conn, _ := p.Get(context.Background())
	conn.Unhealthy()
	conn.Close()

	newConn, _ := p.Get(context.Background())
	if newConn == conn {
		t.Error("Expected a new connection, got the same unhealthy connection")
	}
}

func TestMaxLifeDuration(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, _ := NewPool(factory, 1, 1, time.Second, 50*time.Millisecond, nil)

	conn, _ := p.Get(context.Background())
	time.Sleep(100 * time.Millisecond)
	conn.Close()

	newConn, _ := p.Get(context.Background())
	if newConn == conn {
		t.Error("Expected a new connection due to max life duration, got the same connection")
	}
}

func TestPoolClose(t *testing.T) {
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "example.com", grpc.WithInsecure())
	}

	p, _ := NewPool(factory, 1, 1, time.Second, 5*time.Minute, nil)

	conn, _ := p.Get(context.Background())
	p.Close()

	if conn.GetState() != connectivity.Shutdown {
		t.Error("Connection should be in Shutdown state after pool is closed")
	}

	if !p.IsClosed() {
		t.Error("Pool should be closed")
	}

	_, err := p.Get(context.Background())
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}
}
