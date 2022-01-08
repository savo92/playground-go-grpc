package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

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

var (
	ErrQuit = errors.New("EOF")
)

type chatServer struct {
	pb.UnimplementedChatServer

	mu      sync.RWMutex
	chatLog []*pb.Message
}

func (s *chatServer) RouteChat(stream pb.Chat_RouteChatServer) error {
	g := errgroup.Group{}

	g.Go(func() error {
		for {
			message, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return ErrQuit
			}
			if err != nil {
				return err
			}

			s.mu.Lock()
			s.chatLog = append(s.chatLog, message)
			s.mu.Unlock()
		}
	})

	g.Go(func() error {
		nextMessageI := 0
		for {
			var newMessages *[]*pb.Message
			s.mu.RLock()
			chatLength := len(s.chatLog)
			if delta := chatLength - nextMessageI; delta > 0 {
				newMsgs := make([]*pb.Message, delta)
				copy(newMsgs, s.chatLog[nextMessageI:])
				newMessages = &newMsgs
			}
			s.mu.RUnlock()

			if newMessages != nil {
				for _, msg := range *newMessages {
					if err := stream.Send(msg); err != nil {
						return err
					}
				}
				nextMessageI = chatLength
			}
		}
	})

	if err := g.Wait(); !errors.Is(err, ErrQuit) {
		return err
	}

	return nil
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
		log.Println("SIGINT received, shutting down")

		go func() {
			ticker := time.NewTicker(10 * time.Second)
			<-ticker.C
			log.Println("Graceful shutdown timed out, killing")
			ticker.Stop()
			gRPCServer.Stop()
		}()

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
	if err := listener.Close(); !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}
