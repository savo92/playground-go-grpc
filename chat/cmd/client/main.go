package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/signal"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/savo92/playground-go-grpc/chat/client"
)

const (
	defaultServerAddr  = "localhost:8081"
	defaultDebug       = false
	defaultLogDst      = "stdout"
	defaultLogFilename = "chat-client.log"
	defaultLogPathTmpl = "{{.Separator}}tmp{{.Separator}}{{.Filename}}"
)

var (
	serverAddr  = flag.String("server-addr", defaultServerAddr, fmt.Sprintf("The address of the chat server. Default: %s", defaultServerAddr))
	debug       = flag.Bool("debug", defaultDebug, fmt.Sprintf("Enable debug logs. Default: %t", defaultDebug))
	logDst      = flag.String("log-dst", defaultLogDst, fmt.Sprintf("The destination of logs. Default: %s.", defaultLogDst))
	logFilename = flag.String("logfile-path", defaultLogFilename, fmt.Sprintf("When log-dst is file, allows to specify a custom name for the logfile. Default: %s", defaultLogFilename))
)

// var (
// 	errQuit = errors.New("EOF")
// )

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()
	teardownLog, err := configureLog()
	if err != nil {
		return err
	}
	defer teardownLog()

	author, err := getAuthor()
	if err != nil {
		return err
	}

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
			log.Errorf("Failed to CloseSend: %v", err)
		}
	}()

	sigint := make(chan os.Signal, 1)
	go func() {
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		// TODO send a Quit message
		cancelFunc()
	}()

	return client.Run(author, stream, sigint)
}

func configureLog() (func(), error) {
	log.SetReportCaller(true)
	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	switch *logDst {
	case defaultLogDst:
		// nothing to do.
		return func() {}, nil
	case "file":
		fn := *logFilename
		if strings.Contains(fn, "..") || strings.Contains(fn, "/") || !strings.HasSuffix(fn, ".log") {
			return nil, fmt.Errorf("please provide a valid log file destination, instead of %s", *logFilename)
		}

		w := new(bytes.Buffer)
		logfilePathTmpl, err := template.New("logpath").Parse(defaultLogPathTmpl)
		if err != nil {
			return nil, err
		}
		if err := logfilePathTmpl.Execute(w, struct {
			Filename  string
			Separator string
		}{Filename: fn, Separator: string(os.PathSeparator)},
		); err != nil {
			return nil, err
		}
		logfilePath := w.String()
		log.Debugf("logfile will be %s", logfilePath)

		f, err := os.OpenFile(logfilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return nil, fmt.Errorf("unable to open log file destination %s", logfilePath)
		}
		log.SetOutput(f)

		return func() { f.Close() }, nil
	default:
		return nil, fmt.Errorf("invalid logDst %s", *logDst)
	}
}

func getAuthor() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your name: ")
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.Replace(text, "\n", "", -1), nil
}
