package messenger

import (
	"Pelmenner/TransferBot/orm"
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"github.com/Pelmenner/TransferBot/proto/messenger"
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
	AddChat(chatID int64, chatType string) (*orm.Chat, error)
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

func (c *ControllerServer) HandleNewMessage(ctx context.Context, request *controller.HandleMessageRequest) (
	*controller.HandleMessageResponse, error) {
	message := messageFromProto(request.Message)
	chat := chatFromProto(request.Chat)

	subscribed, err := c.storage.FindSubscribedChats(*chat)
	if err != nil {
		log.Print(err)
		return &controller.HandleMessageResponse{}, err
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
	return &controller.HandleMessageResponse{}, nil
}

func (c *ControllerServer) Subscribe(ctx context.Context, request *controller.SubscribeRequest) (
	*controller.SubscribeResponse, error) {
	subscriber := chatFromProto(request.Chat)
	subscriptionToken := request.Token
	log.Printf("subscribe %+v on chat with token %s", subscriber, subscriptionToken)
	var statusMessage string
	if err := c.storage.Subscribe(subscriber, subscriptionToken); err == nil {
		statusMessage = "successfully subscribed!"
	} else {
		log.Print(err)
		statusMessage = "could not subscribe on chat with given token"
		return &controller.SubscribeResponse{}, fmt.Errorf("subscription failed")
	}
	err := c.messengers[subscriber.Type].SendMessage(&orm.Message{Text: statusMessage}, subscriber)
	return &controller.SubscribeResponse{}, err
}

func (c *ControllerServer) Unsubscribe(ctx context.Context, request *controller.UnsubscribeRequest) (
	*controller.UnsubscribeResponse, error) {
	subscriber := chatFromProto(request.Chat)
	subscriptionToken := request.Token
	log.Printf("unsubscribe chat %+v from chat with token %s", subscriber, subscriptionToken)
	var statusMessage string
	if err := c.storage.Unsubscribe(subscriber, subscriptionToken); err == nil {
		statusMessage = "successfully unsubscribed!"
	} else {
		log.Print(err)
		statusMessage = "could not unsubscribe from chat with given token"
		return &controller.UnsubscribeResponse{}, fmt.Errorf("unsubscription failed")
	}
	err := c.messengers[subscriber.Type].SendMessage(&orm.Message{Text: statusMessage}, subscriber) // TODO: move it to messenger service
	return &controller.UnsubscribeResponse{}, err
}

func (c *ControllerServer) GetChat(ctx context.Context, request *controller.GetChatRequest) (
	*controller.GetChatResponse, error) {
	chat, err := c.storage.GetChat(request.ChatID, request.Messenger)
	if err != nil {
		log.Print(err)
		return &controller.GetChatResponse{}, err
	}
	return &controller.GetChatResponse{
		Chat: chatToProto(chat),
	}, nil
}

func (c *ControllerServer) CreateChat(ctx context.Context, request *controller.CreateChatRequest) (
	*controller.CreateChatResponse, error) {
	chat, err := c.storage.AddChat(request.ChatID, request.Messenger)
	if err != nil {
		log.Print(err)
		return &controller.CreateChatResponse{}, err
	}
	return &controller.CreateChatResponse{
		Chat: chatToProto(chat),
	}, nil
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
		ID:    chat.Id,
		RowID: chat.RowID,
		Type:  chat.Type,
		Token: chat.Token,
		//TODO: create a Name field
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
