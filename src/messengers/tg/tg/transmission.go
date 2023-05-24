package tg

import (
	"context"
	"fmt"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

type Requirement int

const (
	ReqAlways Requirement = iota
	ReqOptional
	ReqNever
)

func (m *Messenger) SendMessage(_ context.Context, request *msg.SendMessageRequest) (*msg.SendMessageResponse, error) {
	success, tried := m.sendSpecialAttachmentType(request.Message, request.Chat, "photo", "photo", ReqOptional)
	if !success {
		return &msg.SendMessageResponse{
			Success: false,
		}, nil
	}
	requirement := ReqAlways
	if tried {
		requirement = ReqNever
	}

	success, _ = m.sendSpecialAttachmentType(request.Message, request.Chat, "doc", "document", requirement)
	return &msg.SendMessageResponse{
		Success: success,
	}, nil
}

// sendSpecialAttachmentType sends all attachments of given type in a message;
//
//	Sends text from provided message only if sendText is Always
//	or sendText is optional and there message is already not empty
//
// Returns result (success) of sending and a value showing need to do it (was message not empty?)
func (m *Messenger) sendSpecialAttachmentType(message *msg.Message, chat *msg.Chat, attachmentType,
	attachmentFullType string, sendText Requirement) (success bool, needToSend bool) {
	text := ""
	if sendText != ReqNever {
		text = m.senderToString(message.Sender) + "\n" + tgbotapi.EscapeText("HTML", message.Text)
	}
	media := getPreparedMediaList(message, attachmentFullType, attachmentType, text)
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

// getPreparedMediaList creates tg media attachments for all message's attachments of given type
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

func (m *Messenger) senderToString(sender *msg.Sender) string {
	if sender.Name == "" {
		return ""
	}
	return fmt.Sprintf("<b><u>%s (%s):</u></b>", sender.Name, sender.Chat.Name)
}
