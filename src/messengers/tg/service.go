package main

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/tg/tg"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
)

func main() {
	port := os.Getenv("PORT")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	controllerHost := os.Getenv("CONTROLLER_HOST")
	connection, err := grpc.Dial(controllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", controllerHost)
	}
	tgMessenger := tg.NewTGMessenger(messenger.NewBaseMessenger(connection))
	log.Printf("connected to controller on %s", controllerHost)
	go tgMessenger.Run(context.Background())

	grpcServer := grpc.NewServer()
	msg.RegisterChatServiceServer(grpcServer, tgMessenger)

	log.Printf("initializing gRPC server on port %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
