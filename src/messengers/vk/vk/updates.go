package vk

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"path/filepath"
	"strings"

	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/object"
)

func (m *Messenger) Run(ctx context.Context) {
	restartLimiter := rate.NewLimiter(rate.Limit(Config.LongPollRestartMaxRate), 1)
	for {
		if err := restartLimiter.Wait(ctx); err != nil {
			log.Printf("error waiting for limiter: %v", err)
		}
		if err := m.longPoll.Run(); err != nil {
			log.Print("VK longpoll error:", err)
		}
	}
}

func (m *Messenger) processMessage(message object.MessagesMessage, chat *msg.Chat) error {
	if message.IsCropped {
		message = m.getFullMessage(message)
	}
	if err := m.processCommand(message, chat); err != nil && err != errCommandNotFound {
		return err
	}
	standardMessage := msg.Message{
		Text: message.Text,
		Sender: &msg.Sender{
			Name: m.getSenderName(message),
			Chat: &msg.Chat{Name: "vk"},
		},
	}
	var walls []*object.WallWallpost
	for _, attachment := range message.Attachments {
		switch attachment.Type {
		case "photo":
			standardMessage.Attachments = m.processPhoto(attachment.Photo, chat.Id, standardMessage.Attachments)
		case "wall":
			walls = append(walls, &attachment.Wall)
		case "doc":
			standardMessage.Attachments = m.processDocument(attachment.Doc, chat.Id, standardMessage.Attachments)
		}
	}
	if message.ReplyMessage != nil {
		standardMessage.Text += "\nin reply to..."
	}
	err := m.MessageCallback(&standardMessage, chat)
	for _, wall := range walls {
		if err = m.processWall(*wall, chat); err != nil {
			return err
		}
	}
	if message.ReplyMessage != nil {
		if err = m.processMessage(*message.ReplyMessage, chat); err != nil {
			return err
		}
	}
	for _, message := range message.FwdMessages {
		_ = m.processMessage(message, chat)
	}
	return nil
}

// Returns not cropped message requesting it by id (extracted from given message)
func (m *Messenger) getFullMessage(message object.MessagesMessage) object.MessagesMessage {
	messageResponse, err := m.vk.MessagesGetByConversationMessageID(api.Params{
		"conversation_message_ids": message.ConversationMessageID,
		"peer_id":                  message.PeerID,
	})
	if err != nil || messageResponse.Count == 0 {
		log.Print("could not get vk message by conversation id ", err)
		return message
	}
	return messageResponse.Items[0]
}

func (m *Messenger) getSenderName(message object.MessagesMessage) string {
	userResponse, err := m.vk.UsersGet(api.Params{"user_ids": message.FromID})
	if err != nil || len(userResponse) == 0 {
		log.Print("could not find vk user with id ", message.FromID, err)
		return ""
	}
	// have not found a way to get chat name
	return userResponse[0].FirstName + " " + userResponse[0].LastName
}

var errCommandNotFound = fmt.Errorf("command not found")

func (m *Messenger) processCommand(message object.MessagesMessage, chat *msg.Chat) error {
	var err error
	if strings.HasPrefix(message.Text, "/get_token") {
		err = m.processGetToken(message, chat)
	} else if strings.HasPrefix(message.Text, "/subscribe") {
		err = m.processSubscribe(message, chat)
	} else if strings.HasPrefix(message.Text, "/unsubscribe") {
		err = m.processUnsubscribe(message, chat)
	} else {
		return errCommandNotFound
	}
	return m.processCommandResult(err, chat)
}

// processCommandResult checks if there is an error that needs to be sent to the user and tries to send it.
// If the error was internal, it is added to the returned error
//
// It might have some messenger-specific logic in the future, so it should not be moved to baseMessenger.
func (m *Messenger) processCommandResult(err error, chat *msg.Chat) error {
	// If err is nil or not a grpc error, it should be returned immediately
	if status.Code(err) == codes.OK {
		return err
	}
	_, sendErr := m.SendMessage(context.TODO(), &msg.SendMessageRequest{
		Message: &msg.Message{Text: status.Convert(err).Message()},
		Chat:    chat,
	})
	if !messenger.IsUserInputError(err) {
		if sendErr != nil {
			return fmt.Errorf("could not process command: %v, could send error %v", err, sendErr)
		}
		return err
	}
	return sendErr
}

func (m *Messenger) processGetToken(_ object.MessagesMessage, chat *msg.Chat) error {
	token, err := m.GetChatToken(chat.Id, chat.Type)
	if err != nil {
		return err
	}
	_, errSend := m.SendMessage(context.TODO(), &msg.SendMessageRequest{
		Message: &msg.Message{Text: token},
		Chat:    chat,
	})
	return errSend
}

func (m *Messenger) processSubscribe(message object.MessagesMessage, chat *msg.Chat) error {
	s := strings.Split(message.Text, " ")
	return m.SubscribeCallback(chat, s[len(s)-1])
}

func (m *Messenger) processUnsubscribe(message object.MessagesMessage, chat *msg.Chat) error {
	s := strings.Split(message.Text, " ")
	return m.UnsubscribeCallback(chat, s[len(s)-1])
}

func (m *Messenger) processWall(wall object.WallWallpost, chat *msg.Chat) error {
	message := msg.Message{
		Text: wall.Text,
		Sender: &msg.Sender{
			Name: m.getWallAuthor(&wall),
			Chat: &msg.Chat{Name: "vk"},
		},
	}

	for _, attachment := range wall.Attachments {
		if attachment.Type == "photo" {
			message.Attachments = m.processPhoto(attachment.Photo, chat.Id, message.Attachments)
		}
	}
	return m.MessageCallback(&message, chat)
}

func (m *Messenger) getWallAuthor(wall *object.WallWallpost) string {
	name := ""
	if wall.FromID > 0 { // user
		userResponse, err := m.vk.UsersGet(api.Params{"user_ids": wall.FromID})
		if err != nil || len(userResponse) == 0 {
			log.Print("could not find user with id ", wall.FromID)
			return ""
		}
		name = userResponse[0].FirstName + " " + userResponse[0].LastName
	} else { // group
		groupResponse, err := m.vk.GroupsGetByID(api.Params{"group_ids": -wall.FromID})
		if err != nil || len(groupResponse) == 0 {
			log.Print("could not find community with id ", -wall.FromID)
			return ""
		}
		name = groupResponse[0].Name
	}

	return name
}

func (m *Messenger) processPhoto(photo object.PhotosPhoto, chatID int64, attachments []*msg.Attachment) []*msg.Attachment {
	url := photo.MaxSize().URL
	ext := filepath.Ext(url)
	attachment := downloadVKFile(url, photo.ID, chatID, ext, "photo")
	if attachment != nil {
		return append(attachments, attachment)
	}
	return attachments
}

func (m *Messenger) processDocument(document object.DocsDoc, chatID int64, attachments []*msg.Attachment) []*msg.Attachment {
	attachment := downloadVKFile(document.URL, document.ID, chatID, document.Title, "doc")
	if attachment != nil {
		return append(attachments, attachment)
	}
	return attachments
}

func downloadVKFile(url string, fileID int, chatID int64, fileTitle string, attachmentType string) *msg.Attachment {
	path := fmt.Sprintf("/transferbot/data/downloads/vk/%d/%d/%s", chatID, fileID, fileTitle)
	err := DownloadFile(path, url)
	if err != nil {
		log.Print("could not download vk", attachmentType, ": ", err)
		return nil
	}
	return &msg.Attachment{
		Type: attachmentType,
		Url:  path,
	}
}
