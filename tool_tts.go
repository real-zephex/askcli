package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	ELEVEN_LABS_BASE_URL = "https://api.elevenlabs.io/v1"
	ELEVEN_LABS_VOICE_ID = "CwhRBWXzGAHq8TQ4Fs17"
)

type textToSpeechRequest struct {
	Text string
}

type textToSpeechResult struct {
	Request      textToSpeechRequest
	FilePath     string
	AudioBytes   int
	ExecutionErr string
}

func getElevenLabsAPIKey() (string, error) {
	key, exists := os.LookupEnv("ELEVEN_LABS_API_KEY")
	if !exists {
		return "", fmt.Errorf("please set ELEVEN_LABS_API_KEY environment variable")
	}
	return key, nil
}

func textToSpeech(text string) ([]byte, error) {
	apiKey, err := getElevenLabsAPIKey()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/text-to-speech/%s?output_format=mp3_44100_128", ELEVEN_LABS_BASE_URL, ELEVEN_LABS_VOICE_ID)

	payloadBody, err := json.Marshal(map[string]string{
		"text":     text,
		"model_id": "eleven_flash_v2",
	})
	if err != nil {
		return nil, fmt.Errorf("error marshaling TTS request: %v", err)
	}

	payload := strings.NewReader(string(payloadBody))

	req, err := http.NewRequest(http.MethodPost, url, payload)
	if err != nil {
		return nil, fmt.Errorf("error crafting TTS request: %v", err)
	}

	req.Header.Add("xi-api-key", apiKey)
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making TTS request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TTS request failed with status: %d", res.StatusCode)
	}

	audio, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading TTS response: %v", err)
	}

	return audio, nil
}

func textToSpeechFile(text string) (string, error) {
	audio, err := textToSpeech(text)
	if err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp("", "ask-tts-*.mp3")
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(audio); err != nil {
		return "", fmt.Errorf("error writing audio to temp file: %v", err)
	}

	return tmpFile.Name(), nil
}

func parseTextToSpeechRequest(args map[string]any) (textToSpeechRequest, error) {
	if args == nil {
		return textToSpeechRequest{}, fmt.Errorf("function args missing")
	}

	text, err := requiredStringArg(args, "text")
	if err != nil {
		return textToSpeechRequest{}, err
	}

	return textToSpeechRequest{Text: text}, nil
}

func executeTextToSpeech(req textToSpeechRequest) textToSpeechResult {
	res := textToSpeechResult{Request: req}

	filePath, err := textToSpeechFile(req.Text)
	if err != nil {
		res.ExecutionErr = err.Error()
		return res
	}

	res.FilePath = filePath
	res.AudioBytes = len([]byte(req.Text))
	return res
}

func (r textToSpeechResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": r.ExecutionErr,
			},
		}
	}

	return map[string]any{
		"output": map[string]any{
			"filepath": r.FilePath,
			"success":  true,
			"message":  "Audio file created. Pass filepath to send_document_over_telegram if you want to deliver it in Telegram.",
		},
	}
}
