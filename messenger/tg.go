package messenger

import (
	"Pelmenner/TransferBot/config"
	"Pelmenner/TransferBot/utils"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	. "Pelmenner/TransferBot/orm"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type IndexedAttachment struct {
	Attachment Attachment
	ID         int
}

type TGMessenger struct {
	BaseMessenger
	tg                 *tgbotapi.BotAPI
	mediaGroups        utils.SafeMap[string, chan IndexedAttachment]
	mediaGroupLoadings utils.SafeMap[string, *sync.WaitGroup]
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
		mediaGroups:        utils.NewSafeMap[string, chan IndexedAttachment](),
		mediaGroupLoadings: utils.NewSafeMap[string, *sync.WaitGroup](),
	}
}

type Requirement int

const (
	ReqAlways Requirement = iota
	ReqOptional
	ReqNo
)

// Sends all attahchments of given type in a message;
//  Sends text from provided message only if sendText is Always
//  or sendText is optional and there message is already not empty
// Returns result (success) of sending and a value showing need to do it (was message not empty?)
func (m *TGMessenger) sendSpecialAttachmentType(message Message, chat *Chat, attachmentType,
	attachmentFullType string, sendText Requirement) (success bool, needToSend bool) {
	text := ""
	if sendText != ReqNo {
		text = message.Sender + "\n" + message.Text
	}

	needToSend = sendText == ReqAlways
	var media []interface{}
	for _, attachment := range message.Attachments {
		if attachment.Type != attachmentType {
			continue
		}

		caption := ""
		if len(media) == 0 {
			caption = text
			needToSend = true
		}

		media = append(media, tgbotapi.InputMediaDocument{
			BaseInputMedia: tgbotapi.BaseInputMedia{
				Type:    attachmentFullType,
				Media:   tgbotapi.FilePath(attachment.URL),
				Caption: caption,
			}})
	}

	success = true
	if !needToSend {
		return
	}

	if len(media) == 0 {
		_, err := m.tg.Send(tgbotapi.NewMessage(chat.ID, text))
		if err != nil {
			log.Print("could not send tg message:", err)
			success = false
		}
		return
	}

	mediaGroup := tgbotapi.NewMediaGroup(chat.ID, media)
	_, err := m.tg.SendMediaGroup(mediaGroup)
	if err != nil {
		log.Print("could not add tg ", attachmentFullType, err)
		success = false
	}
	return
}

func (m *TGMessenger) SendMessage(message Message, chat *Chat) bool {
	success, tried := m.sendSpecialAttachmentType(message, chat, "photo", "photo", ReqOptional)
	if !success {
		return false
	}
	requirement := ReqAlways
	if tried {
		requirement = ReqNo
	}

	success, tried = m.sendSpecialAttachmentType(message, chat, "doc", "document", requirement)
	return success
}

func (m *TGMessenger) ProcessMediaGroup(message *tgbotapi.Message, chat *Chat) {
	// wait for all media in a group to be received and processed (in another goroutine)
	// we don't know when it ends, so just wait fixed time
	mediaGroupID := message.MediaGroupID
	go func() {
		time.Sleep(config.MediaGroupWaitTimeSec * time.Second)
		loadingWaiter := m.mediaGroupLoadings.Get(mediaGroupID)
		mediaGroup := m.mediaGroups.Get(mediaGroupID)
		loadingWaiter.Wait()
		close(mediaGroup)
	}()

	mediaGroup := m.mediaGroups.Get(mediaGroupID)

	standardMessage := Message{
		Text:        message.Text + message.Caption,
		Sender:      getTGSenderName(message),
		Attachments: []*Attachment{},
	}
	indexedAttachments := []IndexedAttachment{}
	for attachment := range mediaGroup {
		indexedAttachments = append(indexedAttachments, attachment)
	}
	sort.SliceStable(indexedAttachments, func(i, j int) bool {
		return indexedAttachments[i].ID < indexedAttachments[j].ID
	})
	for _, indexedAttachment := range indexedAttachments {
		attachment := indexedAttachment.Attachment
		standardMessage.Attachments = append(standardMessage.Attachments, &attachment)
	}
	m.MessageCallback(standardMessage, chat)
	m.mediaGroups.Delete(mediaGroupID)
	m.mediaGroupLoadings.Delete(mediaGroupID)
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
	err = utils.DownloadFile(filePath, file.Link(m.tg.Token))
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

func (m *TGMessenger) addMediaGroupAttachment(fileID, fileName, fileType, mediaGroupID string, messageID int) {
	m.mediaGroupLoadings.Get(mediaGroupID).Add(1)

	url := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if url == "" {
		m.mediaGroupLoadings.Get(mediaGroupID).Done()
		return
	}

	m.mediaGroups.Get(mediaGroupID) <- IndexedAttachment{
		Attachment: Attachment{Type: fileType, URL: url},
		ID:         messageID,
	}
	m.mediaGroupLoadings.Get(mediaGroupID).Done()
}

func getTGSenderName(message *tgbotapi.Message) string {
	sender := utils.ConcatenateMessageSender(message.From.UserName, message.Chat.Title)
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
		m.MessageCallback(standardMessage, chat)
	} else {
		if !m.mediaGroups.Contains(message.MediaGroupID) {
			m.mediaGroups.Set(message.MediaGroupID, make(chan IndexedAttachment))
			m.mediaGroupLoadings.Set(message.MediaGroupID, &sync.WaitGroup{}) // TODO: add buffer size
			// media group is splitted into different messages, we need to catch them all before processing it
			go m.ProcessMediaGroup(message, chat)
		}

		if message.Photo != nil {
			m.addMediaGroupAttachment(message.Photo[len(message.Photo)-1].FileID, "",
				"photo", message.MediaGroupID, message.MessageID)
		}
		if message.Document != nil {
			m.addMediaGroupAttachment(message.Document.FileID, message.Document.FileName,
				"doc", message.MediaGroupID, message.MessageID)
		}
	}
}

func (m *TGMessenger) ProcessCommand(message *tgbotapi.Message, chat *Chat) {
	switch message.Command() {
	case "get_token":
		msg := tgbotapi.NewMessage(message.Chat.ID, chat.Token)
		m.tg.Send(msg)
	case "subscribe":
		m.SubscribeCallback(chat, message.CommandArguments())
	case "unsubscribe":
		m.UnsubscribeCallback(chat, message.CommandArguments())
	}
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

			chat := m.GetChatById(update.Message.Chat.ID, "tg")
			if chat == nil {
				chat = m.CreateNewChat(update.Message.Chat.ID, "tg")
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
