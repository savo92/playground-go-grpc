package server

import (
	"github.com/google/uuid"
	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type participantID string

type Participant struct {
	id       participantID
	username string

	currentRoom           *Room
	serverMessagesChannel chan *pb.ServerMessage
}

func NewParticipant(username string) (*Participant, error) {
	p := &Participant{
		id:                    participantID(uuid.New().String()),
		username:              username,
		serverMessagesChannel: make(chan *pb.ServerMessage),
	}

	return p, nil
}
