package messenger

import . "Pelmenner/TransferBot/orm"

type CallbackOnMessageReceived func(message Message, chat *Chat)
type SubscriptionCallback func(subsriber *Chat, subscriptionToken string)
type ChatGetter func(id int64, messenger string) *Chat
type ChatCreator func(id int64, messenger string) *Chat

type Messenger interface {
	SendMessage(message Message, chat *Chat) bool
	Run()
}

type BaseMessenger struct {
	MessageCallback     CallbackOnMessageReceived
	SubscribeCallback   SubscriptionCallback
	UnsubscribeCallback SubscriptionCallback
	GetChatById         ChatGetter
	CreateNewChat       ChatCreator
}
