package example

import (
	"context"
	"errors"
	"github.com/t34-dev/go-grpc-pool/example/pkg/api/example_v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
)

type Server struct {
	example_v1.UnimplementedExampleServiceServer
	address string
	server  *grpc.Server
}

func NewServer(address string) *Server {
	return &Server{
		address: address,
	}
}

func (s *Server) GetLen(ctx context.Context, in *example_v1.TxtRequest) (*example_v1.TxtResponse, error) {
	if in == nil {
		return nil, errors.New("GetLen: TxtRequest is empty")
	}

	return &example_v1.TxtResponse{
		Number: uint32(len(in.GetText())),
	}, nil
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	example_v1.RegisterExampleServiceServer(s.server, s)
	reflection.Register(s.server)

	log.Printf("Server is running on %s", s.address)
	return s.server.Serve(lis)
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}
