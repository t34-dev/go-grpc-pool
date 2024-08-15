package main

import (
	"context"
	"fmt"
	grpcpool "github.com/t34-dev/go-grpc-pool"
	constants "github.com/t34-dev/go-grpc-pool/example"
	"github.com/t34-dev/go-grpc-pool/example/api/exampleservice"
	"log"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer zapLogger.Sync()

	grpcPool, err := grpcpool.NewPool(
		func(ctx context.Context) (*grpc.ClientConn, error) {
			return grpc.DialContext(ctx, "localhost"+constants.Address,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
				grpc.WithTimeout(5*time.Second))
		},
		3, 10, 10*time.Second, 20*time.Second, nil, // grpcpool.Logger(zapLogger.Sugar()),
	)
	if err != nil {
		zapLogger.Fatal("Failed to create connection pool", zap.Error(err))
	}
	defer grpcPool.Close()

	// Проверяем начальное соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpcPool.Get(ctx)
	if err != nil {
		zapLogger.Fatal("Failed to get initial connection", zap.Error(err))
	}
	conn.Close() // Возвращаем соединение в пул

	testStrings := []string{
		"Hello, World!",
		"gRPC is awesome",
		"Периодический клиент",
		"Test string",
		"Another test",
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 0; ; i++ {
		<-ticker.C

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := grpcPool.Get(ctx)
		if err != nil {
			zapLogger.Error("Failed to get connection from pool", zap.Error(err))
			cancel()
			continue
		}

		client := exampleservice.NewExampleServiceClient(conn)

		testString := testStrings[i%len(testStrings)]

		response, err := client.GetLen(ctx, &exampleservice.TxtRequest{Text: testString})
		cancel()

		if err != nil {
			zapLogger.Error("Error calling GetLen", zap.Error(err), zap.String("testString", testString))
			conn.Unhealthy() // Помечаем соединение как нездоровое
			conn.Close()
			continue
		}

		zapLogger.Info(fmt.Sprintf("String: %s, Result: %d, pool:%d/%d", testString, response.GetNumber(), grpcPool.AvailableConn(), grpcPool.Capacity()))

		conn.Close() // Возвращаем соединение в пул
	}
}
