package server

import (
	"fmt"
	"log"
	"sync"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"
	"github.com/looplab/fsm"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type roomID string

type Room struct {
	ID           roomID
	mu           sync.Mutex
	participants map[participantID]*Participant

	incomingMessages chan *pb.ClientMessage
}

func (r *Room) AddParticipant(p *Participant) error {
	r.mu.Lock()
	r.participants[p.id] = p
	r.mu.Unlock()

	return nil
}

func (r *Room) consumeChan() {
	sm := fsm.NewFSM(
		"idle",
		fsm.Events{
			{Name: pb.ClientMessage_WriteMessage.String(), Src: []string{"idle"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "idle"},
		},
		fsm.Callbacks{
			fmt.Sprintf("after_%s", pb.ClientMessage_WriteMessage.String()): func(e *fsm.Event) {
				cMsgP, err := extractClientMsg(e)
				if err != nil {
					log.Printf("cannot extract client msg: %v", err)

					return
				}

				var writeMsg pb.ClientMessage_ClientWriteMessage
				if err := pbutils.UnmarshalAny(cMsgP.Operation, &writeMsg); err != nil {
					// TODO no helo no party
					return
				}

				for _, p := range r.participants {
					forwardMessage := pb.ServerMessage_ServerForwardMessage{
						Author: "bo", // TODO grab author from somewhere
						Body:   writeMsg.Body,
					}
					op, err := pbutils.MarshalAny(&forwardMessage)
					if err != nil {
						// TODO handle marshalling failed
						return
					}
					sMsg := pb.ServerMessage{
						Command:   pb.ServerMessage_ForwardMessage,
						Operation: op,
					}
					p.serverMessagesChannel <- &sMsg
				}
			},
		},
	)

	for cMsgP := range r.incomingMessages {
		if err := sm.Event(cMsgP.Command.String(), cMsgP); err != nil {
			// TODO handle Event failed
			log.Printf("failed to submit %s: %v", cMsgP.Command.String(), err)
		}
		if err := sm.Event("readyAgain"); err != nil {
			log.Printf("failed to submit readyAgain: %v", err)
		}
	}
}

func NewRoom(name string) (*Room, error) {
	r := &Room{
		ID:               roomID(uuid.New().String()),
		participants:     make(map[participantID]*Participant),
		incomingMessages: make(chan *pb.ClientMessage),
	}

	go r.consumeChan()

	return r, nil
}
