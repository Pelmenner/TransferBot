package main

import (
	"context"
	"fmt"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/vk/vk"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"os"

	"github.com/Pelmenner/TransferBot/messenger"
	"google.golang.org/grpc"
)

func main() {
	port := os.Getenv("PORT")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	messenger := createMessenger()
	go messenger.Run(context.Background())
	grpcServer := grpc.NewServer()
	msg.RegisterChatServiceServer(grpcServer, messenger)

	log.Printf("initializing gRPC server on port %s", port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func createMessenger() *vk.VKMessenger {
	controllerHost := os.Getenv("CONTROLLER_HOST")
	connection, err := grpc.Dial(controllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", controllerHost)
	}
	vkMessenger := vk.NewVKMessenger(messenger.NewBaseMessenger(connection))
	log.Printf("connected to controller on %s", controllerHost)
	return vkMessenger
}
