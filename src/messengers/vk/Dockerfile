# syntax=docker/dockerfile:1
# !!! Run from TransferBot/src/ directory

FROM golang:alpine

WORKDIR /usr/src/app

COPY proto/go.mod proto/go.sum proto/
COPY messengers/messenger/go.mod messengers/messenger/go.sum messengers/messenger/
COPY messengers/vk/go.mod messengers/vk/go.sum messengers/vk/

WORKDIR messengers/vk
RUN go mod download && go mod verify
WORKDIR ../..

COPY proto/ proto/
COPY messengers/messenger/*.go messengers/messenger/
COPY messengers/vk/vk/*.go messengers/vk/vk/
COPY messengers/vk/*.go messengers/vk/

WORKDIR messengers/vk
RUN mkdir -p /usr/local/bin/
RUN go build -v -o /usr/local/bin/app

CMD ["app"]
