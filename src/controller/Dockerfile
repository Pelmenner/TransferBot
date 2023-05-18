# syntax=docker/dockerfile:1

FROM golang:alpine

WORKDIR /app

RUN apk update
RUN apk add postgresql
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

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

RUN chmod +x ./docker_entrypoint.sh

CMD ["./docker_entrypoint.sh"]
