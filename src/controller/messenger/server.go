package messenger

import (
	"Pelmenner/TransferBot/orm"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	shared "github.com/Pelmenner/TransferBot/messenger"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Storage interface {
	Unsubscribe(subscriber *orm.Chat, subscriptionToken string) error
	Subscribe(subscriber *orm.Chat, subscriptionToken string) error
	GetChat(chatID int64, chatType string) (*orm.Chat, error)
	GetOrCreateChat(chatID int64, chatType string) (*orm.Chat, error)
	GetChatToken(chat *orm.Chat) (string, error)
	CreateChat(chat *orm.Chat) (*orm.Chat, error)
	FindSubscribedChats(chat orm.Chat) ([]orm.Chat, error)
}

type Messenger interface {
	SendMessage(*orm.Message, *orm.Chat) error
}

type ControllerServer struct {
	controller.UnimplementedControllerServer
	queue   shared.Queue
	storage Storage
}

func NewControllerServer(storage Storage, queue shared.Queue) controller.ControllerServer {
	return &ControllerServer{storage: storage, queue: queue}
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
		queuedMsg := shared.QueuedMessage{
			Text:              message.Text,
			SenderName:        message.Sender.Name,
			SenderChat:        message.Sender.Chat,
			DestinationChatID: subscription.ID,
		}
		for _, attachment := range message.Attachments {
			queuedMsg.Attachments = append(queuedMsg.Attachments, shared.Attachment{
				Type: attachment.Type,
				URL:  attachment.URL,
			})
		}

		subject := fmt.Sprintf("messages.send.%s.%d", subscription.Type, subscription.ID)
		if err = c.queue.Publish(subject, queuedMsg); err != nil {
			log.Printf("could not publish message %+v: %v", message, err)
			sentToAllSubscribers = false
		}
	}
	// We invoke deletion only if all published successfully, or maybe we just rely on NATS.
	// The original logic deleted attachments if sent to all.
	// If we published, we assume it's "sent" to the queue.
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
	chat, err := c.storage.GetOrCreateChat(request.ChatID, request.Messenger)
	if err != nil {
		return &controller.GetChatTokenResponse{}, status.Error(codes.NotFound, "could not find or create the chat")
	}
	token, err := c.storage.GetChatToken(chat)
	log.Printf("get chat token for chat %d, token %s", request.ChatID, token)
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
