[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Coverage Status](https://coveralls.io/repos/github/t34-dev/go-grpc-pool/badge.svg?branch=main&ver=1723825072)](https://coveralls.io/github/t34-dev/go-grpc-pool?branch=main&ver=1723825072)
![Go Version](https://img.shields.io/badge/Go-1.22-blue?logo=go&ver=1723825072)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/t34-dev/go-grpc-pool?ver=1723825072)
![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/t34-dev/go-grpc-pool?sort=semver&style=flat&logo=git&logoColor=white&label=Latest%20Version&color=blue&ver=1723825072)

# gRPC Connection Pool

This package provides a connection pool for gRPC clients in Go, allowing efficient management and reuse of gRPC connections.

![TypeScript WebSocket Client Logo](./example.gif)

## Features

- Configurable minimum and maximum number of connections
- Automatic connection creation and cleanup
- Idle timeout for unused connections
- Concurrent-safe connection management
- Simple API for getting and releasing connections

## Installation

To install the gRPC connection pool package, use the following command:

```bash
go get github.com/t34-dev/go-grpc-pool
```

Ensure that you have Go modules enabled in your project.

## Usage

Here's a basic example of how to use the gRPC connection pool in your project:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/t34-dev/go-grpc-pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Define a factory function to create gRPC connections
	factory := func() (*grpc.ClientConn, error) {
		return grpc.Dial("localhost:50053",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second))
	}

	// Create a new connection pool
	pool, err := grpcpool.NewPool(factory, grpcpool.PoolOptions{
		MinConn:     2,
		MaxConn:     10,
		IdleTimeout: 5 * time.Minute,
		WaitGetConn: 20 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Get a connection from the pool
	conn, err := pool.Get()
	if err != nil {
		log.Fatalf("Failed to get connection: %v", err)
	}
	defer conn.Free() // Return the connection to the pool when done

	// Use the connection to make gRPC calls
	client := yourpackage.NewYourServiceClient(conn.GetConn())
	resp, err := client.YourMethod(context.Background(), &yourpackage.YourRequest{})
	if err != nil {
		log.Fatalf("Error calling YourMethod: %v", err)
	}

	log.Printf("Response: %v", resp)
}
```

## Configuration

The `PoolOptions` struct allows you to configure the connection pool:

- `MinConn`: Minimum number of connections to maintain in the pool
- `MaxConn`: Maximum number of connections allowed in the pool
- `IdleTimeout`: Duration after which idle connections are closed
- `WaitGetConn`: Maximum duration to wait for an available connection
- `Logger`: Optional logger interface for debugging

## Testing

To run the tests for this package, use the following command:

```bash
make test
```

## Development

To generate protobuf files (if needed):

```bash
make protoc
```

To run the example server:

```bash
make server
```

To run the example client:

```bash
make client
```

## License

This project is licensed under the ISC License. See the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.


---

Developed with ❤️ by [T34](https://github.com/t34-dev)
