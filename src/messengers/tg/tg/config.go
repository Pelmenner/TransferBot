package tg

import (
	"log"
	"os"
	"time"
)

var config = struct {
	TGSleepIntervalSec int
	MediaGroupWaitTime time.Duration
	TGBotAPITimeoutSec int
	Token              string
}{
	TGSleepIntervalSec: 50,
	MediaGroupWaitTime: time.Second * 2,
	TGBotAPITimeoutSec: 60,
	Token:              os.Getenv("TG_TOKEN"),
}

func init() {
	if len(config.Token) == 0 {
		log.Panic("Telegram token not provided")
	}
}
