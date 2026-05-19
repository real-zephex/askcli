# ask

<p align="center"><b>AI agent for your terminal.</b></p>
<p align="center">Written in Go. Local state stays on your machine, but model requests still go to Gemini and optional integrations use their own services.</p>

<p align="center">
  <a href="./assets/demo-smooth.gif">
    <img src="./assets/demo-small.gif" alt="ask demo" width="900" />
  </a>
  <br/>
  <sub>Click to see full demo</sub>
</p>

A CLI for talking to LLMs in your terminal. Use one-shot mode for quick questions or REPL mode for interactive chat. Optional agent mode lets the AI run shell commands, edit files, make HTTP calls, manage local lists and memories, and send files over Telegram, with approval gates you control.

## Features

- **REPL chat** with slash commands for runtime config
- **One-shot mode** for quick questions, including piped stdin
- **Streaming markdown** rendering as responses arrive
- **Agent mode** with tool calling for shell, file ops, HTTP, clipboard, lists, mail, memory, and Telegram media tools
- **Approval gates** per risky action, with `--yolo` to auto-approve in trusted environments
- **Chat history** persisted in SQLite
- **Vector memory** store for long-term context, with explicit CRUD tools and CLI access
- **Named lists/todos** stored locally
- **Shell completions** for bash/zsh/fish
- **Custom system prompts** loaded from a file
- **Telegram background mode** that can process messages, voice notes, images, and documents
- **Remote client/server mode** for connecting to a local ask server over HTTP

## Requirements

- Go 1.25.8+
- `GEMINI_API_KEY` to talk to Gemini

Some features also need extra setup:

- `ASKCLI_SERVER_KEY` for remote server auth
- `ASKCLI_CLIENT_KEY` as a client-side fallback auth key
- `TELEGRAM_BOT_TOKEN` for Telegram background mode
- `GROQ_API_KEY` for Telegram voice-note transcription
- `AGENT_MAIL_API_KEY` and `INBOX_NAME` for the `mail` tool
- `ELEVEN_LABS_API_KEY` for `text_to_speech_file`
- Clipboard:
  - Linux Wayland: `wl-paste`/`wl-copy`
  - Linux X11: `xclip` or `xsel`
  - macOS: `pbcopy`/`pbpaste`
  - Windows: PowerShell `Get-Clipboard`/`Set-Clipboard`
- `PORT` for server mode, defaulting to `3000`

If you only use the local CLI, `GEMINI_API_KEY` is the only required variable.

## Quick Start

```bash
git clone <repo-url>
cd ask
go build -o ask
./ask "Your question here"
```

Or install globally:

```bash
sudo mv ask /usr/local/bin/
```

## Usage

**One-shot mode:**

```bash
ask "What is a goroutine?"
ask --model exp "Analyze this architecture"
cat main.go | ask "Explain this code"
```

**Chat mode:**

```bash
ask --chat
# or
ask chat
```

**Agent mode:**

```bash
ask --chat --agent
```

Auto-approve tool actions:

```bash
ask --chat --agent --yolo
```

## Model Aliases

Quick names for common models:

- `free` – `gemma-4-26b-a4b-it` (default)
- `cheap` – `gemini-3.1-flash-lite-preview`
- `exp` – `gemini-3-flash-preview`

You can also pass any full model name.

## Reasoning Control

Dial up the thinking time:

- `HIGH` – deep reasoning
- `MED` / `MEDIUM` / `MID`
- `LOW`
- `MIN` / `MINIMAL` – fast and lightweight

## Common Flags

```
--chat              Start REPL mode
--agent             Enable tool calling
--yolo              Auto-approve all actions
--stream            Stream markdown as it renders (default: on)
--system <file>     Load custom system prompt
--cache             Enable explicit Gemini context caching (system prompt + tools)
--cache-ttl <dur>   Explicit cache TTL (e.g. 30m, 2h). 0 uses API default
--model <alias>     Pick a model
--reason <level>    Set reasoning level
--clear             Nuke chat history on startup
--connect <url>     Connect to a remote ask server (e.g. http://host:3000)
--server-key <key>  API key for remote server authentication (overrides env vars)
--background        Run Telegram background mode plus the local HTTP server
```

## Remote Server

`ask` can run as a local HTTP server that remote clients connect to.

**Important:** `--background` starts both the Telegram bot and the HTTP server. There is no separate server-only flag.

**Server setup:**

1. Set the API key that clients must provide:
   ```bash
   export ASKCLI_SERVER_KEY="your-secret-key-here"
   export GEMINI_API_KEY="your-gemini-key"
   export TELEGRAM_BOT_TOKEN="your-telegram-bot-token"
   ```

2. Start background mode:
   ```bash
   ask --background=true
   ```

The server exposes:

- `/ask` — authenticated POST endpoint
- `/health` — unauthenticated health check

**Client usage:**

Connect to the remote server from another machine or terminal using `--connect`:

- **One-shot query:**
  ```bash
  ask --connect http://server:3000 --server-key YOUR_KEY "your question"
  ```

- **Interactive chat:**
  ```bash
  ask --connect http://server:3000 --server-key YOUR_KEY --chat
  ```

- **Using env vars on the client:**
  ```bash
  export ASKCLI_CLIENT_KEY="your-secret-key-here"
  ask --connect http://server:3000 --chat
  ```

**Notes:**

- `--server-key` overrides `ASKCLI_CLIENT_KEY` and `ASKCLI_SERVER_KEY`.
- The server validates the `x-askcli-api-key` header on each request.
- The server and Telegram bot share the same SQLite database and vector memory.
- Remote clients do not support streaming yet.
- Remote requests currently run with server-side auto-approval enabled, so tool calls are not blocked by local prompts on the server.

## Chat Mode (REPL)

Drop into an interactive session with slash commands:

```bash
ask --chat
```

**Available commands:**

- `/help` – show this list
- `/status` – show the active model and settings
- `/model <name>` – switch models on the fly
- `/reason <level>` – adjust reasoning (`HIGH`, `MED`, `LOW`, `MIN`)
- `/stream on|off` – toggle streaming output
- `/agent on|off` – enable or disable tool calling
- `/yolo on|off` – toggle auto-approval in agent mode
- `/cache on|off` – toggle explicit Gemini context caching
- `/cache-ttl <dur>` – set explicit cache TTL
- `/pwd` – print working directory
- `/cd <path>` – change directory for tool commands
- `/history [n]` – show last n messages
- `/clear` – wipe current conversation
- `/memories` – open the memory manager
- `/exit` or `/quit` – leave

## Memory (Vector Store)

Store facts locally and let the AI access them across chats. Useful for coding patterns, project context, or anything you want the agent to remember.

**Access:**

- CLI: `ask memories` (list), `ask memories manage` (interactive editor)
- Agent tools: `memory_view`, `memory_add`, `memory_update`, `memory_delete`

**Manager commands:**

- `l` / `list` – show all
- `d <n>` / `del <n>` – delete entry n
- `da` / `delall` – delete everything
- `q` / `quit` – exit manager

### How It Works

**Storage:** Chromem persistent DB in `~/db`.

**IDs:** Each memory uses a stable hash-based ID.

**Management:** Explicit only. Memories do not auto-inject into every prompt; you manage them via CLI or agent tools.

**Status:** Memory is read/write explicit only. There is no automatic context injection yet.

## Agent Tools

Enable with `--agent`. The AI can call these tools automatically, with approval unless `--yolo` is set.

**`run_shell_command`** – Execute shell commands

- Runs in the selected directory
- Returns stdout, stderr, exit code, and timing
- **Approval required** unless `--yolo`

**`read_file`** – Read file contents

- Supports `start_line` / `end_line`
- No approval needed

**`write_file`** – Edit files

- Exact string replacement (`old_str` → `new_str`)
- Shows a diff preview before confirming
- **Approval required** unless `--yolo`

**`clipboard`** – Read or write the system clipboard

- Read: no approval
- Write: **approval required** unless `--yolo`
- Linux: Wayland (`wl-clipboard`) or X11 (`xclip`/`xsel`) with a graphical session
- macOS: `pbcopy`/`pbpaste`
- Windows: PowerShell `Get-Clipboard`/`Set-Clipboard`

**`search_files`** – Find files by name pattern

- Glob-style pattern matching (e.g., `*.go`, `**/*.md`)
- Optional root directory and max results
- No approval needed

**`grep_files`** – Search file contents by regex

- Regex pattern search across files
- Optional root directory, include glob, and max results
- Skips large and binary files
- No approval needed

**`lists`** – Manage todos/lists

- Actions: `create_list`, `delete_list`, `get_lists`, `add_item`, `update_item`, `delete_item`, `get_items`
- Deletions need approval unless `--yolo`

**`http_request`** – Make HTTP calls

- Verbs: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
- GET: no approval
- Write ops: **approval required** unless `--yolo`

**`mail`** – Manage AgentMail inbox threads and messages

- Actions: `get_threads`, `get_thread`, `send_email`, `reply_to_message`, `forward_message`, `delete_thread`
- Requires `AGENT_MAIL_API_KEY` and `INBOX_NAME`
- Send/reply/forward/delete: **approval required** unless `--yolo`

**`memory_add`** – Store a new memory

- No approval needed

**`memory_update`** – Update an existing memory

- No approval needed

**`memory_delete`** – Delete a memory entry

- No approval needed

**`text_to_speech_file`** – Generate voice notes as MP3 files

- Converts plain text into an MP3 using ElevenLabs
- Output can be sent with `send_document_over_telegram`
- Requires `ELEVEN_LABS_API_KEY`

**`send_document_over_telegram`** – Send files over Telegram

- Sends documents, MP3s, voice notes, and similar files to Telegram

**`send_image_over_telegram`** – Send images over Telegram

- Sends image files directly to Telegram chat

## Telegram Integration

Run `ask` as a Telegram bot. Chat with the AI directly in Telegram with slash commands for config.

**Setup:**

1. Create a bot with BotFather on Telegram
2. Set env vars:
   ```bash
   export TELEGRAM_BOT_TOKEN="your_token_here"
   export GEMINI_API_KEY="your_gemini_key"
   ```
3. Start the bot:
   ```bash
   ask --background=true
   ```

**Shared Context:** The Telegram bot uses the same SQLite database and vector memory as the CLI, so chat history and memories persist across both interfaces.

**Available commands:**

- `/start` – welcome message
- `/help` or `/about` – show commands
- `/model <name>` – switch AI model
- `/reasoning <level>` – adjust reasoning

**Voice & File Features:**

- **Send voice notes:** The agent can generate voice notes as MP3 files and send them back over Telegram using `text_to_speech_file` and `send_document_over_telegram`.
- **Receive voice notes:** You can send voice notes to the bot, and it will transcribe them before responding.
- **Send images and documents:** The agent can send images and document files directly to your Telegram chat.
- **Reply context:** Replies to text, image, voice note, or document messages are passed to the agent with the replied-to content included.

Regular messages, voice notes, images, and documents are all processed by the bot, and responses are saved locally in SQLite.

## Shell Completions

Generate completions for your shell:

```bash
ask completion bash
ask completion zsh
ask completion fish
```

## Data Persistence

- **Chat history & lists (SQLite):** `~/.ask-go.db`
- **Vector memory (chromem):** `~/db/`

Chat history, lists, and memories are stored locally. Model requests still go to the configured provider.

## Important Notes

- **`--yolo` is dangerous.** It auto-approves shell commands, file writes, HTTP requests, clipboard writes, mail sends, and similar risky actions. Use it only in controlled environments.
- **Prompts are sent to external services.** Local state stays on your machine, but model requests and optional integrations may leave the machine depending on the features you use.

## License

MIT (see `LICENSE`)
test
