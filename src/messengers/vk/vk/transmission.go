package vk

import (
	"context"
	"fmt"
	"log"
	"os"

	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Messenger) SendMessage(_ context.Context, request *msg.SendMessageRequest) (
	*empty.Empty, error) {
	message := request.Message
	destinationChat := request.Chat
	messageBuilder := params.NewMessagesSendBuilder()
	messageBuilder.Message(m.SenderToString(message.Sender) + "\n" + message.Text)
	messageBuilder.RandomID(0)
	messageBuilder.PeerID(int(destinationChat.Id))

	attachmentString := ""
	for _, attachment := range message.Attachments {
		file, err := os.Open(attachment.Url)
		if err != nil {
			log.Print("error opening file", attachment.Url)
			continue
		}
		if attachment.Type == "photo" {
			response, err := m.vk.UploadMessagesPhoto(int(destinationChat.Id), file)
			if err != nil {
				log.Print("error loading photo (vk):", err)
				continue
			}
			attachmentString += fmt.Sprintf("%s%d_%d,", attachment.Type,
				response[len(response)-1].OwnerID, response[len(response)-1].ID)
		} else if attachment.Type == "doc" {
			response, err := m.vk.UploadMessagesDoc(int(destinationChat.Id), "doc", attachment.Url, "", file)
			if err != nil {
				log.Print("error loading file (vk):", err)
				continue
			}
			attachmentString += fmt.Sprintf("%s%d_%d,", attachment.Type,
				response.Doc.OwnerID, response.Doc.ID)
		}
	}
	messageBuilder.Attachment(attachmentString)

	_, err := m.vk.MessagesSend(messageBuilder.Params)
	if err != nil {
		log.Print(err)
		return &empty.Empty{}, status.Error(codes.Unknown, "could not send the message")
	}
	return &empty.Empty{}, nil
}
