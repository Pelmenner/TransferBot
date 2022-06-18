package main

import (
	"Pelmenner/TransferBot/config"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/api/params"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CallbackOnMessageReceived func(message Message, chat *Chat)
type SubscriptionCallback func(subsriber *Chat, subscriptionToken string)
type ChatGetter func(id int64, messenger string) *Chat
type ChatCreator func(id int64, messenger string) *Chat

type Messenger interface {
	SendMessage(message Message, chat *Chat) bool
	Run()
}

type BaseMessenger struct {
	messageCallback     CallbackOnMessageReceived
	subscribeCallback   SubscriptionCallback
	unsubscribeCallback SubscriptionCallback
	getChatById         ChatGetter
	createNewChat       ChatCreator
}

type VKMessenger struct {
	BaseMessenger
	vk       *api.VK
	longPoll *longpoll.LongPoll
}

type TGMessenger struct {
	BaseMessenger
	tg              *tgbotapi.BotAPI
	mediaGroups     map[string][]*Attachment
	mediaGroupMutex sync.Mutex
}

func NewTGMessenger(baseMessenger BaseMessenger) *TGMessenger {
	token := os.Getenv("TG_TOKEN")
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	return &TGMessenger{
		BaseMessenger: baseMessenger,
		tg:            bot,
		mediaGroups:   make(map[string][]*Attachment),
	}
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
		chat := messenger.getChatById(int64(id), "vk")
		if chat == nil {
			chat = baseMessenger.createNewChat(int64(id), "vk")
		}
		messenger.ProcessMessage(obj, chat)
	})

	return messenger
}

func (m *VKMessenger) SendMessage(message Message, chat *Chat) bool {
	messageBuilder := params.NewMessagesSendBuilder()
	messageBuilder.Message(message.Text)
	messageBuilder.RandomID(0)
	messageBuilder.PeerID(int(chat.ID))

	attachmentString := ""
	for _, attachment := range message.Attachments {
		file, err := os.Open(attachment.URL)
		if err != nil {
			log.Println("error opening file", attachment.URL)
			continue
		}
		response, err := m.vk.UploadMessagesPhoto(int(chat.ID), file)
		if err != nil {
			log.Println("error loading file (vk)")
			continue
		}
		attachmentString += fmt.Sprintf("%s%d_%d,", attachment.Type,
			response[len(response)-1].OwnerID, response[len(response)-1].ID)
	}
	messageBuilder.Attachment(attachmentString)

	_, err := m.vk.MessagesSend(api.Params(messageBuilder.Params))
	if err != nil {
		log.Println(err)
		return false
	}
	return true
}

func (m *TGMessenger) SendMessage(message Message, chat *Chat) bool {
	msg := tgbotapi.NewMessage(chat.ID, message.Text)
	if len(message.Attachments) > 0 {
		var media []interface{}
		for i, attachment := range message.Attachments {
			caption := ""
			if i == 0 {
				caption = message.Text
			}

			if attachment.Type != "photo" {
				log.Println("unknown type: ", attachment.Type)
				continue
			}

			inputMedia := tgbotapi.InputMediaPhoto{
				BaseInputMedia: tgbotapi.BaseInputMedia{
					Type:    attachment.Type,
					Media:   tgbotapi.FilePath(attachment.URL),
					Caption: caption,
				},
			}
			media = append(media, inputMedia)
		}
		mediaGroup := tgbotapi.NewMediaGroup(chat.ID, media)
		_, err := m.tg.SendMediaGroup(mediaGroup)
		if err != nil {
			log.Print("could not add tg attachment:", err)
		}
		return err == nil
	}

	_, err := m.tg.Send(msg)
	if err != nil {
		log.Print("could not send tg message:", err)
	}
	return err == nil
}

func (m *TGMessenger) ProcessMediaGroup(message *tgbotapi.Message, chat *Chat) {
	// wait for all media in a group to be received and processed (in another goroutine)
	// we don't know when it ends, so just wait fixed time
	time.Sleep(config.MediaGroupWaitTimeSec * time.Second)
	m.mediaGroupMutex.Lock()
	defer m.mediaGroupMutex.Unlock()

	standardMessage := Message{message.Text + message.Caption, "usr", []*Attachment{}}
	for _, attachment := range m.mediaGroups[message.MediaGroupID] {
		standardMessage.Attachments = append(standardMessage.Attachments, attachment)
	}
	m.messageCallback(standardMessage, chat)
	m.mediaGroups[message.MediaGroupID] = nil
}

// returns path to saved file
func (m *TGMessenger) saveTelegramFile(config tgbotapi.FileConfig) string {
	file, err := m.tg.GetFile(config)
	if err != nil {
		log.Println("error loading file", err)
		return ""
	}
	filePath := "data/downloads/" + file.FilePath
	err = DownloadFile(filePath, file.Link(m.tg.Token))
	if err != nil {
		log.Println("error downloading file", err)
		return ""
	}
	return filePath
}

func (m *TGMessenger) addAttachment(attachments []*Attachment, fileID, fileType string) []*Attachment {
	url := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID})
	if url != "" {
		return append(attachments,
			&Attachment{
				Type: fileType,
				URL:  url,
			})
	}
	return attachments
}

func (m *TGMessenger) ProcessMessage(message *tgbotapi.Message, chat *Chat) {
	if message.ReplyToMessage != nil {
		m.ProcessMessage(message.ReplyToMessage, chat)
	}

	if message.MediaGroupID == "" {
		standardMessage := Message{
			Text:   message.Text,
			Sender: "usr",
		}
		if message.Photo != nil {
			standardMessage.Attachments = m.addAttachment(
				standardMessage.Attachments, message.Photo[len(message.Photo)-1].FileID, "photo")
		}
		m.messageCallback(standardMessage, chat)
	} else {
		_, exists := m.mediaGroups[message.MediaGroupID]
		if !exists {
			// media group is splitted into different messages, we need to catch them all before processing it
			go m.ProcessMediaGroup(message, chat)
		}

		url := m.saveTelegramFile(tgbotapi.FileConfig{
			FileID: message.Photo[len(message.Photo)-1].FileID,
		})
		if url == "" {
			return
		}

		m.mediaGroupMutex.Lock()
		m.mediaGroups[message.MediaGroupID] = append(m.mediaGroups[message.MediaGroupID],
			&Attachment{"photo", url})
		m.mediaGroupMutex.Unlock()
	}
}

func (m *VKMessenger) ProcessCommand(message object.MessagesMessage, chat *Chat) bool {
	if strings.HasPrefix(message.Text, "/get_token") {
		m.SendMessage(Message{Text: chat.Token}, chat)
	} else if strings.HasPrefix(message.Text, "/subscribe") {
		s := strings.Split(message.Text, " ")
		m.subscribeCallback(chat, s[len(s)-1])
	} else if strings.HasPrefix(message.Text, "/unsubscribe") {
		s := strings.Split(message.Text, " ")
		m.unsubscribeCallback(chat, s[len(s)-1])
	} else {
		return false
	}
	return true
}

func (m *TGMessenger) ProcessCommand(message *tgbotapi.Message, chat *Chat) {
	switch message.Command() {
	case "get_token":
		msg := tgbotapi.NewMessage(message.Chat.ID, chat.Token)
		m.tg.Send(msg)
	case "subscribe":
		m.subscribeCallback(chat, message.CommandArguments())
	case "unsubscribe":
		m.unsubscribeCallback(chat, message.CommandArguments())
	}
}

func (m *VKMessenger) processWall(wall object.WallWallpost, message *Message) {
	if message.Text != "" {
		message.Text += "\n"
	}
	message.Text += wall.Text

	for _, attachment := range wall.Attachments {
		if attachment.Type == "photo" {
			message.Attachments = m.processPhoto(attachment.Photo, message.Attachments)
		}
	}
}

func (m *VKMessenger) processPhoto(photo object.PhotosPhoto, attachments []*Attachment) []*Attachment {
	url := photo.MaxSize().URL
	ext := filepath.Ext(url)
	path := fmt.Sprintf("data/downloads/vk/%d%s", photo.ID, ext)
	err := DownloadFile(path, url)
	if err != nil {
		log.Println("could not download vk photo:", err)
		return attachments
	}
	return append(attachments, &Attachment{
		Type: "photo",
		URL:  path,
	})
}

func (m *VKMessenger) ProcessMessage(obj events.MessageNewObject, chat *Chat) {
	if m.ProcessCommand(obj.Message, chat) {
		return
	}
	standardMessage := Message{
		Text:   obj.Message.Text,
		Sender: "vk", // TODO: find actual name
	}
	for _, attachment := range obj.Message.Attachments {
		switch attachment.Type {
		case "photo":
			standardMessage.Attachments = m.processPhoto(attachment.Photo, standardMessage.Attachments)
		case "wall":
			m.processWall(attachment.Wall, &standardMessage)
		}
	}
	m.messageCallback(standardMessage, chat)
}

func (m *VKMessenger) Run() {
	m.longPoll.Run()
}

func (m *TGMessenger) Run() {
	for {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = config.TGBotAPITimeoutSec
		updates := m.tg.GetUpdatesChan(u)

		for update := range updates {
			if update.Message == nil {
				continue
			}

			chat := m.getChatById(update.Message.Chat.ID, "tg")
			if chat == nil {
				chat = m.createNewChat(update.Message.Chat.ID, "tg")
			}

			if update.Message.IsCommand() {
				m.ProcessCommand(update.Message, chat)
			} else { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				m.ProcessMessage(update.Message, chat)
			}
		}
		time.Sleep(config.TGSleepIntervalSec * time.Second)
	}
}
