package messenger

import (
	"Pelmenner/TransferBot/orm"
	"context"
	"encoding/json"
	"fmt"
	"log"

	shared "github.com/Pelmenner/TransferBot/messenger"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

type Queue interface {
	Publish(topic string, message proto.Message) error
	PublishEvent(topic string, event interface{}) error
	SubscribeEvent(topic string, durableName string, handler func(sub string, data []byte)) (*nats.Subscription, error)
}

type ControllerServer struct {
	controller.UnimplementedControllerServer
	queue           Queue
	storage         Storage
	attachmentStore AttachmentStore
}

func NewControllerServer(storage Storage, queue Queue, attachmentStore AttachmentStore) controller.ControllerServer {
	server := &ControllerServer{
		storage:         storage,
		queue:           queue,
		attachmentStore: attachmentStore,
	}
	go server.runCleaner()
	return server
}

func (c *ControllerServer) runCleaner() {
	// Cleaner subscribes to ONLY success events (AttachmentProcessed)
	_, err := c.queue.SubscribeEvent(shared.AttachmentProcessedSubject, "attachment-cleaner", func(sub string, data []byte) {
		var event shared.AttachmentEvent
		if err := json.Unmarshal(data, &event); err != nil {
			log.Printf("cleaner: unmarshal error: %v", err)
			return
		}

		newCount, err := c.attachmentStore.UpdateRef(event.BlobID, event.Delta)
		if err != nil {
			log.Printf("cleaner: update ref failed: %v", err)
			return
		}
		if newCount <= 0 {
			if err := c.attachmentStore.Delete(event.BlobID); err != nil {
				log.Printf("cleaner: delete failed: %v", err)
			} else {
				log.Printf("cleaner: deleted attachment %s", event.BlobID)
			}
		}
	})
	if err != nil {
		log.Printf("cleaner failed to subscribe: %v", err)
	}
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
	// Manage RefCounts for attachments
	numSubscribers := len(subscribed)

	if len(request.Message.Attachments) > 0 {
		for _, attachment := range request.Message.Attachments {
			// Initialize Ref Count from 1 (implicit upload) to Target (Subscribers)
			// We can safely use InitRef(numSubscribers).

			if err := c.attachmentStore.InitRef(attachment.Url, numSubscribers); err != nil {
				log.Printf("failed to init ref for %s: %v", attachment.Url, err)
			}
		}
	}

	for _, subscription := range subscribed {
		// Construct SendMessageRequest from Proto types directly
		destChat := &messenger.Chat{
			Id:   subscription.ID,
			Type: subscription.Type,
			Name: subscription.Name,
		}

		outMsg := &messenger.SendMessageRequest{
			Message: request.Message, // Forward the original message structure
			Chat:    destChat,
		}

		subject := fmt.Sprintf("messages.send.%s.%d", subscription.Type, subscription.ID)
		if err = c.queue.Publish(subject, outMsg); err != nil {
			log.Printf("could not publish message %+v: %v", message, err)
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
