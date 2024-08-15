package example

import (
	"context"
	"errors"
	"github.com/t34-dev/go-grpc-pool/example/api/exampleservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
)

type Server struct {
	exampleservice.UnimplementedExampleServiceServer
	address string
	server  *grpc.Server
}

func NewServer(address string) *Server {
	return &Server{
		address: address,
	}
}

func (s *Server) GetLen(ctx context.Context, in *exampleservice.TxtRequest) (*exampleservice.TxtResponse, error) {
	if in == nil {
		return nil, errors.New("GetLen: TxtRequest is empty")
	}

	return &exampleservice.TxtResponse{
		Number: uint32(len(in.GetText())),
	}, nil
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	exampleservice.RegisterExampleServiceServer(s.server, s)
	reflection.Register(s.server)

	log.Printf("Server is running on %s", s.address)
	return s.server.Serve(lis)
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}
