package main

import (
	"Pelmenner/TransferBot/config"
	"Pelmenner/TransferBot/messenger"
	"Pelmenner/TransferBot/orm"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"
)

func deleteAttachment(attachment *orm.Attachment) {
	err := os.Remove(attachment.URL)
	if err != nil {
		log.Println("could not delete file", attachment.URL, err)
	}
	err = os.Remove(filepath.Dir(attachment.URL))
	if err != nil {
		log.Println("could not delete directory", filepath.Dir(attachment.URL), err)
	}
}

func repeatedFileCleanup(db *sql.DB) {
	for {
		attachments, err := orm.GetUnusedAttachments(db)
		if err != nil {
			log.Print(err)
		} else {
			for _, attachment := range attachments {
				deleteAttachment(attachment)
			}
		}
		time.Sleep(time.Second * config.FileCleanupIntervalSec)
	}
}

func repeatedProcessUnsentMessages(db *sql.DB, messengers map[string]messenger.Messenger) {
	for {
		messages, err := orm.GetUnsentMessages(db, config.UnsentRetrieveMaxCnt)
		if err != nil {
			log.Print(err)
		} else {
			for _, queuedMessage := range messages {
				destination := &queuedMessage.Destination
				if !messengers[destination.Type].SendMessage(queuedMessage.Message, destination) {
					orm.AddUnsentMessage(db, queuedMessage)
				}
			}
		}
		time.Sleep(time.Second * config.RetrySendIntervalSec)
	}
}

func main() {
	messengers := make(map[string]messenger.Messenger)

	db, err := sql.Open("pgx", os.Getenv("DB_CONNECT_STRING"))
	if err != nil {
		log.Fatal("could not connect to database:", err)
	}
	defer db.Close()

	messageCallback := func(message orm.Message, chat *orm.Chat) {
		log.Print("message:", message)
		subscribed, err := orm.FindSubscribedChats(db, *chat)
		if err != nil {
			log.Print(err)
			return
		}
		sentToAllSubscribers := true
		for _, subscription := range subscribed {
			if !messengers[subscription.Type].SendMessage(message, &subscription) {
				orm.AddUnsentMessage(db, orm.QueuedMessage{Message: message, Destination: subscription})
				sentToAllSubscribers = false
			}
		}
		if sentToAllSubscribers {
			for _, attachment := range message.Attachments {
				deleteAttachment(attachment)
			}
		}
	}

	addSubscription := func(subscriber *orm.Chat, subscriptionToken string) {
		log.Printf("subscribe %+v on chat with token %s", subscriber, subscriptionToken)
		var statusMessage string
		if err := orm.Subscribe(db, subscriber, subscriptionToken); err == nil {
			statusMessage = "successfully subscribed!"
		} else {
			log.Print(err)
			statusMessage = "could not subscribe on chat with given token"
		}
		messengers[subscriber.Type].SendMessage(orm.Message{Text: statusMessage}, subscriber)
	}

	cancelSubscription := func(subscriber *orm.Chat, subscriptionToken string) {
		log.Printf("unsubscribe chat %+v from chat with token %s", subscriber, subscriptionToken)
		var statusMessage string
		if err := orm.Unsubscribe(db, subscriber, subscriptionToken); err == nil {
			statusMessage = "successfully unsubscribed!"
		} else {
			log.Print(err)
			statusMessage = "could not unsubscribe from chat with given token"
		}
		messengers[subscriber.Type].SendMessage(orm.Message{Text: statusMessage}, subscriber)
	}

	getChatById := func(id int64, messenger string) *orm.Chat {
		chat, err := orm.GetChat(db, id, messenger)
		if err != nil {
			log.Print(err)
			return nil
		}
		return chat
	}

	createNewChat := func(id int64, messenger string) *orm.Chat {
		chat, err := orm.AddChat(db, id, messenger)
		if err != nil {
			log.Print(err)
			return nil
		}
		return chat
	}

	baseMessenger := messenger.BaseMessenger{
		MessageCallback:     messageCallback,
		SubscribeCallback:   addSubscription,
		UnsubscribeCallback: cancelSubscription,
		GetChatById:         getChatById,
		CreateNewChat:       createNewChat,
	}

	VKMessenger := messenger.NewVKMessenger(baseMessenger)
	TGMessenger := messenger.NewTGMessenger(baseMessenger)

	messengers["vk"] = VKMessenger
	messengers["tg"] = TGMessenger

	go repeatedProcessUnsentMessages(db, messengers)
	go repeatedFileCleanup(db)
	go TGMessenger.Run()

	log.Println("Start Long Poll")
	VKMessenger.Run()
}
