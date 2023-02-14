package config

const (
	TokenLength            = 10
	MediaGroupWaitTimeSec  = 2
	TGBotAPITimeoutSec     = 60
	TGSleepIntervalSec     = 50
	FileCleanupIntervalSec = 60
	RetrySendIntervalSec   = 60
	UnsentRetrieveMaxCnt   = 10
	LongPollRestartMaxRate = 0.2 // restarts per sec
)
