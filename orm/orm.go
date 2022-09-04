package orm

import (
	"Pelmenner/TransferBot/config"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
)

// TODO: ?
type Attachment struct {
	Type string
	URL  string
}

type Sender struct {
	Name string
	Chat string
}

type Message struct {
	Text string
	Sender
	Attachments []*Attachment
}

type Chat struct {
	ID    int64
	Token string
	Type  string
	RowID int32
}

type QueuedMessage struct {
	Message
	Destination Chat
}

func transact(db *sql.DB, txOpts *sql.TxOptions, txFunc func(*sql.Tx) error) (err error) {
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
func FindSubscribedChats(db *sql.DB, chat Chat) ([]Chat, error) {
	rows, err := db.Query(`SELECT chat_id, token, chat_type, Chats.internal_id
	FROM Subscriptions JOIN Chats ON Subscriptions.destination_chat = Chats.internal_id
	WHERE source_chat = $1`, chat.RowID)
	if err != nil {
		return []Chat{}, err
	}

	res := []Chat{}
	for rows.Next() {
		buf := Chat{}

		err := rows.Scan(&buf.ID, &buf.Token, &buf.Type, &buf.RowID)
		if err != nil {
			return []Chat{}, err
		}

		res = append(res, buf)
	}

	return res, nil
}

func generateToken(chatID int64, chatType string) string {
	salt := time.Now().Format(time.UnixDate)
	hash := sha256.Sum256([]byte(fmt.Sprintf("%v %s %s", chatID, chatType, salt)))
	return fmt.Sprintf("%x", hash)[:config.TokenLength]
}

// addChat creates new chat entry with given id in messenger and type
func AddChat(db *sql.DB, chatID int64, chatType string) (*Chat, error) {
	token := generateToken(chatID, chatType)

	res := db.QueryRow("INSERT INTO Chats VALUES ($1, $2, $3) RETURNING internal_id",
		&chatID, &chatType, &token)

	var rowID int32
	err := res.Scan(&rowID)
	if err != nil {
		return nil, err
	}

	return &Chat{
		ID:    chatID,
		Token: token,
		Type:  chatType,
		RowID: rowID,
	}, nil
}

// getChat returns chat object with given id in messenger
func GetChat(db *sql.DB, chatID int64, chatType string) (*Chat, error) {
	res := Chat{ID: chatID, Type: chatType}
	row := db.QueryRow(`SELECT token, internal_id FROM Chats
	WHERE chat_id = $1 AND chat_type = $2`, &chatID, &chatType)

	err := row.Scan(&res.Token, &res.RowID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &res, nil
}

// addUnsentMessage adds message to send later
func AddUnsentMessage(db *sql.DB, message QueuedMessage) error {
	err := transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			res := tx.QueryRow(`INSERT INTO Messages
								VALUES ($1, $2, $3, $4)
								RETURNING internal_id`,
				&message.Destination.RowID,
				&message.Sender.Name, &message.Text, &message.Sender.Chat)
			var messageRowID int32
			err := res.Scan(&messageRowID)
			if err != nil {
				return err
			}

			// TODO: remove SQL query from loop
			for _, attachment := range message.Attachments {
				_, err := tx.Exec(`INSERT INTO Attachments
								   VALUES ($1, $2, $3)`,
					&attachment.Type, &attachment.URL, &messageRowID)
				if err != nil {
					return err
				}
			}

			return nil
		})

	return err
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
func GetUnsentMessages(db *sql.DB, maxCnt int) ([]QueuedMessage, error) {
	res := []QueuedMessage{}
	err := transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			rows, err := tx.Query(`SELECT sender, sender_chat, message_text, Messages.internal_id, chat_id, chat_type, token, Chats.internal_id
								   FROM Messages JOIN Chats ON Messages.destination_chat = Chats.internal_id
								   LIMIT $1`, &maxCnt)
			if err != nil {
				return err
			}

			messagesToDelete := []int{}
			// TODO: remove queries from loop
			for rows.Next() {
				message := QueuedMessage{}
				messageRowID := -1
				err := rows.Scan(&message.Sender.Name, &message.Sender.Chat, &message.Text, &messageRowID,
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
				_, err := tx.Exec("DELETE FROM Messages WHERE Messages.internal_id = $1", &id)
				if err != nil {
					return err
				}
				_, err = tx.Exec(`UPDATE Attachments SET parent_message = NULL
				                   WHERE parent_message = $1`, &id)
				if err != nil {
					return err
				}
			}

			return nil
		})

	if err != nil {
		return nil, nil
	}

	return res, nil
}

func getChatRowIDByToken(tx *sql.Tx, token string) (int, error) {
	row := tx.QueryRow("SELECT internal_id FROM Chats WHERE token = $1", &token)
	rowID := -1
	err := row.Scan(&rowID)
	return rowID, err
}

// Subscribes proveded chat on another with given token.
func Subscribe(db *sql.DB, subscriber *Chat, subscriptionToken string) error {
	err := transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
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

	return err
}

// Unsubscribes provided chat from another with given token.
func Unsubscribe(db *sql.DB, subscriber *Chat, subscriptionToken string) error {
	err := transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			subscriptionRowID, err := getChatRowIDByToken(tx, subscriptionToken)
			if err != nil {
				return err
			}

			res, err := tx.Exec("DELETE FROM Subscriptions WHERE source_chat = $1 AND destination_chat = $2",
				&subscriptionRowID, &subscriber.RowID)
			if err != nil {
				return err
			}

			cntRemoved, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if cntRemoved < 1 {
				return errors.New("no subscription")
			}

			return nil
		})

	return err
}

// getUnusedAttachments returns all attachments which will be never sent anymore and deletes them
func GetUnusedAttachments(db *sql.DB) ([]*Attachment, error) {
	res := []*Attachment{}
	err := transact(db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			rows, err := tx.Query(`SELECT data_type, data_url, Attachments.internal_id
						   		   FROM Attachments LEFT JOIN Messages
								   ON Attachments.parent_message = Messages.internal_id
								   WHERE destination_chat IS NULL`)
			if err != nil {
				return err
			}

			stmt, err := tx.Prepare("DELETE FROM Attachments WHERE internal_id = $1")
			if err != nil {
				return err
			}

			for rows.Next() {
				attachment := Attachment{}
				rowID := -1
				err = rows.Scan(&attachment.Type, &attachment.URL, &rowID)
				if err != nil {
					return err
				}

				res = append(res, &attachment)
				_, err = stmt.Exec(&rowID)
				if err != nil {
					return err
				}
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	return res, err
}
