package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serverAddr = flag.String("server-addr", "localhost:8081", "The address of the chat server.")
)

var (
	author string
	conn   *grpc.ClientConn
	client pb.ChatClient
)

func main() {
	if err := setup(); err != nil {
		log.Fatalf("Error from setup: %s", err)
	}

	defer func() {
		if err := teardown(); err != nil {
			log.Fatalf("Error from teardown: %s", err)
		}
	}()

	if err := run(); err != nil {
		log.Fatalf("Error from run: %s", err)
	}
}

func setup() error {
	var err error
	credentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if conn, err = grpc.Dial(*serverAddr, credentials); err != nil {
		return err
	}

	client = pb.NewChatClient(conn)

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your name: ")
	text, _ := reader.ReadString('\n')
	author = strings.Replace(text, "\n", "", -1)

	return nil
}

func run() error {
	sigint := make(chan interface{}, 1)
	ctx, cancelFunc := context.WithCancel(context.Background())
	stream, err := client.RouteChat(ctx)
	if err != nil {
		cancelFunc()

		return fmt.Errorf("stream acquisition: %w", err)
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			log.Printf("Failed to CloseSend: %v", err)
		}
	}()
	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		for {
			message, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}
			if message.Author == author {
				continue
			}

			fmt.Fprintf(os.Stdout, "%s: %s\n", message.Author, message.Body)
		}
	})

	g.Go(func() error {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("-> ")
			text, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			message := strings.Replace(text, "\n", "", -1)

			switch message {
			case "q":
				sigint <- struct{}{}

				return nil
			case "":
			default:
				if err := stream.Send(&pb.Message{
					Author: author,
					Body:   message,
				}); err != nil {
					return err
				}
			}
		}
	})

	go func() {
		<-sigint

		cancelFunc()
	}()

	return g.Wait()
}

func teardown() error {
	return conn.Close()
}
