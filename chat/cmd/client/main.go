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
	"os/signal"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/savo92/playground-go-grpc/chat/client"
	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

var (
	serverAddr = flag.String("server-addr", "localhost:8081", "The address of the chat server.")
)

var (
	errQuit = errors.New("EOF")
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your name: ")
	text, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	author := strings.Replace(text, "\n", "", -1)

	c, err := client.NewClient(*serverAddr)
	if err != nil {
		return err
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	stream, err := c.RouteChat(ctx)
	if err != nil {
		return fmt.Errorf("stream acquisition: %w", err)
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			log.Printf("Failed to CloseSend: %v", err)
		}
	}()
	sigint := make(chan os.Signal, 1)
	g := errgroup.Group{}

	g.Go(func() error {
		for {
			message, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return errQuit
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
				return errQuit
			case "":
				// do not send empty messages.
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
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancelFunc()
	}()

	if err := g.Wait(); !errors.Is(err, errQuit) {
		return err
	}

	return nil
}
