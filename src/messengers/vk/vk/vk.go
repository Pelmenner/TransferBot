package vk

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
	msg "github.com/Pelmenner/TransferBot/proto/messenger"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"log"
)

type Messenger struct {
	*messenger.BaseMessenger
	vk       *api.VK
	longPoll *longpoll.LongPoll
}

func NewMessenger(baseMessenger *messenger.BaseMessenger) (*Messenger, error) {
	vk := api.NewVK(Config.Token)
	group, err := vk.GroupsGetByID(nil)
	if err != nil || len(group) == 0 {
		return nil, fmt.Errorf("could not get group id: %v", err)
	}

	newMessenger := &Messenger{
		BaseMessenger: baseMessenger,
		vk:            vk,
	}
	err = newMessenger.initLongPoll(group[0].ID)
	if err != nil {
		return nil, fmt.Errorf("could not create longPoll: %v", err)
	}
	return newMessenger, nil
}

func (m *Messenger) initLongPoll(groupID int) error {
	lp, err := longpoll.NewLongPoll(m.vk, groupID)
	if err != nil {
		return err
	}
	m.longPoll = lp
	lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {
		m.logUpdate(&obj)
		if obj.Message.Action.Type != "" {
			return
		}
		chat := msg.Chat{
			Id:   int64(obj.Message.PeerID),
			Type: "vk",
			Name: "vk",
		}
		if err := m.processMessage(obj.Message, &chat); err != nil {
			log.Printf("error processing message: %v", err)
		}
	})
	return nil
}

func (m *Messenger) logUpdate(update *events.MessageNewObject) {
	if update.Message.Action.Type != "" {
		return // TODO: when we start processing other events, they should be logged too
	}
	message := &update.Message
	const template = "new message: conversation message id: %d; chat id: %d; attachments: %d; forwarded: %d; reply: %t"
	log.Printf(template, message.ConversationMessageID, message.PeerID, len(message.Attachments),
		len(message.FwdMessages), message.ReplyMessage != nil)
}
