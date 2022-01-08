package server

import (
	"errors"
	"fmt"
	"io"
	"log"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

func (s *Server) RouteChat(stream pb.Chat_RouteChatServer) error {
	var p *Participant

	sm := fsm.NewFSM(
		"booting",
		fsm.Events{
			{Name: pb.ClientMessage_Helo.String(), Src: []string{"booting"}, Dst: "ready"},
			{Name: pb.ClientMessage_WriteMessage.String(), Src: []string{"ready"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "ready"},
			{Name: pb.ClientMessage_Quit.String(), Src: []string{"booting", "ready"}, Dst: "closed"},
		},
		fsm.Callbacks{
			fmt.Sprintf("after_%s", pb.ClientMessage_Helo.String()): func(e *fsm.Event) {
				var err error
				var cMsgP *pb.ClientMessage
				cMsgP, err = extractClientMsg(e)
				if err != nil {
					log.Printf("cannot extract client msg: %v", err)

					return
				}
				var heloMsg pb.ClientMessage_ClientHelo
				if err := pbutils.UnmarshalAny(cMsgP.Operation, &heloMsg); err != nil {
					log.Printf("cannot unmarshal to ClientMessage_ClientHelo: %v", err)
					// TODO no helo no party
					return
				}

				p, err = NewParticipant(heloMsg.Author)
				if err != nil {
					log.Printf("participant creation failed: %v", err)
					// TODO helo failed
					return
				}
				if err := s.rooms[s.defaultRoom].AddParticipant(p); err != nil {
					log.Printf("room checkout failed: %v", err)
					// TODO participant registration failed
					return
				}
				p.currentRoom = s.rooms[s.defaultRoom]

				confirmRoomMsg := pb.ServerMessage_ServerConfirmRoomCheckout{}
				op, err := pbutils.MarshalAny(&confirmRoomMsg)
				if err != nil {
					log.Printf("room checkout confirmation failed: %v", err)
					// TODO room checkout confirmation failed
					return
				}
				sMsg := pb.ServerMessage{
					Command:   pb.ServerMessage_ConfirmRoomCheckout,
					Operation: op,
				}

				go func() {
					for sMsgP := range p.serverMessagesChannel {
						messageHolder := sMsgP
						if err := stream.Send(messageHolder); err != nil {
							log.Printf("send failed: %v", err)

							return
						}
					}
				}()

				p.serverMessagesChannel <- &sMsg
			},
			fmt.Sprintf("after_%s", pb.ClientMessage_WriteMessage.String()): func(e *fsm.Event) {
				cMsgP, err := extractClientMsg(e)
				if err != nil {
					log.Printf("cannot extract client msg: %v", err)

					return
				}

				p.currentRoom.incomingMessages <- cMsgP
			},
		},
	)

	for {
		cMsgP, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// TODO what do we do now?
			return nil
		}
		if err != nil {
			return err
		}

		log.Printf("Got %s", cMsgP.Command.String())

		if err := sm.Event(cMsgP.Command.String(), cMsgP); err != nil {
			log.Printf("failed to submit %s: %v", cMsgP.Command.String(), err)
		}
		if sm.Current() == "receiving" {
			if err := sm.Event("readyAgain"); err != nil {
				log.Printf("failed to submit readyAgain: %v", err)
			}
		}
	}
}
