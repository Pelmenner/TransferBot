-- +goose Up
DROP TABLE Attachments;
DROP TABLE Messages;    

-- +goose Down
CREATE TABLE Messages
(
    destination_chat INTEGER NOT NULL,
    sender           TEXT,
    message_text     TEXT,
    sender_chat      TEXT    NOT NULL,
    internal_id      SERIAL PRIMARY KEY,
    FOREIGN KEY (destination_chat) REFERENCES Chats (internal_id)
);

CREATE TABLE Attachments
(
    data_type      TEXT NOT NULL,
    data_url       TEXT NOT NULL,
    parent_message INTEGER,
    internal_id    SERIAL PRIMARY KEY,
    FOREIGN KEY (parent_message) REFERENCES Messages (internal_id)
);
