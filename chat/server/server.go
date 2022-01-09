package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/looplab/fsm"
	"google.golang.org/grpc"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type Server struct {
	pb.UnimplementedChatServer

	listener   net.Listener
	gRPCServer *grpc.Server

	rm          *roomManager
	defaultRoom roomID
}

func (s *Server) Serve() error {
	return s.gRPCServer.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	gracefulShutdownSignal := make(chan struct{}, 1)
	go func() {
		s.rm.close()
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
	rm, err := newRoomManager()
	if err != nil {
		return nil, fmt.Errorf("newRoomManager failed: %w", err)
	}

	s := &Server{
		listener:   listener,
		gRPCServer: grpc.NewServer(),
		rm:         rm,
	}

	pb.RegisterChatServer(s.gRPCServer, s)

	rID, err := rm.createRoom()
	if err != nil {
		return nil, fmt.Errorf("createRoom failed: %w", err)
	}
	s.defaultRoom = rID

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
