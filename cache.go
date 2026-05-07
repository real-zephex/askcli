package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

type CacheSettings struct {
	Enabled bool
	TTL     time.Duration
}

var (
	cachedContentMu    sync.Mutex
	cachedContentByKey = map[string]*genai.CachedContent{}
)

func applyExplicitCache(ctx context.Context, client *genai.Client, model string, config *genai.GenerateContentConfig, settings CacheSettings) {
	if !settings.Enabled || client == nil || config == nil {
		return
	}

	cache, err := ensureCachedContent(ctx, client, model, config, settings)
	if err != nil {
		log.Printf("explicit cache init failed: %v", err)
		return
	}
	if cache == nil || cache.Name == "" {
		return
	}

	config.CachedContent = cache.Name
	config.SystemInstruction = nil
	config.Tools = nil
	config.ToolConfig = nil
}

func ensureCachedContent(ctx context.Context, client *genai.Client, model string, config *genai.GenerateContentConfig, settings CacheSettings) (*genai.CachedContent, error) {
	if config.SystemInstruction == nil && len(config.Tools) == 0 && config.ToolConfig == nil {
		return nil, nil
	}

	key := buildCacheKey(model, config)
	cachedContentMu.Lock()
	if existing, ok := cachedContentByKey[key]; ok {
		cachedContentMu.Unlock()
		return existing, nil
	}
	cachedContentMu.Unlock()

	createConfig := &genai.CreateCachedContentConfig{
		SystemInstruction: config.SystemInstruction,
		Tools:             config.Tools,
		ToolConfig:        config.ToolConfig,
		DisplayName:       cacheDisplayName(key),
	}
	if settings.TTL > 0 {
		createConfig.TTL = settings.TTL
	}

	cached, err := client.Caches.Create(ctx, model, createConfig)
	if err != nil {
		return nil, err
	}

	cachedContentMu.Lock()
	cachedContentByKey[key] = cached
	cachedContentMu.Unlock()
	return cached, nil
}

func buildCacheKey(model string, config *genai.GenerateContentConfig) string {
	systemText := systemInstructionText(config.SystemInstruction)
	toolSig := toolsSignature(config.Tools)
	toolConfigSig := toolConfigSignature(config.ToolConfig)
	base := fmt.Sprintf("model=%s|system=%s|tools=%s|toolcfg=%s", model, systemText, toolSig, toolConfigSig)
	return hash(base)
}

func cacheDisplayName(key string) string {
	shortKey := key
	if len(shortKey) > 12 {
		shortKey = shortKey[:12]
	}
	return "askcli-" + shortKey
}

func systemInstructionText(content *genai.Content) string {
	if content == nil || len(content.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func toolsSignature(tools []*genai.Tool) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if tool.GoogleSearch != nil {
			b.WriteString("google_search;")
		}
		if len(tool.FunctionDeclarations) > 0 {
			b.WriteString("functions:")
			for _, decl := range tool.FunctionDeclarations {
				if decl == nil {
					continue
				}
				b.WriteString(decl.Name)
				b.WriteByte(',')
			}
			b.WriteString(";")
		}
	}
	return b.String()
}

func toolConfigSignature(cfg *genai.ToolConfig) string {
	if cfg == nil || cfg.IncludeServerSideToolInvocations == nil {
		return ""
	}
	return fmt.Sprintf("include_server_side=%t", *cfg.IncludeServerSideToolInvocations)
}

func isCachedContentNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "cachedcontent not found") ||
		strings.Contains(errStr, "permission_denied") ||
		strings.Contains(errStr, "permission denied")
}

func invalidateExplicitCache(model string, config *genai.GenerateContentConfig) {
	key := buildCacheKey(model, config)
	cachedContentMu.Lock()
	delete(cachedContentByKey, key)
	cachedContentMu.Unlock()
}
