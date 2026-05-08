package codeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type geminiProvider struct{}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) Available() bool {
	_, err := exec.LookPath("gemini")
	return err == nil
}

func (p *geminiProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
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

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = nil
	cmd.Env = appendHeadlessEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("gemini: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("gemini: %w", err)
	}

	// Parse JSON output: {"response": "..."}
	var raw struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return &CodeResponse{
			Result:  stdout.String(),
			Success: true,
		}, nil
	}

	return &CodeResponse{
		Result:  raw.Response,
		Success: true,
	}, nil
}
