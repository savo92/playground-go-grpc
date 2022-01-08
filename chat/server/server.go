package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/looplab/fsm"
	"google.golang.org/grpc"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type Server struct {
	pb.UnimplementedChatServer

	listener   net.Listener
	gRPCServer *grpc.Server

	rooms       map[roomID]*Room
	roomsMU     sync.RWMutex
	defaultRoom roomID
}

func (s *Server) Serve() error {
	return s.gRPCServer.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	gracefulShutdownSignal := make(chan struct{}, 1)
	go func() {
		// TODO here we should also kick handlers shutdown (channels? contexts?)
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
		rooms:      make(map[roomID]*Room),
	}

	pb.RegisterChatServer(s.gRPCServer, s)

	r, err := NewRoom("default")
	if err != nil {
		return nil, err
	}
	s.roomsMU.Lock()
	s.rooms[r.ID] = r
	s.defaultRoom = r.ID
	s.roomsMU.Unlock()

	return s, nil
}

func extractClientMsg(e *fsm.Event) (*pb.ClientMessage, error) {
	if len(e.Args) == 0 {
		return nil, fmt.Errorf("not enough Args")
	}
	cMsgP, ok := e.Args[0].(*pb.ClientMessage)
	if !ok {
		return nil, fmt.Errorf("type assertion to *pb.ClientMessage failed")
	}

	return cMsgP, nil
}
