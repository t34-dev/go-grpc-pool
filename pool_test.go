package grpcpool

import (
	"context"
	"github.com/t34-dev/go-grpc-pool/example"
	"github.com/t34-dev/go-grpc-pool/example/api/exampleservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sync"
	"testing"
	"time"
)

const Address = ":50053"

func setupTestServer(t *testing.T) (*example.Server, func()) {
	server := example.NewServer(Address)
	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Даем серверу время на запуск
	time.Sleep(100 * time.Millisecond)

	return server, func() {
		server.Stop()
	}
}

func TestPoolConcurrentRequests(t *testing.T) {
	_, cleanup := setupTestServer(t)
	defer cleanup()

	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "localhost"+Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second))
	}

	pool, err := NewPool(factory, 2, 3, 5*time.Minute, 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	initialConn := pool.AvailableConn()
	t.Logf("Initial connections: %d", initialConn)
	if initialConn != 2 {
		t.Errorf("Expected 2 initial connections, got %d", initialConn)
	}

	var wg sync.WaitGroup
	wg.Add(3)

	makeRequest := func(id int) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := pool.Get(ctx)
		if err != nil {
			t.Errorf("Request %d: Failed to get connection: %v", id, err)
			return
		}
		t.Logf("Request %d: Got connection", id)

		client := exampleservice.NewExampleServiceClient(conn.ClientConn)
		_, err = client.GetLen(ctx, &exampleservice.TxtRequest{Text: "test"})
		if err != nil {
			t.Errorf("Request %d: Failed to make gRPC call: %v", id, err)
			return
		}

		t.Logf("Request %d: Successfully made gRPC call", id)

		time.Sleep(100 * time.Millisecond)

		err = conn.Close()
		if err != nil {
			t.Errorf("Request %d: Error closing connection: %v", id, err)
		} else {
			t.Logf("Request %d: Successfully closed connection", id)
		}

		t.Logf("Request %d: Available connections after close: %d", id, pool.AvailableConn())
	}

	t.Logf("Available connections before requests: %d", pool.AvailableConn())

	for i := 0; i < 3; i++ {
		go makeRequest(i)
	}

	wg.Wait()

	// Даем время на возврат соединений в пул
	time.Sleep(1 * time.Second)

	finalConn := pool.AvailableConn()
	t.Logf("Final connections: %d", finalConn)
	if finalConn != 3 {
		t.Errorf("Expected 3 connections after requests, got %d", finalConn)
	}

	t.Logf("Pool capacity: %d", pool.Capacity())
	t.Logf("Pool available connections: %d", pool.AvailableConn())

	// Дополнительная проверка: попробуем взять все соединения из пула
	var conns []*ClientConn
	for i := 0; i < 3; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil {
			t.Errorf("Failed to get connection %d after test: %v", i, err)
		} else {
			conns = append(conns, conn)
			t.Logf("Got connection %d after test", i)
		}
	}

	// Возвращаем соединения в пул
	for i, conn := range conns {
		err := conn.Close()
		if err != nil {
			t.Errorf("Failed to close connection %d after test: %v", i, err)
		} else {
			t.Logf("Closed connection %d after test", i)
		}
	}

	t.Logf("Final pool available connections: %d", pool.AvailableConn())
}

func TestPoolIdleTimeout(t *testing.T) {
	_, cleanup := setupTestServer(t)
	defer cleanup()

	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "localhost"+Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second))
	}

	// Устанавливаем короткий idleTimeout для целей тестирования
	idleTimeout := 3 * time.Second
	pool, err := NewPool(factory, 2, 3, idleTimeout, 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	initialConn := pool.AvailableConn()
	t.Logf("Initial connections: %d", initialConn)
	if initialConn != 2 {
		t.Errorf("Expected 2 initial connections, got %d", initialConn)
	}

	// Функция для выполнения запроса
	makeRequest := func(id int, wg *sync.WaitGroup) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := pool.Get(ctx)
		if err != nil {
			t.Errorf("Request %d: Failed to get connection: %v", id, err)
			return
		}
		defer conn.Close()

		client := exampleservice.NewExampleServiceClient(conn.ClientConn)
		_, err = client.GetLen(ctx, &exampleservice.TxtRequest{Text: "test"})
		if err != nil {
			t.Errorf("Request %d: Failed to make gRPC call: %v", id, err)
			return
		}
		t.Logf("Request %d: Successfully made gRPC call", id)
	}

	// Выполняем 3 запроса одновременно
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go makeRequest(i, &wg)
	}
	wg.Wait()

	// Даем небольшое время для возврата соединений в пул
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что пул расширился до 3 соединений
	expandedConn := pool.AvailableConn()
	t.Logf("Connections after requests: %d", expandedConn)
	if expandedConn != 3 {
		t.Errorf("Expected 3 connections after requests, got %d", expandedConn)
	}

	// Ждем, пока истечет idleTimeout
	time.Sleep(idleTimeout + 1*time.Second)

	// Проверяем, что количество соединений уменьшилось до начального значения
	// Добавляем цикл для ожидания уменьшения количества соединений
	var finalConn int
	for i := 0; i < 5; i++ { // пробуем 5 раз
		finalConn = pool.AvailableConn()
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
