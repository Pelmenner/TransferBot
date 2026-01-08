package main

import (
	"Pelmenner/TransferBot/config"
	"Pelmenner/TransferBot/messenger"
	"Pelmenner/TransferBot/orm"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	shared "github.com/Pelmenner/TransferBot/messenger"
	"github.com/Pelmenner/TransferBot/proto/controller"
	"google.golang.org/grpc"
)

type Messenger = messenger.Messenger

func main() {
	db := orm.NewDB()
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("could not close db: %v", err)
		}
	}()

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}
	queue, err := shared.ConnectToQueue(natsURL)
	if err != nil {
		log.Fatalf("could not connect to nats: %v", err)
	}
	defer queue.Close()

	listener := newHTTPListener()
	server := newGRPCServer(db, queue)
	log.Printf("created grpc server")

	if err := server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

func newHTTPListener() net.Listener {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", config.ServerPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	return lis
}

func newGRPCServer(storage messenger.Storage, queue shared.Queue) *grpc.Server {
	server := grpc.NewServer()
	controllerServer := messenger.NewControllerServer(storage, queue)
	controller.RegisterControllerServer(server, controllerServer)
	return server
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
