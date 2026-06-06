package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

type replState struct {
	model     string
	reasoning string
	stream    bool
	agent     bool
	yolo      bool
	cache     CacheSettings
}

func startREPL(ctx context.Context, db *sql.DB, key string, model string, reasoning string, cacheSettings CacheSettings) {
	state := &replState{
		model:     model,
		reasoning: reasoning,
		stream:    *stream,
		agent:     *agent,
		yolo:      *yolo,
		cache:     cacheSettings,
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		runTUI(ctx, db, key, "", "", state)
		return
	}

	printREPLHeader(state.model, state.reasoning, state.stream, state.agent, state.yolo, state.cache.Enabled)

	scanner := bufio.NewScanner(os.Stdin)
	// allow longer pasted prompts
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for {
		flushRememberResults()
		fmt.Print(chatPrompt())
		if !scanner.Scan() {
			waitForRememberTasks("Closing chat")
			fmt.Println("\nGoodbye!")
			return
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Slash commands are local control actions and should never be learned as memories.
		if strings.HasPrefix(input, "/") {
			handled, shouldExit := handleSlashCommand(input, db, state)
			if shouldExit {
				waitForRememberTasks("Closing chat")
				fmt.Println("Goodbye!")
				return
			}
			if handled {
				continue
			}
		}

		handled, shouldExit := handleSlashCommand(input, db, state)
		if shouldExit {
			waitForRememberTasks("Closing chat")
			fmt.Println("Goodbye!")
			return
		}
		if handled {
			continue
		}

		var res string
		if state.agent {
			printThinking()
			res = runAgentTurn(ctx, db, key, input, state.model, state.reasoning, state.cache, state.yolo, 0, nil)
			clearThinking()
			printFinalRenderLabel()
			render(res)
			fmt.Println()
		} else if state.stream {
			printStreamingLabel()
			preview := newMarkdownStreamPreview()
			res = runStream(
				ctx,
				db,
				key,
				input,
				state.model,
				state.reasoning,
				state.cache,
				preview.onChunk,
				preview.onComplete,
			)
		} else {
			printThinking()
			res = run(ctx, db, key, input, state.model, state.reasoning, state.cache)
			clearThinking()

			printFinalRenderLabel()
			render(res)
			fmt.Println()
		}

		saveMessage(db, "user", input)
		saveMessage(db, "assistant", res)

		//		scheduleRememberTurn(input, res)
	}
}

func handleSlashCommand(input string, db *sql.DB, state *replState) (handled bool, shouldExit bool) {
	if !strings.HasPrefix(input, "/") {
		return false, false
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return true, false
	}

	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "/exit", "/quit":
		return true, true
	case "/help":
		printSlashHelp()
		return true, false
	case "/status":
		printStatus(state)
		return true, false
	case "/clear":
		clearDatabase(db)
		uiPrintln("Conversation history cleared.")
		return true, false
	case "/agent":
		on, ok := parseOnOffArg(args)
		if !ok {
			uiPrintln("Usage: /agent on|off")
			return true, false
		}
		state.agent = on
		uiPrintf("Agent mode: %t\n", state.agent)
		return true, false
	case "/yolo":
		on, ok := parseOnOffArg(args)
		if !ok {
			uiPrintln("Usage: /yolo on|off")
			return true, false
		}
		state.yolo = on
		uiPrintf("YOLO mode: %t\n", state.yolo)
		return true, false
	case "/stream":
		on, ok := parseOnOffArg(args)
		if !ok {
			uiPrintln("Usage: /stream on|off")
			return true, false
		}
		state.stream = on
		uiPrintf("Streaming: %t\n", state.stream)
		return true, false
	case "/cache":
		on, ok := parseOnOffArg(args)
		if !ok {
			uiPrintln("Usage: /cache on|off")
			return true, false
		}
		state.cache.Enabled = on
		uiPrintf("Explicit caching: %t\n", state.cache.Enabled)
		return true, false
	case "/cache-ttl":
		if len(args) != 1 {
			uiPrintln("Usage: /cache-ttl <duration>")
			return true, false
		}
		d, err := time.ParseDuration(args[0])
		if err != nil || d < 0 {
			uiPrintln("Invalid duration. Examples: 30m, 2h, 10s")
			return true, false
		}
		state.cache.TTL = d
		uiPrintf("Explicit cache TTL: %s\n", state.cache.TTL)
		return true, false
	case "/model":
		if len(args) == 0 {
			uiPrintf("Current model: %s\n", state.model)
			uiPrintln("Aliases: free | cheap | exp")
			return true, false
		}
		state.model = resolveModels(args[0])
		uiPrintf("Model set to: %s\n", state.model)
		return true, false
	case "/reason":
		if len(args) == 0 {
			uiPrintf("Current reasoning: %s\n", state.reasoning)
			uiPrintln("Values: HIGH | MED | LOW | MIN")
			return true, false
		}
		normalized, ok := normalizeReasonArg(args[0])
		if !ok {
			uiPrintln("Usage: /reason HIGH|MED|LOW|MIN")
			return true, false
		}
		state.reasoning = normalized
		uiPrintf("Reasoning set to: %s\n", state.reasoning)
		return true, false
	case "/pwd":
		wd, err := os.Getwd()
		if err != nil {
			uiPrintf("pwd error: %v\n", err)
			return true, false
		}
		uiPrintln(wd)
		return true, false
	case "/cd":
		if len(args) == 0 {
			uiPrintln("Usage: /cd <path>")
			return true, false
		}
		target := strings.Join(args, " ")
		if err := os.Chdir(target); err != nil {
			uiPrintf("cd error: %v\n", err)
			return true, false
		}
		wd, _ := os.Getwd()
		uiPrintf("cwd: %s\n", wd)
		return true, false
	case "/history":
		limit := 10
		if len(args) > 0 {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				uiPrintln("Usage: /history [positive_number]")
				return true, false
			}
			if n > 50 {
				n = 50
			}
			limit = n
		}
		printHistory(db, limit)
		return true, false
	case "/memories":
		waitForRememberTasks("Syncing memory tasks before opening memory manager")
		runMemoryManager(context.Background())
		return true, false
	default:
		fmt.Printf("Unknown command: %s (use /help)\n", cmd)
		return true, false
	}
}

func parseOnOffArg(args []string) (bool, bool) {
	if len(args) != 1 {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on", "true", "1", "yes", "y":
		return true, true
	case "off", "false", "0", "no", "n":
		return false, true
	default:
		return false, false
	}
}

func normalizeReasonArg(value string) (string, bool) {
	r := strings.ToUpper(strings.TrimSpace(value))
	switch r {
	case "HIGH":
		return string(HIGH), true
	case "MED", "MID", "MEDIUM":
		return string(MED), true
	case "LOW":
		return string(LOW), true
	case "MIN", "MINIMAL":
		return string(MIN), true
	default:
		return "", false
	}
}

func printSlashHelp() {
	uiPrintln("Slash commands:")
	uiPrintln("  /help                Show this help")
	uiPrintln("  /status              Show current session state")
	uiPrintln("  /clear               Clear local conversation history")
	uiPrintln("  /agent on|off        Toggle shell agent mode")
	uiPrintln("  /yolo on|off         Toggle auto-approval in agent mode")
	uiPrintln("  /stream on|off       Toggle streaming output")
	uiPrintln("  /cache on|off        Toggle explicit context caching")
	uiPrintln("  /cache-ttl <dur>     Set explicit cache TTL (e.g. 30m, 2h)")
	uiPrintln("  /model [name|alias]  Get/set model (free|cheap|exp or full model name)")
	uiPrintln("  /reason [level]      Get/set reasoning (HIGH|MED|LOW|MIN)")
	uiPrintln("  /pwd                 Print current working directory")
	uiPrintln("  /cd <path>           Change current working directory")
	uiPrintln("  /history [n]         Show last n saved messages (default 10, max 50)")
	uiPrintln("  /memories            Open interactive memory manager (list/delete)")
	uiPrintln("  /exit                Exit chat mode")
}

func printStatus(state *replState) {
	wd, _ := os.Getwd()
	uiPrintln("Session status:")
	uiPrintf("  model:     %s\n", state.model)
	uiPrintf("  reasoning: %s\n", state.reasoning)
	uiPrintf("  stream:    %t\n", state.stream)
	uiPrintf("  agent:     %t\n", state.agent)
	uiPrintf("  yolo:      %t\n", state.yolo)
	uiPrintf("  cache:     %t\n", state.cache.Enabled)
	if state.cache.TTL > 0 {
		uiPrintf("  cache ttl: %s\n", state.cache.TTL)
	}
	uiPrintf("  cwd:       %s\n", wd)
}

func printHistory(db *sql.DB, limit int) {
	messages := getHistory(db, limit)
	if len(messages) == 0 {
		uiPrintln("No history found.")
		return
	}

	uiPrintf("Last %d messages:\n", len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		preview := strings.ReplaceAll(m.Content, "\n", " ")
		preview = strings.TrimSpace(preview)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		uiPrintf("  [%d] %-9s %s\n", m.ID, m.Role, preview)
	}
}
