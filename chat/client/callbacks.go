package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

func PairHandler(stream pb.Chat_RouteChatClient, author string) fsm.Callback {
	return func(e *fsm.Event) {
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
	}
}

func ConfirmRoomHandler(stream pb.Chat_RouteChatClient) fsm.Callback {
	return func(e *fsm.Event) {
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
	}
}

func ForwardMessageHandler(author string) fsm.Callback {
	return func(e *fsm.Event) {
		sMsgP, err := extractServerMsg(e)
		if err != nil {
			log.Errorf("Cannot extract server msg: %v", err)

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
	}
}

func ShutdownHandler(stream pb.Chat_RouteChatClient, sigint chan<- os.Signal) fsm.Callback {
	return func(e *fsm.Event) {
		quitMsg := pb.ClientMessage_ClientQuit{}
		op, err := pbutils.MarshalAny(&quitMsg)
		if err != nil {
			log.Errorf("Marshal from quitMsg failed: %v", err)
			// TODO handle marshaller failure
			return
		}
		cMsg := pb.ClientMessage{
			Command:   pb.ClientMessage_Quit,
			Operation: op,
		}

		if err := stream.Send(&cMsg); err != nil && !errors.As(err, &io.EOF) {
			log.Errorf("Send failed: %v", err)
			// TODO end send failure
			return
		}
		sigint <- os.Interrupt
	}
}

func extractServerMsg(e *fsm.Event) (*pb.ServerMessage, error) {
	if len(e.Args) == 0 {
		return nil, fmt.Errorf("e.Args is empty")
	}
	sMsgP, ok := e.Args[0].(*pb.ServerMessage)
	if !ok {
		return nil, fmt.Errorf("type assertion to *pb.ServerMessage failed")
	}

	return sMsgP, nil
}
