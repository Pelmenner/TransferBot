package vk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Messenger) SendMessage(_ context.Context, request *msg.SendMessageRequest) (*empty.Empty, error) {
	destinationChatID := int(request.Chat.Id)
	messageBuilder := params.NewMessagesSendBuilder()
	messageBuilder.Message(m.SenderToString(request.Message.Sender) + "\n" + request.Message.Text)
	messageBuilder.RandomID(0)
	messageBuilder.PeerID(destinationChatID)

	var attachmentStringBuilder strings.Builder
	for _, attachment := range request.Message.Attachments {
		attachmentString, err := m.uploadAttachment(destinationChatID, attachment)
		if err != nil {
			log.Printf("error uploading file of type %s: %v", attachment.Type, err)
			continue
		}
		attachmentStringBuilder.WriteString(attachmentString)
	}
	messageBuilder.Attachment(attachmentStringBuilder.String())

	_, err := m.vk.MessagesSend(messageBuilder.Params)
	if err != nil {
		log.Print(err)
		return &empty.Empty{}, status.Error(codes.Unknown, "could not send the message")
	}
	return &empty.Empty{}, nil
}

func (m *Messenger) uploadAttachment(chatID int, attachment *msg.Attachment) (string, error) {
	data, filename, err := m.AttachmentStore.Get(attachment.Url)
	if err != nil {
		return "", fmt.Errorf("could not get attachment %s: %v", attachment.Url, err)
	}

	file := bytes.NewReader(data)

	var result string
	if attachment.Type == "photo" {
		result, err = m.uploadPhoto(chatID, file)
	} else if attachment.Type == "doc" {
		// Used to be file path, now it's a key. content-disposition might matter but for VK upload it takes title.
		result, err = m.uploadDocument(chatID, filename, file)
	} else {
		return "", fmt.Errorf("unknown attachment type: %s", attachment.Type)
	}

	if err == nil {
		// Decrement handled by event subscription in service.go
	}

	return result, err
}

func (m *Messenger) uploadPhoto(chatID int, file io.Reader) (string, error) {
	response, err := m.vk.UploadMessagesPhoto(chatID, file)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d_%d,", "photo",
		response[len(response)-1].OwnerID, response[len(response)-1].ID), nil
}

func (m *Messenger) uploadDocument(chatID int, title string, file io.Reader) (string, error) {
	response, err := m.vk.UploadMessagesDoc(chatID, "doc", title, "", file)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d_%d,", "doc",
		response.Doc.OwnerID, response.Doc.ID), nil
}
