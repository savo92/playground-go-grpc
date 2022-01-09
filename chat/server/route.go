package server

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	internal "github.com/savo92/playground-go-grpc/chat/server/internal"
	utils "github.com/savo92/playground-go-grpc/chat/utils"
)

type closeCMD struct {
	delay bool
}

func (s *Server) RouteChat(stream pb.Chat_RouteChatServer) error {
	var p *internal.Participant

	var wg sync.WaitGroup
	closeChan := make(chan closeCMD)
	ctx, cancelFunc := context.WithCancel(stream.Context())
	defer cancelFunc()

	sm := fsm.NewFSM(
		"booting",
		fsm.Events{
			{Name: pb.ClientMessage_Helo.String(), Src: []string{"booting"}, Dst: "ready"},
			{Name: pb.ClientMessage_WriteMessage.String(), Src: []string{"ready"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "ready"},
			{Name: pb.ClientMessage_Quit.String(), Src: []string{"booting", "ready", "receiving"}, Dst: "closed"},
		},
		fsm.Callbacks{
			utils.AfterEvent(pb.ClientMessage_Helo): func(e *fsm.Event) {
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
					for {
						select {
						case <-ctx.Done():
							return
						case <-p.DisconnectChan:
							shutdownMsg := pb.ServerMessage_ServerShutdown{}
							op, err := pbutils.MarshalAny(&shutdownMsg)
							if err != nil {
								log.Errorf("marshal from shutdownMsg failed: %v", err)
								// TODO shutdown marshalling failed
								return
							}
							sMsg := pb.ServerMessage{
								Command:   pb.ServerMessage_Shutdown,
								Operation: op,
							}
							p.Out <- &sMsg
							closeChan <- closeCMD{delay: true}
						case sMsgP := <-p.Out:
							if err := stream.Send(sMsgP); err != nil {
								if errors.Is(err, io.EOF) {
									closeChan <- closeCMD{}

									return
								}
								log.Errorf("Send to %s failed: %v", p, err)

								return
							}
						}
					}
				}()

				p.Out <- &sMsg
			},
			utils.AfterEvent(pb.ClientMessage_WriteMessage): func(e *fsm.Event) {
				cMsgP, err := extractClientMsg(e)
				if err != nil {
					log.Errorf("Cannot extract client msg: %v", err)

					return
				}

				p.CurrentRoom.In <- internal.RoomMessage{
					CMsgP:       cMsgP,
					Participant: p,
				}
			},
			utils.AfterEvent(pb.ClientMessage_Quit): func(e *fsm.Event) {
				closeChan <- closeCMD{}
			},
		},
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		cCMD := <-closeChan
		close := func() {
			cancelFunc()
		}

		if cCMD.delay {
			t := time.NewTimer(5 * time.Second)
			<-t.C
		}
		close()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			cMsgP, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				closeChan <- closeCMD{}

				return
			}
			if err != nil {
				return
			}

			cmd := cMsgP.Command.String()
			log.Debugf("Got %s", cmd)
			if err := sm.Event(cmd, cMsgP); err != nil {
				log.Errorf("Failed to submit %s: %v", cmd, err)
			}
			if sm.Current() == "receiving" {
				if err := sm.Event("readyAgain"); err != nil {
					log.Errorf("Failed to submit readyAgain: %v", err)
				}
			}
		}
	}()

	wg.Wait()

	return nil
}
