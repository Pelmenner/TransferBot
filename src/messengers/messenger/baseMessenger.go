package messenger

import (
	"context"
	"fmt"

	"github.com/Pelmenner/TransferBot/proto/controller"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"google.golang.org/grpc"
)

type BaseMessenger struct {
	msg.UnimplementedChatServiceServer
	controller.ControllerClient
}

func NewBaseMessenger(cc grpc.ClientConnInterface) *BaseMessenger {
	client := controller.NewControllerClient(cc)
	return &BaseMessenger{ControllerClient: client}
}

func (bm *BaseMessenger) MessageCallback(message *msg.Message, chat *msg.Chat) error {
	_, err := bm.HandleNewMessage(context.TODO(), &controller.HandleMessageRequest{
		Message: message,
		Chat:    chat,
	})
	return err
}

func (bm *BaseMessenger) SubscribeCallback(subscriber *msg.Chat, subscriptionToken string) error {
	_, err := bm.Subscribe(context.TODO(), &controller.SubscribeRequest{
		Chat:  subscriber,
		Token: subscriptionToken,
	})
	return err
}

func (bm *BaseMessenger) UnsubscribeCallback(subscriber *msg.Chat, subscriptionToken string) error {
	_, err := bm.Unsubscribe(context.TODO(), &controller.UnsubscribeRequest{
		Chat:  subscriber,
		Token: subscriptionToken,
	})
	return err
}

func (bm *BaseMessenger) GetChatToken(id int64, messenger string) (string, error) {
	resp, err := bm.ControllerClient.GetChatToken(context.TODO(), &controller.GetChatTokenRequest{
		ChatID:    id,
		Messenger: messenger,
	})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

func (bm *BaseMessenger) SenderToString(sender *msg.Sender) string {
	if sender == nil || sender.Name == "" {
		return ""
	}
	if sender.Chat == nil {
		return sender.Name
	}
	return fmt.Sprintf("%s (%s):", sender.Name, sender.Chat.Name)
}
