# TransferBot

## Running in Docker container

Bot stores some persistent files in `data` directory, hence you'd better mount this directory to some docker volume to update more easily. 
Storing environmental variables in local `.env` is also a good practice.

To run migrations on database, you can pass `MIGRATE_DB=1` environment variable to container

Thus, your Docker call may look somewhat like this: 

```
docker run -d --env-file .env \
    --mount source=bot-data,destination=/app/data \
    transfer-bot
```