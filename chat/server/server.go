package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type Server struct {
	pb.UnimplementedChatServer

	listener   net.Listener
	gRPCServer *grpc.Server

	mu      sync.RWMutex
	chatLog []*pb.Message
}

func (s *Server) Serve() error {
	return s.gRPCServer.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	gracefulShutdownSignal := make(chan struct{}, 1)
	go func() {
		s.gRPCServer.GracefulStop()
		close(gracefulShutdownSignal)
	}()

	select {
	case <-ctx.Done():
		log.Println("Graceful shutdown timed out, killing")
		s.gRPCServer.Stop()
	case <-gracefulShutdownSignal:
	}

	if err := s.listener.Close(); !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}

func NewServer(port int) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	s := &Server{
		listener:   listener,
		gRPCServer: grpc.NewServer(),
	}

	pb.RegisterChatServer(s.gRPCServer, s)

	return s, nil
}
