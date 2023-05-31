package messenger

import (
	"Pelmenner/TransferBot/orm"
	"context"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"os"
	"path/filepath"
)

type Storage interface {
	GetUnusedAttachments() ([]*orm.Attachment, error)
	Unsubscribe(subscriber *orm.Chat, subscriptionToken string) error
	Subscribe(subscriber *orm.Chat, subscriptionToken string) error
	GetUnsentMessages(maxCnt int) ([]orm.QueuedMessage, error)
	AddUnsentMessage(message orm.QueuedMessage) error
	GetChat(chatID int64, chatType string) (*orm.Chat, error)
	GetChatToken(chatID int64, chatType string) (string, error)
	CreateChat(chat *orm.Chat) (*orm.Chat, error)
	FindSubscribedChats(chat orm.Chat) ([]orm.Chat, error)
}

type Messenger interface {
	SendMessage(*orm.Message, *orm.Chat) error
}

type ControllerServer struct {
	controller.UnimplementedControllerServer
	messengers map[string]Messenger
	storage    Storage
}

func NewControllerServer(storage Storage, messengers map[string]Messenger) controller.ControllerServer {
	return &ControllerServer{storage: storage, messengers: messengers}
}

func (c *ControllerServer) HandleNewMessage(_ context.Context, request *controller.HandleMessageRequest) (
	*empty.Empty, error) {
	message := messageFromProto(request.Message)
	chat := chatFromProto(request.Chat)

	subscribed, err := c.storage.FindSubscribedChats(*chat)
	if err != nil {
		log.Printf("could not find subscribed chats: %v", err)
		return &empty.Empty{}, status.Error(codes.Unknown, "something went wrong")
	}
	sentToAllSubscribers := true
	for _, subscription := range subscribed {
		if err = c.messengers[subscription.Type].SendMessage(message, &subscription); err != nil {
			log.Printf("could not send message %+v: %v", message, err)
			if err = c.storage.AddUnsentMessage(orm.QueuedMessage{
				Message: *message, Destination: subscription}); err != nil {
				log.Printf("could not save unsent message: %v", err)
			}
			sentToAllSubscribers = false
		}
	}
	if sentToAllSubscribers {
		for _, attachment := range message.Attachments {
			deleteAttachment(attachment)
		}
	}
	return &empty.Empty{}, nil
}

func (c *ControllerServer) Subscribe(_ context.Context, request *controller.SubscribeRequest) (
	*controller.SubscribeResponse, error) {
	subscriber := chatFromProto(request.Chat)
	subscriptionToken := request.Token
	log.Printf("subscribe %+v on chat with token %s", subscriber, subscriptionToken)
	err := c.storage.Subscribe(subscriber, subscriptionToken)

	if err != nil {
		log.Printf("subscription failed: %v", err)
		return &controller.SubscribeResponse{}, status.Error(400, "could not subscribe on chat with given token")
	}
	return &controller.SubscribeResponse{}, nil
}

func (c *ControllerServer) Unsubscribe(_ context.Context, request *controller.UnsubscribeRequest) (
	*controller.UnsubscribeResponse, error) {
	subscriber := chatFromProto(request.Chat)
	subscriptionToken := request.Token

	log.Printf("unsubscribe chat %+v from chat with token %s", subscriber, subscriptionToken)
	err := c.storage.Unsubscribe(subscriber, subscriptionToken)

	if err != nil {
		// TODO: add error differentiation
		log.Printf("unsubscription failed: %v", err)
		return &controller.UnsubscribeResponse{}, status.Error(codes.Unknown, "could not unsubscribe from chat with given token")
	}
	return &controller.UnsubscribeResponse{}, nil
}

func (c *ControllerServer) GetChatToken(_ context.Context, request *controller.GetChatTokenRequest) (
	*controller.GetChatTokenResponse, error) {
	token, err := c.storage.GetChatToken(request.ChatID, request.Messenger)
	if err != nil {
		return &controller.GetChatTokenResponse{}, status.Error(codes.NotFound, "could not find the chat")
	}
	return &controller.GetChatTokenResponse{Token: token}, nil
}

func deleteAttachment(attachment *orm.Attachment) {
	err := os.Remove(attachment.URL)
	if err != nil {
		log.Println("could not delete file", attachment.URL, err)
	}
	err = os.Remove(filepath.Dir(attachment.URL))
	if err != nil {
		log.Println("could not delete directory", filepath.Dir(attachment.URL), err)
	}
}

func messageFromProto(message *messenger.Message) *orm.Message {
	if message == nil {
		return nil
	}
	ormSender := senderFromProto(message.Sender)
	ormMessage := orm.Message{
		Text:   message.Text,
		Sender: *ormSender,
	}
	for _, attachment := range message.Attachments {
		ormMessage.Attachments = append(ormMessage.Attachments, attachmentFromProto(attachment))
	}
	return &ormMessage
}

func chatFromProto(chat *messenger.Chat) *orm.Chat {
	if chat == nil {
		return nil
	}
	return &orm.Chat{
		ID:   chat.Id,
		Type: chat.Type,
		Name: chat.Name,
	}
}

func attachmentFromProto(attachment *messenger.Attachment) *orm.Attachment {
	if attachment == nil {
		return nil
	}
	return &orm.Attachment{
		Type: attachment.Type,
		URL:  attachment.Url,
	}
}

func senderFromProto(sender *messenger.Sender) *orm.Sender {
	if sender == nil {
		return nil
	}
	return &orm.Sender{
		Name: sender.Name,
		Chat: sender.Chat.Name,
	}
}
