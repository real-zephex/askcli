package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/philippgille/chromem-go"
)

var memoryCollection *chromem.Collection

func getDb() (*chromem.Collection, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error while fetching the home directory of the user: %w", err)
	}

	joinedPaths := filepath.Join(homeDir, "db")

	db, err := chromem.NewPersistentDB(joinedPaths, false)
	if err != nil {
		return nil, fmt.Errorf("an error occured while trying to connect to vector database: %w", err)
	}

	c, err := db.GetOrCreateCollection("user-memory", nil, chromemCustomGenerator)
	if err != nil {
		return nil, fmt.Errorf("an error occured while trying to initialize the vector database: %w", err)
	}

	return c, nil
}

// Store documents in the vector database.
func storeDocuments(ctx context.Context, collection *chromem.Collection, messages []chromem.Document) error {
	if collection == nil {
		return fmt.Errorf("vector collection is nil")
	}
	if len(messages) == 0 {
		return nil
	}

	if err := collection.AddDocuments(ctx, messages, runtime.NumCPU()); err != nil {
		return fmt.Errorf("an error occured while trying to store memories: %w", err)
	}

	return nil
}

// Get documents from the vector database.
func queryDocuments(ctx context.Context, collection *chromem.Collection, query string, limit int) ([]chromem.Result, error) {
	if collection == nil {
		return nil, fmt.Errorf("vector collection is nil")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}

	res, err := collection.Query(ctx, query, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("an error occured while trying to query the vector database: %w", err)
	}

	return res, nil
}

func recallMemories(ctx context.Context, query string, limit int) ([]string, error) {
	if memoryCollection == nil {
		return nil, nil
	}

	results, err := queryDocuments(ctx, memoryCollection, query, limit)
	if err != nil {
		return nil, err
	}

	memories := make([]string, 0, len(results))
	seen := map[string]struct{}{}
	for _, result := range results {
		text := strings.TrimSpace(result.Content)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		memories = append(memories, text)
	}

	return memories, nil
}

func injectMemoryContext(ctx context.Context, query string) string {
	memories, err := recallMemories(ctx, query, 5)
	if err != nil || len(memories) == 0 {
		return query
	}

	var b strings.Builder
	b.WriteString("Use these long-term memories only if relevant:\n")
	for i, m := range memories {
		fmt.Fprintf(&b, "%d. %s\n", i+1, m)
	}
	b.WriteString("\nUser query:\n")
	b.WriteString(query)
	return b.String()
}
