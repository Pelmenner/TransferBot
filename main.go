package main

import (
	"log"
)

func main() {
	subscriptions := make(map[*Chat][]*Chat)
	messengers := make(map[string]Messenger)
	tokens := make(map[string]*Chat)

	messageCallback := func(message Message, chat *Chat) {
		log.Print("message:", message)
		for _, subscription := range subscriptions[chat] {
			messengers[subscription.Type].SendMessage(message, subscription)
		}
	}

	subscriptionCallback := func(subscriber *Chat, subscriptionToken string) {
		log.Print("subscription:", subscriber, subscriptionToken)
		if subscription, exists := tokens[subscriptionToken]; exists {
			subscriptions[subscription] = append(subscriptions[subscription], subscriber)
			messengers[subscriber.Type].SendMessage(Message{Text: "successfully subcsribed!"}, subscriber)
		}
	}

	chatCreatedCallback := func(chat *Chat) {
		tokens[chat.Token] = chat
	}

	baseMessenger := BaseMessenger{messageCallback, subscriptionCallback, chatCreatedCallback}

	VKMessenger := newVKMessenger(baseMessenger)
	TGMessenger := newTGMessenger(baseMessenger)

	messengers["vk"] = VKMessenger
	messengers["tg"] = TGMessenger

	go TGMessenger.Run()

	// Run Bots Long Poll
	log.Println("Start Long Poll")
	VKMessenger.Run()
}
