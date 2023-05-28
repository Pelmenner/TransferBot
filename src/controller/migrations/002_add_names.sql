-- +goose Up
ALTER TABLE Chats
ADD COLUMN name TEXT;

UPDATE Chats
SET name = chat_type;

-- +goose Down
ALTER TABLE Chats
DROP COLUMN name;