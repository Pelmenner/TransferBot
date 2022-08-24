#!/bin/sh

set -euo pipefail

if ! [ -z $MIGRATE_DB ]; then
    goose -dir ./migrations postgres "$DB_CONNECT_STRING" up
fi

/transfer_bot
