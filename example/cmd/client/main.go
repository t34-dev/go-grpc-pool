// main.go

package main

import (
	"context"
	"fmt"
	grpcpool "github.com/t34-dev/go-grpc-pool"
	constants "github.com/t34-dev/go-grpc-pool/example"
	"github.com/t34-dev/go-grpc-pool/example/pkg/api/example_v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"sync"
	"time"
)

// Global variables
var (
	zapLogger   *zap.Logger
	testStrings = []string{
		"Hello, World!",
		"gRPC is awesome",
		"Periodic client",
		"Test string",
		"Another test",
	}
)

// printStats prints the current statistics of the connection pool
func printStats(msg string, pool *grpcpool.Pool) {
	stats := pool.GetStats()
	fmt.Printf("%s: Pool stats: Min=%d, Max=%d, Current=%d, WorkConn=%d\n",
		msg, stats.MinConnections, stats.MaxConnections, stats.CurrentConnections, stats.WorkConnections)
}

func main() {
	// Initialize zap logger
	zapLog, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	zapLogger = zapLog
	defer zapLog.Sync()

	// Define the factory function for creating gRPC connections
	factory := func() (*grpc.ClientConn, error) {
		//opts := []grpc.DialOption{
		//	grpc.WithTransportCredentials(insecure.NewCredentials()),
		//}
		//return grpc.NewClient("localhost"+constants.Address, opts...)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		return grpc.DialContext(ctx, "localhost"+constants.Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
	}

	idleTimeout := 3 * time.Second

	// Create a new gRPC connection pool
	grpcPool, err := grpcpool.NewPool(factory, grpcpool.PoolOptions{
		MinConn:     2,
		MaxConn:     40,
		IdleTimeout: idleTimeout,
		WaitGetConn: 3 * time.Second,
		//Logger:      grpcpool.Logger(zapLogger.Sugar()),
	})
	if err != nil {
		log.Fatalf("Failed to create grpcPool: %v", err)
	}
	defer grpcPool.Close()
	time.Sleep(5 * time.Second)

	printStats("Init", grpcPool)

	// Run high or low test
	//highLoad(grpcPool)
	lowLoad(grpcPool)

	fmt.Println("Waiting for idle connections to close...")
	time.Sleep(idleTimeout + time.Second)
	printStats("End", grpcPool)
}

// highLoad simulates a high load scenario using multiple goroutines
func highLoad(pool *grpcpool.Pool) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1; j++ {
				// Get a connection from the pool
				conn, err := pool.Get()
				if err != nil {
					printStats(fmt.Sprintf("Goroutine [%d]: Failed Get: %v", i, err), pool)
					continue
				}

				// Use the connection to call the gRPC service
				testString := testStrings[i%len(testStrings)]
				client := example_v1.NewExampleServiceClient(conn.GetConn())
				response, err := client.GetLen(context.Background(), &example_v1.TxtRequest{Text: testString})
				if err != nil {
					printStats(fmt.Sprintf("Goroutine [%d]: Failed calling GetLen: %v", i, err), pool)
					continue
				}

				printStats(fmt.Sprintf("Goroutine [%d]: %s, Result: %d", i, testString, response.GetNumber()), pool)
				conn.Free() // Return the connection to the pool
			}
		}(i)
	}
	wg.Wait()
	printStats("Wait", pool)
}

// lowLoad simulates a low load scenario using a ticker
func lowLoad(pool *grpcpool.Pool) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; i < 50; i++ {
		<-ticker.C // Wait for the ticker
		for j := 0; j < 1; j++ {
			// Get a connection from the pool
			conn, err := pool.Get()
			if err != nil {
				printStats(fmt.Sprintf("Goroutine [%d]: Failed Get: %v", i, err), pool)
				continue
			}

			// Use the connection to call the gRPC service
			testString := testStrings[i%len(testStrings)]
			client := example_v1.NewExampleServiceClient(conn.GetConn())
			response, err := client.GetLen(context.Background(), &example_v1.TxtRequest{Text: testString})
			if err != nil {
				printStats(fmt.Sprintf("Goroutine [%d]: Failed calling GetLen: %v", i, err), pool)
				continue
			}

			printStats(fmt.Sprintf("Goroutine [%d]: %s, Result: %d", i, testString, response.GetNumber()), pool)
			conn.Free() // Return the connection to the pool
		}
	}
	printStats("Wait", pool)
}
