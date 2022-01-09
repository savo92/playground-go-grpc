package client

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	utils "github.com/savo92/playground-go-grpc/chat/utils"
)

func Run(author string, stream pb.Chat_RouteChatClient, sigint chan<- os.Signal) error {
	sm := fsm.NewFSM(
		"booting",
		fsm.Events{
			{Name: "pair", Src: []string{"booting"}, Dst: "pairing"},
			{Name: pb.ServerMessage_ConfirmRoomCheckout.String(), Src: []string{"pairing"}, Dst: "ready"},
			{Name: pb.ServerMessage_ForwardMessage.String(), Src: []string{"ready"}, Dst: "receiving"},
			{Name: "readyAgain", Src: []string{"receiving"}, Dst: "ready"},
			{Name: pb.ServerMessage_Shutdown.String(), Src: []string{"booting", "pairing", "ready", "receiving"}, Dst: "closed"},
		},
		fsm.Callbacks{
			"after_pair": PairHandler(stream, author),
			utils.AfterEvent(pb.ServerMessage_ConfirmRoomCheckout): ConfirmRoomHandler(stream),
			utils.AfterEvent(pb.ServerMessage_ForwardMessage):      ForwardMessageHandler(author),
			utils.AfterEvent(pb.ServerMessage_Shutdown):            ShutdownHandler(stream, sigint),
		},
	)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			sMsgP, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				sigint <- os.Interrupt

				return
			}
			if err != nil {
				log.Errorf("Recv failed: %v", err)
				// TODO handle recv error
				return
			}

			cmd := sMsgP.Command.String()
			log.Debugf("Got %s", cmd)
			if err := sm.Event(cmd, sMsgP); err != nil {
				log.Errorf("Failed to submit %s: %v", cmd, err)
			}
			if sm.Current() == "receiving" {
				if err := sm.Event("readyAgain"); err != nil {
					log.Errorf("Failed to submit readyAgain: %v", err)
				}
			}
		}
	}()

	if err := sm.Event("pair"); err != nil {
		return err
	}

	wg.Wait()

	return nil
}
