# syntax=docker/dockerfile:1

FROM golang:alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download
RUN apk update
RUN apk add gcc musl-dev
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

COPY *.go ./
COPY config/*.go ./config/
COPY messenger/*.go ./messenger/
COPY utils/*.go ./utils/
COPY orm/*.go ./orm/
COPY migrations/*.sql ./migrations/

RUN mkdir data
RUN go build -o /transfer_bot

COPY docker_entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

CMD ["./entrypoint.sh"]
