package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/tg/tg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	connection, err := grpc.Dial(tg.Config.ControllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", tg.Config.ControllerHost)
	}
	tgMessenger := tg.NewMessenger(messenger.NewBaseMessenger(connection))
	log.Printf("connected to controller on %s", tg.Config.ControllerHost)
	go tgMessenger.Run(context.Background())

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}
	queue, err := messenger.ConnectToQueue(natsURL)
	if err != nil {
		log.Fatalf("could not connect to nats: %v", err)
	}
	defer queue.Close()

	log.Println("connected to nats, subscribing to messages.send.tg.>")
	_, err = queue.Subscribe("messages.send.tg.>", "tg-sender", func(sub string, data []byte) {
		var qm messenger.QueuedMessage
		if err := json.Unmarshal(data, &qm); err != nil {
			log.Printf("failed to unmarshal message: %v", err)
			return
		}

		protoMsg := &msg.SendMessageRequest{
			Message: &msg.Message{
				Text: qm.Text,
				Sender: &msg.Sender{
					Name: qm.SenderName,
					Chat: &msg.Chat{Name: qm.SenderChat},
				},
			},
			Chat: &msg.Chat{
				Id:   qm.DestinationChatID,
				Type: "tg",
			},
		}
		for _, a := range qm.Attachments {
			protoMsg.Message.Attachments = append(protoMsg.Message.Attachments, &msg.Attachment{
				Type: a.Type,
				Url:  a.URL,
			})
		}

		if _, err := tgMessenger.SendMessage(context.Background(), protoMsg); err != nil {
			log.Printf("failed to send message to tg: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("failed to subscribe: %v", err)
	}

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("shutting down...")
}
