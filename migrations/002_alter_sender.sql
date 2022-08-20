-- +goose Up
ALTER TABLE Messages
ADD sender_chat TEXT NOT NULL
DEFAULT "";

-- +goose Down
ALTER TABLE Messages
DROP COLUMN sender_chat IF EXISTS;
