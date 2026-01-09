module github.com/Pelmenner/TransferBot/messenger

go 1.24.0

replace github.com/Pelmenner/TransferBot/proto v0.0.0 => ../../proto

require (
	github.com/Pelmenner/TransferBot/proto v0.0.0
	github.com/nats-io/nats.go v1.48.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251213004720-97cd9d5aeac2 // indirect
)
