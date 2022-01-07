package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var (
	port = flag.Int("port", 8081, "A port for the grpc server to listen to.")
)

var (
	listener   net.Listener
	gRPCServer *grpc.Server
)

type chatServer struct {
	pb.UnimplementedChatServer

	mu      sync.RWMutex
	chatLog []*pb.Message
}

func (s *chatServer) RouteChat(stream pb.Chat_RouteChatServer) error {
	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		for {
			in, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}

			s.mu.Lock()
			s.chatLog = append(s.chatLog, in)
			s.mu.Unlock()
		}
	})

	g.Go(func() error {
		mp := 0
		for {
			var newMessages *[]*pb.Message
			s.mu.RLock()
			chatLength := len(s.chatLog)
			if delta := chatLength - mp; delta > 0 {
				newMsgs := make([]*pb.Message, delta)
				copy(newMsgs, s.chatLog[mp:])
				newMessages = &newMsgs
			}
			s.mu.RUnlock()

			if newMessages != nil {
				for _, msg := range *newMessages {
					if err := stream.Send(msg); err != nil {
						return err
					}
				}
				mp = chatLength
			}
		}
	})

	return g.Wait()
}

func main() {
	if err := setup(); err != nil {
		log.Fatalf("Error from setup: %s", err)
	}

	defer func() {
		if err := teardown(); err != nil {
			log.Fatalf("Error from teardown: %s", err)
		}
	}()
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		gRPCServer.GracefulStop()
	}()

	if err := run(); err != nil {
		log.Fatalf("Error from run: %s", err)
	}
}

func setup() error {
	var err error
	if listener, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", *port)); err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	gRPCServer = grpc.NewServer()
	pb.RegisterChatServer(gRPCServer, &chatServer{})

	return nil
}

func run() error {
	log.Printf("server listening at %v", listener.Addr())

	return gRPCServer.Serve(listener)
}

func teardown() error {
	return listener.Close()
}
