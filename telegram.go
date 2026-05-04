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
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"google.golang.org/genai"
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
	Text  string `json:"text"`
	XGroq XGroq  `json:"x_groq"`
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
		end := min(start+maxLen, len(runes))
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
		// Convert Markdown to sanitized HTML for Telegram
		html := mdToTelegramHTML(chunk)

		msg := bot.NewMessage(chatId, html)
		msg.ParseMode = "HTML"
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

func mdToTelegramHTML(md string) string {
	// Render Markdown to HTML (unsafe)
	unsafe := blackfriday.Run([]byte(md))

	// only Telegram-safe tags
	p := bluemonday.NewPolicy()
	// basic formatting
	p.AllowElements("b", "strong", "i", "em", "u")
	// code blocks and inline code
	p.AllowElements("pre", "code")
	// links (allow href attribute)
	p.AllowElements("a")
	p.AllowAttrs("href").OnElements("a")

	// Sanitize the rendered HTML
	safe := p.SanitizeBytes(unsafe)
	return string(safe)
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

// function to get the file url from telegram (2 hops)
func getTelegramFileUrl(botKey, fileId string) (string, error) {
	resp, err := http.Get(
		fmt.Sprintf(TELEGRAM_FILE_URL+"/bot%s/getFile?file_id=%s", botKey, fileId),
	)
	if err != nil {
		return "", fmt.Errorf("error querying Telegram for file: %v", err)
	}
	defer resp.Body.Close()

	var telegramResponse GetFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&telegramResponse); err != nil {
		return "", fmt.Errorf("error parsing Telegram getFile response: %v", err)
	}

	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botKey, telegramResponse.Result.FilePath), nil
}

// function to get the file
func fetchTelegramFile(botKey, fileId string) ([]byte, error) {
	fileUrl, err := getTelegramFileUrl(botKey, fileId)
	if err != nil {
		return nil, err
	}

	fileResp, err := http.Get(fileUrl)
	if err != nil {
		return nil, fmt.Errorf("error fetching file from Telegram: %v", err)
	}
	defer fileResp.Body.Close()

	fileBytes, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading file bytes: %v", err)
	}

	return fileBytes, nil
}

// transcribe shit
func handleAudio(fileId string) (string, error) {
	botKey, err := telegramBotKeyCheck()
	if err != nil {
		return "", fmt.Errorf("error fetching Telegram API key")
	}
	groqApiKey, err := getGroqApiKey()
	if err != nil {
		return "", err
	}

	audioBytes, err := fetchTelegramFile(botKey, fileId)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "voice.ogg")
	part.Write(audioBytes)
	w.WriteField("model", "whisper-large-v3-turbo")
	w.Close()

	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/audio/transcriptions", &buf)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", groqApiKey))
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error receiving response from Groq: %v", err)
	}
	defer resp.Body.Close()

	var groqResponse GroqResponse
	if err := json.NewDecoder(resp.Body).Decode(&groqResponse); err != nil {
		return "", fmt.Errorf("error parsing Groq response")
	}

	return groqResponse.Text, nil
}

func botConfig(ctx context.Context, db *sql.DB) {
	// some configs i copied from https://go-telegram-bot-api.dev
	updateConfig := bot.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := telegramBot.GetUpdatesChan(updateConfig)
	botkey, err := telegramBotKeyCheck()
	if err != nil {
		// handle it somehow
		fmt.Println("Telegram Bot Key was not found. Please set it and try again.")
		os.Exit(1)
	}

	fmt.Println("Alright! Going to listen for events from telegram!")
	for update := range updates {
		message := update.Message
		if message == nil {
			continue
		}

		voice := message.Voice
		audio := message.Audio
		image := message.Photo
		document := message.Document
		var prepared []MultiModalMessage

		// commands handler - in telegram commands start with /
		if message.IsCommand() {
			commandsHandler(message)
			continue
		}

		// the message from the update
		receivedMessage := update.Message.Text
		// my user id
		id := update.Message.Chat.ID

		extractFile := func(msg *bot.Message) (string, string, error) {
			if msg == nil {
				return "", "", nil
			}

			if len(msg.Photo) > 0 {
				lastEntry := msg.Photo[len(msg.Photo)-1]
				fileLink, err := getTelegramFileUrl(botkey, lastEntry.FileID)
				return "image/jpg", fileLink, err
			}
			if msg.Document != nil {
				fileLink, err := getTelegramFileUrl(botkey, msg.Document.FileID)
				return msg.Document.MimeType, fileLink, err
			}
			if msg.Audio != nil {
				fileLink, err := getTelegramFileUrl(botkey, msg.Audio.FileID)
				return msg.Audio.MimeType, fileLink, err
			}

			return "", "", nil
		}

		if image != nil || document != nil || audio != nil {
			var fileId string
			var mimetype string

			if image != nil {
				imageArraySize := len(image)
				lastEntry := image[imageArraySize-1]
				fileId = lastEntry.FileID
				mimetype = "image/jpg"
			} else if document != nil {
				fileId = document.FileID
				mimetype = document.MimeType
			} else if audio != nil {
				fileId = audio.FileID
				mimetype = audio.MimeType
			}

			fileLink, err := getTelegramFileUrl(botkey, fileId)
			if err != nil {
				sendMessage("There was an error receiving media URL from Telegram", message)
			}
			fmt.Println("Final media URL: ", fileLink)

			caption := message.Caption
			if caption != "" {
				receivedMessage = caption
			} else {
				sendMessage("Please provide some instructions or a message with the media.", message)
				continue
			}

			prepared = append(prepared, MultiModalMessage{
				Mimetype: mimetype,
				File:     fileLink,
			})
		}

		if voice != nil {
			fmt.Println("Audio file detected. Running it through transcription pipeline")
			text, err := handleAudio(voice.FileID)
			if err != nil {
				sendMessage(err.Error(), message)
				continue
			}
			receivedMessage = text
		}

		reply := message.ReplyToMessage
		if reply != nil {
			replyText := strings.TrimSpace(reply.Text)
			if replyText == "" {
				replyText = strings.TrimSpace(reply.Caption)
			}

			replyMime, replyFileLink, err := extractFile(reply)
			if err != nil {
				sendMessage("There was an error receiving replied media URL from Telegram", message)
			} else {
				if strings.TrimSpace(replyFileLink) != "" {
					prepared = append(prepared, MultiModalMessage{
						Mimetype: replyMime,
						File:     replyFileLink,
					})
					if replyText != "" {
						replyText += "\n"
					}
					replyText += fmt.Sprintf("[Replied media URL: %s]", replyFileLink)
				}
			}

			if replyText != "" {
				if receivedMessage != "" {
					receivedMessage = fmt.Sprintf("Reply context:\n%s\n\nUser message:\n%s", replyText, receivedMessage)
				} else {
					receivedMessage = fmt.Sprintf("Reply context:\n%s", replyText)
				}
			}
		}

		// do not proceed if there is no text
		if receivedMessage == "" {
			continue
		}

		multiModalContents := make([]*genai.Content, 0, len(prepared))
		for _, p := range prepared {
			content := p.ToGenAIImageContent()
			if content != nil {
				multiModalContents = append(multiModalContents, content)
			}
		}

		res := runAgentTurn(ctx, db, geminiKey, receivedMessage, tgModel, tgReasoning, true, id, multiModalContents)

		sendMessage(res, message)

		// saving the message and response to local sqlite database
		saveMessage(db, "user", receivedMessage)
		saveMessage(db, "assistant", res)
	}
}
