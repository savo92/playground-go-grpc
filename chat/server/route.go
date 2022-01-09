package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	pbutils "github.com/golang/protobuf/ptypes"
	"github.com/looplab/fsm"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type closeCMD struct {
	delay bool
}

func (s *Server) RouteChat(stream pb.Chat_RouteChatServer) error {
	var p *participant

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

				p, err = newParticipant(heloMsg.Author)
				if err != nil {
					log.Printf("participant creation failed: %v", err)
					// TODO helo failed
					return
				}
				if err := s.rm.rooms[s.defaultRoom].addParticipant(p); err != nil {
					log.Printf("room checkout failed: %v", err)
					// TODO participant registration failed
					return
				}

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

				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						case <-p.disconnectChan:
							shutdownMsg := pb.ServerMessage_ServerShutdown{}
							op, err := pbutils.MarshalAny(&shutdownMsg)
							if err != nil {
								log.Printf("shutdown msg marshalling failed: %v", err)
								// TODO shutdown marshalling failed
								return
							}
							sMsg := pb.ServerMessage{
								Command:   pb.ServerMessage_Shutdown,
								Operation: op,
							}
							p.out <- &sMsg
							closeChan <- closeCMD{delay: true}
						case sMsgP := <-p.out:
							if err := stream.Send(sMsgP); err != nil {
								if errors.Is(err, io.EOF) {
									closeChan <- closeCMD{}

									return
								}
								log.Printf("send failed: %v", err)

								return
							}
						}
					}
				}()

				p.out <- &sMsg
			},
			fmt.Sprintf("after_%s", pb.ClientMessage_WriteMessage.String()): func(e *fsm.Event) {
				cMsgP, err := extractClientMsg(e)
				if err != nil {
					log.Printf("cannot extract client msg: %v", err)

					return
				}

				p.currentRoom.in <- roomMessage{
					cMsgP:       cMsgP,
					participant: p,
				}
			},
			fmt.Sprintf("after_%s", pb.ClientMessage_Quit.String()): func(e *fsm.Event) {
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
	}()

	wg.Wait()

	return nil
}
