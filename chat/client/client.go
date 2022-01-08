package client

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/savo92/playground-go-grpc/chat/pbuf"
)

type Client struct {
	pb.ChatClient
	conn *grpc.ClientConn
}

func (c *Client) Shutdown() error {
	return c.conn.Close()
}

func NewClient(serverAddr string) (*Client, error) {
	credentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.Dial(serverAddr, credentials)
	if err != nil {
		return nil, err
	}

	c := &Client{
		ChatClient: pb.NewChatClient(conn),
		conn:       conn,
	}

	return c, nil
}
