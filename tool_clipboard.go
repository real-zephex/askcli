package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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

type clipboardProviderKind int

const (
	clipboardProviderUnknown clipboardProviderKind = iota
	clipboardProviderWayland
	clipboardProviderX11
	clipboardProviderMacOS
	clipboardProviderWindows
)

type clipboardProvider struct {
	kind    clipboardProviderKind
	x11Tool string
}

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

func detectClipboardProvider() (clipboardProvider, error) {
	switch runtime.GOOS {
	case "windows":
		return clipboardProvider{kind: clipboardProviderWindows}, nil
	case "darwin":
		if _, err := exec.LookPath("pbpaste"); err != nil {
			return clipboardProvider{}, errors.New("pbpaste not found. Ensure the macOS clipboard utilities are available")
		}
		if _, err := exec.LookPath("pbcopy"); err != nil {
			return clipboardProvider{}, errors.New("pbcopy not found. Ensure the macOS clipboard utilities are available")
		}
		return clipboardProvider{kind: clipboardProviderMacOS}, nil
	case "linux":
		waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
		if waylandDisplay != "" {
			if _, err := exec.LookPath("wl-paste"); err != nil {
				return clipboardProvider{}, errors.New("wl-paste not found. Install wl-clipboard to use the clipboard on Wayland")
			}
			if _, err := exec.LookPath("wl-copy"); err != nil {
				return clipboardProvider{}, errors.New("wl-copy not found. Install wl-clipboard to use the clipboard on Wayland")
			}
			return clipboardProvider{kind: clipboardProviderWayland}, nil
		}

		display := os.Getenv("DISPLAY")
		if display == "" {
			return clipboardProvider{}, errors.New("no display server detected ($DISPLAY and $WAYLAND_DISPLAY are not set). Clipboard operations require a graphical environment")
		}

		if _, err := exec.LookPath("xclip"); err == nil {
			return clipboardProvider{kind: clipboardProviderX11, x11Tool: "xclip"}, nil
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return clipboardProvider{kind: clipboardProviderX11, x11Tool: "xsel"}, nil
		}

		return clipboardProvider{}, errors.New("xclip or xsel not found. Install one to use the clipboard on X11")
	default:
		return clipboardProvider{}, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func executeClipboard(req clipboardRequest) clipboardResult {
	provider, err := detectClipboardProvider()
	if err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: err.Error(),
		}
	}

	switch provider.kind {
	case clipboardProviderWayland:
		if req.Action == "read" {
			return executeWaylandClipboardRead(req)
		}
		return executeWaylandClipboardWrite(req)
	case clipboardProviderX11:
		if req.Action == "read" {
			return executeX11ClipboardRead(req, provider.x11Tool)
		}
		return executeX11ClipboardWrite(req, provider.x11Tool)
	case clipboardProviderMacOS:
		if req.Action == "read" {
			return executeMacOSClipboardRead(req)
		}
		return executeMacOSClipboardWrite(req)
	case clipboardProviderWindows:
		if req.Action == "read" {
			return executeWindowsClipboardRead(req)
		}
		return executeWindowsClipboardWrite(req)
	default:
		return clipboardResult{
			Request:      req,
			ExecutionErr: "no clipboard provider available",
		}
	}
}

func executeWaylandClipboardRead(req clipboardRequest) clipboardResult {
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

func executeWaylandClipboardWrite(req clipboardRequest) clipboardResult {
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

func executeX11ClipboardRead(req clipboardRequest, tool string) clipboardResult {
	cmd := buildX11ReadCommand(tool)
	stdout, stderr, err := runCommand(cmd, req)
	if err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to read clipboard", err, stderr),
		}
	}

	return buildClipboardReadResult(req, stdout)
}

func executeX11ClipboardWrite(req clipboardRequest, tool string) clipboardResult {
	cmd := buildX11WriteCommand(tool)
	cmd.Stdin = strings.NewReader(req.Content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to write clipboard", err, stderr.String()),
		}
	}

	return clipboardResult{
		Request:   req,
		CharCount: len([]rune(req.Content)),
	}
}

func executeMacOSClipboardRead(req clipboardRequest) clipboardResult {
	cmd := exec.Command("pbpaste")
	stdout, stderr, err := runCommand(cmd, req)
	if err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to read clipboard", err, stderr),
		}
	}

	return buildClipboardReadResult(req, stdout)
}

func executeMacOSClipboardWrite(req clipboardRequest) clipboardResult {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(req.Content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to write clipboard", err, stderr.String()),
		}
	}

	return clipboardResult{
		Request:   req,
		CharCount: len([]rune(req.Content)),
	}
}

func executeWindowsClipboardRead(req clipboardRequest) clipboardResult {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw")
	stdout, stderr, err := runCommand(cmd, req)
	if err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to read clipboard", err, stderr),
		}
	}

	return buildClipboardReadResult(req, stdout)
}

func executeWindowsClipboardWrite(req clipboardRequest) clipboardResult {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard")
	cmd.Stdin = strings.NewReader(req.Content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return clipboardResult{
			Request:      req,
			ExecutionErr: formatCommandError("failed to write clipboard", err, stderr.String()),
		}
	}

	return clipboardResult{
		Request:   req,
		CharCount: len([]rune(req.Content)),
	}
}

func buildClipboardReadResult(req clipboardRequest, output string) clipboardResult {
	if output == "" {
		return clipboardResult{
			Request:   req,
			Content:   "",
			CharCount: 0,
		}
	}

	truncated := false
	content := output
	if len(content) > maxClipboardOutputLength {
		runes := []rune(content)
		content = string(runes[:maxClipboardOutputLength])
		truncated = true
	}

	return clipboardResult{
		Request:   req,
		Content:   content,
		CharCount: len([]rune(output)),
		Truncated: truncated,
	}
}

func runCommand(cmd *exec.Cmd, req clipboardRequest) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", stderr.String(), err
	}

	return stdout.String(), "", nil
}

func formatCommandError(prefix string, err error, stderr string) string {
	if stderr != "" {
		return fmt.Sprintf("%s: %v (%s)", prefix, err, stderr)
	}
	return fmt.Sprintf("%s: %v", prefix, err)
}

func buildX11ReadCommand(tool string) *exec.Cmd {
	if tool == "xsel" {
		return exec.Command("xsel", "-ob")
	}
	return exec.Command("xclip", "-o", "-selection", "clipboard")
}

func buildX11WriteCommand(tool string) *exec.Cmd {
	if tool == "xsel" {
		return exec.Command("xsel", "-ib")
	}
	return exec.Command("xclip", "-selection", "clipboard")
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
