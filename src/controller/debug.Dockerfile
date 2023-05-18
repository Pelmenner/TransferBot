# syntax=docker/dockerfile:1

# Delve (Go debugger) seems to conflict with gRPC server.
# Current debug configuration breaks connections before the responses are sent.
# However, it allows requests to reach server, so it is not completely useless.

FROM golang:alpine

WORKDIR /app

RUN apk update
RUN apk add postgresql
RUN go install github.com/pressly/goose/v3/cmd/goose@latest
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY proto/go.mod proto/go.sum proto/
COPY controller/go.mod controller/go.sum controller/
WORKDIR controller
RUN go mod download
WORKDIR ..

COPY proto/ proto/
COPY controller/ controller/

WORKDIR controller
RUN mkdir data
RUN go build -o /transfer_bot

RUN chmod +x ./docker_entrypoint_debug.sh

EXPOSE 8000 40000

CMD ["./docker_entrypoint_debug.sh"]
