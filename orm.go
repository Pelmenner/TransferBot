package main

import (
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
}
