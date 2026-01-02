module github.com/Pelmenner/TransferBot/messenger

go 1.24.0

replace github.com/Pelmenner/TransferBot/proto v0.0.0 => ../../proto

require (
	github.com/Pelmenner/TransferBot/proto v0.0.0
	google.golang.org/grpc v1.56.3
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
