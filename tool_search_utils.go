package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultSearchMaxResults = 50
	maxSearchMaxResults     = 200
	maxGrepFileSizeBytes    = 1024 * 1024
	maxGrepLineLength       = 256 * 1024
	maxGrepLineOutput       = 4000
)

var errSearchLimitReached = errors.New("search limit reached")

func resolveRootPath(raw any) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if raw == nil {
		return cwd, nil
	}

	root, ok := raw.(string)
	if !ok || strings.TrimSpace(root) == "" {
		return cwd, nil
	}

	root = strings.TrimSpace(root)
	if filepath.IsAbs(root) {
		return filepath.Clean(root), nil
	}

	return filepath.Clean(filepath.Join(cwd, root)), nil
}

func clampMaxResults(raw any) (int, error) {
	if raw == nil {
		return defaultSearchMaxResults, nil
	}

	value, err := parseInt(raw)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, errors.New("max_results must be >= 1")
	}
	if value > maxSearchMaxResults {
		return maxSearchMaxResults, nil
	}
	return value, nil
}

func matchPattern(pattern string, relPath string, baseName string) bool {
	if strings.Contains(pattern, "/") || strings.Contains(pattern, "\\") {
		pattern = filepath.ToSlash(pattern)
		candidate := filepath.ToSlash(relPath)
		matched, err := filepath.Match(pattern, candidate)
		return err == nil && matched
	}

	matched, err := filepath.Match(pattern, baseName)
	return err == nil && matched
}

func isBinaryFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	count, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}

	return bytes.Contains(buffer[:count], []byte{0}), nil
}
