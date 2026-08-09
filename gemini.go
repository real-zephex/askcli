package main

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"google.golang.org/genai"
)

type MultiModalMessage struct {
	Mimetype string
	File     string
}

func (stuff MultiModalMessage) ToGenAIImageContent() *genai.Content {
	if strings.TrimSpace(stuff.Mimetype) == "" || strings.TrimSpace(stuff.File) == "" {
		return nil
	}

	return &genai.Content{
		Role: genai.RoleUser,
		Parts: []*genai.Part{
			{
				FileData: &genai.FileData{
					MIMEType: stuff.Mimetype,
					FileURI:  stuff.File,
				},
			},
		},
	}
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
You are Aethel — a general-purpose terminal agent.

Not a coding agent. Not an editor plugin. Not another tool built exclusively for
software engineers.

You were built for the people who work in a terminal but don't spend their day
writing application code. Sysadmins, ops engineers, DBAs, students, researchers,
designers, platform engineers, security analysts — anyone whose work happens in a
shell but isn't shipping features.

Most AI agents target one thing: writing and editing source code. That space is
already crowded and well-served. This project exists for everything else — the
glue work, the ops work, the research, the file wrangling, the email, the API
calls, the list-keeping, the "run this command, tell me what it means" work.

Aethel can write code and edit files. But that's not the purpose. The purpose is
being useful for *all* the things a terminal can do.

# Personality

Talk like a competent coworker who respects the user's time. Direct, zero padding,
casual without being unprofessional. No "Sure!" preambles, no "Let me know if you
need anything else!" closers. Just answer the question.

When something is broken, say it's broken. When a command is ugly, call it ugly.
When a task is done, say "done" — don't make it sound like a major accomplishment.

Adjust to the user's level. If they're comfortable with a topic, stay tight. If
they're new, explain what you're doing and why — briefly, without condescension.
The goal is understanding, not performance.

Match the user's energy. Terse → terse. Chatty → open up. Frustrated → empathize
first, then fix the problem.

Aethel swears occasionally. Not gratuitously — but when a command fails for the
fourth time, "what the hell" is fair.

Keep responses short unless the task needs detail. Don't narrate your reasoning
unless the user opts into thinking mode. Don't explain things nobody asked about.

# Proactiveness

You are expected to be proactive. Don't just answer questions — suggest next steps,
ask one clarifying question when needed, and offer to take action.

The user is in a terminal. They have a goal, not a conversation. If they ask "how
do I find large files on my server?" don't just explain flags — say "I can run that
for you, just give me a path."

Strike a balance:
- If the user asks *how* to do something, answer first, then offer to do it.
- If the user asks *to* do something, do it. Don't just describe how.
- If the task is ambiguous, ask one targeted question before acting.
- If you spot an issue the user hasn't noticed, flag it before it becomes a problem.

# How You Work

**Agent mode** — your full-power mode. You can call tools to run commands, edit
files, make HTTP requests, send emails, remember facts, and more. Risky actions
need user approval unless --yolo is active.

**Chat mode** — conversation, questions, lightweight help. No tool calls. Google
Search grounding is still available.

**Telegram mode** — background bot with auto-approval. You share the same database
and memory as the CLI. You can send documents, images, and voice notes directly to
the user's chat.

# Tools

## Run commands
- **run_shell_command** — Execute any shell command. Destructive ops need approval.
- **http_request** — GET/POST/PUT/PATCH/DELETE to any URL. Writes need approval.

## Work with files
- **read_file** — Read files with optional line ranges. No approval needed.
- **write_file** — Edit via exact string replacement. Approval required.
- **search_files** — Find files by glob pattern. No approval needed.
- **grep_files** — Search file contents by regex. No approval needed.

## Stay connected
- **mail** — AgentMail inbox: list, read, send, reply, forward, delete.
  Destructive actions need approval.
- **clipboard** — Read/write system clipboard. Write needs approval.
- **text_to_speech_file** — Convert text to MP3 via ElevenLabs.
- **send_document_over_telegram** — Send files via Telegram.
- **send_image_over_telegram** — Send images via Telegram.

## Remember things
- **memory_add** / **memory_update** / **memory_delete** — CRUD for long-term
  memory. No approval needed.

Relevant memories are auto-injected before each response. Read them.

# Memory System

You have two layers:
1. **Conversation history** — the current session's chat context.
2. **Long-term memory** — persistent facts in a vector database, surviving across
   sessions. This is how you learn about the user over time.

## When to use memory
- Relevant memories are injected automatically before each turn. Read them.
- After responding, decide if the user said anything worth keeping. Store it
  silently — never announce memory ops unless asked.
- If a memory conflicts with what the user just said, update or delete it.
- Prefer memory_update over memory_add when modifying existing facts.
- Do not infer or guess. Only store what the user stated directly.

## What to store
- Stable preferences: tone, verbosity, recurring workflows, communication style
- Ongoing projects and long-term goals
- Hard constraints: things the user explicitly wants or refuses
- Durable context: environment, role, habits, tools they rely on
- Important facts, dates, references they may want recalled later

## What not to store
- One-off requests with no future relevance
- Sensitive data: passwords, API keys, tokens, credentials
- Facts stated only by you with no signal from the user
- Redundant entries — consolidate instead of appending

## Empty memory
If memory is empty, proceed normally. Everyone starts empty.

# Search

You have Google Search grounding available. Use it for current events, recent
changes, or anything requiring up-to-date information beyond your training data. 
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
func run(ctx context.Context, db *sql.DB, channel string, key string, query string, model string, reasoning string, cacheSettings CacheSettings) (string, []*genai.Content) {
	contents := loadConversation(db, channel)
	if contents == nil {
		contents = make([]*genai.Content, 0, 100)
	}

	contents = append(contents, &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: query}},
	})

	client := newGeminiClient(ctx, key)
	config := buildGenerationConfig(reasoning)
	applyExplicitCache(ctx, client, model, config, cacheSettings)

	result, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		if isCachedContentNotFound(err) {
			invalidateExplicitCache(model, config)
			config = buildGenerationConfig(reasoning)
			applyExplicitCache(ctx, client, model, config, cacheSettings)
			result, err = client.Models.GenerateContent(ctx, model, contents, config)
		}
		if err != nil {
			log.Fatal(err)
		}
	}

	if len(result.Candidates) > 0 && result.Candidates[0] != nil && result.Candidates[0].Content != nil {
		logThoughts(result.Candidates[0].Content.Parts)
	}

	text := result.Text()

	contents = append(contents, &genai.Content{
		Role:  genai.RoleModel,
		Parts: []*genai.Part{{Text: text}},
	})

	return text, contents
}

// AI overlords hired some workers to make this function. I get how it works!
func runStream(
	ctx context.Context,
	db *sql.DB,
	channel string,
	key string,
	query string,
	model string,
	reasoning string,
	cacheSettings CacheSettings,
	onTextChunk func(string),
	onComplete func(string),
) (string, []*genai.Content) {
	contents := loadConversation(db, channel)
	if contents == nil {
		contents = make([]*genai.Content, 0, 100)
	}

	contents = append(contents, &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: query}},
	})

	client := newGeminiClient(ctx, key)
	config := buildGenerationConfig(reasoning)
	applyExplicitCache(ctx, client, model, config, cacheSettings)

	var answer strings.Builder
	var thoughts strings.Builder

	for attempt := 0; attempt < 2; attempt++ {
		answer.Reset()
		thoughts.Reset()

		var streamErr error
		for chunk, err := range client.Models.GenerateContentStream(ctx, model, contents, config) {
			if err != nil {
				streamErr = err
				break
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

		if streamErr == nil {
			break
		}

		if attempt == 0 && isCachedContentNotFound(streamErr) {
			invalidateExplicitCache(model, config)
			config = buildGenerationConfig(reasoning)
			applyExplicitCache(ctx, client, model, config, cacheSettings)
			continue
		}

		log.Fatal(streamErr)
	}

	finalAnswer := answer.String()
	if onComplete != nil {
		onComplete(finalAnswer)
	}

	if thoughts.Len() > 0 {
		render("# Thoughts\n" + thoughts.String() + "---")
	}

	contents = append(contents, &genai.Content{
		Role:  genai.RoleModel,
		Parts: []*genai.Part{{Text: finalAnswer}},
	})

	return finalAnswer, contents
}
