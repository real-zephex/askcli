package main

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

type searchFilesRequest struct {
	Pattern    string
	Root       string
	MaxResults int
}

type searchFilesResult struct {
	Request      searchFilesRequest
	Files        []string
	Truncated    bool
	ExecutionErr string
}

func parseSearchFilesRequest(args map[string]any) (searchFilesRequest, error) {
	if args == nil {
		return searchFilesRequest{}, errors.New("function args missing")
	}

	pattern, err := requiredStringArg(args, "pattern")
	if err != nil {
		return searchFilesRequest{}, err
	}

	root, err := resolveRootPath(args["root"])
	if err != nil {
		return searchFilesRequest{}, err
	}

	maxResults, err := clampMaxResults(args["max_results"])
	if err != nil {
		return searchFilesRequest{}, errors.New("argument 'max_results' must be an integer >= 1")
	}

	return searchFilesRequest{
		Pattern:    pattern,
		Root:       root,
		MaxResults: maxResults,
	}, nil
}

func executeSearchFiles(req searchFilesRequest) searchFilesResult {
	result := searchFilesResult{Request: req}

	err := filepath.WalkDir(req.Root, func(path string, entry fs.DirEntry, err error) error {
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

		if matchPattern(req.Pattern, relPath, entry.Name()) {
			result.Files = append(result.Files, relPath)
			if len(result.Files) >= req.MaxResults {
				result.Truncated = true
				return errSearchLimitReached
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, errSearchLimitReached) {
		result.ExecutionErr = fmt.Sprintf("search failed: %v", err)
		return result
	}

	return result
}

func (r searchFilesResult) toToolResponse() map[string]any {
	if r.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": r.ExecutionErr,
			},
		}
	}

	output := map[string]any{
		"pattern": r.Request.Pattern,
		"root":    r.Request.Root,
		"count":   len(r.Files),
		"files":   r.Files,
	}

	if r.Truncated {
		output["truncated"] = true
		output["message"] = fmt.Sprintf("Results truncated at %d files", r.Request.MaxResults)
	}

	return map[string]any{"output": output}
}
