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
	tg                 *tgbotapi.BotAPI
	mediaGroups        map[string][]*Attachment
	mediaGroupMutex    sync.Mutex
	mediaGroupLoadings map[string]*sync.WaitGroup
}

func NewTGMessenger(baseMessenger BaseMessenger) *TGMessenger {
	token := os.Getenv("TG_TOKEN")
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	return &TGMessenger{
		BaseMessenger:      baseMessenger,
		tg:                 bot,
		mediaGroups:        make(map[string][]*Attachment),
		mediaGroupLoadings: make(map[string]*sync.WaitGroup),
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

func (m *TGMessenger) SendMessage(message Message, chat *Chat) bool {
	text := message.Sender + "\n" + message.Text
	msg := tgbotapi.NewMessage(chat.ID, text)
	if len(message.Attachments) > 0 {
		var media []interface{}
		for i, attachment := range message.Attachments {
			caption := ""
			if i == 0 {
				caption = text
			}

			fileType := ""
			if attachment.Type == "photo" {
				fileType = "photo"
			} else if attachment.Type == "doc" {
				fileType = "document"
			}

			baseInputMedia := tgbotapi.BaseInputMedia{
				Type:    fileType,
				Media:   tgbotapi.FilePath(attachment.URL),
				Caption: caption,
			}

			if attachment.Type == "photo" {
				media = append(media, tgbotapi.InputMediaPhoto{BaseInputMedia: baseInputMedia})
			} else if attachment.Type == "doc" {
				media = append(media, tgbotapi.InputMediaDocument{BaseInputMedia: baseInputMedia})
			}
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
	loadingWaiter := m.mediaGroupLoadings[message.MediaGroupID]
	m.mediaGroupMutex.Unlock()
	loadingWaiter.Wait()
	m.mediaGroupMutex.Lock()
	defer m.mediaGroupMutex.Unlock()

	standardMessage := Message{message.Text + message.Caption, getTGSenderName(message), []*Attachment{}}
	for _, attachment := range m.mediaGroups[message.MediaGroupID] {
		standardMessage.Attachments = append(standardMessage.Attachments, attachment)
	}
	m.messageCallback(standardMessage, chat)
	delete(m.mediaGroups, message.MediaGroupID)
	delete(m.mediaGroupLoadings, message.MediaGroupID)
}

// returns path to saved file
func (m *TGMessenger) saveTelegramFile(config tgbotapi.FileConfig, fileName string) string {
	file, err := m.tg.GetFile(config)
	if err != nil {
		log.Print("error loading file", err)
		return ""
	}

	if fileName == "" {
		fileName = filepath.Base(file.FilePath)
	}
	filePath := fmt.Sprintf("data/downloads/tg/%s/%s", file.FileID, fileName)
	err = DownloadFile(filePath, file.Link(m.tg.Token))
	if err != nil {
		log.Print("error downloading file", err)
		return ""
	}
	return filePath
}

func (m *TGMessenger) addAttachment(attachments []*Attachment, fileID, fileName, fileType string) []*Attachment {
	url := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if url != "" {
		return append(attachments,
			&Attachment{
				Type: fileType,
				URL:  url,
			})
	}
	return attachments
}

func (m *TGMessenger) addMediaGroupAttachment(fileID, fileName, fileType, mediaGroupID string) {
	m.mediaGroupMutex.Lock()
	if _, exists := m.mediaGroupLoadings[mediaGroupID]; !exists {
		m.mediaGroupLoadings[mediaGroupID] = &sync.WaitGroup{}
	}
	m.mediaGroupLoadings[mediaGroupID].Add(1)
	m.mediaGroupMutex.Unlock()

	url := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if url == "" {
		return
	}

	m.mediaGroupMutex.Lock()
	m.mediaGroups[mediaGroupID] = append(m.mediaGroups[mediaGroupID],
		&Attachment{fileType, url})
	m.mediaGroupLoadings[mediaGroupID].Done()
	m.mediaGroupMutex.Unlock()
}

func getTGSenderName(message *tgbotapi.Message) string {
	sender := concatenateMessageSender(message.From.UserName, message.Chat.Title)
	if message.ForwardFrom != nil {
		sender += "\n" + message.ForwardFrom.UserName
	}
	if message.ForwardFromChat != nil {
		sender += "\n" + message.ForwardFromChat.Title
	}
	return sender
}

func (m *TGMessenger) ProcessMessage(message *tgbotapi.Message, chat *Chat) {
	if message.ReplyToMessage != nil {
		m.ProcessMessage(message.ReplyToMessage, chat)
	}

	if message.MediaGroupID == "" {
		standardMessage := Message{
			Text:   message.Text + message.Caption,
			Sender: getTGSenderName(message),
		}
		if message.Photo != nil {
			standardMessage.Attachments = m.addAttachment(
				standardMessage.Attachments, message.Photo[len(message.Photo)-1].FileID, "", "photo")
		}
		if message.Document != nil {
			standardMessage.Attachments = m.addAttachment(
				standardMessage.Attachments, message.Document.FileID, message.Document.FileName, "doc")
		}
		m.messageCallback(standardMessage, chat)
	} else {
		_, exists := m.mediaGroups[message.MediaGroupID]
		if !exists {
			m.mediaGroupMutex.Lock()
			m.mediaGroups[message.MediaGroupID] = make([]*Attachment, 0)
			m.mediaGroupMutex.Unlock()
			// media group is splitted into different messages, we need to catch them all before processing it
			go m.ProcessMediaGroup(message, chat)
		}

		if message.Photo != nil {
			m.addMediaGroupAttachment(message.Photo[len(message.Photo)-1].FileID, "", "photo", message.MediaGroupID)
		}
		if message.Document != nil {
			m.addMediaGroupAttachment(message.Document.FileID, message.Document.FileName, "doc", message.MediaGroupID)
		}
	}
}

func (m *VKMessenger) getSenderName(message object.MessagesMessage) string {
	userResponse, err := m.vk.UsersGet(api.Params{"user_ids": message.FromID})
	if err != nil {
		log.Print("could not find vk user with id ", message.FromID, err)
		return ""
	}
	// have not found a way to get chat name
	return concatenateMessageSender(userResponse[0].FirstName+" "+userResponse[0].LastName, "vk")
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

func (m *VKMessenger) getWallAuthor(wall *object.WallWallpost) string {
	userResponce, err := m.vk.GroupsGetByID(api.Params{"user_ids": wall.FromID})
	if err != nil {
		log.Print("could not find community with id ", wall.FromID)
		return ""
	}
	return concatenateMessageSender(userResponce[0].Name, "vk")
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
	m.messageCallback(message, chat)
}

func downloadVKFile(url string, fileID int, chatID int64, fileTitle string, attachmentType string) *Attachment {
	path := fmt.Sprintf("data/downloads/vk/%d/%d/%s", chatID, fileID, fileTitle)
	err := DownloadFile(path, url)
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

func (m *VKMessenger) ProcessMessage(message object.MessagesMessage, chat *Chat) {
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
	m.messageCallback(standardMessage, chat)
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
				go m.ProcessCommand(update.Message, chat)
			} else { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				go m.ProcessMessage(update.Message, chat)
			}
		}
		time.Sleep(config.TGSleepIntervalSec * time.Second)
	}
}
