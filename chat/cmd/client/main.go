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
	"sync"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"

	"github.com/savo92/playground-go-grpc/chat/client"
	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

var (
	serverAddr = flag.String("server-addr", "localhost:8081", "The address of the chat server.")
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

	sm := fsm.NewFSM(
		"booting",
		fsm.Events{
			{Name: "pair", Src: []string{"booting"}, Dst: "pairing"},
			{Name: pb.ServerMessage_ConfirmRoomCheckout.String(), Src: []string{"pairing"}, Dst: "ready"},
			{Name: pb.ServerMessage_ForwardMessage.String(), Src: []string{"ready"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "ready"},
		},
		fsm.Callbacks{
			"after_pair": func(e *fsm.Event) {
				heloMsg := pb.ClientMessage_ClientHelo{
					Author: author,
				}
				op, err := pbutils.MarshalAny(&heloMsg)
				if err != nil {
					// TODO handle marshaller failure
					return
				}
				cMsg := pb.ClientMessage{
					Command:   pb.ClientMessage_Helo,
					Operation: op,
				}

				if err := stream.Send(&cMsg); err != nil {
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
								// TODO handle marshaller error
								return
							}
							cMsg := pb.ClientMessage{
								Command:   pb.ClientMessage_WriteMessage,
								Operation: op,
							}

							if err := stream.Send(&cMsg); err != nil {
								// TODO handle send error
								return
							}
						}
					}
				}()
			},
			fmt.Sprintf("after_%s", pb.ServerMessage_ForwardMessage.String()): func(e *fsm.Event) {
				if len(e.Args) == 0 {
					log.Print("e.Args is empty")
					// TODO handle missing message
					return
				}
				sMsgP, ok := e.Args[0].(*pb.ServerMessage)
				if !ok {
					log.Print("cannot cast to pb.ServerMessage")
					// TODO handle bad message
					return
				}
				var forwardMsg pb.ServerMessage_ServerForwardMessage
				if err := pbutils.UnmarshalAny(sMsgP.Operation, &forwardMsg); err != nil {
					log.Printf("unmarshal to forwardMessage failed: %v", err)
					// TODO handler unmarshal errors
					return
				}

				if forwardMsg.Author == author {
					return
				}
				log.Printf("%s: %s", forwardMsg.Author, forwardMsg.Body)
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

			log.Printf("Got %s", sMsgP.Command.String())

			if err := sm.Event(sMsgP.Command.String(), sMsgP); err != nil {
				log.Printf("failed to submit %s: %v", sMsgP.Command.String(), err)
			}
			if sm.Current() == "receiving" {
				if err := sm.Event("readyAgain"); err != nil {
					log.Printf("failed to submit %s: %v", sMsgP.Command.String(), err)
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
