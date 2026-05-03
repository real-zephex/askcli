package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultShellTimeout  = 30
	minimumShellTimeout  = 1
	maximumShellTimeout  = 180
	maxShellOutputLength = 8000
)

type shellCommandRequest struct {
	Command          string
	WorkingDirectory string
	TimeoutSeconds   int
	Reason           string
}

type shellCommandResult struct {
	Request      shellCommandRequest
	Stdout       string
	Stderr       string
	ExitCode     int
	Duration     time.Duration
	TimedOut     bool
	ExecutionErr string
	StdoutCut    bool
	StderrCut    bool
}

func parseShellCommandRequest(args map[string]any) (shellCommandRequest, error) {
	if args == nil {
		return shellCommandRequest{}, errors.New("function args missing")
	}

	cmdValue, ok := args["command"]
	if !ok {
		return shellCommandRequest{}, errors.New("missing required argument: command")
	}

	command, ok := cmdValue.(string)
	if !ok || strings.TrimSpace(command) == "" {
		return shellCommandRequest{}, errors.New("argument 'command' must be a non-empty string")
	}

	wd, err := resolveWorkingDirectory(args["working_directory"])
	if err != nil {
		return shellCommandRequest{}, err
	}

	timeoutSeconds := defaultShellTimeout
	if rawTimeout, ok := args["timeout_seconds"]; ok {
		t, err := parseInt(rawTimeout)
		if err != nil {
			return shellCommandRequest{}, errors.New("argument 'timeout_seconds' must be an integer")
		}
		if t < minimumShellTimeout {
			t = minimumShellTimeout
		}
		if t > maximumShellTimeout {
			t = maximumShellTimeout
		}
		timeoutSeconds = t
	}

	reason := ""
	if rawReason, ok := args["reason"]; ok {
		if s, ok := rawReason.(string); ok {
			reason = strings.TrimSpace(s)
		}
	}

	return shellCommandRequest{
		Command:          strings.TrimSpace(command),
		WorkingDirectory: wd,
		TimeoutSeconds:   timeoutSeconds,
		Reason:           reason,
	}, nil
}

func resolveWorkingDirectory(raw any) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if raw == nil {
		return cwd, nil
	}

	wd, ok := raw.(string)
	if !ok || strings.TrimSpace(wd) == "" {
		return cwd, nil
	}

	if filepath.IsAbs(wd) {
		return filepath.Clean(wd), nil
	}

	return filepath.Clean(filepath.Join(cwd, wd)), nil
}

func parseInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		parsed, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, errors.New("unsupported number type")
	}
}

func executeShellCommand(req shellCommandRequest) shellCommandResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", req.Command)
	cmd.Dir = req.WorkingDirectory

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	execErr := ""

	if err != nil {
		execErr = err.Error()
		exitCode = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
	}

	trimmedStdout, stdoutCut := truncateOutput(stdout.String(), maxShellOutputLength)
	trimmedStderr, stderrCut := truncateOutput(stderr.String(), maxShellOutputLength)

	return shellCommandResult{
		Request:      req,
		Stdout:       trimmedStdout,
		Stderr:       trimmedStderr,
		ExitCode:     exitCode,
		Duration:     duration,
		TimedOut:     timedOut,
		ExecutionErr: execErr,
		StdoutCut:    stdoutCut,
		StderrCut:    stderrCut,
	}
}

func truncateOutput(s string, max int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	return string(runes[:max]) + "\n...[truncated]", true
}

func (r shellCommandResult) toToolResponse() map[string]any {
	payload := map[string]any{
		"command":           r.Request.Command,
		"working_directory": r.Request.WorkingDirectory,
		"timeout_seconds":   r.Request.TimeoutSeconds,
		"exit_code":         r.ExitCode,
		"duration_ms":       r.Duration.Milliseconds(),
		"stdout":            r.Stdout,
		"stderr":            r.Stderr,
		"stdout_truncated":  r.StdoutCut,
		"stderr_truncated":  r.StderrCut,
		"timed_out":         r.TimedOut,
	}

	if r.ExecutionErr == "" && !r.TimedOut && r.ExitCode == 0 {
		return map[string]any{"output": payload}
	}

	errorPayload := map[string]any{"message": r.ExecutionErr}
	if r.TimedOut {
		errorPayload["message"] = "command timed out"
	}

	return map[string]any{
		"output": payload,
		"error":  errorPayload,
	}
}