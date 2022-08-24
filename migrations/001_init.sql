-- +goose Up
CREATE TABLE IF NOT EXISTS Chats (
    chat_id BIGINT NOT NULL,
    chat_type TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    internal_id SERIAL PRIMARY KEY,
    CONSTRAINT unique_chat UNIQUE (chat_id, chat_type)
);

CREATE TABLE IF NOT EXISTS Subscriptions (
    source_chat SERIAL NOT NULL,
    destination_chat SERIAL NOT NULL,
    FOREIGN KEY (source_chat) REFERENCES Chats(internal_id),
    FOREIGN KEY (destination_chat) REFERENCES Chats(internal_id),
    CONSTRAINT single_subscription UNIQUE (source_chat, destination_chat)
);

CREATE TABLE IF NOT EXISTS Messages (
    destination_chat SERIAL NOT NULL,
    sender TEXT,
    message_text TEXT,
    sender_chat TEXT NOT NULL,
    internal_id SERIAL PRIMARY KEY,
    FOREIGN KEY (destination_chat) REFERENCES Chats(internal_id)
);

CREATE TABLE IF NOT EXISTS Attachments (
    data_type TEXT NOT NULL,
    data_url TEXT NOT NULL,
    parent_message SERIAL,
    FOREIGN KEY (parent_message) REFERENCES Messages(internal_id)
);

-- +goose Down
DROP TABLE IF EXISTS Attachments;
DROP TABLE IF EXISTS Messages;
DROP TABLE IF EXISTS Subscriptions;
DROP TABLE IF EXISTS Chats;
