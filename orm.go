package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"

	_ "github.com/mattn/go-sqlite3"
)

// TODO: ?
type Attachment struct {
	Type string
	URL  string
}

type Message struct {
	Text        string
	Sender      string
	Attachments []*Attachment
}

type Chat struct {
	ID    int
	Token string
	Type  string
	RowID int
}

type QueuedMessage struct {
	Message
	Destination Chat
}

func Transact(db *sql.DB, txOpts *sql.TxOptions, txFunc func(*sql.Tx) error) error {
	tx, err := db.BeginTx(context.TODO(), txOpts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = txFunc(tx)
	return err
}

// findSubscribedChats returns all chats subscribed on the given one
func findSubscribedChats(db *sql.DB, chat Chat) []Chat {
	rows, err := db.Query(`SELECT chat_id, token, chat_type, Chats.rowid
	FROM Subscriptions JOIN Chats ON Subscriptions.destination_chat = Chats.rowid
	WHERE source_chat = $1`, chat.RowID)
	if err != nil {
		log.Fatal(err)
	}

	res := []Chat{}
	for rows.Next() {
		buf := Chat{}

		err := rows.Scan(&buf.ID, &buf.Token, &buf.Type, &buf.RowID)
		if err != nil {
			log.Fatal(err)
		}

		res = append(res, buf)
	}

	return res
}

// addChat creates new chat entry with given id in messenger and type
func addChat(db *sql.DB, chatID int, chatType string) bool {
	length := 10
	b := make([]byte, length)
	rand.Read(b)
	token := fmt.Sprintf("%x", b)[:length]

	_, err := db.Exec("INSERT INTO Chats VALUES ($1, $2, $3)",
		&chatID, &chatType, &token)
	if err != nil {
		log.Print(err)
		return false
	}

	return true
}

// getChat returns chat object with given id in messenger
// if an error occured, returns nil and logs error
func getChat(db *sql.DB, chatID int, chatType string) *Chat {
	res := Chat{ID: chatID, Type: chatType}
	row := db.QueryRow(`SELECT token, rowid FROM Chats
	WHERE chat_id = $1 AND chat_type = $2`, &chatID, &chatType)

	err := row.Scan(&res.Token, &res.RowID)
	if err != nil {
		log.Print(err)
		return nil
	}

	return &res
}

// addUnsentMessage adds message to send later
func addUnsentMessage(db *sql.DB, message Message) {

}

func getMessageAttachments(tx *sql.Tx, messageRowID int, attachments []*Attachment) error {
	rows, err := tx.Query(`SELECT data_type, data_url
						   FROM Attachments
						   WHERE parent_message = $1`, &messageRowID)
	if err != nil {
		return err
	}

	for rows.Next() {
		attachment := &Attachment{}
		err = rows.Scan(&attachment.Type, &attachment.URL)
		if err != nil {
			return err
		}
		attachments = append(attachments, attachment)
	}

	return nil
}

// getUnsentMessages returns all messages to send and deletes them from db
func getUnsentMessages(db *sql.DB, maxCnt int) []QueuedMessage {
	res := []QueuedMessage{}
	err := Transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false},
		func(tx *sql.Tx) error {
			rows, err := tx.Query(`SELECT sender, message_text, Messages.rowid, chat_id, chat_type, token, Chats.rowid
								   FROM Messages JOIN Chats ON Messages.destination_chat = Chats.rowid
								   LIMIT $1`, &maxCnt)
			if err != nil {
				return err
			}

			messagesToDelete := []int{}
			// TODO: remove queries from loop
			for rows.Next() {
				message := QueuedMessage{}
				messageRowID := -1
				err := rows.Scan(&message.Sender, &message.Text, &messageRowID,
					&message.Destination.ID, &message.Destination.Type,
					&message.Destination.Token, &message.Destination.RowID)
				if err != nil {
					return err
				}

				if err = getMessageAttachments(tx, messageRowID, message.Attachments); err != nil {
					return err
				}
				messagesToDelete = append(messagesToDelete, messageRowID)
				res = append(res, message)
			}

			for _, id := range messagesToDelete {
				_, err := tx.Exec("DELETE FROM Messages WHERE Messages.rowid = $1", &id)
				if err != nil {
					return err
				}
				_, err = tx.Exec(`UPDATE Messages SET parent_message = NULL
				                   WHERE parent_message = $1`, &id)
				if err != nil {
					return err
				}
			}

			return nil
		})

	if err != nil {
		log.Print(err)
		return []QueuedMessage{}
	}

	return res
}

func getChatRowIDByToken(tx *sql.Tx, token string) (int, error) {
	row := tx.QueryRow("SELECT rowid FROM Chats WHERE token = $1", &token)
	rowID := -1
	err := row.Scan(&rowID)
	return rowID, err
}

// Subscribes proveded chat on another with given token.
//  Returns true on success
func subscribe(db *sql.DB, subscriber *Chat, subscriptionToken string) bool {
	err := Transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false},
		func(tx *sql.Tx) error {
			subscriptionRowID, err := getChatRowIDByToken(tx, subscriptionToken)
			if err != nil {
				return err
			}

			if _, err := tx.Exec("INSERT INTO Subscriptions VALUES ($1, $2)",
				&subscriptionRowID, &subscriber.RowID); err != nil {
				return err
			}

			return nil
		})

	if err != nil {
		log.Print(err)
		return false
	}

	return true
}

// Unsubscribes provided chat from another with given token.
//  Returns true on success
func unsubscribe(db *sql.DB, subscriber *Chat, subscriptionToken string) bool {
	err := Transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false},
		func(tx *sql.Tx) error {
			subscriptionRowID, err := getChatRowIDByToken(tx, subscriptionToken)
			if err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM Subscriptions WHERE source_chat = $1 AND destination_chat = $2",
				&subscriptionRowID, &subscriber.RowID); err != nil {
				return err
			}

			return nil
		})

	if err != nil {
		log.Print(err)
		return false
	}

	return true
}

// getUnusedAttachments returns all attachments which will be never sent anymore and deletes them
func getUnusedAttachments(db *sql.DB) []*Attachment {
	return []*Attachment{}
}
