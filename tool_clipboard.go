package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	clipboardDaemonMutex sync.Mutex
	clipboardDaemonCmd   *exec.Cmd
)

const (
	maxClipboardOutputLength = 8000
)

type clipboardRequest struct {
	Action  string
	Content string
}

type clipboardResult struct {
	Request      clipboardRequest
	Content      string
	CharCount    int
	Truncated    bool
	ExecutionErr string
}

func parseClipboardRequest(args map[string]any) (clipboardRequest, error) {
	if args == nil {
		return clipboardRequest{}, errors.New("function args missing")
	}

	action, err := requiredStringArg(args, "action")
	if err != nil {
		return clipboardRequest{}, err
	}

	action = strings.ToLower(action)
	if action != "read" && action != "write" {
		return clipboardRequest{}, errors.New("action must be either 'read' or 'write'")
	}

	content := ""
	if action == "write" {
		contentValue, ok := args["content"]
		if !ok {
			return clipboardRequest{}, errors.New("content is required when action is 'write'")
		}
		contentStr, ok := contentValue.(string)
		if !ok {
			return clipboardRequest{}, errors.New("content must be a string")
		}
		content = contentStr
	}

	return clipboardRequest{
		Action:  action,
		Content: content,
	}, nil
}

func detectClipboardTool() error {
	// Check if we have a display server
	display := os.Getenv("DISPLAY")
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if display == "" && waylandDisplay == "" {
		return errors.New("no display server detected ($DISPLAY and $WAYLAND_DISPLAY are not set). Clipboard operations require a graphical environment")
	}

	// Check for wl-clipboard tools
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return errors.New("wl-paste not found. Please install wl-clipboard (e.g., sudo dnf install wl-clipboard)")
	}
	if _, err := exec.LookPath("wl-copy"); err != nil {
		return errors.New("wl-copy not found. Please install wl-clipboard (e.g., sudo dnf install wl-clipboard)")
	}

	return nil
}

func executeClipboard(req clipboardRequest) clipboardResult {
	if err := detectClipboardTool(); err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: err.Error(),
		}
	}

	if req.Action == "read" {
		return executeClipboardRead(req)
	}
	return executeClipboardWrite(req)
}

func executeClipboardRead(req clipboardRequest) clipboardResult {
	cmd := exec.Command("wl-paste", "--no-newline")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		stderrStr := stderr.String()
		// wl-paste returns error when clipboard is empty, check stderr
		if strings.Contains(stderrStr, "Nothing is copied") || strings.Contains(stderrStr, "No selection") {
			return clipboardResult{
				Request:   req,
				Content:   "",
				CharCount: 0,
			}
		}
		if stderrStr != "" {
			return clipboardResult{
				Request:      req,
				ExecutionErr: fmt.Sprintf("failed to read clipboard: %v (%s)", err, stderrStr),
			}
		}
		return clipboardResult{
			Request:      req,
			ExecutionErr: fmt.Sprintf("failed to read clipboard: %v", err),
		}
	}

	content := stdout.String()
	if content == "" {
		return clipboardResult{
			Request:   req,
			Content:   "",
			CharCount: 0,
		}
	}

	truncated := false
	if len(content) > maxClipboardOutputLength {
		runes := []rune(content)
		content = string(runes[:maxClipboardOutputLength])
		truncated = true
	}

	return clipboardResult{
		Request:   req,
		Content:   content,
		CharCount: len([]rune(stdout.String())),
		Truncated: truncated,
	}
}

func executeClipboardWrite(req clipboardRequest) clipboardResult {
	clipboardDaemonMutex.Lock()
	defer clipboardDaemonMutex.Unlock()

	// Kill any existing clipboard daemon
	if clipboardDaemonCmd != nil && clipboardDaemonCmd.Process != nil {
		_ = clipboardDaemonCmd.Process.Kill()
		clipboardDaemonCmd = nil
	}

	// Start wl-copy as a background daemon
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(req.Content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Start the process but don't wait for it
	err := cmd.Start()
	if err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return clipboardResult{
				Request:      req,
				ExecutionErr: fmt.Sprintf("failed to write clipboard: %v (%s)", err, stderrStr),
			}
		}
		return clipboardResult{
			Request:      req,
			ExecutionErr: fmt.Sprintf("failed to write clipboard: %v", err),
		}
	}

	// Store the command reference so we can kill it later
	clipboardDaemonCmd = cmd

	// Give it a moment to read stdin and set up the clipboard
	time.Sleep(50 * time.Millisecond)

	return clipboardResult{
		Request:   req,
		CharCount: len([]rune(req.Content)),
	}
}

func (r clipboardResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": r.ExecutionErr,
			},
		}
	}

	if r.Request.Action == "read" {
		if r.Content == "" {
			return map[string]any{
				"output": map[string]any{
					"action":  "read",
					"content": "",
					"message": "Clipboard is empty",
				},
			}
		}

		output := map[string]any{
			"action":     "read",
			"content":    r.Content,
			"char_count": r.CharCount,
		}

		if r.Truncated {
			output["truncated"] = true
			output["message"] = fmt.Sprintf("Content truncated at %d characters", maxClipboardOutputLength)
		}

		return map[string]any{"output": output}
	}

	// write action
	return map[string]any{
		"output": map[string]any{
			"action":     "write",
			"char_count": r.CharCount,
			"success":    true,
			"message":    fmt.Sprintf("Wrote %d characters to clipboard", r.CharCount),
		},
	}
}