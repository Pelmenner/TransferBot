# TransferBot

## Description

A bot for transfering messages from one messenger to another.

Supported attachment types: photos, files, files up to 50 MB  
Supported messengers: VK, Telegram

## Links

- VK bot: [transfer_bot](https://vk.com/transfer_bot)
- Telegram bot: [@content_transfer_bot](https://t.me/content_transfer_bot)

## How to use

- Add the bot (use the links above to find it) to the sender and receiver groups or start a private dialogue with it
- Retrieve a token of the chat from which messages should be transfered by using <b><i>/get_token</i></b> command
- Subscribe on the channel by using <b><i>/subscribe \<token\></i></b> command in receiving chat
- In case the subscription is no more needed, unsubscribe from a channel using <b><i>/unsubscribe \<token\></i></b>

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
