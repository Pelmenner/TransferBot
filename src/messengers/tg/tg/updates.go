package tg

import (
	"context"
	"fmt"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/time/rate"
	"log"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

func (m *Messenger) Run(ctx context.Context) {
	requestRateLimiter := rate.NewLimiter(rate.Limit(1/Config.TGSleepIntervalSec), 1)
	lastUpdateID := -1

	for {
		if err := requestRateLimiter.Wait(ctx); err != nil {
			log.Printf("Request stopped: %v", err)
			return
		}
		u := tgbotapi.NewUpdate(lastUpdateID + 1)
		u.Timeout = Config.TGBotAPITimeoutSec
		updates := m.tg.GetUpdatesChan(u)

		for update := range updates {
			if update.UpdateID > lastUpdateID {
				lastUpdateID = update.UpdateID
			}
			if update.Message == nil {
				continue
			}

			chat, err := m.GetOrCreateChat(update.Message.Chat.ID, "tg")
			if err != nil || chat == nil {
				log.Printf("getOrCreate chat failed: %v", err)
				continue
			}
			go m.processUpdate(&update, chat)
		}
	}
}

func (m *Messenger) processUpdate(update *tgbotapi.Update, chat *msg.Chat) {
	if update.Message.IsCommand() {
		if err := m.processCommand(update.Message, chat); err != nil && err != errCommandNotFound {
			log.Printf("error processing command: %v", err)
		}
	} else { // If we got a message
		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
		if err := m.processMessage(update.Message, chat); err != nil {
			log.Printf("error processing message: %v", err)
		}
	}
}

var errCommandNotFound = fmt.Errorf("command not found")

func (m *Messenger) processCommand(message *tgbotapi.Message, chat *msg.Chat) error {
	switch message.Command() {
	case "get_token":
		return m.processGetToken(message, chat)
	case "subscribe":
		return m.processSubscribe(message, chat)
	case "unsubscribe":
		return m.processUnsubscribe(message, chat)
	}
	return errCommandNotFound
}

func (m *Messenger) processGetToken(message *tgbotapi.Message, chat *msg.Chat) error {
	token, err := m.getChatToken(chat.Id)
	if err != nil {
		return err
	}
	response := tgbotapi.NewMessage(message.Chat.ID, token)
	_, err = m.tg.Send(response)
	return err
}

func (m *Messenger) processSubscribe(message *tgbotapi.Message, chat *msg.Chat) error {
	return m.SubscribeCallback(chat, message.CommandArguments())
}

func (m *Messenger) processUnsubscribe(message *tgbotapi.Message, chat *msg.Chat) error {
	return m.UnsubscribeCallback(chat, message.CommandArguments())
}

func (m *Messenger) processMessage(message *tgbotapi.Message, chat *msg.Chat) (err error) {
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

func (m *Messenger) processSingleMessage(message *tgbotapi.Message, chat *msg.Chat) (err error) {
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

func (m *Messenger) addAttachment(attachments []*msg.Attachment, fileID, fileName, fileType string) ([]*msg.Attachment, error) {
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

func (m *Messenger) processPartOfGroupMessage(message *tgbotapi.Message, chat *msg.Chat) error {
	if !m.mediaGroups.Contains(message.MediaGroupID) {
		m.mediaGroups.Set(message.MediaGroupID, make(chan *IndexedAttachment))
		m.mediaGroupLoadings.Set(message.MediaGroupID, &sync.WaitGroup{})
		// media group is split into different messages, we need to catch them all before processing it
		go m.processMediaGroup(message, chat)
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

func (m *Messenger) addMediaGroupAttachment(fileID, fileName, fileType, mediaGroupID string, messageID int) error {
	m.mediaGroupLoadings.Get(mediaGroupID).Add(1)

	url, err := m.saveTelegramFile(tgbotapi.FileConfig{FileID: fileID}, fileName)
	if err != nil {
		m.mediaGroupLoadings.Get(mediaGroupID).Done()
		return err
	}

	m.mediaGroups.Get(mediaGroupID) <- &IndexedAttachment{
		Attachment: msg.Attachment{Type: fileType, Url: url},
		ID:         messageID,
	}
	m.mediaGroupLoadings.Get(mediaGroupID).Done()
	return nil
}

func (m *Messenger) processMediaGroup(message *tgbotapi.Message, chat *msg.Chat) {
	// wait for all media in a group to be received and processed (in another goroutine)
	// we don't know when it ends, so just wait fixed time
	mediaGroupID := message.MediaGroupID
	go func() {
		time.Sleep(Config.MediaGroupWaitTime)
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
	var indexedAttachments []*IndexedAttachment
	for attachment := range mediaGroup {
		indexedAttachments = append(indexedAttachments, attachment)
	}
	sort.SliceStable(indexedAttachments, func(i, j int) bool {
		return indexedAttachments[i].ID < indexedAttachments[j].ID
	})
	for _, indexedAttachment := range indexedAttachments {
		attachment := &indexedAttachment.Attachment
		standardMessage.Attachments = append(standardMessage.Attachments, attachment)
	}
	if err := m.MessageCallback(&standardMessage, chat); err != nil {
		log.Printf("message callback error: %v", err)
	}
	m.mediaGroups.Delete(mediaGroupID)
	m.mediaGroupLoadings.Delete(mediaGroupID)
}

// saveTelegramFile saves a file to local storage and returns the path to saved file
func (m *Messenger) saveTelegramFile(config tgbotapi.FileConfig, fileName string) (string, error) {
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

func getTGUserName(user *tgbotapi.User) string {
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func chatFromMessage(message *tgbotapi.Message) *msg.Chat {
	return &msg.Chat{
		Name: message.Chat.Title,
	}
}
