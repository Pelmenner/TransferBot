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
	Attachment
	ID int
}

type TGMessenger struct {
	BaseMessenger
	tg                 *tgbotapi.BotAPI
	mediaGroups        utils.Map[string, chan IndexedAttachment]
	mediaGroupLoadings utils.Map[string, *sync.WaitGroup]
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
		mediaGroups:        utils.NewMap[string, chan IndexedAttachment](),
		mediaGroupLoadings: utils.NewMap[string, *sync.WaitGroup](),
	}
}

// Creates tg media attachments for all message's attachments of given type
func getPreparedMediaList(message *Message, attachmentFullType, attachmentType, caption string) []interface{} {
	var media []interface{}
	for _, attachment := range message.Attachments {
		if attachment.Type != attachmentType {
			continue
		}
		curCaption := ""
		if len(media) == 0 {
			curCaption = caption
		}
		media = append(media, tgbotapi.InputMediaDocument{
			BaseInputMedia: tgbotapi.BaseInputMedia{
				Type:      attachmentFullType,
				Media:     tgbotapi.FilePath(attachment.URL),
				Caption:   curCaption,
				ParseMode: "HTML",
			},
		})
	}
	return media
}

type Requirement int

const (
	ReqAlways Requirement = iota
	ReqOptional
	ReqNever
)

// Sends all attahchments of given type in a message;
//  Sends text from provided message only if sendText is Always
//  or sendText is optional and there message is already not empty
// Returns result (success) of sending and a value showing need to do it (was message not empty?)
func (m *TGMessenger) sendSpecialAttachmentType(message Message, chat *Chat, attachmentType,
	attachmentFullType string, sendText Requirement) (success bool, needToSend bool) {
	text := ""
	if sendText != ReqNever {
		text = m.senderToString(message.Sender) + "\n" + tgbotapi.EscapeText("HTML", message.Text)
	}
	media := getPreparedMediaList(&message, attachmentFullType, attachmentType, text)
	if len(media) == 0 && sendText != ReqAlways {
		return true, false
	}
	success = true
	if len(media) == 0 {
		tgMessage := tgbotapi.NewMessage(chat.ID, text)
		tgMessage.ParseMode = "HTML"
		_, err := m.tg.Send(tgMessage)
		if err != nil {
			log.Print("could not send tg message:", err)
			success = false
		}
		return success, true
	}
	mediaGroup := tgbotapi.NewMediaGroup(chat.ID, media)
	_, err := m.tg.SendMediaGroup(mediaGroup)
	if err != nil {
		log.Print("could not add tg ", attachmentFullType, err)
		success = false
	}
	return success, true
}

func (m *TGMessenger) senderToString(sender Sender) string {
	if sender.Name == "" {
		return ""
	}
	return fmt.Sprintf("<b><u>%s (%s):</u></b>", sender.Name, sender.Chat)
}

func (m *TGMessenger) SendMessage(message Message, chat *Chat) bool {
	success, tried := m.sendSpecialAttachmentType(message, chat, "photo", "photo", ReqOptional)
	if !success {
		return false
	}
	requirement := ReqAlways
	if tried {
		requirement = ReqNever
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
		Sender:      getTGSender(message),
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

func getTGUserName(user *tgbotapi.User) string {
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func getTGSender(message *tgbotapi.Message) Sender {
	sender := Sender{
		Name: getTGUserName(message.From),
		Chat: message.Chat.Title,
	}
	if message.ForwardFrom != nil {
		sender.Name += "\n" + message.ForwardFrom.UserName
	}
	if message.ForwardFromChat != nil {
		sender.Chat += "\n" + message.ForwardFromChat.Title
	}
	if sender.Chat == "" {
		sender.Chat = "tg"
	}
	return sender
}

func (m *TGMessenger) processSingleMessage(message *tgbotapi.Message, chat *Chat) {
	standardMessage := Message{
		Text:   message.Text + message.Caption,
		Sender: getTGSender(message),
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
}

func (m *TGMessenger) processPartOfGroupMessage(message *tgbotapi.Message, chat *Chat) {
	if !m.mediaGroups.Contains(message.MediaGroupID) {
		m.mediaGroups.Set(message.MediaGroupID, make(chan IndexedAttachment))
		m.mediaGroupLoadings.Set(message.MediaGroupID, &sync.WaitGroup{})
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

func (m *TGMessenger) processMessage(message *tgbotapi.Message, chat *Chat) {
	if message.ReplyToMessage != nil {
		m.processMessage(message.ReplyToMessage, chat)
	}

	if message.MediaGroupID == "" {
		m.processSingleMessage(message, chat)
	} else {
		m.processPartOfGroupMessage(message, chat)
	}
}

func (m *TGMessenger) processCommand(message *tgbotapi.Message, chat *Chat) {
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
				go m.processCommand(update.Message, chat)
			} else { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				go m.processMessage(update.Message, chat)
			}
		}
		time.Sleep(config.TGSleepIntervalSec * time.Second)
	}
}
