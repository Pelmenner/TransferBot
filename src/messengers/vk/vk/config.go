package vk

import "time"

var config = struct { // TODO;
	TGSleepIntervalSec     int
	MediaGroupWaitTime     time.Duration
	TGBotAPITimeoutSec     int
	LongPollRestartMaxRate int
}{
	TGSleepIntervalSec:     50,
	MediaGroupWaitTime:     time.Second * 2,
	TGBotAPITimeoutSec:     60,
	LongPollRestartMaxRate: 10,
}
