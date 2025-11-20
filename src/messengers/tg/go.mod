module github.com/Pelmenner/TransferBot/tg

go 1.23.0

require (
	github.com/Pelmenner/TransferBot/messenger v0.0.0
	github.com/Pelmenner/TransferBot/proto v0.0.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/golang/protobuf v1.5.3
	golang.org/x/time v0.3.0
	google.golang.org/grpc v1.56.3
)

require (
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

replace github.com/Pelmenner/TransferBot/messenger v0.0.0 => ../messenger

replace github.com/Pelmenner/TransferBot/proto v0.0.0 => ../../proto
