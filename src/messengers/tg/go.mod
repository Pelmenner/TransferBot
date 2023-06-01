module github.com/Pelmenner/TransferBot/tg

go 1.20

require (
	github.com/Pelmenner/TransferBot/messenger v0.0.0
	github.com/Pelmenner/TransferBot/proto v0.0.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/golang/protobuf v1.5.3
	golang.org/x/time v0.3.0
	google.golang.org/grpc v1.55.0
)

require (
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)

replace github.com/Pelmenner/TransferBot/messenger v0.0.0 => ../messenger

replace github.com/Pelmenner/TransferBot/proto v0.0.0 => ../../proto
