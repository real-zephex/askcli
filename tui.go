package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	viewportStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)
)

type tuiModel struct {
	db        *sql.DB
	ctx       context.Context
	cancel    context.CancelFunc
	state     *replState
	key       string
	server    string
	apiKey    string

	viewport   viewport.Model
	textinput  textinput.Model
	messages   []string
	activeText string
	width      int
	height     int

	thinking   bool
	statusText string
	approval   *approvalState
	history    []string
	historyIdx int
}

type tuiLogMsg string
type tuiStreamChunkMsg string
type tuiStreamCompleteMsg string
type tuiThinkingMsg struct {
	thinking bool
	status   string
}
type tuiApprovalRequestMsg struct {
	prompt       string
	responseChan chan bool
}
type tuiQueryFinishedMsg struct{}

type approvalState struct {
	prompt       string
	responseChan chan bool
}

func (m *tuiModel) runQuery(query string) {
	m.thinking = true
	m.statusText = "thinking..."

	go func() {
		var res string
		ctx := m.ctx

		// Bind global TUI hooks for the duration of the query execution
		TUIPrintHook = func(text string) {
			if activeTUIProgram != nil {
				activeTUIProgram.Send(tuiLogMsg(text))
			}
		}
		TUIStreamHook = func(chunk string) {
			if activeTUIProgram != nil {
				activeTUIProgram.Send(tuiStreamChunkMsg(chunk))
			}
		}
		TUIStreamCompleteHook = func(finalText string) {
			if activeTUIProgram != nil {
				activeTUIProgram.Send(tuiStreamCompleteMsg(finalText))
			}
		}
		TUIThinkingHook = func(thinking bool, status string) {
			if activeTUIProgram != nil {
				activeTUIProgram.Send(tuiThinkingMsg{thinking: thinking, status: status})
			}
		}
		TUIApprovalHook = func(prompt string) bool {
			if activeTUIProgram != nil {
				respChan := make(chan bool)
				activeTUIProgram.Send(tuiApprovalRequestMsg{
					prompt:       prompt,
					responseChan: respChan,
				})
				return <-respChan
			}
			return false
		}

		if m.server != "" {
			TUIThinkingHook(true, "posting to remote ask...")
			var err error
			res, err = postToRemoteAsk(ctx, m.server, m.apiKey, query, m.state.model, m.state.reasoning)
			TUIThinkingHook(false, "")
			if err != nil {
				TUIPrintHook(fmt.Sprintf("Error: %v\n", err))
			} else {
				TUIPrintHook(renderToStringWithWidth(res, m.viewport.Width) + "\n")
			}
		} else {
			if m.state.agent {
				TUIThinkingHook(true, "thinking...")
				res, contents := runAgentTurn(ctx, m.db, "default", m.key, query, m.state.model, m.state.reasoning, m.state.cache, m.state.yolo, 0, nil)
				TUIThinkingHook(false, "")
				TUIPrintHook(renderToStringWithWidth(res, m.viewport.Width) + "\n")
				saveConversation(m.db, "default", contents)
			} else if m.state.stream {
				_, contents := runStream(
					ctx,
					m.db,
					"default",
					m.key,
					query,
					m.state.model,
					m.state.reasoning,
					m.state.cache,
					TUIStreamHook,
					TUIStreamCompleteHook,
				)
				saveConversation(m.db, "default", contents)
			} else {
				TUIThinkingHook(true, "thinking...")
				res, contents := run(ctx, m.db, "default", m.key, query, m.state.model, m.state.reasoning, m.state.cache)
				TUIThinkingHook(false, "")
				TUIPrintHook(renderToStringWithWidth(res, m.viewport.Width) + "\n")
				saveConversation(m.db, "default", contents)
			}
		}

		if activeTUIProgram != nil {
			activeTUIProgram.Send(tuiQueryFinishedMsg{})
		}
	}()
}

func (m tuiModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m tuiModel) headerView() string {
	title := headerStyle.Render("ask • interactive mode")
	meta := subtleStyle.Render(fmt.Sprintf(
		"model: %s • reasoning: %s • stream: %t • agent: %t • yolo: %t • cache: %t",
		m.state.model, m.state.reasoning, m.state.stream, m.state.agent, m.state.yolo, m.state.cache.Enabled,
	))
	bar := subtleStyle.Render(strings.Repeat("─", m.width))
	if len(bar) > 0 {
		return title + "\n" + meta + "\n" + bar
	}
	return title + "\n" + meta
}

func (m tuiModel) renderContent() string {
	var sb strings.Builder
	for _, text := range m.messages {
		sb.WriteString(text)
	}
	if m.activeText != "" {
		sb.WriteString(renderToStringWithWidth(m.activeText, m.viewport.Width))
	}
	return sb.String()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.approval != nil {
			switch msg.String() {
			case "y", "Y":
				m.approval.responseChan <- true
				m.messages = append(m.messages, "ask ❯ approve? Yes\n")
				m.approval = nil
				m.viewport.SetContent(m.renderContent())
				m.viewport.GotoBottom()
			case "n", "N", "esc":
				m.approval.responseChan <- false
				m.messages = append(m.messages, "ask ❯ approve? No\n")
				m.approval = nil
				m.viewport.SetContent(m.renderContent())
				m.viewport.GotoBottom()
			case "enter":
				m.approval.responseChan <- false
				m.messages = append(m.messages, "ask ❯ approve? No\n")
				m.approval = nil
				m.viewport.SetContent(m.renderContent())
				m.viewport.GotoBottom()
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "pgup":
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case "pgdown":
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case "up":
			if len(m.history) > 0 && m.historyIdx > 0 {
				m.historyIdx--
				m.textinput.SetValue(m.history[m.historyIdx])
				m.textinput.SetCursor(len(m.history[m.historyIdx]))
			}
			return m, nil
		case "down":
			if len(m.history) > 0 && m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textinput.SetValue(m.history[m.historyIdx])
				m.textinput.SetCursor(len(m.history[m.historyIdx]))
			} else {
				m.historyIdx = len(m.history)
				m.textinput.SetValue("")
			}
			return m, nil
		case "enter":
			input := strings.TrimSpace(m.textinput.Value())
			if input == "" {
				return m, nil
			}

			m.textinput.SetValue("")
			
			wrapLimit := m.viewport.Width - 4
			if wrapLimit < 20 {
				wrapLimit = 20
			}
			wrappedInput := wrapText(input, wrapLimit)
			lines := strings.Split(wrappedInput, "\n")
			var formattedUserMsg strings.Builder
			formattedUserMsg.WriteString("\n")
			for _, line := range lines {
				formattedUserMsg.WriteString(userMsgStyle.Render(line) + "\n")
			}
			formattedUserMsg.WriteString("\n")
			m.messages = append(m.messages, formattedUserMsg.String())

			m.viewport.SetContent(m.renderContent())
			m.viewport.GotoBottom()

			m.history = append(m.history, input)
			m.historyIdx = len(m.history)

			if input == "/exit" || input == "/quit" {
				m.cancel()
				return m, tea.Quit
			}

			if strings.HasPrefix(input, "/") {
				var buf strings.Builder
				TUIPrintHook = func(text string) {
					buf.WriteString(text)
				}
				handled, shouldExit := handleSlashCommand(input, m.db, m.state)
				TUIPrintHook = nil

				if shouldExit {
					return m, tea.Quit
				}
				if handled {
					m.messages = append(m.messages, buf.String())
					m.viewport.SetContent(m.renderContent())
					m.viewport.GotoBottom()
					return m, nil
				}
			}

			m.runQuery(input)
			return m, nil
		}

	case tuiLogMsg:
		m.messages = append(m.messages, string(msg))
		m.viewport.SetContent(m.renderContent())
		m.viewport.GotoBottom()
		return m, nil

	case tuiStreamChunkMsg:
		m.activeText += string(msg)
		m.viewport.SetContent(m.renderContent())
		m.viewport.GotoBottom()
		return m, nil

	case tuiStreamCompleteMsg:
		m.activeText = ""
		m.messages = append(m.messages, renderToStringWithWidth(string(msg), m.viewport.Width)+"\n")
		m.viewport.SetContent(m.renderContent())
		m.viewport.GotoBottom()
		return m, nil

	case tuiThinkingMsg:
		m.thinking = msg.thinking
		m.statusText = msg.status
		return m, nil

	case tuiApprovalRequestMsg:
		m.approval = &approvalState{
			prompt:       msg.prompt,
			responseChan: msg.responseChan,
		}
		m.messages = append(m.messages, msg.prompt)
		m.viewport.SetContent(m.renderContent())
		m.viewport.GotoBottom()
		return m, nil

	case tuiQueryFinishedMsg:
		m.thinking = false
		m.statusText = ""
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := 3
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - headerHeight - footerHeight - 2
		m.viewport.SetContent(m.renderContent())
		m.viewport.GotoBottom()
	}

	m.textinput, cmd = m.textinput.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	var s strings.Builder

	// Header
	s.WriteString(m.headerView() + "\n")

	// Viewport (chat scrollback)
	s.WriteString(viewportStyle.Render(m.viewport.View()) + "\n")

	// Status line / Approval prompt
	if m.approval != nil {
		s.WriteString(statusStyle.Render("⚠️  "+m.approval.prompt) + " Press [y] for Yes, [n] for No\n")
	} else if m.thinking {
		s.WriteString(statusStyle.Render("● "+m.statusText) + "\n")
	} else {
		s.WriteString("\n")
	}

	// Bottom fixed Input bar
	if m.approval != nil {
		s.WriteString(promptStyle.Render("approve? ❯") + " ")
	} else {
		s.WriteString(chatPrompt() + m.textinput.View())
	}

	return s.String()
}

var activeTUIProgram *tea.Program

func runTUI(ctx context.Context, db *sql.DB, key string, server string, apiKey string, state *replState) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ti := textinput.New()
	ti.Placeholder = "Ask a question..."
	ti.Focus()
	ti.CharLimit = 2048
	ti.Width = 120

	initWidth := terminalWidth()
	vp := viewport.New(initWidth-4, 20)

	m := &tuiModel{
		db:        db,
		ctx:       ctx,
		cancel:    cancel,
		state:     state,
		key:       key,
		server:    server,
		apiKey:    apiKey,
		textinput: ti,
		viewport:  vp,
	}



	// Pre-load last 20 messages from history database
	contents := loadConversation(db, "default")
	start := 0
	if len(contents) > 20 {
		start = len(contents) - 20
	}
	for i := start; i < len(contents); i++ {
		msg := contents[i]
		if msg.Role == "user" {
			wrapLimit := vp.Width - 4
			if wrapLimit < 20 {
				wrapLimit = 20
			}
			var text string
			for _, part := range msg.Parts {
				if part != nil && part.Text != "" {
					text = part.Text
					break
				}
			}
			wrappedInput := wrapText(text, wrapLimit)
			lines := strings.Split(wrappedInput, "\n")
			var formattedUserMsg strings.Builder
			formattedUserMsg.WriteString("\n")
			for _, line := range lines {
				formattedUserMsg.WriteString(userMsgStyle.Render(line) + "\n")
			}
			formattedUserMsg.WriteString("\n")
			m.messages = append(m.messages, formattedUserMsg.String())
		} else {
			var text string
			for _, part := range msg.Parts {
				if part != nil && part.Text != "" {
					text = part.Text
					break
				}
			}
			m.messages = append(m.messages, renderToStringWithWidth(text, vp.Width)+"\n")
		}
	}

	m.viewport.SetContent(m.renderContent())
	m.viewport.GotoBottom()

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	activeTUIProgram = p
	defer func() {
		activeTUIProgram = nil
		TUIPrintHook = nil
		TUIStreamHook = nil
		TUIStreamCompleteHook = nil
		TUIThinkingHook = nil
		TUIApprovalHook = nil
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}
}

func renderToStringWithWidth(text string, width int) string {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return out
}

func wrapText(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var wrappedLines []string

	for _, line := range lines {
		if len(line) <= limit {
			wrappedLines = append(wrappedLines, line)
			continue
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			wrappedLines = append(wrappedLines, "")
			continue
		}

		var currentLine string
		for _, word := range words {
			if len(currentLine)+len(word)+1 > limit {
				if currentLine != "" {
					wrappedLines = append(wrappedLines, currentLine)
				}
				for len(word) > limit {
					wrappedLines = append(wrappedLines, word[:limit])
					word = word[limit:]
				}
				currentLine = word
			} else {
				if currentLine == "" {
					currentLine = word
				} else {
					currentLine += " " + word
				}
			}
		}
		if currentLine != "" {
			wrappedLines = append(wrappedLines, currentLine)
		}
	}

	return strings.Join(wrappedLines, "\n")
}
