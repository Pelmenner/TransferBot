package messenger

import (
	"Pelmenner/TransferBot/utils"

	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"

	. "Pelmenner/TransferBot/orm"
)

type VKMessenger struct {
	BaseMessenger
	vk       *api.VK
	longPoll *longpoll.LongPoll
}

func NewVKMessenger(baseMessenger BaseMessenger) *VKMessenger {
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

	messenger := &VKMessenger{
		BaseMessenger: baseMessenger,
		vk:            vk,
		longPoll:      lp,
	}

	lp.MessageNew(func(_ context.Context, obj events.MessageNewObject) {
		id := obj.Message.PeerID
		chat := messenger.GetChatById(int64(id), "vk")
		if chat == nil {
			chat = baseMessenger.CreateNewChat(int64(id), "vk")
		}
		messenger.ProcessMessage(obj.Message, chat)
	})

	return messenger
}

func (m *VKMessenger) SendMessage(message Message, chat *Chat) bool {
	messageBuilder := params.NewMessagesSendBuilder()
	messageBuilder.Message(message.Sender + "\n" + message.Text)
	messageBuilder.RandomID(0)
	messageBuilder.PeerID(int(chat.ID))

	attachmentString := ""
	for _, attachment := range message.Attachments {
		file, err := os.Open(attachment.URL)
		if err != nil {
			log.Print("error opening file", attachment.URL)
			continue
		}
		if attachment.Type == "photo" {
			response, err := m.vk.UploadMessagesPhoto(int(chat.ID), file)
			if err != nil {
				log.Print("error loading photo (vk):", err)
				continue
			}
			attachmentString += fmt.Sprintf("%s%d_%d,", attachment.Type,
				response[len(response)-1].OwnerID, response[len(response)-1].ID)
		} else if attachment.Type == "doc" {
			response, err := m.vk.UploadMessagesDoc(int(chat.ID), "doc", attachment.URL, "", file)
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
		return false
	}
	return true
}

func (m *VKMessenger) getSenderName(message object.MessagesMessage) string {
	userResponse, err := m.vk.UsersGet(api.Params{"user_ids": message.FromID})
	if err != nil {
		log.Print("could not find vk user with id ", message.FromID, err)
		return ""
	}
	// have not found a way to get chat name
	return utils.ConcatenateMessageSender(userResponse[0].FirstName+" "+userResponse[0].LastName, "vk")
}

func (m *VKMessenger) ProcessCommand(message object.MessagesMessage, chat *Chat) bool {
	if strings.HasPrefix(message.Text, "/get_token") {
		m.SendMessage(Message{Text: chat.Token}, chat)
	} else if strings.HasPrefix(message.Text, "/subscribe") {
		s := strings.Split(message.Text, " ")
		m.SubscribeCallback(chat, s[len(s)-1])
	} else if strings.HasPrefix(message.Text, "/unsubscribe") {
		s := strings.Split(message.Text, " ")
		m.UnsubscribeCallback(chat, s[len(s)-1])
	} else {
		return false
	}
	return true
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

	return utils.ConcatenateMessageSender(name, "vk")
}

func (m *VKMessenger) processWall(wall object.WallWallpost, chat *Chat) {
	message := Message{
		Text:   wall.Text,
		Sender: m.getWallAuthor(&wall),
	}

	for _, attachment := range wall.Attachments {
		if attachment.Type == "photo" {
			message.Attachments = m.processPhoto(attachment.Photo, chat.ID, message.Attachments)
		}
	}
	m.MessageCallback(message, chat)
}

func downloadVKFile(url string, fileID int, chatID int64, fileTitle string, attachmentType string) *Attachment {
	path := fmt.Sprintf("data/downloads/vk/%d/%d/%s", chatID, fileID, fileTitle)
	err := utils.DownloadFile(path, url)
	if err != nil {
		log.Print("could not download vk", attachmentType, ": ", err)
		return nil
	}
	return &Attachment{
		Type: attachmentType,
		URL:  path,
	}
}

func (m *VKMessenger) processPhoto(photo object.PhotosPhoto, chatID int64, attachments []*Attachment) []*Attachment {
	url := photo.MaxSize().URL
	ext := filepath.Ext(url)
	attachment := downloadVKFile(url, photo.ID, chatID, ext, "photo")
	if attachment != nil {
		return append(attachments, attachment)
	}
	return attachments
}

func (m *VKMessenger) processDocument(document object.DocsDoc, chatID int64, attachments []*Attachment) []*Attachment {
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

func (m *VKMessenger) ProcessMessage(message object.MessagesMessage, chat *Chat) {
	if message.IsCropped {
		message = m.getFullMessage(message)
	}
	if m.ProcessCommand(message, chat) {
		return
	}
	standardMessage := Message{
		Text:   message.Text,
		Sender: m.getSenderName(message),
	}
	walls := []*object.WallWallpost{}
	for _, attachment := range message.Attachments {
		switch attachment.Type {
		case "photo":
			standardMessage.Attachments = m.processPhoto(attachment.Photo, chat.ID, standardMessage.Attachments)
		case "wall":
			walls = append(walls, &attachment.Wall)
		case "doc":
			standardMessage.Attachments = m.processDocument(attachment.Doc, chat.ID, standardMessage.Attachments)
		}
	}
	m.MessageCallback(standardMessage, chat)
	for _, wall := range walls {
		m.processWall(*wall, chat)
	}

	for _, message := range message.FwdMessages {
		m.ProcessMessage(message, chat)
	}
}

func (m *VKMessenger) Run() {
	m.longPoll.Run()
}
