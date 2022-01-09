package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/savo92/playground-go-grpc/chat/server"
)

const (
	defaultDebug = false
)

var (
	port  = flag.Int("port", 8081, "A port for the grpc server to listen to.")
	debug = flag.Bool("debug", defaultDebug, fmt.Sprintf("Enable debug logs. Default: %t", defaultDebug))
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	s, err := server.NewServer(*port)
	if err != nil {
		return err
	}
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Info("SIGINT received, shutting down")
		ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelFunc()
		if err := s.Shutdown(ctx); err != nil {
			log.Panic("Unable to shutdown the server")
		}
	}()

	log.Infof("Starting server on port %d", *port)

	return s.Serve()
}
