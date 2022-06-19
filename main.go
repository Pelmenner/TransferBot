package main

import (
	"Pelmenner/TransferBot/config"
	"database/sql"
	"log"
	"os"
	"time"
)

func deleteAttachment(attachment *Attachment) {
	err := os.Remove(attachment.URL)
	if err != nil {
		log.Println("could not delete file", attachment.URL)
	}
}

func repeatedFileCleanup(db *sql.DB) {
	for {
		attachments := getUnusedAttachments(db)
		for _, attachment := range attachments {
			deleteAttachment(attachment)
		}
		time.Sleep(time.Second * config.FileCleanupIntervalSec)
	}
}

func main() {
	messengers := make(map[string]Messenger)

	db, err := sql.Open("sqlite3", "data/db.sqlite3")
	if err != nil {
		log.Fatal("could not connect to database:", err)
	}
	defer db.Close()

	messageCallback := func(message Message, chat *Chat) {
		log.Print("message:", message)
		subscribed := findSubscribedChats(db, *chat)
		sentToAllSubscribers := true
		for _, subscription := range subscribed {
			if !messengers[subscription.Type].SendMessage(message, &subscription) {
				addUnsentMessage(db, QueuedMessage{Message: message, Destination: subscription})
				sentToAllSubscribers = false
			}
		}
		if sentToAllSubscribers {
			for _, attachment := range message.Attachments {
				deleteAttachment(attachment)
			}
		}
	}

	addSubscription := func(subscriber *Chat, subscriptionToken string) {
		log.Printf("subscribe %+v on chat with token %s", subscriber, subscriptionToken)
		var statusMessage string
		if subscribe(db, subscriber, subscriptionToken) {
			statusMessage = "successfully subscribed!"
		} else {
			statusMessage = "could not subscribe on chat with given token"
		}
		messengers[subscriber.Type].SendMessage(Message{Text: statusMessage}, subscriber)
	}

	cancelSubscription := func(subscriber *Chat, subscriptionToken string) {
		log.Printf("unsubscribe chat %+v from chat with token %s", subscriber, subscriptionToken)
		var statusMessage string
		if unsubscribe(db, subscriber, subscriptionToken) {
			statusMessage = "successfully unsubscribed!"
		} else {
			statusMessage = "could not unsubscribe from chat with given token"
		}
		messengers[subscriber.Type].SendMessage(Message{Text: statusMessage}, subscriber)
	}

	getChatById := func(id int64, messenger string) *Chat {
		return getChat(db, id, messenger)
	}

	createNewChat := func(id int64, messenger string) *Chat {
		return addChat(db, id, messenger)
	}

	baseMessenger := BaseMessenger{messageCallback, addSubscription, cancelSubscription, getChatById, createNewChat}

	VKMessenger := NewVKMessenger(baseMessenger)
	TGMessenger := NewTGMessenger(baseMessenger)

	messengers["vk"] = VKMessenger
	messengers["tg"] = TGMessenger

	go repeatedFileCleanup(db)
	go TGMessenger.Run()

	log.Println("Start Long Poll")
	VKMessenger.Run()
}
