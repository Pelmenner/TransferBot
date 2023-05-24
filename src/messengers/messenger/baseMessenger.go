package messenger

import (
	"context"
	"github.com/Pelmenner/TransferBot/proto/controller"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"google.golang.org/grpc"
)

type Messenger interface {
	SendMessage(context.Context, *msg.SendMessageRequest) (*msg.SendMessageResponse, error)
}

type CallbackOnMessageReceived func(message msg.Message, chat *msg.Chat)
type SubscriptionCallback func(subscriber *msg.Chat, subscriptionToken string)
type ChatGetter func(id int64, messenger string) *msg.Chat
type ChatCreator func(id int64, messenger string) *msg.Chat

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

func (bm *BaseMessenger) GetChatByID(id int64, messenger string) (*msg.Chat, error) {
	resp, err := bm.GetChat(context.TODO(), &controller.GetChatRequest{
		ChatID:    id,
		Messenger: messenger,
	})
	if err != nil {
		return nil, err
	}
	return resp.Chat, err
}

func (bm *BaseMessenger) CreateNewChat(id int64, messenger string) (*msg.Chat, error) {
	resp, err := bm.CreateChat(context.TODO(), &controller.CreateChatRequest{
		ChatID:    id,
		Messenger: messenger,
	})
	if err != nil {
		return nil, err
	}
	return resp.Chat, nil
}
