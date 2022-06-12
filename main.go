package main

import (
	"database/sql"
	"log"
)

func main() {
	messengers := make(map[string]Messenger)

	db, err := sql.Open("sqlite3", "./db.sqlite3")
	defer db.Close()
	if err != nil {
		log.Fatal("could not connect to database:", err)
	}

	messageCallback := func(message Message, chat *Chat) {
		log.Print("message:", message)
		subscribed := findSubscribedChats(db, *chat)
		for _, subscription := range subscribed {
			messengers[subscription.Type].SendMessage(message, &subscription)
		}
	}

	subscriptionCallback := func(subscriber *Chat, subscriptionToken string) {
		log.Print("subscription:", subscriber, subscriptionToken)
		if subscribe(db, subscriber, subscriptionToken) {
			messengers[subscriber.Type].SendMessage(Message{Text: "successfully subscribed!"}, subscriber)
		}
	}

	getChatById := func(id int64, messenger string) *Chat {
		return getChat(db, id, messenger)
	}

	createNewChat := func(id int64, messenger string) *Chat {
		return addChat(db, id, messenger)
	}

	baseMessenger := BaseMessenger{messageCallback, subscriptionCallback, getChatById, createNewChat}

	VKMessenger := NewVKMessenger(baseMessenger)
	TGMessenger := NewTGMessenger(baseMessenger)

	messengers["vk"] = VKMessenger
	messengers["tg"] = TGMessenger

	go TGMessenger.Run()

	log.Println("Start Long Poll")
	VKMessenger.Run()
}
