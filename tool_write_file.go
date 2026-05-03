package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type writeFileRequest struct {
	Path   string
	OldStr string
	NewStr string
	Reason string
}

type writeFileResult struct {
	Request       writeFileRequest
	MatchCount    int
	ModifiedLines string
	ExecutionErr  string
	UserDenied    bool
}

func parseWriteFileRequest(args map[string]any) (writeFileRequest, error) {
	if args == nil {
		return writeFileRequest{}, errors.New("function args missing")
	}

	path, err := requiredStringArg(args, "path")
	if err != nil {
		return writeFileRequest{}, err
	}

	oldStr, err := requiredStringArg(args, "old_str")
	if err != nil {
		return writeFileRequest{}, err
	}

	newStrValue, ok := args["new_str"]
	if !ok {
		return writeFileRequest{}, errors.New("missing required argument: new_str")
	}

	newStr, ok := newStrValue.(string)
	if !ok {
		return writeFileRequest{}, errors.New("argument 'new_str' must be a string")
	}

	reason := ""
	if rawReason, ok := args["reason"]; ok {
		if s, ok := rawReason.(string); ok {
			reason = strings.TrimSpace(s)
		}
	}

	return writeFileRequest{
		Path:   path,
		OldStr: oldStr,
		NewStr: newStr,
		Reason: reason,
	}, nil
}

func executeWriteFile(req writeFileRequest) writeFileResult {
	resolvedPath := req.Path
	if !filepath.IsAbs(req.Path) {
		cwd, err := os.Getwd()
		if err != nil {
			return writeFileResult{
				Request:      req,
				ExecutionErr: fmt.Sprintf("failed to get working directory: %v", err),
			}
		}
		resolvedPath = filepath.Join(cwd, req.Path)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return writeFileResult{
			Request:      req,
			ExecutionErr: fmt.Sprintf("failed to read file: %v", err),
		}
	}

	content := string(data)
	matchCount := strings.Count(content, req.OldStr)

	if matchCount == 0 {
		return writeFileResult{
			Request:      req,
			MatchCount:   0,
			ExecutionErr: "old_str not found in file",
		}
	}

	if matchCount > 1 {
		return writeFileResult{
			Request:      req,
			MatchCount:   matchCount,
			ExecutionErr: fmt.Sprintf("old_str appears %d times in file. Please provide a more specific old_str that matches exactly once", matchCount),
		}
	}

	newContent := strings.Replace(content, req.OldStr, req.NewStr, 1)

	err = os.WriteFile(resolvedPath, []byte(newContent), 0644)
	if err != nil {
		return writeFileResult{
			Request:      req,
			MatchCount:   matchCount,
			ExecutionErr: fmt.Sprintf("failed to write file: %v", err),
		}
	}

	oldLines := strings.Split(req.OldStr, "\n")
	startLineNum := strings.Count(content[:strings.Index(content, req.OldStr)], "\n") + 1
	endLineNum := startLineNum + len(oldLines) - 1

	modifiedLines := fmt.Sprintf("lines %d-%d", startLineNum, endLineNum)
	if startLineNum == endLineNum {
		modifiedLines = fmt.Sprintf("line %d", startLineNum)
	}

	return writeFileResult{
		Request:       req,
		MatchCount:    matchCount,
		ModifiedLines: modifiedLines,
	}
}

func (r writeFileResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		payload := map[string]any{
			"path":        r.Request.Path,
			"match_count": r.MatchCount,
		}
		return map[string]any{
			"error":  map[string]any{"message": r.ExecutionErr},
			"output": payload,
		}
	}

	return map[string]any{
		"output": map[string]any{
			"path":           r.Request.Path,
			"modified_lines": r.ModifiedLines,
			"success":        true,
		},
	}
}

func generateDiff(oldStr, newStr string) string {
	var diff strings.Builder

	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	for _, line := range oldLines {
		diff.WriteString(fmt.Sprintf("- %s\n", line))
	}

	for _, line := range newLines {
		diff.WriteString(fmt.Sprintf("+ %s\n", line))
	}

	return diff.String()
}