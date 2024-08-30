package grpcpool

import (
	"context"
	"github.com/t34-dev/go-grpc-pool/example"
	"github.com/t34-dev/go-grpc-pool/example/pkg/api/example_v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sync"
	"testing"
	"time"
)

const Address = ":50053"

// setupTestServer creates and starts a test server, returning a function to stop it
func setupTestServer(t *testing.T) (*example.Server, func()) {
	server := example.NewServer(Address)
	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	return server, func() {
		server.Stop()
	}
}

// TestPoolIdleTimeout tests the idle timeout functionality of the connection pool
func TestPoolIdleTimeout(t *testing.T) {
	_, cleanup := setupTestServer(t)
	defer cleanup()

	// Factory for creating gRPC connections
	factory := func() (*grpc.ClientConn, error) {
		//opts := []grpc.DialOption{
		//	grpc.WithTransportCredentials(insecure.NewCredentials()),
		//}
		//return grpc.NewClient("localhost"+constants.Address, opts...)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		return grpc.DialContext(ctx, "localhost"+Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
	}

	// Set a short idleTimeout for testing purposes
	idleTimeout := 4 * time.Second
	pool, err := NewPool(factory, PoolOptions{
		MinConn:     2,
		MaxConn:     3,
		IdleTimeout: idleTimeout,
		WaitGetConn: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Check initial connection count
	initialConn := pool.GetStats().CurrentConnections
	t.Logf("Initial connections: %d", initialConn)
	if initialConn != 2 {
		t.Errorf("Expected 2 initial connections, got %d", initialConn)
	}

	// Function to make a request
	makeRequest := func(id int, wg *sync.WaitGroup) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := pool.Get()
		if err != nil {
			t.Errorf("Request %d: Failed to get Connection: %v", id, err)
			return
		}
		defer conn.Free()

		client := example_v1.NewExampleServiceClient(conn.GetConn())
		_, err = client.GetLen(ctx, &example_v1.TxtRequest{Text: "test"})
		if err != nil {
			t.Errorf("Request %d: Failed to make gRPC call: %v", id, err)
			return
		}
		t.Logf("Request %d: Successfully made gRPC call", id)
	}

	// Make 3 concurrent requests
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go makeRequest(i, &wg)
	}
	wg.Wait()

	// Allow time for connections to return to the pool
	time.Sleep(100 * time.Millisecond)

	// Check that the pool expanded to 3 connections
	expandedConn := pool.GetStats().CurrentConnections
	t.Logf("Connections after requests: %d", expandedConn)
	if expandedConn != 3 {
		t.Errorf("Expected 3 connections after requests, got %d", expandedConn)
	}

	// Wait for idleTimeout to expire
	time.Sleep(idleTimeout + 2*time.Second)

	// Check that the number of connections decreased to the initial value
	// Add a loop to wait for the number of connections to decrease
	var finalConn int
	for i := 0; i < 5; i++ { // try 5 times
		finalConn = pool.GetStats().CurrentConnections
		if finalConn == 2 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	t.Logf("Final connections after idle timeout: %d", finalConn)
	if finalConn != 2 {
		t.Errorf("Expected 2 connections after idle timeout, got %d", finalConn)
	}
}

// TestPoolConcurrentRequests tests the connection pool with concurrent requests
func TestPoolConcurrentRequests(t *testing.T) {
	_, cleanup := setupTestServer(t)
	defer cleanup()

	// Factory for creating gRPC connections
	factory := func() (*grpc.ClientConn, error) {
		return grpc.Dial("localhost"+Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second))
	}

	// Create connection pool
	pool, err := NewPool(factory, PoolOptions{
		MinConn:     2,
		MaxConn:     3,
		IdleTimeout: 5 * time.Minute,
		WaitGetConn: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Check initial connection count
	initialConn := pool.GetStats().CurrentConnections
	t.Logf("Initial connections: %d", initialConn)
	if initialConn != 2 {
		t.Errorf("Expected 2 initial connections, got %d", initialConn)
	}

	var wg sync.WaitGroup
	wg.Add(3)

	// Function to make a request
	makeRequest := func(id int) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := pool.Get()
		if err != nil {
			t.Errorf("Request %d: Failed to get Connection: %v", id, err)
			return
		}
		t.Logf("Request %d: Got Connection", id)

		client := example_v1.NewExampleServiceClient(conn.GetConn())
		_, err = client.GetLen(ctx, &example_v1.TxtRequest{Text: "test"})
		if err != nil {
			t.Errorf("Request %d: Failed to make gRPC call: %v", id, err)
			return
		}

		t.Logf("Request %d: Successfully made gRPC call", id)

		time.Sleep(100 * time.Millisecond)

		conn.Free()
		if err != nil {
			t.Errorf("Request %d: Error closing Connection: %v", id, err)
		} else {
			t.Logf("Request %d: Successfully closed Connection", id)
		}

		t.Logf("Request %d: Available connections after destroy: %d", id, pool.GetStats().CurrentConnections)
	}

	t.Logf("Available connections before requests: %d", pool.GetStats().CurrentConnections)

	// Make concurrent requests
	for i := 0; i < 3; i++ {
		go makeRequest(i)
	}

	wg.Wait()

	// Allow time for connections to return to the pool
	time.Sleep(1 * time.Second)

	// Check final connection count
	finalConn := pool.GetStats().CurrentConnections
	t.Logf("Final connections: %d", finalConn)
	if finalConn != 3 {
		t.Errorf("Expected 3 connections after requests, got %d", finalConn)
	}

	t.Logf("Pool capacity: %d", pool.GetStats().MaxConnections)
	t.Logf("Pool available connections: %d", pool.GetStats().CurrentConnections)

	// Additional check: try to take all connections from the pool
	var conns []*Connection
	for i := 0; i < 3; i++ {
		conn, err := pool.Get()
		if err != nil {
			t.Errorf("Failed to get Connection %d after test: %v", i, err)
		} else {
			conns = append(conns, conn)
			t.Logf("Got Connection %d after test", i)
		}
	}

	// Return connections to the pool
	for i, conn := range conns {
		conn.Free()
		t.Logf("Closed Connection %d after test", i)
	}

	t.Logf("Final pool available connections: %d", pool.GetStats().CurrentConnections)
}

// TestPoolGetTimeout tests the timeout behavior when no connections are available
func TestPoolGetTimeout(t *testing.T) {
	_, cleanup := setupTestServer(t)
	defer cleanup()

	// Factory for creating gRPC connections
	factory := func() (*grpc.ClientConn, error) {
		return grpc.Dial("localhost"+Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second))
	}

	// Create connection pool with a short timeout
	pool, err := NewPool(factory, PoolOptions{
		MinConn:     1,
		MaxConn:     1,
		IdleTimeout: 5 * time.Minute,
		WaitGetConn: 2 * time.Second, // Set a short wait time for testing timeout
	})
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Get the first connection, which should succeed
	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get initial Connection: %v", err)
	}
	defer conn.Free()

	// Try to get another connection, which should timeout
	_, err = pool.Get()
	if err != ErrNoAvailableConn {
		t.Errorf("Expected ErrNoAvailableConn, got: %v", err)
	}
}
