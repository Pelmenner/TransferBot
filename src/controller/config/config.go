package config

import "os"

const (
	TokenLength            = 10
	FileCleanupIntervalSec = 60
	RetrySendIntervalSec   = 60
	UnsentRetrieveMaxCnt   = 10
)

var MessengerAddresses = map[string]string{
	"vk": os.Getenv("VK_SERVICE_HOST"),
	"tg": os.Getenv("TG_SERVICE_HOST"),
}
