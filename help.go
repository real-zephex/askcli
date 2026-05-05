package main

import (
	"fmt"
	"os"
)

func helpMenu() {
	fmt.Println(`
╔════════════════════════════════════════╗
║           ASK - CLI AI Assistant       ║
╚════════════════════════════════════════╝

USAGE:
  ask [flags] <prompt>
  ask chat
  ask memories
  ask memories manage
  ask --chat
  ask completion [bash|zsh|fish]
  ask --help

EXAMPLES:
  ask "what is a goroutine"
  ask "explain interfaces in go"
  ask --model exp "analyze this architecture deeply"
  ask --reason HIGH "design a scalable queue worker system"
  ask chat
  ask --chat --agent
  ask --chat --agent --yolo
  ask --system ./system.txt "review this architecture"
  ask memories
  ask memories manage
  ask completion bash
  cat main.go | ask "explain this code"
  tail -n 50 app.log | ask --model cheap "summarize errors"

MODEL ALIASES:
  free         gemma-4-26b-a4b-it (default)
  cheap        gemini-3.1-flash-lite-preview
  exp          gemini-3-flash-preview

REASONING LEVELS:
  MIN          minimal reasoning (default)
  LOW          light reasoning
  MED          medium reasoning
  HIGH         high reasoning effort

FLAGS:
  --help        Show this help menu
  --version     Show current version
  --model       Model name or alias: free|cheap|exp
  --reason      Reasoning effort: HIGH|MED|LOW|MIN
  --clear       Clear local conversation history database
  --chat        Start interactive chat (REPL) mode
  --stream      Stream incremental rendered markdown updates (default: true)
  --agent       Enable agent mode in chat (model can request shell commands)
  --yolo        Auto-approve shell commands in agent mode (dangerous)
  --system      Path to a file containing system prompt text
  --cache       Enable explicit Gemini context caching (system prompt + tools)
  --cache-ttl   TTL for explicit cache (e.g. 30m, 2h). 0 uses API default

CHAT TIP:
  Use /help inside chat mode to see all slash commands.
    `)
	os.Exit(0)
}
