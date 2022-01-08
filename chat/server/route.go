package server

import (
	"errors"
	"io"

	"golang.org/x/sync/errgroup"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

var (
	errQuit = errors.New("EOF")
)

func (s *Server) RouteChat(stream pb.Chat_RouteChatServer) error {
	g := errgroup.Group{}

	g.Go(func() error {
		for {
			message, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return errQuit
			}
			if err != nil {
				return err
			}

			s.mu.Lock()
			s.chatLog = append(s.chatLog, message)
			s.mu.Unlock()
		}
	})

	g.Go(func() error {
		nextMessageI := 0
		for {
			var newMessages *[]*pb.Message
			s.mu.RLock()
			chatLength := len(s.chatLog)
			if delta := chatLength - nextMessageI; delta > 0 {
				newMsgs := make([]*pb.Message, delta)
				copy(newMsgs, s.chatLog[nextMessageI:])
				newMessages = &newMsgs
			}
			s.mu.RUnlock()

			if newMessages != nil {
				for _, msg := range *newMessages {
					if err := stream.Send(msg); err != nil {
						return err
					}
				}
				nextMessageI = chatLength
			}
		}
	})

	if err := g.Wait(); !errors.Is(err, errQuit) {
		return err
	}

	return nil
}
