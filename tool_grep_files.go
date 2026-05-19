package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type grepFilesRequest struct {
	Pattern    string
	Root       string
	Include    string
	MaxResults int
}

type grepMatch struct {
	File       string
	LineNumber int
	Content    string
}

type grepFilesResult struct {
	Request      grepFilesRequest
	Matches      []grepMatch
	Truncated    bool
	ExecutionErr string
}

func parseGrepFilesRequest(args map[string]any) (grepFilesRequest, error) {
	if args == nil {
		return grepFilesRequest{}, errors.New("function args missing")
	}

	pattern, err := requiredStringArg(args, "pattern")
	if err != nil {
		return grepFilesRequest{}, err
	}

	if _, err := regexp.Compile(pattern); err != nil {
		return grepFilesRequest{}, errors.New("argument 'pattern' must be a valid regex")
	}

	root, err := resolveRootPath(args["root"])
	if err != nil {
		return grepFilesRequest{}, err
	}

	include := ""
	if rawInclude, ok := args["include"]; ok {
		value, ok := rawInclude.(string)
		if !ok {
			return grepFilesRequest{}, errors.New("argument 'include' must be a string")
		}
		include = strings.TrimSpace(value)
	}

	maxResults, err := clampMaxResults(args["max_results"])
	if err != nil {
		return grepFilesRequest{}, errors.New("argument 'max_results' must be an integer >= 1")
	}

	return grepFilesRequest{
		Pattern:    pattern,
		Root:       root,
		Include:    include,
		MaxResults: maxResults,
	}, nil
}

func executeGrepFiles(req grepFilesRequest) grepFilesResult {
	result := grepFilesResult{Request: req}

	matcher, err := regexp.Compile(req.Pattern)
	if err != nil {
		result.ExecutionErr = fmt.Sprintf("invalid pattern: %v", err)
		return result
	}

	err = filepath.WalkDir(req.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(req.Root, path)
		if relErr != nil {
			relPath = path
		}

		if req.Include != "" {
			if !matchPattern(req.Include, relPath, entry.Name()) {
				return nil
			}
		}

		info, statErr := entry.Info()
		if statErr == nil && info.Size() > maxGrepFileSizeBytes {
			return nil
		}

		isBinary, binErr := isBinaryFile(path)
		if binErr != nil || isBinary {
			return nil
		}

		file, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, maxGrepLineLength)
		lineNumber := 0

		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			if matcher.MatchString(line) {
				content := line
				if len(content) > maxGrepLineOutput {
					runes := []rune(content)
					if len(runes) > maxGrepLineOutput {
						content = string(runes[:maxGrepLineOutput]) + "..."
					}
				}

				result.Matches = append(result.Matches, grepMatch{
					File:       relPath,
					LineNumber: lineNumber,
					Content:    content,
				})
				if len(result.Matches) >= req.MaxResults {
					result.Truncated = true
					return errSearchLimitReached
				}
			}
		}

		if scanErr := scanner.Err(); scanErr != nil {
			if errors.Is(scanErr, bufio.ErrTooLong) {
				return nil
			}
			return scanErr
		}

		return nil
	})

	if err != nil && !errors.Is(err, errSearchLimitReached) {
		result.ExecutionErr = fmt.Sprintf("search failed: %v", err)
		return result
	}

	return result
}

func (r grepFilesResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": r.ExecutionErr,
			},
		}
	}

	items := make([]map[string]any, 0, len(r.Matches))
	for _, match := range r.Matches {
		items = append(items, map[string]any{
			"file":        match.File,
			"line_number": match.LineNumber,
			"content":     match.Content,
		})
	}

	output := map[string]any{
		"pattern": r.Request.Pattern,
		"root":    r.Request.Root,
		"count":   len(r.Matches),
		"matches": items,
	}

	if r.Request.Include != "" {
		output["include"] = r.Request.Include
	}

	if r.Truncated {
		output["truncated"] = true
		output["message"] = fmt.Sprintf("Results truncated at %d matches", r.Request.MaxResults)
	}

	return map[string]any{"output": output}
}
