package vk

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"
	"golang.org/x/time/rate"
)

type VKMessenger struct {
	*messenger.BaseMessenger
	vk       *api.VK
	longPoll *longpoll.LongPoll
}

func NewVKMessenger(baseMessenger *messenger.BaseMessenger) *VKMessenger {
	token := os.Getenv("VK_TOKEN")
	vk := api.NewVK(token)
	group, err := vk.GroupsGetByID(nil)
	if err != nil {
		log.Fatal(err)
	}

	lp, err := longpoll.NewLongPoll(vk, group[0].ID)
	if err != nil {
		log.Fatal(err)
	}

	newMessenger := &VKMessenger{
		BaseMessenger: baseMessenger,
		vk:            vk,
		longPoll:      lp,
	}

	lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {
		log.Printf("new event: %+v", obj)
		if obj.Message.Action.Type != "" {
			return
		}
		id := int64(obj.Message.PeerID)
		chat, err := newMessenger.GetChatByID(id, "vk")
		if err != nil {
			log.Printf("could not get chat by id %d: %v", id, err)
			return
		}
		if chat == nil {
			chat, err = baseMessenger.CreateNewChat(id, "vk")
			if err != nil {
				log.Printf("could not create chat with id %d: %v", id, err)
				return
			}
		}
		if err := newMessenger.ProcessMessage(obj.Message, chat); err != nil {
			log.Printf("error processing message: %v", err)
		}
	})

	return newMessenger
}

func (m *VKMessenger) senderToString(sender *msg.Sender) string {
	if sender == nil {
		return ""
	}
	if sender.Name == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s):", sender.Name, sender.Chat)
}

func (m *VKMessenger) SendMessage(ctx context.Context, request *msg.SendMessageRequest) (
	*msg.SendMessageResponse, error) {
	message := request.Message
	destinationChat := request.Chat
	messageBuilder := params.NewMessagesSendBuilder()
	messageBuilder.Message(m.senderToString(message.Sender) + "\n" + message.Text)
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

	_, err := m.vk.MessagesSend(api.Params(messageBuilder.Params))
	if err != nil {
		log.Print(err)
		return &msg.SendMessageResponse{}, err
	}
	return &msg.SendMessageResponse{}, nil
}

func (m *VKMessenger) getSenderName(message object.MessagesMessage) string {
	userResponse, err := m.vk.UsersGet(api.Params{"user_ids": message.FromID})
	if err != nil || len(userResponse) == 0 {
		log.Print("could not find vk user with id ", message.FromID, err)
		return ""
	}
	// have not found a way to get chat name
	return userResponse[0].FirstName + " " + userResponse[0].LastName
}

var errCommandNotFound = fmt.Errorf("command not found")

func (m *VKMessenger) ProcessCommand(message object.MessagesMessage, chat *msg.Chat) error {
	if strings.HasPrefix(message.Text, "/get_token") {
		_, err := m.SendMessage(context.TODO(), &msg.SendMessageRequest{
			Message: &msg.Message{Text: chat.Token},
			Chat:    chat,
		})
		return err
	} else if strings.HasPrefix(message.Text, "/subscribe") {
		s := strings.Split(message.Text, " ")
		return m.SubscribeCallback(chat, s[len(s)-1])
	} else if strings.HasPrefix(message.Text, "/unsubscribe") {
		s := strings.Split(message.Text, " ")
		return m.UnsubscribeCallback(chat, s[len(s)-1])
	}
	return errCommandNotFound
}

func (m *VKMessenger) getWallAuthor(wall *object.WallWallpost) string {
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

func (m *VKMessenger) processWall(wall object.WallWallpost, chat *msg.Chat) error {
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

func (m *VKMessenger) processPhoto(photo object.PhotosPhoto, chatID int64, attachments []*msg.Attachment) []*msg.Attachment {
	url := photo.MaxSize().URL
	ext := filepath.Ext(url)
	attachment := downloadVKFile(url, photo.ID, chatID, ext, "photo")
	if attachment != nil {
		return append(attachments, attachment)
	}
	return attachments
}

func (m *VKMessenger) processDocument(document object.DocsDoc, chatID int64, attachments []*msg.Attachment) []*msg.Attachment {
	attachment := downloadVKFile(document.URL, document.ID, chatID, document.Title, "doc")
	if attachment != nil {
		return append(attachments, attachment)
	}
	return attachments
}

// Returns not cropped message requesting it by id (extracted from given message)
func (m *VKMessenger) getFullMessage(message object.MessagesMessage) object.MessagesMessage {
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

func (m *VKMessenger) ProcessMessage(message object.MessagesMessage, chat *msg.Chat) error {
	if message.IsCropped {
		message = m.getFullMessage(message)
	}
	if err := m.ProcessCommand(message, chat); err != nil && err != errCommandNotFound {
		return err
	}
	standardMessage := msg.Message{
		Text: message.Text,
		Sender: &msg.Sender{
			Name: m.getSenderName(message),
			Chat: &msg.Chat{Name: "vk"},
		},
	}
	walls := []*object.WallWallpost{}
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
		err = m.processWall(*wall, chat)
		return err
	}
	if message.ReplyMessage != nil {
		if err = m.ProcessMessage(*message.ReplyMessage, chat); err != nil {
			return err
		}
	}
	for _, message := range message.FwdMessages {
		_ = m.ProcessMessage(message, chat)
	}
	return nil
}

func (m *VKMessenger) Run(ctx context.Context) {
	restartLimiter := rate.NewLimiter(rate.Limit(config.LongPollRestartMaxRate), 1)
	for {
		if err := restartLimiter.Wait(ctx); err != nil {
			log.Printf("error waiting for limiter: %v", err)
		}
		if err := m.longPoll.Run(); err != nil {
			log.Print("VK longpoll error:", err)
		}
	}
}
