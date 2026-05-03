package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var runtimeSystemPrompt string

func loadSystemPromptFromFile(path string) string {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		log.Fatalf("failed to read system prompt file %q: %v", cleanPath, err)
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		log.Fatalf("system prompt file %q is empty", cleanPath)
	}

	return prompt
}

// function to ask user's approval to write to clipboard
func askForClipboardApproval() bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Write to clipboard? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(line))
		switch v {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		}
	}
}

// check user's approval for editing file
func askForEditApproval() bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Apply this edit? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(line))
		switch v {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		}
	}
}

// check user's approval for running bash commands
func askForCommandApproval() bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Approve command? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(line))
		switch v {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		}
	}
}

func fileExists(path string) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		return false, "the file does not exist"
	}

	return !info.IsDir(), "file exists"
}
