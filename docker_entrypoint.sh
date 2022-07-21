#!/bin/sh

set -euo pipefail

if ! test -z ${MIGRATE_DB+x}; then
    goose -dir ./migrations sqlite3 ./data/db.sqlite3 up
fi

/transfer_bot
