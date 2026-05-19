package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID       int
	Role     string
	Content  string
	CreateAt string
}

func initDB() *sql.DB {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".ask-go.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize lists tables
	if err := initListsTables(db); err != nil {
		log.Fatal(err)
	}

	return db
}

func saveMessage(db *sql.DB, role string, content string) {
	_, err := db.Exec(
		"INSERT INTO messages (role, content) VALUES (?, ?)",
		role, content,
	)
	if err != nil {
		log.Fatal(err)
	}
}

func getHistory(db *sql.DB, limit int) []Message {
	rows, err := db.Query(
		"SELECT id, role, content, created_at FROM messages ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreateAt)
		messages = append(messages, m)
	}
	return messages
}

func clearDatabase(db *sql.DB) {
	_, err := db.Exec(
		"DELETE FROM messages",
	)
	if err != nil {
		log.Fatal(err)
	}
}
