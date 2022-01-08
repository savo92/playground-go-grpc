package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/savo92/playground-go-grpc/chat/server"
)

var (
	port = flag.Int("port", 8081, "A port for the grpc server to listen to.")
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()
	s, err := server.NewServer(*port)
	if err != nil {
		return err
	}
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Println("SIGINT received, shutting down")
		ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelFunc()
		if err := s.Shutdown(ctx); err != nil {
			panic("Unable to shutdown the server")
		}
	}()

	log.Printf("Starting server on port %d", *port)

	return s.Serve()
}
