# syntax=docker/dockerfile:1
# !!! Run from TransferBot/src/ directory

FROM golang:alpine

WORKDIR /usr/src/app

COPY proto/go.mod proto/go.sum proto/
COPY messengers/messenger/go.mod messengers/messenger/go.sum messengers/messenger/
COPY messengers/tg/go.mod messengers/tg/go.sum messengers/tg/

WORKDIR messengers/tg
RUN go mod download && go mod verify
WORKDIR ../..

COPY proto/ proto/
COPY messengers/messenger/*.go messengers/messenger/
COPY messengers/tg/tg/*.go messengers/tg/tg/
COPY messengers/tg/*.go messengers/tg/

WORKDIR messengers/tg
RUN mkdir -p /usr/local/bin/
RUN go mod tidy
RUN go build -v -o /usr/local/bin/app

CMD ["app"]
