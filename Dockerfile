# syntax=docker/dockerfile:1

FROM golang:1.18.3-bullseye

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./
COPY config/*.go ./config/
RUN go build -o /transfer_bot

CMD ["/transfer_bot"]
