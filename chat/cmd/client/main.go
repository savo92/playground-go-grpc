package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	"github.com/savo92/playground-go-grpc/chat/client"
	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
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
			log.Errorf("Failed to CloseSend: %v", err)
		}
	}()
	sigint := make(chan os.Signal, 1)

	sm := fsm.NewFSM(
		"booting",
		fsm.Events{
			{Name: "pair", Src: []string{"booting"}, Dst: "pairing"},
			{Name: pb.ServerMessage_ConfirmRoomCheckout.String(), Src: []string{"pairing"}, Dst: "ready"},
			{Name: pb.ServerMessage_ForwardMessage.String(), Src: []string{"ready"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "ready"},
			{Name: pb.ServerMessage_Shutdown.String(), Src: []string{"booting", "pairing", "ready", "receiving"}, Dst: "closed"},
		},
		fsm.Callbacks{
			"after_pair": func(e *fsm.Event) {
				heloMsg := pb.ClientMessage_ClientHelo{
					Author: author,
				}
				op, err := pbutils.MarshalAny(&heloMsg)
				if err != nil {
					log.Errorf("Marshal from helo failed: %v", err)
					// TODO handle marshaller failure
					return
				}
				cMsg := pb.ClientMessage{
					Command:   pb.ClientMessage_Helo,
					Operation: op,
				}

				if err := stream.Send(&cMsg); err != nil {
					log.Errorf("Failed to send helo: %v", err)
					// TODO end send failure
					return
				}
			},
			fmt.Sprintf("after_%s", pb.ServerMessage_ConfirmRoomCheckout.String()): func(e *fsm.Event) {
				go func() {
					reader := bufio.NewReader(os.Stdin)
					for {
						fmt.Print("-> ")
						text, err := reader.ReadString('\n')
						if err != nil {
							log.Errorf("Failed to read from stdin: %v", err)

							return
						}
						message := strings.Replace(text, "\n", "", -1)

						switch message {
						case "q":
							return
						case "":
							// do not send empty messages.
						default:
							writeMsg := pb.ClientMessage_ClientWriteMessage{
								Body: message,
							}
							op, err := pbutils.MarshalAny(&writeMsg)
							if err != nil {
								log.Errorf("Marshal from writeMsg failed: %v", err)
								// TODO handle marshaller error
								return
							}
							cMsg := pb.ClientMessage{
								Command:   pb.ClientMessage_WriteMessage,
								Operation: op,
							}

							if err := stream.Send(&cMsg); err != nil {
								log.Errorf("Failed to send writeMsg: %v", err)
								// TODO handle send error
								return
							}
						}
					}
				}()
			},
			fmt.Sprintf("after_%s", pb.ServerMessage_ForwardMessage.String()): func(e *fsm.Event) {
				if len(e.Args) == 0 {
					log.Errorf("e.Args is empty")
					// TODO handle missing message
					return
				}
				sMsgP, ok := e.Args[0].(*pb.ServerMessage)
				if !ok {
					log.Errorf("Cannot cast to pb.ServerMessage")
					// TODO handle bad message
					return
				}
				var forwardMsg pb.ServerMessage_ServerForwardMessage
				if err := pbutils.UnmarshalAny(sMsgP.Operation, &forwardMsg); err != nil {
					log.Errorf("Unmarshal to forwardMessage failed: %vv", err)
					// TODO handler unmarshal errors
					return
				}

				if forwardMsg.Author == author {
					return
				}
				fmt.Printf("%s: %s\n", forwardMsg.Author, forwardMsg.Body)
			},
			fmt.Sprintf("after_%s", pb.ServerMessage_Shutdown.String()): func(e *fsm.Event) {
				quitMsg := pb.ClientMessage_ClientQuit{}
				op, err := pbutils.MarshalAny(&quitMsg)
				if err != nil {
					// TODO handle marshaller failure
					return
				}
				cMsg := pb.ClientMessage{
					Command:   pb.ClientMessage_Quit,
					Operation: op,
				}

				if err := stream.Send(&cMsg); err != nil {
					// TODO end send failure
					return
				}
				sigint <- os.Interrupt
			},
		},
	)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			sMsgP, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				// TODO handle EOF
				return
			}
			if err != nil {
				// TODO handle recv error
				return
			}

			log.Debugf("Got %s", sMsgP.Command.String())

			if err := sm.Event(sMsgP.Command.String(), sMsgP); err != nil {
				log.Errorf("Failed to submit %s: %v", sMsgP.Command.String(), err)
			}
			if sm.Current() == "receiving" {
				if err := sm.Event("readyAgain"); err != nil {
					log.Errorf("Failed to submit readyAgain: %v", err)
				}
			}
		}
	}()

	if err := sm.Event("pair"); err != nil {
		return err
	}

	go func() {
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancelFunc()
	}()

	wg.Wait()

	return nil
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
