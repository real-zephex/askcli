package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxFileOutputLength = 8000
)

type readFileRequest struct {
	Path      string
	StartLine int
	EndLine   int
}

type readFileResult struct {
	Request      readFileRequest
	Content      string
	TotalLines   int
	StartLine    int
	EndLine      int
	Truncated    bool
	ExecutionErr string
}

func parseReadFileRequest(args map[string]any) (readFileRequest, error) {
	if args == nil {
		return readFileRequest{}, errors.New("function args missing")
	}

	pathValue, ok := args["path"]
	if !ok {
		return readFileRequest{}, errors.New("missing required argument: path")
	}

	path, ok := pathValue.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return readFileRequest{}, errors.New("argument 'path' must be a non-empty string")
	}

	req := readFileRequest{
		Path:      strings.TrimSpace(path),
		StartLine: 0,
		EndLine:   0,
	}

	if rawStart, ok := args["start_line"]; ok {
		start, err := parseInt(rawStart)
		if err != nil {
			return readFileRequest{}, errors.New("argument 'start_line' must be an integer")
		}
		if start < 1 {
			return readFileRequest{}, errors.New("argument 'start_line' must be >= 1")
		}
		req.StartLine = start
	}

	if rawEnd, ok := args["end_line"]; ok {
		end, err := parseInt(rawEnd)
		if err != nil {
			return readFileRequest{}, errors.New("argument 'end_line' must be an integer")
		}
		if end < 1 {
			return readFileRequest{}, errors.New("argument 'end_line' must be >= 1")
		}
		req.EndLine = end
	}

	if req.StartLine > 0 && req.EndLine > 0 && req.StartLine > req.EndLine {
		return readFileRequest{}, errors.New("start_line cannot be greater than end_line")
	}

	return req, nil
}

func executeReadFile(req readFileRequest) readFileResult {
	resolvedPath := req.Path
	if !filepath.IsAbs(req.Path) {
		cwd, err := os.Getwd()
		if err != nil {
			return readFileResult{
				Request:      req,
				ExecutionErr: fmt.Sprintf("failed to get working directory: %v", err),
			}
		}
		resolvedPath = filepath.Join(cwd, req.Path)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return readFileResult{
			Request:      req,
			ExecutionErr: fmt.Sprintf("failed to read file: %v", err),
		}
	}

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	startLine := 1
	endLine := totalLines

	if req.StartLine > 0 {
		startLine = req.StartLine
	}
	if req.EndLine > 0 {
		endLine = req.EndLine
	}

	if startLine > totalLines {
		return readFileResult{
			Request:      req,
			ExecutionErr: fmt.Sprintf("start_line %d exceeds total lines %d", startLine, totalLines),
		}
	}

	if endLine > totalLines {
		endLine = totalLines
	}

	var builder strings.Builder
	for i := startLine - 1; i < endLine; i++ {
		builder.WriteString(fmt.Sprintf("%d: %s\n", i+1, lines[i]))
	}

	content := builder.String()
	truncated := false

	if len(content) > maxFileOutputLength {
		runes := []rune(content)
		content = string(runes[:maxFileOutputLength]) + "\n...[truncated]"
		truncated = true
	}

	return readFileResult{
		Request:    req,
		Content:    content,
		TotalLines: totalLines,
		StartLine:  startLine,
		EndLine:    endLine,
		Truncated:  truncated,
	}
}

func (r readFileResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": r.ExecutionErr,
			},
		}
	}

	header := ""
	if r.StartLine > 1 || r.EndLine < r.TotalLines {
		header = fmt.Sprintf("Lines %d-%d of %d:\n", r.StartLine, r.EndLine, r.TotalLines)
	} else {
		header = fmt.Sprintf("Total lines: %d\n", r.TotalLines)
	}

	return map[string]any{
		"output": map[string]any{
			"path":        r.Request.Path,
			"content":     header + r.Content,
			"total_lines": r.TotalLines,
			"start_line":  r.StartLine,
			"end_line":    r.EndLine,
			"truncated":   r.Truncated,
		},
	}
}