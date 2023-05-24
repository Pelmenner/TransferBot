package main

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/vk/vk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
)

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", vk.Config.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	vkMessenger := createMessenger()
	go vkMessenger.Run(context.Background())
	grpcServer := grpc.NewServer()
	msg.RegisterChatServiceServer(grpcServer, vkMessenger)

	log.Printf("initializing gRPC server on port %d", vk.Config.Port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func createMessenger() *vk.Messenger {
	connection, err := grpc.Dial(vk.Config.ControllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", vk.Config.ControllerHost)
	}
	vkMessenger, err := vk.NewMessenger(messenger.NewBaseMessenger(connection))
	if err != nil {
		log.Fatalf("could not create messenger: %v", err)
	}
	log.Printf("connected to controller on %s", vk.Config.ControllerHost)
	return vkMessenger
}
