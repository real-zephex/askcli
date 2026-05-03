package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var telegramBot *bot.BotAPI
var geminiKey string
var tgModel string = "gemini-3.1-flash-lite-preview"
var tgReasoning string = "MINIMAL"

const (
	TELEGRAM_FILE_URL string = "https://api.telegram.org"
)

const telegramMaxMessageLen = 4000

type GetFileResponse struct {
	OK     bool       `json:"ok"`
	Result FileResult `json:"result"`
}

type FileResult struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int    `json:"file_size"`
	FilePath     string `json:"file_path"`
}

type GroqResponse struct {
	Text string `json:"text"`
	xGroq XGroq `json:"x_groq"`
}

type XGroq struct {
	ID string `json:"id"`
}

func setGeminiKey() error {
	var exists bool
	geminiKey, exists = checkForEnv()

	if !exists {
		fError := fmt.Errorf("No GEMINI KEY was found in environment")
		return fError
	}
	return nil
}

func getGroqApiKey() (string, error) {
	groqKey, exists := os.LookupEnv("GROQ_API_KEY")
	if !exists {
		return "", fmt.Errorf("GROQ_API_KEY does not exists in the environment. Please set the key and try again")
	}
	return groqKey, nil
}

func botClient(key string) error {
	var err error
	telegramBot, err = bot.NewBotAPI(key)
	if err != nil {
		fError := fmt.Errorf("There was an error initializing the telegram client: %v", err)
		return fError
	}
	telegramBot.Debug = true
	return nil
}

func splitTelegramMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}

	chunks := make([]string, 0, (len(runes)+maxLen-1)/maxLen)
	for start := 0; start < len(runes); start += maxLen {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}

	return chunks
}

func sendDocument(chatID int64, filepath string) error {
	fmt.Println("[DEBUG] sendDocument called with filepath:", filepath)
	if chatID <= 0 {
		return fmt.Errorf("telegram chat id is not set")
	}

	exists, reason := fileExists(filepath)
	if !exists {
		fmt.Println("[ERROR] File not found:", filepath, "reason:", reason)
		fError := fmt.Errorf("There was an error verifying the existence of file: %v", reason)
		return fError
	}
	fmt.Println("[DEBUG] File exists, proceeding to send document")

	msg := bot.NewDocument(chatID, bot.FilePath(filepath))
	fmt.Println("[DEBUG] Document message created for chat ID:", chatID)

	_, err := telegramBot.Send(msg)
	if err != nil {
		fmt.Println("[ERROR] Failed to send document:", err)
		fError := fmt.Errorf("There was an error sending the document over telegram: %v", err)
		return fError
	}
	fmt.Println("[DEBUG] Document sent successfully")
	return nil
}

func sendImage(chatID int64, filepath string) error {
	fmt.Println("[DEBUG] sendImage called with filepath:", filepath)
	if chatID <= 0 {
		return fmt.Errorf("telegram chat id is not set")
	}

	exists, reason := fileExists(filepath)
	if !exists {
		fmt.Println("[ERROR] Image file not found:", filepath, "reason:", reason)
		fError := fmt.Errorf("There was an error verifying the existence of file: %v", reason)
		return fError
	}
	fmt.Println("[DEBUG] Image file exists, proceeding to send image")

	msg := bot.NewPhoto(chatID, bot.FilePath(filepath))
	fmt.Println("[DEBUG] Image message created for chat ID:", chatID)

	_, err := telegramBot.Send(msg)
	if err != nil {
		fmt.Println("[ERROR] Failed to send image:", err)
		fError := fmt.Errorf("There was an error sending the image over telegram: %v", err)
		return fError
	}
	fmt.Println("[DEBUG] Image sent successfully")
	return nil
}

func sendMessage(text string, message *bot.Message) {
	if message == nil {
		return
	}
	if strings.TrimSpace(text) == "" {
		return
	}

	chatId := message.Chat.ID
	messageID := message.MessageID
	chunks := splitTelegramMessage(text, telegramMaxMessageLen)

	for i, chunk := range chunks {
		msg := bot.NewMessage(chatId, chunk)
		if i == 0 {
			msg.ReplyToMessageID = messageID
		}

		_, err := telegramBot.Send(msg)
		if err != nil {
			fError := fmt.Errorf("Error while sending message to client: %v", err)
			fmt.Println(fError)
			return
		}
	}
}

func commandsHandler(message *bot.Message) {

	commands := message.Command()
	commandsArguments := message.CommandArguments()

	switch commands {
	case "start":
		sendMessage("👋 Welcome to the Gemini Telegram Bot!\nUse /help or /about to see available commands.", message)
	case "help", "about":
		helpText := "📋 *Available Commands* \n\n/start - Show welcome message\n/model [name] - Change the AI model\n/help or /about - Show this help menu\n\nCurrent model: " + tgModel
		sendMessage(helpText, message)
	case "model":
		if commandsArguments == "" {
			// since no arguments were passed, list all the models
			sendMessage(fmt.Sprintf("Available Models are:\n1. gemini-3-flash-preview\n2. gemini-3.1-flash-preview-lite\n3. any model from google\nCurrent model: %s", tgModel), message)
		} else {
			tgModel = resolveModels(commandsArguments)
			sendMessage(fmt.Sprintf("Model changed to: %s", tgModel), message)
		}
	case "reasoning":
		if commandsArguments == "" {
			sendMessage(fmt.Sprintf("Available Reasoning Levels: \n1. HIGH\n2. MEDIUM\n3. LOW\n4. MINIMAL\nCurrent reasoning level: %s", tgReasoning), message)
		} else {
			tgReasoning = resolveReasoningLevel(commandsArguments)
			sendMessage(fmt.Sprintf("Reasoning changed to: %s", tgReasoning), message)
		}

	default:
		sendMessage(fmt.Sprintf("No such commands found: %s", commands), message)
	}
}

func handleAudio(fileId string) (string, error) {
	botKey, err := telegramBotKeyCheck()
	if err != nil {
		return "", fmt.Errorf("An error occured while fetching API key for telegram.")
	}
	groqApiKey, err := getGroqApiKey()
	if err != nil {
		return "", err
	}

	resp, err := http.Get(
		fmt.Sprintf(TELEGRAM_FILE_URL+"/bot%s/getFile?file_id=%s", botKey, fileId),
	)
	if err != nil {
		return "", fmt.Errorf("An error occured while querying Telegram for the file: %v", err)
	}
	defer resp.Body.Close()

	var telegramResponse GetFileResponse
	fileResponseParsingError := json.NewDecoder(resp.Body).Decode(&telegramResponse)
	if fileResponseParsingError != nil {
		return "", fmt.Errorf("An error occured while parsing response from telegram: %v", err)
	}

	fileUrl := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botKey, telegramResponse.Result.FilePath)
	audioResp, err := http.Get(fileUrl)
	if err != nil {
		return "", fmt.Errorf("An error occured while fetching file from telegram: %v", err)
	}
	defer audioResp.Body.Close()

	audioBytes, err := io.ReadAll(audioResp.Body)
	if err != nil {
		return "", fmt.Errorf("error converting response to bytes: %v", err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, _ := w.CreateFormFile("file", "voice.ogg")
	part.Write(audioBytes)
	w.WriteField("model", "whisper-large-v3-turbo")
	w.Close()

	// Send to Groq
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/audio/transcriptions", &buf)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", groqApiKey))
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, audioErr := http.DefaultClient.Do(req)
	if audioErr != nil {
		return "", fmt.Errorf("There was an error receiving response from Groq: %v", audioErr)
	}
	defer resp.Body.Close()

	var groqResponse GroqResponse
	groqResponseParsingError := json.NewDecoder(resp.Body).Decode(&groqResponse)
	if groqResponseParsingError != nil {
		return "", fmt.Errorf("There was an issue parsing the response from Groq")
	}

	fmt.Println(groqResponse)

	return groqResponse.Text, nil
}

func botConfig(ctx context.Context, db *sql.DB) {
	// some configs i copied from https://go-telegram-bot-api.dev
	updateConfig := bot.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := telegramBot.GetUpdatesChan(updateConfig)

	fmt.Println("Alright! Going to listen for events from telegram!")
	for update := range updates {
		message := update.Message
		audio := message.Voice

		if message == nil {
			continue
		}

		if message.IsCommand() {
			commandsHandler(message)
			continue
		}

		// the message from the update
		receivedMessage := update.Message.Text
		// my user id
		id := update.Message.Chat.ID

		if audio != nil {
			fmt.Println("Audio detected. Support coming soon!")
			text, err := handleAudio(audio.FileID)
			if err != nil {
				sendMessage(err.Error(), message)
				continue
			}
			receivedMessage = text
		}

		// do not proceed if there is no text
		if receivedMessage == "" {
			continue
		}

		res := runAgentTurn(ctx, db, geminiKey, receivedMessage, tgModel, tgReasoning, true, id)

		sendMessage(res, message)

		// saving the message and response to local sqlite database
		saveMessage(db, "user", receivedMessage)
		saveMessage(db, "assistant", res)
	}
}
