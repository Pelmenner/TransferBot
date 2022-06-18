# TransferBot

## Running in Docker container

Bot stores some persistent files in `data` directory, hence you'd better mount this directory to some docker volume to update more easily. 
Storing environmental variables in local `.env` is also a good practice.
Thus, your Docker call may look somewhat like this: 

```
docker run -d --rm --env-file .env \
    --mount source=bot-data,destination=/app/data \
    transfer-bot
```