package server

import (
	"context"
	"errors"
	"io"
	"sync"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	internal "github.com/savo92/playground-go-grpc/chat/server/internal"
)

func heloHandler(
	ctx context.Context,
	wg *sync.WaitGroup,
	stream pb.Chat_RouteChatServer,
	s *Server,
	p *internal.Participant,
	closeC chan<- closeCMD,
) fsm.Callback {
	return func(e *fsm.Event) {
		var err error
		var cMsgP *pb.ClientMessage
		cMsgP, err = extractClientMsg(e)
		if err != nil {
			log.Errorf("Cannot extract client msg: %v", err)

			return
		}
		var heloMsg pb.ClientMessage_ClientHelo
		if err := pbutils.UnmarshalAny(cMsgP.Operation, &heloMsg); err != nil {
			log.Errorf("Cannot unmarshal to helo: %v", err)
			// TODO no helo no party
			return
		}

		p, err = internal.NewParticipant(heloMsg.Author)
		if err != nil {
			log.Errorf("Participant creation failed: %v", err)
			// TODO helo failed
			return
		}
		room, ok := s.rm.GetRoom(s.defaultRoom)
		if !ok {
			log.Errorf("Unable to get room %s", s.defaultRoom)
		}
		if err := room.AddParticipant(p); err != nil {
			log.Errorf("Room checkout failed: %v", err)
			// TODO participant registration failed
			return
		}

		confirmRoomMsg := pb.ServerMessage_ServerConfirmRoomCheckout{}
		op, err := pbutils.MarshalAny(&confirmRoomMsg)
		if err != nil {
			log.Errorf("Room checkout confirmation failed: %v", err)
			// TODO room checkout confirmation failed
			return
		}
		sMsg := pb.ServerMessage{
			Command:   pb.ServerMessage_ConfirmRoomCheckout,
			Operation: op,
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			sendFunc := func(msg *pb.ServerMessage) error {
				if err := stream.Send(msg); err != nil {
					if errors.Is(err, io.EOF) {
						closeC <- closeCMD{}

						return nil
					}

					return err
				}

				return nil
			}

			for {
				select {
				case <-ctx.Done():
					return
				case <-p.DisconnectChan:
					log.Debugf("Got DisconnectChan")
					shutdownMsg := pb.ServerMessage_ServerShutdown{}
					op, err := pbutils.MarshalAny(&shutdownMsg)
					if err != nil {
						log.Errorf("Marshal from shutdownMsg failed: %v", err)
						// TODO shutdown marshalling failed
						return
					}
					sMsg := pb.ServerMessage{
						Command:   pb.ServerMessage_Shutdown,
						Operation: op,
					}
					if err := sendFunc(&sMsg); err != nil {
						log.Errorf("Send Shutdown to %s failed: %v", p, err)
					}
					closeC <- closeCMD{delay: true}
					log.Debugf("Done DisconnectChan")
				case sMsgP := <-p.Out:
					if err := sendFunc(sMsgP); err != nil {
						log.Errorf("Send to %s failed: %v", p, err)
					}
				}
			}
		}()

		p.Out <- &sMsg
	}
}

func writeMessageHandler(
	ctx context.Context,
	wg *sync.WaitGroup,
	stream pb.Chat_RouteChatServer,
	s *Server,
	p *internal.Participant,
	closeC chan<- closeCMD,
) fsm.Callback {
	return func(e *fsm.Event) {
		cMsgP, err := extractClientMsg(e)
		if err != nil {
			log.Errorf("Cannot extract client msg: %v", err)

			return
		}

		p.CurrentRoom.In <- internal.RoomMessage{
			CMsgP:       cMsgP,
			Participant: p,
		}
	}
}

func quitHandler(
	ctx context.Context,
	wg *sync.WaitGroup,
	stream pb.Chat_RouteChatServer,
	s *Server,
	p *internal.Participant,
	closeC chan<- closeCMD,
) fsm.Callback {
	return func(e *fsm.Event) {
		closeC <- closeCMD{}
	}
}
