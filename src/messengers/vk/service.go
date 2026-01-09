package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/vk/vk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}
	queue, err := messenger.ConnectToQueue(natsURL)
	if err != nil {
		log.Fatalf("could not connect to nats: %v", err)
	}
	defer queue.Close()

	vkMessenger := createMessenger(queue.GetObjectStore())
	go vkMessenger.Run(context.Background())

	log.Println("connected to nats, subscribing to messages.send.vk.>")
	_, err = queue.Subscribe("messages.send.vk.>", "vk-sender", func(sub string, request *msg.SendMessageRequest) {
		err := func() error {
			defer func() {
				// Always Publish task completion for attachments
				if request.Message != nil {
					for _, attachment := range request.Message.Attachments {
						event := messenger.AttachmentEvent{BlobID: attachment.Url, Delta: -1}
						if err := queue.PublishEvent(messenger.AttachmentProcessedSubject, event); err != nil {
							log.Printf("failed to publish attachment processed event: %v", err)
						}
					}
				}
			}()
			_, err := vkMessenger.SendMessage(context.Background(), request)
			return err
		}()
		if err != nil {
			log.Printf("failed to send message to vk: %v", err)
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

func createMessenger(objectStore messenger.ObjectStore) *vk.Messenger {
	connection, err := grpc.Dial(vk.Config.ControllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", vk.Config.ControllerHost)
	}
	vkMessenger, err := vk.NewMessenger(messenger.NewBaseMessenger(connection, objectStore))
	if err != nil {
		log.Fatalf("could not create messenger: %v", err)
	}
	log.Printf("connected to controller on %s", vk.Config.ControllerHost)
	return vkMessenger
}
