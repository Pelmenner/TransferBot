module github.com/Pelmenner/TransferBot/tg

go 1.24.0

require (
	github.com/Pelmenner/TransferBot/messenger v0.0.0
	github.com/Pelmenner/TransferBot/proto v0.0.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/golang/protobuf v1.5.4
	golang.org/x/time v0.3.0
	google.golang.org/grpc v1.76.0
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nats.go v1.48.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251213004720-97cd9d5aeac2 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/Pelmenner/TransferBot/messenger v0.0.0 => ../messenger

replace github.com/Pelmenner/TransferBot/proto v0.0.0 => ../../proto
