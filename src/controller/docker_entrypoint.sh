#!/bin/sh

set -euo pipefail

until psql --dbname="$DB_CONNECT_STRING" -c '\q'; do
    echo "Database is down - sleeping" >&2
    sleep 3
done

if [ -n "$MIGRATE_DB" ]; then
    goose -dir ./migrations postgres "$DB_CONNECT_STRING" up
fi

if [ -n "${DEBUG:-}" ];  then
    dlv --listen=:$DEBUG_PORT --headless=true --api-version=2 --accept-multiclient exec /transfer_bot
else
    /transfer_bot
fi
