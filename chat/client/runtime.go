package client

import (
	"errors"
	"io"
	"os"

	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
	utils "github.com/savo92/playground-go-grpc/chat/utils"
)

func Run(author string, stream pb.Chat_RouteChatClient, sigint chan os.Signal, stopFunc func()) error {
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
			utils.AfterEvent(pb.ServerMessage_ConfirmRoomCheckout): ConfirmRoomHandler(stream, sigint),
			utils.AfterEvent(pb.ServerMessage_ForwardMessage):      ForwardMessageHandler(author),
			utils.AfterEvent(pb.ServerMessage_Shutdown):            ShutdownHandler(stream, sigint),
		},
	)

	g := new(errgroup.Group)

	go func() {
		<-sigint

		defer stopFunc()
		cmd := pb.ServerMessage_Shutdown.String()
		if sm.Current() != "closed" {
			if err := sm.Event(cmd); err != nil {
				log.Errorf("Send quitMsg failed: %v", err)
			}
		}
	}()

	g.Go(func() error {
		for {
			sMsgP, err := stream.Recv()
			if errors.As(err, &io.EOF) {
				return nil
			}
			if err != nil {
				return err
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
	})

	if err := sm.Event("pair"); err != nil {
		return err
	}

	return g.Wait()
}
