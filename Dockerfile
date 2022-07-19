# syntax=docker/dockerfile:1

FROM golang:1.18.3-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download
RUN apk update
RUN apk add sqlite gcc musl-dev

COPY *.go ./
COPY config/*.go ./config/
COPY messenger/*.go ./messenger/
COPY utils/*.go ./utils/
COPY orm/*.go ./orm/
COPY db_init.sql ./
RUN mkdir data
RUN sqlite3 data/db.sqlite3 < db_init.sql
RUN go build -o /transfer_bot

CMD ["/transfer_bot"]
