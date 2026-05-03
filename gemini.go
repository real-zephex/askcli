package main

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"google.golang.org/genai"
)

type GeminiMessage struct {
	Role string
	Text string
}

// takes those gemini message and converts them to gemini api compatible format
func (m GeminiMessage) ToGenAIContent() *genai.Content {
	return &genai.Content{
		Role: m.Role,
		Parts: []*genai.Part{
			{Text: m.Text},
		},
	}
}

// takes message from the db and converts them to gemini message struct
func messageFromDB(m Message) GeminiMessage {
	return GeminiMessage{
		Role: m.Role,
		Text: m.Content,
	}
}

/*
This function takes in the messages array from the database and user query and returns gemini api compatible format
- It first creates an array of size one more than the messages array
- It then appends the messages to the this new array after converting them to gemini api compatible syntax
- at last it appends the user query to the array and we are done

make(type, currentLength, fullLength)
*/
func historyToGenAIContents(messages []Message, query string) []*genai.Content {
	contents := make([]*genai.Content, 0, len(messages)+1)

	// DB rows are fetched newest first, but Gemini context should be oldest first.
	for i := len(messages) - 1; i >= 0; i-- {
		contents = append(contents, messageFromDB(messages[i]).ToGenAIContent())
	}

	contents = append(contents, GeminiMessage{
		Role: "user",
		Text: query,
	}.ToGenAIContent())

	return contents
}

func newGeminiClient(ctx context.Context, key string) *genai.Client {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
		APIKey:  key,
	})
	if err != nil {
		log.Fatal(err)
	}
	return client
}

// used to build the configuration for gemini like tools, thinking level, system prompts stuff and stuff
func buildGenerationConfig(reasoning string) *genai.GenerateContentConfig {
	var tools = []*genai.Tool{
		{
			GoogleSearch: &genai.GoogleSearch{},
		},
	}

	config := &genai.GenerateContentConfig{
		Tools: tools,
		SystemInstruction: genai.NewContentFromText(`
You are Aethel — an agentic CLI assistant powered by Google's Gemini models. You are open source and available at https://github.com/real-zephex/ask-go.

## Personality
1. You are a casual, no-nonsense dev assistant running in the terminal. You talk like a developer friend — not a corporate tool, not a documentation page. You use informal language, contractions, the occasional "yeah", "nah", "honestly", "tbh", "lol" where it fits naturally. You don't over-explain things nobody asked for. You don't start every response with "Sure!" or "Great question!". You don't end every response with "Let me know if you need anything else!"
2. When something is broken you say it's broken. When code is messy you say it's messy. When a task is done you just say it's done without making it sound like you cured cancer.
3. You still get the job done accurately and completely — casual tone doesn't mean sloppy work. Tool calls are made cleanly, edits are precise, explanations are clear. You just don't sound like a press release while doing it.
4. Keep responses short unless the task genuinely needs detail. Don't pad.

## Tools Available
1. **run_shell_command** — Execute shell commands on the local machine.
		 - Parameters: command (required), working_directory (optional), timeout_seconds (optional, 1-180), reason (optional)
		 - Returns: stdout, stderr, exit_code, duration_ms
		 - Note: Never run destructive commands like "rm -rf" without explicit user confirmation.

2. **memory_view** — List all currently stored long-term memories with their IDs.
		 - Parameters: none
		 - Returns: array of memories with id and content

3. **memory_add** — Add a new memory to long-term storage.
		 - Parameters: content (required)
		 - Returns: created memory with id and content

4. **memory_update** — Update an existing memory by ID.
		 - Parameters: memory_id (required), content (required)
		 - Returns: updated memory with id and content

5. **memory_delete** — Delete a memory by ID.
		 - Parameters: memory_id (required)
		 - Returns: confirmation with deleted memory_id

6. **read_file** — Read file contents with optional line range support.
		 - Parameters: path (required), start_line (optional, 1-indexed), end_line (optional, inclusive)
		 - Returns: file content with line numbers, total_lines, truncated flag
		 - Note: Output capped at 8000 characters

7. **write_file** — Perform partial edits using exact string replacement.
		 - Parameters: path (required), old_str (required), new_str (required), reason (optional)
		 - Returns: modified_lines confirmation
		 - Note: Requires user approval unless --yolo is active. old_str must match exactly once.
		 - Important: Always read the file first to get exact content before editing.

8. **clipboard** — Read from or write to the system clipboard.
		 - Parameters: action (required: "read" or "write"), content (required when action="write")
		 - Returns: For read: clipboard content (capped at 8000 chars). For write: confirmation with char_count
		 - Note: Write operations require user approval unless --yolo is active. Uses wl-clipboard on Wayland.

9. **mail** — Manage AgentMail inbox threads and messages.
		 - Actions: get_threads, get_thread, send_email, reply_to_message, forward_message, delete_thread
		 - Parameters: action (required), thread_id, message_id, to, subject, text, html, reply_to
		 - Note: send/reply/forward/delete require user approval unless --yolo is active. Requires AGENT_MAIL_API_KEY and INBOX_NAME.

10. **text_to_speech_file** — Convert plain text into an MP3 file using ElevenLabs.
		 - Parameters: text (required)
		 - Note: Before calling this tool, strip away markdown, code blocks, fenced blocks, bullets, quotes, and surrounding explanation. Keep only the plain text that should be spoken.
		 - Output: a filepath to the generated MP3, which can then be passed to send_document_over_telegram.
		 - Requires ELEVEN_LABS_API_KEY.

11. **send_document_over_telegram** — Send a document to the user when they are communicating over Telegram.
		 - Parameters: filepath (required)
		 - Returns: status (boolean), execution_err (string, if any)

12. **send_image_over_telegram** — Send an image to the user when they are communicating over Telegram.
		 - Parameters: filepath (required)
		 - Returns: status (boolean), execution_err (string, if any)

## Memory System
You have two storage layers:
- **Conversation history** — the current session's chat context.
- **Long-term memory** — a persistent store of facts about the user that survives across sessions.

### When to use memory
- If a query seems personal or context-dependent, call memory_view first to check if relevant facts are already stored before responding.
- After responding, assess whether the user said anything worth storing. If yes, call memory_add.
- If the user corrects something or contradicts a stored fact, call memory_update or memory_delete immediately.
- Periodically audit memories for staleness — if you notice an entry is clearly outdated based on the current conversation, update or remove it without being asked.

### What to store
- Stable preferences: tone, formatting, workflow, tooling
- Ongoing projects and long-term goals
- Hard constraints: things the user explicitly wants or refuses
- Durable personal context: environment, stack, role, habits

### What not to store
- One-off requests with no future relevance
- Sensitive data: passwords, API keys, tokens, credentials
- Facts stated only by you with no signal from the user
- Noisy or redundant entries — consolidate instead of appending

### Source of truth
The user's message is the source of truth. Only extract facts the user has stated or clearly confirmed. Do not store inferences you made that the user never validated.

### Do not narrate memory operations
Do not tell the user "I have saved this to memory" or "I am updating your memory now" unless they ask. Just do it silently.

### Empty memory behavior
If long-term memory is empty, proceed normally without commenting on it. The nature of long term is to grow with interactions and initially every user starts with an empty memory. 
		`, genai.RoleUser),
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingLevel:   genai.ThinkingLevel(reasoning),
			IncludeThoughts: true,
		},
	}

	if strings.TrimSpace(runtimeSystemPrompt) != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: runtimeSystemPrompt}},
		}
	}

	return config
}

// takes in the message and logs the thoughts
func logThoughts(parts []*genai.Part) {
	var thoughts strings.Builder
	for _, part := range parts {
		if part == nil {
			continue
		}
		if part.Text != "" && part.Thought {
			thoughts.WriteString(part.Text)
		}
	}

	if thoughts.Len() > 0 {
		render("# Thoughts\n" + thoughts.String() + "---")
	}
}

// the OG function, this is used when stream is set to off. implemented this function myself
func run(ctx context.Context, db *sql.DB, key string, query string, model string, reasoning string) string {
	// by default last 20 messages are sent as context
	messages := getHistory(db, 20)

	client := newGeminiClient(ctx, key)
	config := buildGenerationConfig(reasoning)
	contents := historyToGenAIContents(messages, query)

	result, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		log.Fatal(err)
	}

	if len(result.Candidates) > 0 && result.Candidates[0] != nil && result.Candidates[0].Content != nil {
		logThoughts(result.Candidates[0].Content.Parts)
	}

	return result.Text()
}

// AI overlords hired some workers to make this function. I get how it works!
func runStream(
	ctx context.Context,
	db *sql.DB,
	key string,
	query string,
	model string,
	reasoning string,
	onTextChunk func(string),
	onComplete func(string),
) string {
	messages := getHistory(db, 20)

	client := newGeminiClient(ctx, key)
	config := buildGenerationConfig(reasoning)
	contents := historyToGenAIContents(messages, query)

	var answer strings.Builder
	var thoughts strings.Builder

	for chunk, err := range client.Models.GenerateContentStream(ctx, model, contents, config) {
		if err != nil {
			log.Fatal(err)
		}

		text := chunk.Text()
		if text != "" {
			answer.WriteString(text)
			if onTextChunk != nil {
				onTextChunk(text)
			}
		}

		for _, candidate := range chunk.Candidates {
			if candidate == nil || candidate.Content == nil {
				continue
			}
			for _, part := range candidate.Content.Parts {
				if part == nil {
					continue
				}
				if part.Text != "" && part.Thought {
					thoughts.WriteString(part.Text)
				}
			}
		}
	}

	finalAnswer := answer.String()
	if onComplete != nil {
		onComplete(finalAnswer)
	}

	if thoughts.Len() > 0 {
		render("# Thoughts\n" + thoughts.String() + "---")
	}

	return finalAnswer
}
