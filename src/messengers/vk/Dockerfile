# syntax=docker/dockerfile:1
# !!! Run from TransferBot/src/ directory

FROM golang:alpine

WORKDIR /usr/src/app

ARG DEBUG
ENV DEBUG=$DEBUG
RUN if [[ -n "$DEBUG" ]]; then \
        go install github.com/go-delve/delve/cmd/dlv@latest; \
    fi

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

RUN if [[ -n "$DEBUG" ]]; then \
        go build -gcflags "all=-N -l" -v -o /usr/local/bin/app; \
    else \
        go build -v -o /usr/local/bin/app; \
    fi

ARG DEBUG_PORT
ENV DEBUG_PORT=$DEBUG_PORT
EXPOSE ${DEBUG_PORT}

CMD if [ -n "$DEBUG" ]; then \
        dlv --listen=:$DEBUG_PORT --headless=true --api-version=2 --accept-multiclient exec app \
    else \
        app \
    fi
