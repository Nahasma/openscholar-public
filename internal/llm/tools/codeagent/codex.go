package codeagent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type codexProvider struct{}

func (p *codexProvider) Name() string { return "codex" }

func (p *codexProvider) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (p *codexProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"exec", req.Prompt}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = nil
	cmd.Env = appendHeadlessEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("codex: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("codex: %w", err)
	}

	return &CodeResponse{
		Result:  stdout.String(),
		Success: true,
	}, nil
}
