package server

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

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
	closeC := make(chan closeCMD)
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
			utils.AfterEvent(pb.ClientMessage_Helo):         heloHandler(ctx, &wg, stream, s, p, closeC),
			utils.AfterEvent(pb.ClientMessage_WriteMessage): writeMessageHandler(ctx, &wg, stream, s, p, closeC),
			utils.AfterEvent(pb.ClientMessage_Quit):         quitHandler(ctx, &wg, stream, s, p, closeC),
		},
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		cCMD := <-closeC
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
				closeC <- closeCMD{}

				return
			}
			if err != nil {
				log.Errorf("Recv failed: %v", err)

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
