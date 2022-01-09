package server

import (
	"fmt"
	"sync"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	utils "github.com/savo92/playground-go-grpc/chat/utils"
)

type RoomID string

type RoomMessage struct {
	CMsgP       *pb.ClientMessage
	Participant *Participant
}

type room struct {
	id RoomID

	rm *RoomManager

	participants map[participantID]*Participant
	mu           sync.Mutex

	In chan RoomMessage

	closeC chan interface{}
	closed bool
}

func (r *room) AddParticipant(p *Participant) error {
	if r.closed {
		return fmt.Errorf("room is closed")
	}
	p.CurrentRoom = r
	r.mu.Lock()
	r.participants[p.id] = p
	r.mu.Unlock()

	return nil
}

func (r *room) removeParticipant(id participantID) {
	r.mu.Lock()
	delete(r.participants, id)
	r.mu.Unlock()
}

func (r *room) consumeChan() {
	sm := fsm.NewFSM(
		"idle",
		fsm.Events{
			{Name: pb.ClientMessage_WriteMessage.String(), Src: []string{"idle"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "idle"},
		},
		fsm.Callbacks{
			utils.AfterEvent(pb.ClientMessage_WriteMessage): func(e *fsm.Event) {
				rMsg, err := extractRoomMsg(e)
				if err != nil {
					return
				}
				var writeMsg pb.ClientMessage_ClientWriteMessage
				if err := pbutils.UnmarshalAny(rMsg.CMsgP.Operation, &writeMsg); err != nil {
					// TODO no writeMsg no party
					return
				}

				for _, p := range copyParticipants(r) {
					forwardMessage := pb.ServerMessage_ServerForwardMessage{
						Author: rMsg.Participant.username,
						Body:   writeMsg.Body,
					}
					op, err := pbutils.MarshalAny(&forwardMessage)
					if err != nil {
						log.Errorf("Marshal from forwardMessage failed: %v", err)
						// TODO handle marshalling failed
						return
					}
					sMsg := pb.ServerMessage{
						Command:   pb.ServerMessage_ForwardMessage,
						Operation: op,
					}
					p.Out <- &sMsg
				}
			},
		},
	)

	for {
		select {
		case <-r.closeC:
			r.closed = true
			for _, p := range copyParticipants(r) {
				p.disconnect()
			}

			return
		case rMsgP := <-r.In:
			cmd := rMsgP.CMsgP.Command.String()
			if err := sm.Event(cmd, rMsgP); err != nil {
				log.Errorf("Failed to submit %s: %v", cmd, err)
			}
			if sm.Current() == "receiving" {
				if err := sm.Event("readyAgain"); err != nil {
					log.Errorf("Failed to submit readyAgain: %v", err)
				}
			}
		}
	}
}

func (r *room) close() {
	if r.closed {
		return
	}
	r.closed = true
	r.rm.removeRoom(r.id)
	r.closeC <- struct{}{}
}

func newRoom(name string) (*room, error) {
	r := &room{
		id:           RoomID(uuid.New().String()),
		participants: make(map[participantID]*Participant),
		In:           make(chan RoomMessage),
		closeC:       make(chan interface{}),
	}

	go r.consumeChan()

	return r, nil
}

func copyParticipants(r *room) []*Participant {
	r.mu.Lock()
	defer r.mu.Unlock()
	participants := make([]*Participant, len(r.participants))
	i := 0
	for _, p := range r.participants {
		participants[i] = p
		i++
	}

	return participants
}

func extractRoomMsg(e *fsm.Event) (RoomMessage, error) {
	if len(e.Args) == 0 {
		return RoomMessage{}, fmt.Errorf("not enough Args")
	}
	rMsg, ok := e.Args[0].(RoomMessage)
	if !ok {
		return RoomMessage{}, fmt.Errorf("type assertion to roomMessage failed")
	}

	return rMsg, nil
}
