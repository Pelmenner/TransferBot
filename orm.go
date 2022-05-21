package main

import (
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

func getChat(db *sql.DB, chatID int, charType string) Chat {
	return Chat{}
}

func addUnsentMessage(db *sql.DB, message Message) {

}

func getUnsentMessages(db *sql.DB, maxCnt int) []Message {
	return []Message{}
}
