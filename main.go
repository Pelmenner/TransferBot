package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"
)

type User struct {
	Name string
	TGId int
	VKId int
}

func goDotEnvVariable(key string) string {
	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return os.Getenv(key)
}

func sendInTelegram(user User, photoURLs []string /*TODO: make it structure*/) {
	// TODO: Implement
}

func getUserByVKId(id int) User {
	return User{}
}

func findOriginal(message object.MessagesMessage) object.MessagesMessage {
	// recursion deeper than 2 doesn't work for some reason
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

func processVkMessage(vk *api.VK, obj events.MessageNewObject) {
	log.Printf("%d: %s", obj.Message.PeerID, obj.Message.Text)
	images := getImages(obj.Message)
	log.Printf("image links: %v", images)
	user := getUserByVKId()
	sendInTelegram(user, images)
}

func main() {
	token := goDotEnvVariable("VK_TOKEN")
	vk := api.NewVK(token)

	// get information about the group
	group, err := vk.GroupsGetByID(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Initializing Long Poll
	lp, err := longpoll.NewLongPoll(vk, group[0].ID)
	if err != nil {
		log.Fatal(err)
	}

	// New message event
	lp.MessageNew(func(_ context.Context, obj events.MessageNewObject) {
		processVkMessage(vk, obj)
	})

	// Run Bots Long Poll
	log.Println("Start Long Poll")
	if err := lp.Run(); err != nil {
		log.Fatal(err)
	}
}
