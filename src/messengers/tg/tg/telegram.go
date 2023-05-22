package tg

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/time/rate"
)

type IndexedAttachment struct {
	msg.Attachment
	ID int
}

type TGMessenger struct {
	*messenger.BaseMessenger
	tg                 *tgbotapi.BotAPI
	mediaGroups        Map[string, chan IndexedAttachment]
	mediaGroupLoadings Map[string, *sync.WaitGroup]
}

func NewTGMessenger(baseMessenger *messenger.BaseMessenger) *TGMessenger {
	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		log.Panic(err)
	}
	return &TGMessenger{
		BaseMessenger:      baseMessenger,
		tg:                 bot,
		mediaGroups:        NewMap[string, chan IndexedAttachment](),
		mediaGroupLoadings: NewMap[string, *sync.WaitGroup](),
	}
}

func (m *TGMessenger) SendMessage(ctx context.Context, request *msg.SendMessageRequest) (*msg.SendMessageResponse, error) {
	success, tried := m.sendSpecialAttachmentType(*request.GetMessage(), request.Chat, "photo", "photo", ReqOptional)
	if !success {
		return &msg.SendMessageResponse{
			Success: false,
		}, nil
	}
	requirement := ReqAlways
	if tried {
		requirement = ReqNever
	}

	success, _ = m.sendSpecialAttachmentType(*request.GetMessage(), request.Chat, "doc", "document", requirement)
	return &msg.SendMessageResponse{
		Success: success,
	}, nil
}

// Creates tg media attachments for all message's attachments of given type
func getPreparedMediaList(message *msg.Message, attachmentFullType, attachmentType, caption string) []interface{} {
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
				Media:     tgbotapi.FilePath(attachment.Url),
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

// Sends all attachments of given type in a message;
//
//	Sends text from provided message only if sendText is Always
//	or sendText is optional and there message is already not empty
//
// Returns result (success) of sending and a value showing need to do it (was message not empty?)
func (m *TGMessenger) sendSpecialAttachmentType(message msg.Message, chat *msg.Chat, attachmentType,
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
		tgMessage := tgbotapi.NewMessage(chat.Id, text)
		tgMessage.ParseMode = "HTML"
		_, err := m.tg.Send(tgMessage)
		if err != nil {
			log.Print("could not send tg message:", err)
			success = false
		}
		return success, true
	}
	mediaGroup := tgbotapi.NewMediaGroup(chat.Id, media)
	_, err := m.tg.SendMediaGroup(mediaGroup)
	if err != nil {
		log.Print("could not add tg ", attachmentFullType, err)
		success = false
	}
	return success, true
}

func (m *TGMessenger) senderToString(sender *msg.Sender) string {
	if sender.Name == "" {
		return ""
	}
	return fmt.Sprintf("<b><u>%s (%s):</u></b>", sender.Name, sender.Chat.Name)
}

func (m *TGMessenger) ProcessMediaGroup(message *tgbotapi.Message, chat *msg.Chat) {
	// wait for all media in a group to be received and processed (in another goroutine)
	// we don't know when it ends, so just wait fixed time
	mediaGroupID := message.MediaGroupID
	go func() {
		time.Sleep(config.MediaGroupWaitTime)
		loadingWaiter := m.mediaGroupLoadings.Get(mediaGroupID)
		mediaGroup := m.mediaGroups.Get(mediaGroupID)
		loadingWaiter.Wait()
		close(mediaGroup)
	}()

	mediaGroup := m.mediaGroups.Get(mediaGroupID)

	standardMessage := msg.Message{
		Text:        message.Text + message.Caption,
		Sender:      getTGSender(message),
		Attachments: []*msg.Attachment{},
	}
	var indexedAttachments []IndexedAttachment
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
	if err := m.MessageCallback(&standardMessage, chat); err != nil {
		log.Printf("message callback error: %v", err)
	}
	m.mediaGroups.Delete(mediaGroupID)
	m.mediaGroupLoadings.Delete(mediaGroupID)
}

// returns path to saved file
func (m *TGMessenger) saveTelegramFile(config tgbotapi.FileConfig, fileName string) (string, error) {
	file, err := m.tg.GetFile(config)
	if err != nil {
		return "", fmt.Errorf("error loading file: %w", err)
	}

	if fileName == "" {
		fileName = filepath.Base(file.FilePath)
	}
	filePath := fmt.Sprintf("/transferbot/data/downloads/tg/%s/%s", file.FileID, fileName)
	err = DownloadFile(filePath, file.Link(m.tg.Token))
	if err != nil {
		return "", fmt.Errorf("error downloading file: %w", err)
	}
	return filePath, nil
}

func (m *TGMessenger) addAttachment(attachments []*msg.Attachment, fileID, fileName, fileType string) ([]*msg.Attachment, error) {
	url, err := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if err != nil {
		return attachments, err
	}
	return append(attachments,
		&msg.Attachment{
			Type: fileType,
			Url:  url,
		}), nil
}

func (m *TGMessenger) addMediaGroupAttachment(fileID, fileName, fileType, mediaGroupID string, messageID int) error {
	m.mediaGroupLoadings.Get(mediaGroupID).Add(1)

	url, err := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if err != nil {
		m.mediaGroupLoadings.Get(mediaGroupID).Done()
		return err
	}

	m.mediaGroups.Get(mediaGroupID) <- IndexedAttachment{
		Attachment: msg.Attachment{Type: fileType, Url: url},
		ID:         messageID,
	}
	m.mediaGroupLoadings.Get(mediaGroupID).Done()
	return nil
}

func getTGUserName(user *tgbotapi.User) string {
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func getTGSender(message *tgbotapi.Message) *msg.Sender {
	sender := msg.Sender{
		Name: getTGUserName(message.From),
		Chat: chatFromMessage(message),
	}
	if message.ForwardFrom != nil {
		sender.Name += "\n" + message.ForwardFrom.UserName
	}
	if message.ForwardFromChat != nil {
		sender.Chat.Name += "\n" + message.ForwardFromChat.Title
	}
	if sender.Chat.Name == "" {
		sender.Chat.Name = "tg"
	}
	return &sender
}

func chatFromMessage(message *tgbotapi.Message) *msg.Chat {
	return &msg.Chat{
		Name: message.Chat.Title,
	}
}

func (m *TGMessenger) processSingleMessage(message *tgbotapi.Message, chat *msg.Chat) (err error) {
	standardMessage := msg.Message{
		Text:   message.Text + message.Caption,
		Sender: getTGSender(message),
	}
	if message.Photo != nil {
		standardMessage.Attachments, err = m.addAttachment(
			standardMessage.Attachments, message.Photo[len(message.Photo)-1].FileID, "", "photo")
		if err != nil {
			return err
		}
	}
	if message.Document != nil {
		standardMessage.Attachments, err = m.addAttachment(
			standardMessage.Attachments, message.Document.FileID, message.Document.FileName, "doc")
		if err != nil {
			return err
		}
	}
	return m.MessageCallback(&standardMessage, chat)
}

func (m *TGMessenger) processPartOfGroupMessage(message *tgbotapi.Message, chat *msg.Chat) error {
	if !m.mediaGroups.Contains(message.MediaGroupID) {
		m.mediaGroups.Set(message.MediaGroupID, make(chan IndexedAttachment))
		m.mediaGroupLoadings.Set(message.MediaGroupID, &sync.WaitGroup{})
		// media group is split into different messages, we need to catch them all before processing it
		go m.ProcessMediaGroup(message, chat)
	}

	if message.Photo != nil {
		if err := m.addMediaGroupAttachment(message.Photo[len(message.Photo)-1].FileID, "",
			"photo", message.MediaGroupID, message.MessageID); err != nil {
			return err
		}
	}
	if message.Document != nil {
		if err := m.addMediaGroupAttachment(message.Document.FileID, message.Document.FileName,
			"doc", message.MediaGroupID, message.MessageID); err != nil {
			return err
		}
	}
	return nil
}

func (m *TGMessenger) processMessage(message *tgbotapi.Message, chat *msg.Chat) (err error) {
	if message.ReplyToMessage != nil {
		message.Text += "\nin reply to..."
	}
	if message.MediaGroupID == "" {
		err = m.processSingleMessage(message, chat)
	} else {
		err = m.processPartOfGroupMessage(message, chat)
	}
	if message.ReplyToMessage != nil {
		return m.processMessage(message.ReplyToMessage, chat)
	}
	return err
}

func (m *TGMessenger) getChatToken(chatID int64) (string, error) {
	chat, err := m.GetChatByID(chatID, "tg")
	if err != nil {
		return "", err
	}
	return chat.Token, nil
}

var errCommandNotFound = fmt.Errorf("command not found")

func (m *TGMessenger) processCommand(message *tgbotapi.Message, chat *msg.Chat) error {
	switch message.Command() {
	case "get_token":
		return m.processGetToken(message, chat)
	case "subscribe":
		return m.SubscribeCallback(chat, message.CommandArguments())
	case "unsubscribe":
		return m.UnsubscribeCallback(chat, message.CommandArguments())
	}
	return errCommandNotFound
}

func (m *TGMessenger) processGetToken(message *tgbotapi.Message, chat *msg.Chat) error {
	token, err := m.getChatToken(chat.Id)
	response := tgbotapi.NewMessage(message.Chat.ID, token)
	_, err = m.tg.Send(response)
	return err
}

func (m *TGMessenger) processSubscribe(message *tgbotapi.Message, chat *msg.Chat) error {
	return m.SubscribeCallback(chat, message.CommandArguments())
}

func (m *TGMessenger) processUnsubscribe(message *tgbotapi.Message, chat *msg.Chat) error {
	return m.UnsubscribeCallback(chat, message.CommandArguments())
}

func (m *TGMessenger) Run(ctx context.Context) {
	requestRateLimiter := rate.NewLimiter(rate.Limit(1/config.TGSleepIntervalSec), 1)
	lastUpdateID := -1

	for {
		if err := requestRateLimiter.Wait(ctx); err != nil {
			log.Printf("Request stopped: %v", err)
			return
		}
		u := tgbotapi.NewUpdate(lastUpdateID + 1)
		u.Timeout = config.TGBotAPITimeoutSec
		updates := m.tg.GetUpdatesChan(u)

		for update := range updates {
			if update.UpdateID > lastUpdateID {
				lastUpdateID = update.UpdateID
			}
			if update.Message == nil {
				continue
			}

			chat, err := m.GetChatByID(update.Message.Chat.ID, "tg")
			if err != nil {
				log.Printf("could not get chat by id: %v", err)
				continue
			}
			if chat == nil {
				chat, err = m.CreateNewChat(update.Message.Chat.ID, "tg")
				if err != nil {
					log.Printf("could not create a new tg chat with id %d: %v", update.Message.Chat.ID, err)
					continue
				}
			}

			if update.Message.IsCommand() {
				go func() {
					if err = m.processCommand(update.Message, chat); err != nil && err != errCommandNotFound {
						log.Printf("error processing command: %v", err)
					}
				}()
			} else { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				go func() {
					if err = m.processMessage(update.Message, chat); err != nil {
						log.Printf("error processing message: %v", err)
					}
				}()
			}
		}
	}
}
