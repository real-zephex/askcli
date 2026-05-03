package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)

	subtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	streamStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	finalStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	toolStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("204"))
	warnStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	memoryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	memoryOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
)

var (
	promptStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1)
	toolBlockStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("61")).Background(lipgloss.Color("236")).Padding(0, 1)
)

var thinkingActive bool

func printREPLHeader(model string, reasoning string, stream bool, agent bool, yolo bool) {
	fmt.Println(headerStyle.Render("ask • interactive mode"))
	fmt.Println(subtleStyle.Render("commands: /help for slash commands"))
	fmt.Println(subtleStyle.Render(
		"model: " + model +
			" • reasoning: " + reasoning +
			" • stream: " + fmt.Sprintf("%t", stream) +
			" • agent: " + fmt.Sprintf("%t", agent) +
			" • yolo: " + fmt.Sprintf("%t", yolo),
	))
	fmt.Println()
}

func chatPrompt() string {
	return promptStyle.Render("ask ❯ ")
}

func renderToolBlock(lines []string) string {
	ensureThinkingCleared()
	content := strings.Join(lines, "\n")
	return toolBlockStyle.Render(content)
}

func ensureThinkingCleared() {
	if thinkingActive {
		clearThinking()
	}
}

func printThinking() {
	thinkingActive = true
	fmt.Print(statusStyle.Render("thinking..."))
}

func clearThinking() {
	if !thinkingActive {
		return
	}
	thinkingActive = false
	fmt.Print("\r" + strings.Repeat(" ", 24) + "\r")
}

func printStreamingLabel() {
	fmt.Println(streamStyle.Render("↳ streaming rendered markdown:"))
}

func printFinalRenderLabel() {
	fmt.Println(finalStyle.Render("↳ rendered markdown:"))
}

func printToolCall(req shellCommandRequest) {
	lines := []string{toolStyle.Render("↳ tool: run_shell_command")}
	if req.Reason != "" {
		lines = append(lines, subtleStyle.Render("reason: "+req.Reason))
	}
	lines = append(lines, subtleStyle.Render("cwd: "+req.WorkingDirectory+" • timeout: "+fmt.Sprintf("%ds", req.TimeoutSeconds)))
	lines = append(lines, "$ "+req.Command)
	fmt.Println(renderToolBlock(lines))
}

func printToolDenied() {
	fmt.Println(renderToolBlock([]string{warnStyle.Render("command denied by user")}))
}

func printToolResult(result shellCommandResult) {
	status := "ok"
	if result.ExecutionErr != "" || result.ExitCode != 0 || result.TimedOut {
		status = "error"
	}
	line := subtleStyle.Render(
		fmt.Sprintf("tool result: %s • exit=%d • duration=%dms", status, result.ExitCode, result.Duration.Milliseconds()),
	)
	fmt.Println(renderToolBlock([]string{line}))
}

func printMailCall(req mailRequest) {
	lines := []string{toolStyle.Render("↳ tool: mail"), subtleStyle.Render("action: " + req.Action)}
	if req.ThreadID != "" {
		lines = append(lines, subtleStyle.Render("thread: "+req.ThreadID))
	}
	if req.MessageID != "" {
		lines = append(lines, subtleStyle.Render("message: "+req.MessageID))
	}
	if req.To != "" {
		lines = append(lines, subtleStyle.Render("to: "+req.To))
	}
	if req.Subject != "" {
		lines = append(lines, subtleStyle.Render("subject: "+req.Subject))
	}
	if req.Text != "" {
		preview := truncateMailPreview(req.Text, 200, 5)
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("text (%d chars):", len([]rune(req.Text)))))
		if preview != "" {
			lines = append(lines, preview)
		}
	}
	if req.HTML != "" {
		preview := truncateMailPreview(req.HTML, 200, 5)
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("html (%d chars):", len([]rune(req.HTML)))))
		if preview != "" {
			lines = append(lines, preview)
		}
	}
	fmt.Println(renderToolBlock(lines))
}

func truncateMailPreview(value string, maxChars int, maxLines int) string {
	if value == "" {
		return ""
	}
	preview := value
	if len([]rune(preview)) > maxChars {
		runes := []rune(preview)
		preview = string(runes[:maxChars]) + "..."
	}
	previewLines := strings.Split(preview, "\n")
	if len(previewLines) > maxLines {
		preview = strings.Join(previewLines[:maxLines], "\n") + "\n..."
	}
	return preview
}

func printMailDenied() {
	fmt.Println(renderToolBlock([]string{warnStyle.Render("mail action denied by user")}))
}

func printMailResult(res mailResult) {
	if res.ExecutionErr != "" {
		line := subtleStyle.Render(fmt.Sprintf("tool result: error • %s", res.ExecutionErr))
		fmt.Println(renderToolBlock([]string{line}))
		return
	}
	if res.UserDenied {
		printMailDenied()
		return
	}
	if res.Request.Action == "get_threads" {
		line := subtleStyle.Render(fmt.Sprintf("tool result: ok • %d thread(s)", len(res.Threads)))
		fmt.Println(renderToolBlock([]string{line}))
		return
	}
	if res.Request.Action == "get_thread" {
		line := subtleStyle.Render("tool result: ok • thread fetched")
		fmt.Println(renderToolBlock([]string{line}))
		return
	}
	if res.Request.Action == "delete_thread" {
		line := subtleStyle.Render("tool result: ok • thread deleted")
		fmt.Println(renderToolBlock([]string{line}))
		return
	}
	if res.MessageResult != nil {
		line := subtleStyle.Render(fmt.Sprintf("tool result: ok • message_id=%s thread_id=%s", res.MessageResult.MessageID, res.MessageResult.ThreadID))
		fmt.Println(renderToolBlock([]string{line}))
	}
}

func printTextToSpeechCall(req textToSpeechRequest) {
	lines := []string{toolStyle.Render("↳ tool: text_to_speech_file")}
	preview := truncateMailPreview(req.Text, 240, 5)
	lines = append(lines, subtleStyle.Render(fmt.Sprintf("text (%d chars):", len([]rune(req.Text)))))
	if preview != "" {
		lines = append(lines, preview)
	}
	fmt.Println(renderToolBlock(lines))
}

func printTextToSpeechResult(res textToSpeechResult) {
	if res.ExecutionErr != "" {
		line := subtleStyle.Render(fmt.Sprintf("tool result: error • %s", res.ExecutionErr))
		fmt.Println(renderToolBlock([]string{line}))
		return
	}

	lines := []string{
		subtleStyle.Render("tool result: ok • mp3 file created"),
		subtleStyle.Render("filepath: " + res.FilePath),
		subtleStyle.Render("next: send_document_over_telegram"),
	}
	fmt.Println(renderToolBlock(lines))
}

func printMemorySaved(stored int) {
	fmt.Println(memoryOKStyle.Render(fmt.Sprintf("🧠 memory: saved %d item(s)", stored)))
}

func printMemoryNoop() {
	fmt.Println(memoryStyle.Render("🧠 memory: no new items saved"))
}

func printMemoryWarning(err error) {
	fmt.Println(warnStyle.Render(fmt.Sprintf("🧠 memory warning: %v", err)))
}

func printMemoryWait(reason string, pending int64) {
	fmt.Println(memoryStyle.Render(fmt.Sprintf("%s. Please wait, finishing %d memory task(s)...", reason, pending)))
}

func printMemorySyncComplete() {
	fmt.Println(memoryOKStyle.Render("🧠 memory sync complete."))
}

type markdownStreamPreview struct {
	buffer            strings.Builder
	lastRenderAt      time.Time
	lastRenderedLines int
	minInterval       time.Duration
}

func newMarkdownStreamPreview() *markdownStreamPreview {
	return &markdownStreamPreview{
		minInterval: 120 * time.Millisecond,
	}
}

func (p *markdownStreamPreview) onChunk(chunk string) {
	if chunk == "" {
		return
	}

	p.buffer.WriteString(chunk)
	if time.Since(p.lastRenderAt) < p.minInterval && !strings.Contains(chunk, "\n") {
		return
	}
	p.renderCurrent()
}

func (p *markdownStreamPreview) onComplete(finalText string) {
	p.buffer.Reset()
	p.buffer.WriteString(finalText)
	p.renderCurrent()

	if !strings.HasSuffix(finalText, "\n") {
		fmt.Println()
	}
}

func (p *markdownStreamPreview) renderCurrent() {
	out := renderToString(p.buffer.String())
	if p.lastRenderedLines > 0 {
		fmt.Printf("\033[%dA", p.lastRenderedLines)
		fmt.Print("\033[J")
	}

	fmt.Print(out)
	p.lastRenderedLines = visualLineCount(out)
	p.lastRenderAt = time.Now()
}

func visualLineCount(s string) int {
	if s == "" {
		return 0
	}

	count := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		count++
	}
	if count == 0 {
		return 1
	}
	return count
}

func printWriteFileCall(req writeFileRequest) {
	lines := []string{toolStyle.Render("↳ tool: write_file")}
	if req.Reason != "" {
		lines = append(lines, subtleStyle.Render("reason: "+req.Reason))
	}
	lines = append(lines, subtleStyle.Render("path: "+req.Path))

	diff := strings.TrimRight(generateDiffPreview(req.OldStr, req.NewStr), "\n")
	lines = append(lines, subtleStyle.Render("diff:"))
	if diff != "" {
		lines = append(lines, diff)
	}
	fmt.Println(renderToolBlock(lines))
}

func printEditDenied() {
	fmt.Println(renderToolBlock([]string{warnStyle.Render("edit denied by user")}))
}

func printWriteFileResult(result writeFileResult) {
	if result.ExecutionErr != "" {
		line := subtleStyle.Render(fmt.Sprintf("tool result: error • %s", result.ExecutionErr))
		fmt.Println(renderToolBlock([]string{line}))
	} else {
		line := subtleStyle.Render(fmt.Sprintf("tool result: ok • modified %s", result.ModifiedLines))
		fmt.Println(renderToolBlock([]string{line}))
	}
}

func generateDiffPreview(oldStr, newStr string) string {
	var diff strings.Builder

	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	maxLines := 10
	contextLines := 2

	if len(oldLines) <= maxLines && len(newLines) <= maxLines {
		for _, line := range oldLines {
			diff.WriteString(fmt.Sprintf("  - %s\n", line))
		}
		for _, line := range newLines {
			diff.WriteString(fmt.Sprintf("  + %s\n", line))
		}
	} else {
		diff.WriteString(fmt.Sprintf("  [showing first %d lines of diff]\n", contextLines))
		for i := 0; i < contextLines && i < len(oldLines); i++ {
			diff.WriteString(fmt.Sprintf("  - %s\n", oldLines[i]))
		}
		if len(oldLines) > contextLines {
			diff.WriteString(fmt.Sprintf("  ... (%d more old lines)\n", len(oldLines)-contextLines))
		}
		for i := 0; i < contextLines && i < len(newLines); i++ {
			diff.WriteString(fmt.Sprintf("  + %s\n", newLines[i]))
		}
		if len(newLines) > contextLines {
			diff.WriteString(fmt.Sprintf("  ... (%d more new lines)\n", len(newLines)-contextLines))
		}
	}

	return diff.String()
}

func printClipboardWriteCall(req clipboardRequest) {
	lines := []string{toolStyle.Render("↳ tool: clipboard"), subtleStyle.Render("action: write")}

	charCount := len([]rune(req.Content))
	preview := req.Content
	maxPreview := 200

	if charCount > maxPreview {
		runes := []rune(req.Content)
		preview = string(runes[:maxPreview]) + "..."
	}

	previewLines := strings.Split(preview, "\n")
	if len(previewLines) > 5 {
		preview = strings.Join(previewLines[:5], "\n") + "\n..."
	}

	lines = append(lines, subtleStyle.Render(fmt.Sprintf("content (%d chars):", charCount)))
	if strings.TrimSpace(preview) != "" {
		lines = append(lines, preview)
	}
	fmt.Println(renderToolBlock(lines))
}

func printClipboardDenied() {
	fmt.Println(renderToolBlock([]string{warnStyle.Render("clipboard write denied by user")}))
}

func printClipboardResult(result clipboardResult) {
	if result.ExecutionErr != "" {
		line := subtleStyle.Render(fmt.Sprintf("tool result: error • %s", result.ExecutionErr))
		fmt.Println(renderToolBlock([]string{line}))
	} else if result.Request.Action == "read" {
		if result.Content == "" {
			line := subtleStyle.Render("tool result: ok • clipboard is empty")
			fmt.Println(renderToolBlock([]string{line}))
		} else {
			truncMsg := ""
			if result.Truncated {
				truncMsg = " (truncated)"
			}
			line := subtleStyle.Render(fmt.Sprintf("tool result: ok • read %d chars%s", result.CharCount, truncMsg))
			fmt.Println(renderToolBlock([]string{line}))
		}
	} else {
		line := subtleStyle.Render(fmt.Sprintf("tool result: ok • wrote %d chars", result.CharCount))
		fmt.Println(renderToolBlock([]string{line}))
	}
}
