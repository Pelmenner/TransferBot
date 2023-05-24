package vk

import (
	"context"
	"fmt"
	"github.com/Pelmenner/TransferBot/messenger"
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
		log.Printf("new event: %+v", obj)
		if obj.Message.Action.Type != "" {
			return
		}
		id := int64(obj.Message.PeerID)
		chat, err := m.GetChatByID(id, "vk")
		if err != nil {
			log.Printf("could not get chat by id %d: %v", id, err)
			return
		}
		if chat == nil {
			chat, err = m.CreateNewChat(id, "vk")
			if err != nil {
				log.Printf("could not create chat with id %d: %v", id, err)
				return
			}
		}
		if err := m.processMessage(obj.Message, chat); err != nil {
			log.Printf("error processing message: %v", err)
		}
	})
	return nil
}
