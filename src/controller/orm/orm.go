package orm

import (
	"Pelmenner/TransferBot/config"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
)

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
	ID         int64
	Type       string
	Name       string
	internalID int32
	complete   bool
}

// fillOrCreate tries to find a chat with given id and type in the database.
// If no matches are found, a new instance is created and a new internalID assigned.
func (c *Chat) fillOrCreate(db *DB) error {
	if c.complete {
		return nil
	}
	chat, err := db.GetOrCreateChat(c.ID, c.Type)
	if err != nil {
		return err
	}
	*c = *chat // does it work?
	return err
}

type DB struct {
	*sql.DB
}

func NewDB() *DB {
	db, err := sql.Open("pgx", config.DBConnectString)
	if err != nil {
		log.Fatal("could not connect to database:", err)
	}
	return &DB{db}
}

func (db *DB) transact(txOpts *sql.TxOptions, txFunc func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(context.TODO(), txOpts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			err = tx.Rollback()
			panic(p)
		} else if err != nil {
			err = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = txFunc(tx)
	return err
}

// FindSubscribedChats returns all chats subscribed on the given one
func (db *DB) FindSubscribedChats(chat Chat) ([]Chat, error) {
	if err := chat.fillOrCreate(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT chat_id, chat_type, name, Chats.internal_id
	FROM Subscriptions JOIN Chats ON Subscriptions.destination_chat = Chats.internal_id
	WHERE source_chat = $1`, chat.internalID)
	if err != nil {
		return []Chat{}, err
	}

	var res []Chat
	for rows.Next() {
		buf := Chat{complete: true}
		err := rows.Scan(&buf.ID, &buf.Type, &buf.Name, &buf.internalID)
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

// GetOrCreateChat tries to find a chat by id and messenger
// If it does not exist, a new instance is created.
// In both cases either a complete chat object or an error is returned.
func (db *DB) GetOrCreateChat(id int64, messenger string) (*Chat, error) {
	chat, err := db.GetChat(id, messenger)
	if err != nil || chat != nil {
		return chat, err
	}
	chat = &Chat{ID: id, Type: messenger, Name: messenger, complete: true}
	return db.CreateChat(chat)
}

// CreateChat creates new chat entry with given id in messenger, type and name
func (db *DB) CreateChat(chat *Chat) (*Chat, error) {
	token := generateToken(chat.ID, chat.Type)

	res := db.QueryRow("INSERT INTO Chats (chat_id, chat_type, token, name) VALUES ($1, $2, $3, $4) RETURNING internal_id",
		&chat.ID, &chat.Type, &token, &chat.Name)

	var internalID int32
	err := res.Scan(&internalID)
	if err != nil {
		return nil, err
	}

	return &Chat{
		ID:         chat.ID,
		Type:       chat.Type,
		internalID: internalID,
		Name:       chat.Name,
		complete:   true,
	}, nil
}

// GetChat returns chat object with given id in messenger
func (db *DB) GetChat(chatID int64, chatType string) (*Chat, error) {
	res := Chat{ID: chatID, Type: chatType, complete: true}
	row := db.QueryRow(`SELECT name, internal_id FROM Chats
	WHERE chat_id = $1 AND chat_type = $2`, &chatID, &chatType)

	err := row.Scan(&res.Name, &res.internalID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &res, nil
}

func (db *DB) GetChatToken(chat *Chat) (string, error) {
	var token string
	row := db.QueryRow(`SELECT token FROM Chats
	WHERE chat_id = $1 AND chat_type = $2`, &chat.ID, &chat.Type)

	err := row.Scan(&token)
	log.Printf("getChatToken(%d, %s) = %s, %v", chat.ID, chat.Type, token, err)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return token, nil
}

func getChatRowIDByToken(tx *sql.Tx, token string) (int, error) {
	row := tx.QueryRow("SELECT internal_id FROM Chats WHERE token = $1", &token)
	rowID := -1
	err := row.Scan(&rowID)
	return rowID, err
}

// Subscribe subscribes provided chat on another with given token.
func (db *DB) Subscribe(subscriber *Chat, subscriptionToken string) error {
	if err := subscriber.fillOrCreate(db); err != nil {
		return err
	}
	err := db.transact(&sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			subscriptionRowID, err := getChatRowIDByToken(tx, subscriptionToken)
			if err != nil {
				return err
			}

			if _, err := tx.Exec("INSERT INTO Subscriptions VALUES ($1, $2)",
				&subscriptionRowID, &subscriber.internalID); err != nil {
				return err
			}

			return nil
		})

	return err
}

// Unsubscribe unsubscribes provided chat from another with given token.
func (db *DB) Unsubscribe(subscriber *Chat, subscriptionToken string) error {
	if err := subscriber.fillOrCreate(db); err != nil {
		return err
	}
	err := db.transact(&sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	},
		func(tx *sql.Tx) error {
			subscriptionRowID, err := getChatRowIDByToken(tx, subscriptionToken)
			if err != nil {
				return err
			}

			res, err := tx.Exec("DELETE FROM Subscriptions WHERE source_chat = $1 AND destination_chat = $2",
				&subscriptionRowID, &subscriber.internalID)
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
