# syntax=docker/dockerfile:1

FROM golang:alpine

WORKDIR /app

RUN apk update
RUN apk add postgresql
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

ARG DEBUG
ENV DEBUG=$DEBUG

# Delve (Go debugger) seems to conflict with gRPC server.
# Current debug configuration breaks connections before the responses are sent.
# Nevertheless, it allows requests to reach server, so it is not completely useless.
RUN if [ -n "$DEBUG" ]; then \
        go install github.com/go-delve/delve/cmd/dlv@latest; \
    fi

COPY proto/go.mod proto/go.sum proto/
COPY controller/go.mod controller/go.sum controller/
WORKDIR controller
RUN go mod download
WORKDIR ..

COPY proto/ proto/
COPY controller/ controller/

WORKDIR controller
RUN mkdir data
RUN if [ -n "$DEBUG" ]; then \
        go build -gcflags "all=-N -l" -o /transfer_bot; \
    else \
        go build -o /transfer_bot; \
    fi

RUN chmod +x ./docker_entrypoint.sh

ARG DEBUG_PORT
ENV DEBUG_PORT=$DEBUG_PORT
EXPOSE ${DEBUG_PORT}

CMD ["./docker_entrypoint.sh"]
