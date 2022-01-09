package server

import (
	"github.com/google/uuid"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type participantID string

type Participant struct {
	id       participantID
	username string

	CurrentRoom *room

	Out chan *pb.ServerMessage

	DisconnectChan chan interface{}
}

func (p Participant) String() string {
	return string(p.id)
}

func (p *Participant) disconnect() {
	p.CurrentRoom.removeParticipant(p.id)
	p.DisconnectChan <- struct{}{}
}

func NewParticipant(username string) (*Participant, error) {
	p := &Participant{
		id:             participantID(uuid.New().String()),
		username:       username,
		Out:            make(chan *pb.ServerMessage),
		DisconnectChan: make(chan interface{}),
	}

	return p, nil
}
