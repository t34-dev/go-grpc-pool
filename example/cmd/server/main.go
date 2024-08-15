package main

import (
	"github.com/t34-dev/go-grpc-pool/example"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	server := example.NewServer(example.Address)

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	server.Stop()

	log.Println("Server exiting")
}
