package integration_tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	controllerPkg "Pelmenner/TransferBot/messenger" // Alias finding package in controller module
	"Pelmenner/TransferBot/orm"

	shared "github.com/Pelmenner/TransferBot/messenger"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/nats-io/nats.go"
)

// assumes docker-compose is running with exposed ports or internal networking if running in container
// Since we run "go test" from host, we need exposed ports.
// Debug compose has nats:4222.
// DB port is optional if we assume default or use env.

func TestFullFlow(t *testing.T) {
	// 1. Setup NATS connection
	natsURL := "nats://localhost:4222"
	if os.Getenv("NATS_URL") != "" {
		natsURL = os.Getenv("NATS_URL")
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Skipf("skipping test, nats connection failed: %v", err)
	}
	defer nc.Close()

	// Mock Storage and AttachmentStore
	mockStorage := &MockStorage{}
	mockAttachmentStore := &MockAttachmentStore{}

	q, err := shared.ConnectToQueue(natsURL)
	if err != nil {
		t.Fatalf("failed to connect queue: %v", err)
	}
	defer q.Close()

	server := controllerPkg.NewControllerServer(mockStorage, q, mockAttachmentStore)

	ctx := context.Background()

	token := "test-token"
	mockStorage.checkToken = token
	subReq := &controller.SubscribeRequest{
		Chat:  &messenger.Chat{Id: 100, Type: "vk", Name: "Source Chat"},
		Token: token,
	}
	_, err = server.Subscribe(ctx, subReq)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// C. Subscribe to NATS to verify publication
	received := make(chan *messenger.SendMessageRequest, 1)
	sub, err := q.Subscribe("messages.send.vk.>", "test-verifier", func(sub string, data *messenger.SendMessageRequest) {
		received <- data
	})
	if err != nil {
		t.Fatalf("NATS subscribe failed: %v", err)
	}
	defer sub.Unsubscribe()

	destChatID := int64(999)
	destChatType := "tg"
	mockStorage.destChat = &orm.Chat{ID: destChatID, Type: destChatType, Name: "Target"}

	req := &controller.HandleMessageRequest{
		Message: &messenger.Message{
			Text:   "Hello Integration",
			Sender: &messenger.Sender{Name: "Tester", Chat: &messenger.Chat{Name: "Target"}},
		},
		Chat: &messenger.Chat{Id: destChatID, Type: destChatType, Name: "Target"},
	}

	_, err = server.HandleNewMessage(ctx, req)
	if err != nil {
		t.Fatalf("HandleNewMessage failed: %v", err)
	}

	select {
	case msg := <-received:
		if msg.Message.Text != "Hello Integration" {
			t.Errorf("Expected text 'Hello Integration', got '%s'", msg.Message.Text)
		}
		if msg.Chat.Id != 100 {
			t.Errorf("Expected dest ID 100, got %d", msg.Chat.Id)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("Timed out waiting for message")
	}
}

// MockStorage implements messenger.Storage
type MockStorage struct {
	destChat    *orm.Chat
	checkToken  string
	subscribers []orm.Chat
}

func (m *MockStorage) Unsubscribe(subscriber *orm.Chat, subscriptionToken string) error { return nil }
func (m *MockStorage) Subscribe(subscriber *orm.Chat, subscriptionToken string) error {
	if subscriptionToken == m.checkToken {
		m.subscribers = append(m.subscribers, *subscriber)
		return nil
	}
	return fmt.Errorf("invalid token")
}
func (m *MockStorage) GetChat(chatID int64, chatType string) (*orm.Chat, error) { return nil, nil }
func (m *MockStorage) GetOrCreateChat(chatID int64, chatType string) (*orm.Chat, error) {
	return nil, nil
}
func (m *MockStorage) GetChatToken(chat *orm.Chat) (string, error) {
	return m.checkToken, nil
}
func (m *MockStorage) CreateChat(chat *orm.Chat) (*orm.Chat, error) { return chat, nil }
func (m *MockStorage) FindSubscribedChats(chat orm.Chat) ([]orm.Chat, error) {
	if chat.ID == m.destChat.ID && chat.Type == m.destChat.Type {
		return m.subscribers, nil
	}
	return nil, nil
}

// MockAttachmentStore implements messenger.AttachmentStore
type MockAttachmentStore struct{}

func (m *MockAttachmentStore) InitRef(key string, count int) error          { return nil }
func (m *MockAttachmentStore) UpdateRef(key string, delta int) (int, error) { return 1, nil }
func (m *MockAttachmentStore) Delete(key string) error                      { return nil }
