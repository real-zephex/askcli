package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

func telegramBotKeyCheck() (string, error) {
	key, exists := os.LookupEnv("TELEGRAM_BOT_TOKEN")
	if !exists {
		fError := fmt.Errorf("The Telegram Bot Token was not found in the environment variabled. Please set it and try again")
		return "", fError
	}
	fmt.Println("Telegram Bot Token found! Proceeding with the checks...")
	return key, nil
}

func backgroundManager(db *sql.DB, ctx context.Context) {
	fmt.Println("Welcome to background agent. We will not initalize the agent to function in background.")

	// get gemini key
	gErr := setGeminiKey()
	if gErr != nil {
		fmt.Println(gErr)
		os.Exit(1)
	}

	key, err := telegramBotKeyCheck()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = botClient(key)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	botConfig(ctx, db)
}
