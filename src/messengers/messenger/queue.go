package messenger

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

type QueuedMessage struct {
	Text              string       `json:"text"`
	SenderName        string       `json:"sender_name"`
	SenderChat        string       `json:"sender_chat"`
	DestinationChatID int64        `json:"destination_chat_id"`
	Attachments       []Attachment `json:"attachments"`
}

type Attachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Queue interface {
	Publish(topic string, message interface{}) error
	Subscribe(topic string, durableName string, handler func(sub string, data []byte)) (*nats.Subscription, error)
	Close()
}

type NatsQueue struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

func ConnectToQueue(url string) (Queue, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	// Ideally ensure stream exists here or in setup, but for now we assume stream is created or auto-created
	// "messages.>" subject pattern needs a stream.
	// specific stream creation can be done here if needed.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "MESSAGES",
		Subjects: []string{"messages.>"},
	})
	if err != nil {
		log.Printf("check stream failed (might exist): %v", err)
	}

	return &NatsQueue{conn: nc, js: js}, nil
}

func (q *NatsQueue) Publish(topic string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	_, err = q.js.Publish(topic, data)
	return err
}

func (q *NatsQueue) Subscribe(topic string, durableName string, handler func(sub string, data []byte)) (*nats.Subscription, error) {
	// QueueSubscribe acts as a load-balanced consumer group.
	// Durable ensures the consumer cursor is saved on the server.
	sub, err := q.js.QueueSubscribe(topic, durableName, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
		msg.Ack()
	}, nats.Durable(durableName), nats.ManualAck())

	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (q *NatsQueue) Close() {
	q.conn.Close()
}
