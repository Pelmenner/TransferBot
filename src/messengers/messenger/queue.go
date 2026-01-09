package messenger

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

type Queue interface {
	Publish(topic string, message proto.Message) error
	PublishEvent(topic string, event interface{}) error
	Subscribe(topic string, durableName string, handler func(sub string, data *messenger.SendMessageRequest)) (*nats.Subscription, error)
	SubscribeEvent(topic string, durableName string, handler func(sub string, data []byte)) (*nats.Subscription, error)
	Close()
	GetObjectStore() ObjectStore
}

func (q *NatsQueue) SubscribeEvent(topic string, durableName string, handler func(sub string, data []byte)) (*nats.Subscription, error) {
	return q.js.QueueSubscribe(topic, durableName, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
		msg.Ack()
	}, nats.ManualAck())
}

func (q *NatsQueue) PublishEvent(topic string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = q.js.Publish(topic, data)
	return err
}

type NatsQueue struct {
	conn        *nats.Conn
	js          nats.JetStreamContext
	objectStore ObjectStore
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

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "EVENTS",
		Subjects: []string{"events.>"},
	})
	if err != nil {
		log.Printf("check events stream failed (might exist): %v", err)
	}

	objectStore, err := NewNatsObjectStore(js)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("object store init: %w", err)
	}

	return &NatsQueue{conn: nc, js: js, objectStore: objectStore}, nil
}

func (q *NatsQueue) Publish(topic string, message proto.Message) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	_, err = q.js.Publish(topic, data)
	return err
}

func (q *NatsQueue) Subscribe(topic string, durableName string, handler func(sub string, data *messenger.SendMessageRequest)) (*nats.Subscription, error) {
	// QueueSubscribe acts as a load-balanced consumer group.
	// Durable ensures the consumer cursor is saved on the server.
	sub, err := q.js.QueueSubscribe(topic, durableName, func(msg *nats.Msg) {
		var payload messenger.SendMessageRequest
		if err := proto.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("unmarshal failed: %v", err)
			msg.Ack() // Ack bad message to avoid redelivery loop? Or Nak?
			return
		}
		handler(msg.Subject, &payload)
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

func (q *NatsQueue) GetObjectStore() ObjectStore {
	return q.objectStore
}
