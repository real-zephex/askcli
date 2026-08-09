package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"google.golang.org/genai"
)

func initDB() *sql.DB {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".ask-go.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS conversations (
			channel TEXT PRIMARY KEY,
			params_json TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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

// saveConversation upserts the entire conversation array as JSON for a channel.
func saveConversation(db *sql.DB, channel string, contents []*genai.Content) {
	data, err := json.Marshal(contents)
	if err != nil {
		log.Fatalf("Failed to marshal conversation: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO conversations (channel, params_json) VALUES (?, ?)
		 ON CONFLICT(channel) DO UPDATE SET params_json = ?, updated_at = CURRENT_TIMESTAMP`,
		channel, string(data), string(data),
	)
	if err != nil {
		log.Fatalf("Failed to save conversation: %v", err)
	}
}

// loadConversation loads the full conversation array from JSON for a channel.
func loadConversation(db *sql.DB, channel string) []*genai.Content {
	var data string
	err := db.QueryRow("SELECT params_json FROM conversations WHERE channel = ?", channel).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		log.Fatalf("Failed to load conversation: %v", err)
	}
	var contents []*genai.Content
	if err := json.Unmarshal([]byte(data), &contents); err != nil {
		log.Fatalf("Failed to unmarshal conversation: %v", err)
	}
	return contents
}

// telegramChannel returns the channel key for a Telegram chat.
func telegramChannel(chatID int64) string {
	return fmt.Sprintf("telegram:%d", chatID)
}

// clearConversation deletes one channel's conversation.
func clearConversation(db *sql.DB, channel string) {
	_, err := db.Exec("DELETE FROM conversations WHERE channel = ?", channel)
	if err != nil {
		log.Fatalf("Failed to clear conversation: %v", err)
	}
}

// clearAllConversations deletes all conversations across all channels.
func clearAllConversations(db *sql.DB) {
	_, err := db.Exec("DELETE FROM conversations")
	if err != nil {
		log.Fatalf("Failed to clear conversations: %v", err)
	}
}
