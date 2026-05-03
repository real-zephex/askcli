package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const VERSION string = "1.0.0"

// alias for model
type Model string

const (
	free  Model = "gemma-4-26b-a4b-it"
	cheap Model = "gemini-3.1-flash-lite-preview"
	exp   Model = "gemini-3-flash-preview"
)

// alias for reasoning level
type Thought string

const (
	HIGH Thought = "HIGH"
	MED  Thought = "MEDIUM"
	LOW  Thought = "LOW"
	MIN  Thought = "MINIMAL"
)

// maps model constants to string variants because these constants are not available at runtime ig
func resolveModels(m string) string {
	switch m {
	case "free":
		return string(free)
	case "cheap":
		return string(cheap)
	case "exp":
		return string(exp)
	default:
		return m
	}
}

// same functionality as above function
func resolveReasoningLevel(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "HIGH":
		return string(HIGH)
	case "MED", "MID", "MEDIUM":
		return string(MED)
	case "LOW":
		return string(LOW)
	case "MIN", "MINIMAL":
		return string(MIN)
	default:
		return t
	}
}

// flags
var help = flag.Bool("help", false, "Show help menu")
var model = flag.String(
	"model",
	string(free),
	"the model name, e.g. gemma-4-26b-a4b-it")
var version = flag.Bool(
	"version",
	false,
	"the current version of the package",
)
var reasoning = flag.String(
	"reason",
	string(MIN),
	"reasoning effort, e.g. HIGH, MED, LOW, MIN",
)
var clr = flag.Bool(
	"clear",
	false,
	"clear the local database. WARNING: This will remove all the past conversations",
)
var chat = flag.Bool(
	"chat",
	false,
	"start interactive chat (REPL) mode",
)
var stream = flag.Bool(
	"stream",
	true,
	"stream incremental rendered markdown updates",
)
var agent = flag.Bool(
	"agent",
	false,
	"enable agent mode in chat (model can request shell commands)",
)
var yolo = flag.Bool(
	"yolo",
	false,
	"auto-approve shell commands in agent mode (dangerous)",
)
var systemFile = flag.String(
	"system",
	"",
	"path to a file containing system prompt text",
)
var background = flag.Bool(
	"background", 
	false,
	"whether to function as a full blown background agent or not",
)

// simple, checks for gemini api key
func checkForEnv() (string, bool) {
	value, exists := os.LookupEnv("GEMINI_API_KEY")
	if !exists {
		log.Fatal("GEMINI API KEY not found in the PATH. Add the key to the path and restart the program")
	}
	return value, true
}

func main() {
	flag.Parse()
	ctx := context.Background()
	args := flag.Args()

	installInterruptHandler()

	if *help {
		helpMenu()
		os.Exit(0)
	}

	if *version {
		fmt.Println("ask\nversion: ", VERSION)
		os.Exit(0)
	}

	if code := handleCompletionCommand(args); code >= 0 {
		os.Exit(code)
	}

	// sqlite database for storing chat history
	db := initDB()
	defer db.Close()

	// chromem-go database for vectorization
	collection, err := getDb()
	if err != nil {
		log.Fatal(err)
	}
	memoryCollection = collection

	if *clr {
		clearDatabase(db)
		fmt.Println("Removed all the rows from the database. Start afresh!")
		os.Exit(0)
	}

	if *background {
		backgroundManager(db, ctx)
		os.Exit(0)
	}

	if len(args) == 1 && args[0] == "memories" {
		memories, err := listStoredMemories()
		if err != nil {
			log.Fatalf("failed to list memories: %v", err)
		}
		if len(memories) == 0 {
			fmt.Println("No stored memories found.")
			os.Exit(0)
		}
		for i, memory := range memories {
			fmt.Printf("%d. %s\n", i+1, memory)
		}
		os.Exit(0)
	}
	if len(args) == 2 && args[0] == "memories" && args[1] == "manage" {
		runMemoryManager(ctx)
		os.Exit(0)
	}

	if *systemFile != "" {
		runtimeSystemPrompt = loadSystemPromptFromFile(*systemFile)
	}

	e, _ := checkForEnv()

	if *chat || (len(args) == 1 && args[0] == "chat") {
		startREPL(ctx, db, e, resolveModels(*model), resolveReasoningLevel(*reasoning))
		os.Exit(0)
	}

	var stdinInput string
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		stdinInput = string(bytes)
	}

	if len(args) == 0 {
		helpMenu()
		os.Exit(1)
	}
	query := strings.Join(args, " ")

	if stdinInput != "" && query != "" {
		query = query + "\n\n" + stdinInput
	} else if stdinInput != "" {
		query = stdinInput
	}
	if query == "" {
		helpMenu()
		os.Exit(1)
	}

	resolvedModel := resolveModels(*model)
	resolvedReasoning := resolveReasoningLevel(*reasoning)

	var res string
	if *stream {
		printStreamingLabel()
		preview := newMarkdownStreamPreview()
		res = runStream(
			ctx,
			db,
			e,
			query,
			resolvedModel,
			resolvedReasoning,
			preview.onChunk,
			preview.onComplete,
		)
	} else {
		printThinking()
		res = run(ctx, db, e, query, resolvedModel, resolvedReasoning)
		clearThinking()
		printFinalRenderLabel()
		render(res)
	}

	// save conversation after successful response
	saveMessage(db, "user", query)
	saveMessage(db, "assistant", res)

	// scheduleRememberTurn(query, res)
	// waitForRememberTasks("Finishing memory sync before exit")
}
