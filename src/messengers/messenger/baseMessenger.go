package messenger

import (
	"context"
	"github.com/Pelmenner/TransferBot/proto/controller"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"google.golang.org/grpc"
	"log"
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

func (bm *BaseMessenger) MessageCallback(message *msg.Message, chat *msg.Chat) {
	_, err := bm.HandleNewMessage(context.TODO(), &controller.HandleMessageRequest{
		Message: message,
		Chat:    chat,
	})
	if err != nil {
		log.Print(err)
	}
	// TODO: handle errors
}

func (bm *BaseMessenger) SubscribeCallback(subscriber *msg.Chat, subscriptionToken string) {
	_, err := bm.Subscribe(context.TODO(), &controller.SubscribeRequest{
		Chat:  subscriber,
		Token: subscriptionToken,
	})
	if err != nil {
		log.Print(err)
	}
	// TODO: handle errors
}

func (bm *BaseMessenger) UnsubscribeCallback(subscriber *msg.Chat, subscriptionToken string) {
	_, err := bm.Unsubscribe(context.TODO(), &controller.UnsubscribeRequest{
		Chat:  subscriber,
		Token: subscriptionToken,
	})
	if err != nil {
		log.Print(err)
	}
	// TODO: handle errors
}

func (bm *BaseMessenger) GetChatByID(id int64, messenger string) *msg.Chat {
	resp, err := bm.GetChat(context.TODO(), &controller.GetChatRequest{
		ChatID:    id,
		Messenger: messenger,
	})
	if err != nil {
		log.Printf("get chat by id error: %v", err)
		return nil
	}
	if resp == nil {
		return nil
	}
	return resp.Chat // TODO: handle errors
}

func (bm *BaseMessenger) CreateNewChat(id int64, messenger string) *msg.Chat {
	resp, err := bm.CreateChat(context.TODO(), &controller.CreateChatRequest{
		ChatID:    id,
		Messenger: messenger,
	})
	if err != nil {
		log.Printf("create new chat error: %v", err)
		return nil
	}
	if resp == nil {
		return nil
	}
	return resp.Chat // TODO: handle errors
}
