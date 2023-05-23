package vk

import (
	"log"
	"os"
	"strconv"
	"time"
)

var Config = struct { // TODO;
	TGSleepIntervalSec     int
	MediaGroupWaitTime     time.Duration
	TGBotAPITimeoutSec     int
	LongPollRestartMaxRate int
	Token                  string
	Port                   int
	ControllerHost         string
}{
	TGSleepIntervalSec:     50,
	MediaGroupWaitTime:     time.Second * 2,
	TGBotAPITimeoutSec:     60,
	LongPollRestartMaxRate: 10,
	Token:                  os.Getenv("VK_TOKEN"),
	ControllerHost:         os.Getenv("CONTROLLER_HOST"),
}

func init() {
	if len(Config.Token) == 0 {
		log.Panic("VK token not provided")
	}
}

func init() {
	port := os.Getenv("PORT")
	var err error
	Config.Port, err = strconv.Atoi(port)
	if err != nil {
		log.Panic("Invalid VK service port")
	}
}
