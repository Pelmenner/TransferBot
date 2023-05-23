package main

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/Pelmenner/TransferBot/tg/tg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
)

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", tg.Config.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	connection, err := grpc.Dial(tg.Config.ControllerHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to controller on %s", tg.Config.ControllerHost)
	}
	tgMessenger := tg.NewTGMessenger(messenger.NewBaseMessenger(connection))
	log.Printf("connected to controller on %s", tg.Config.ControllerHost)
	go tgMessenger.Run(context.Background())

	grpcServer := grpc.NewServer()
	msg.RegisterChatServiceServer(grpcServer, tgMessenger)

	log.Printf("initializing gRPC server on port %d", tg.Config.Port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
