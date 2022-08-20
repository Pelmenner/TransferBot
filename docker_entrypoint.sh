#!/bin/sh

set -euo pipefail

if ! [ -z $MIGRATE_DB ]; then
    goose -dir ./migrations sqlite3 ./data/db.sqlite3 up
fi

/transfer_bot
