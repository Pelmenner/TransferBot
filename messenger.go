package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/SevereCloud/vksdk/api/params"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CallbackOnMessageReceived func(message Message, chat *Chat)
type CallbackOnSubscribe func(subsriber *Chat, subscriptionToken string)
type CallbackOnChatCreated func(chat *Chat)

type Messenger interface {
	SendMessage(message Message, chat *Chat) bool
	Run()
}

type BaseMessenger struct {
	messageCallback     CallbackOnMessageReceived
	subscribeCallback   CallbackOnSubscribe
	chatCreatedCallback CallbackOnChatCreated
}

type VKMessenger struct {
	BaseMessenger
	vk       *api.VK
	longPoll *longpoll.LongPoll
}

type TGMessenger struct {
	BaseMessenger
	tg *tgbotapi.BotAPI
}

func newTGMessenger(baseMessenger BaseMessenger) *TGMessenger {
	token := goDotEnvVariable("TG_TOKEN")
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	return &TGMessenger{baseMessenger, bot}
}

func newVKMessenger(baseMessenger BaseMessenger) *VKMessenger {
	token := goDotEnvVariable("VK_TOKEN")
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
		messenger.ProcessMessage(obj)
	})

	return messenger
}

func findOriginal(message object.MessagesMessage) object.MessagesMessage {
	for len(message.FwdMessages) != 0 {
		message = message.FwdMessages[0]
	}
	return message
}

func getImages(message object.MessagesMessage) []string {
	message = findOriginal(message)
	images := []string{}

	for _, attachment := range message.Attachments {
		if attachment.Type == "wall" {
			for _, photo := range attachment.Wall.Attachments {
				if photo.Type != "photo" {
					continue
				}
				sizes := photo.Photo.Sizes
				images = append(images, sizes[len(sizes)-1].URL)
			}
		}
	}

	return images
}

func (m VKMessenger) SendMessage(message Message, chat *Chat) bool {
	b := params.NewMessagesSendBuilder()
	b.Message(message.Text)
	b.RandomID(0)
	b.PeerID(int(chat.ID))

	_, err := m.vk.MessagesSend(api.Params(b.Params))
	if err != nil {
		log.Fatal(err)
	}
	return false
}

func (m TGMessenger) SendMessage(message Message, chat *Chat) bool {
	msg := tgbotapi.NewMessage(int64(chat.ID), message.Text)

	_, err := m.tg.Send(msg)
	return err == nil
}

func (m TGMessenger) ProcessMessage(message *tgbotapi.Message, chat *Chat) {
	m.messageCallback(Message{Text: message.Text}, chat)
}

func getChatByVKID(id int64) *Chat {
	return &Chat{}
}

func (m TGMessenger) ProcessCommand(message *tgbotapi.Message, chat *Chat) {
	switch message.Command() {
	case "get_token":
		msg := tgbotapi.NewMessage(message.Chat.ID, chat.Token)
		m.tg.Send(msg)
	case "subscribe":
		m.subscribeCallback(chat, message.CommandArguments())
	}
}

func (m VKMessenger) ProcessMessage(obj events.MessageNewObject) {
	m.messageCallback(Message{Text: findOriginal(obj.Message).Text}, &Chat{int64(obj.Message.PeerID), "vk_token", "vk", 0})
}

func (m VKMessenger) Run() {
	m.longPoll.Run()
}

func NewChat(id int64) *Chat {
	length := 10
	b := make([]byte, length)
	rand.Read(b)
	token := fmt.Sprintf("%x", b)[:length]
	return &Chat{ID: id, Token: token, Type: "tg"}
}

func (m TGMessenger) Run() {
	chats := make(map[int64]*Chat)

	for {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := m.tg.GetUpdatesChan(u)

		for update := range updates {
			if update.Message == nil {
				continue
			}

			// TODO: replace with db call
			chat, present := chats[update.Message.Chat.ID]
			if !present {
				chat = NewChat(update.Message.Chat.ID)
				m.chatCreatedCallback(chat)
				chats[update.Message.Chat.ID] = chat
			}

			if update.Message.IsCommand() {
				m.ProcessCommand(update.Message, chat)
			} else { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				m.ProcessMessage(update.Message, chat)
			}
		}
		time.Sleep(50000 * time.Millisecond)
	}
}
