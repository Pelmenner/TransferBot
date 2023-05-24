package main

import (
	"Pelmenner/TransferBot/config"
	"Pelmenner/TransferBot/messenger"
	"Pelmenner/TransferBot/orm"
	"fmt"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
)

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

func repeatedFileCleanup(db *orm.DB) {
	for {
		attachments, err := db.GetUnusedAttachments()
		if err != nil {
			log.Print(err)
		} else {
			for _, attachment := range attachments {
				deleteAttachment(attachment)
			}
		}
		time.Sleep(time.Second * config.FileCleanupIntervalSec)
	}
}

type Messenger = messenger.Messenger

func repeatedProcessUnsentMessages(db *orm.DB, messengers map[string]Messenger) {
	for {
		messages, err := db.GetUnsentMessages(config.UnsentRetrieveMaxCnt)
		if err != nil {
			log.Print(err)
		} else {
			for _, queuedMessage := range messages {
				destination := &queuedMessage.Destination
				if err = messengers[destination.Type].SendMessage(&queuedMessage.Message, destination); err != nil {
					log.Printf("could not send message: %v", err)
					if err := db.AddUnsentMessage(queuedMessage); err != nil {
						log.Printf("could not save unsent message %v", err)
					}
				}
			}
		}
		time.Sleep(time.Second * config.RetrySendIntervalSec)
	}
}

func main() {
	db := orm.NewDB()
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("could not close db: %v", err)
		}
	}()

	messengers, err := initMessengers()
	if err != nil {
		log.Fatalf("could not connect to messengers: %v", err)
	}

	listener := newHTTPListener()
	server := newGRPCServer(db, messengers)
	log.Printf("created grpc server")

	go repeatedProcessUnsentMessages(db, messengers)
	go repeatedFileCleanup(db)

	if err := server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

func initMessengers() (map[string]Messenger, error) {
	messengers := make(map[string]Messenger)
	for messengerName, host := range config.MessengerAddresses {
		connection, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return messengers, fmt.Errorf("connection to %s failed: %v", messengerName, err)
		}
		log.Printf("connected to %s service on %s", messengerName, host)
		messengers[messengerName] = messenger.NewMessengerClient(connection)
	}
	return messengers, nil
}

func newHTTPListener() net.Listener {

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", config.ServerPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	return lis
}

func newGRPCServer(storage messenger.Storage, messengers map[string]Messenger) *grpc.Server {
	server := grpc.NewServer()
	controllerServer := messenger.NewControllerServer(storage, messengers)
	controller.RegisterControllerServer(server, controllerServer)
	return server
}
