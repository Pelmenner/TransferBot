CREATE TABLE IF NOT EXISTS Chats (
    chat_id INTEGER NOT NULL,
    chat_type TEXT NOT NULL,
    token TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS Subscriptions (
    source_chat INTEGER NOT NULL,
    destination_chat INTEGER NOT NULL,
    FOREIGN KEY (source_chat) REFERENCES Chats(rowid),
    FOREIGN KEY (destination_chat) REFERENCES Chats(rowid)
);

CREATE TABLE IF NOT EXISTS Messages (
    destination_chat INTEGER NOT NULL,
    sender TEXT,
    message_text TEXT,
    FOREIGN KEY (destination_chat) REFERENCES Chats(rowid)
);

CREATE TABLE IF NOT EXISTS Attachments (
    data_type TEXT NOT NULL,
    data_url TEXT NOT NULL,
    parent_message INTEGER,
    FOREIGN KEY (parent_message) REFERENCES Messages(rowid)
);
