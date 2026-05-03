package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type askRequestPayload struct {
	Message   string `json:"message"`
	Mode      string `json:"mode"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
}

type askResponsePayload struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func postToRemoteAsk(ctx context.Context, server string, apiKey string, message string, model string, reasoning string) (string, error) {
	if server == "" {
		return "", fmt.Errorf("server URL cannot be empty")
	}

	// ensure the server string does not end with a trailing slash
	server = strings.TrimRight(server, "/")
	url := server + "/ask"

	payload := askRequestPayload{
		Message:   message,
		Mode:      "terminal",
		Model:     model,
		Reasoning: reasoning,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("x-askcli-api-key", apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		var ar askResponsePayload
		_ = json.Unmarshal(body, &ar)
		if ar.Error != "" {
			return "", fmt.Errorf("server error: %s", ar.Error)
		}
		return "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out askResponsePayload
	if err := json.Unmarshal(body, &out); err != nil {
		return string(body), nil
	}
	if out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	return out.Response, nil
}

func getServerAPIKey() string {
	if serverKey != nil && *serverKey != "" {
		return *serverKey
	}
	if v, ok := os.LookupEnv("ASKCLI_SERVER_KEY"); ok {
		return v
	}
	if v, ok := os.LookupEnv("ASKCLI_CLIENT_KEY"); ok {
		return v
	}
	return ""
}

func startREPLRemote(ctx context.Context, db *sql.DB, server string, apiKey string, model string, reasoning string) {
	state := &replState{
		model:     model,
		reasoning: reasoning,
		stream:    *stream,
		agent:     *agent,
		yolo:      *yolo,
	}

	printREPLHeader(state.model, state.reasoning, state.stream, state.agent, state.yolo)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for {
		flushRememberResults()
		fmt.Print(chatPrompt())
		if !scanner.Scan() {
			waitForRememberTasks("Closing remote chat")
			fmt.Println("\nGoodbye!")
			return
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			handled, shouldExit := handleSlashCommand(input, db, state)
			if shouldExit {
				waitForRememberTasks("Closing remote chat")
				fmt.Println("Goodbye!")
				return
			}
			if handled {
				continue
			}
		}

		// send to remote
		printThinking()
		res, err := postToRemoteAsk(ctx, server, apiKey, input, state.model, state.reasoning)
		clearThinking()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		printFinalRenderLabel()
		render(res)
		fmt.Println()

		saveMessage(db, "user", input)
		saveMessage(db, "assistant", res)
	}
}
