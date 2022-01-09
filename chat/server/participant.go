package server

import (
	"github.com/google/uuid"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type participantID string

type participant struct {
	id       participantID
	username string

	currentRoom *room

	out chan *pb.ServerMessage

	disconnectChan chan interface{}
}

func (p *participant) disconnect() {
	p.currentRoom.removeParticipant(p.id)
	p.disconnectChan <- struct{}{}
}

func newParticipant(username string) (*participant, error) {
	p := &participant{
		id:             participantID(uuid.New().String()),
		username:       username,
		out:            make(chan *pb.ServerMessage),
		disconnectChan: make(chan interface{}),
	}

	return p, nil
}
