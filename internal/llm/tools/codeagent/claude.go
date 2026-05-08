package codeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type claudeProvider struct{}

func (p *claudeProvider) Name() string { return "claude" }

func (p *claudeProvider) Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (p *claudeProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-p", req.Prompt,
		"--output-format", "json",
	}

	// Resume existing session
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}

	// Max turns
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
	}

	// Additional system prompt appended after the default.
	if req.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.AppendSystemPrompt)
	}

	// Bare mode: suppress interactive UI chrome.
	if req.Bare {
		args = append(args, "--bare")
	}

	// Restrict available tools.
	if req.AllowedTools != "" {
		args = append(args, "--allowedTools", req.AllowedTools)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = nil
	cmd.Env = appendHeadlessEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	runErr := cmd.Run()
	endedAt := time.Now()

	if runErr != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("claude: %w\nstderr: %s", runErr, stderrStr)
		}
		return nil, fmt.Errorf("claude: %w", runErr)
	}

	// Parse JSON output: {"result": "...", "session_id": "..."}
	var raw struct {
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		// If JSON parsing fails, return raw output
		return &CodeResponse{
			Result:     stdout.String(),
			Success:    true,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
			DurationMs: endedAt.Sub(startedAt).Milliseconds(),
			Provider:   p.Name(),
			ExitReason: "success",
		}, nil
	}

	return &CodeResponse{
		Result:     raw.Result,
		SessionID:  raw.SessionID,
		Success:    true,
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		DurationMs: endedAt.Sub(startedAt).Milliseconds(),
		Provider:   p.Name(),
		ExitReason: "success",
	}, nil
}
