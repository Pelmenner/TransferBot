package tg

import (
	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"sync"
)

type Messenger struct {
	*messenger.BaseMessenger
	tg                 *tgbotapi.BotAPI
	mediaGroups        Map[string, chan *IndexedAttachment]
	mediaGroupLoadings Map[string, *sync.WaitGroup]
}

type IndexedAttachment struct {
	msg.Attachment
	ID int
}

func NewMessenger(baseMessenger *messenger.BaseMessenger) *Messenger {
	bot, err := tgbotapi.NewBotAPI(Config.Token)
	if err != nil {
		log.Panic(err)
	}
	return &Messenger{
		BaseMessenger:      baseMessenger,
		tg:                 bot,
		mediaGroups:        NewMap[string, chan *IndexedAttachment](),
		mediaGroupLoadings: NewMap[string, *sync.WaitGroup](),
	}
}

func (m *Messenger) getChatToken(chatID int64) (string, error) {
	chat, err := m.GetChatByID(chatID, "tg")
	if err != nil {
		return "", err
	}
	return chat.Token, nil
}
