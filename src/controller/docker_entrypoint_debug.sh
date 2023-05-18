#!/bin/sh

set -euo pipefail

until psql --dbname="$DB_CONNECT_STRING" -c '\q'; do
    echo "Database is down - sleeping" >&2
    sleep 3
done

if ! [ -z $MIGRATE_DB ]; then
    goose -dir ./migrations postgres "$DB_CONNECT_STRING" up
fi

dlv --listen=:40000 --headless=true --api-version=2 --continue --accept-multiclient exec /transfer_bot
