protoc --go_out=messenger --go_opt=paths=source_relative \
    --go-grpc_out=messenger --go-grpc_opt=paths=source_relative \
    --experimental_allow_proto3_optional ./messenger.proto
protoc --go_out=controller --go_opt=paths=source_relative  \
    --go-grpc_out=controller --go-grpc_opt=paths=source_relative \
    --experimental_allow_proto3_optional ./controller.proto
