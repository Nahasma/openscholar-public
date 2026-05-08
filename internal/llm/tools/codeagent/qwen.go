package codeagent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type qwenProvider struct{}

func (p *qwenProvider) Name() string { return "qwen-code" }

func (p *qwenProvider) Available() bool {
	_, err := exec.LookPath("qwen-code")
	return err == nil
}

func (p *qwenProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-p", req.Prompt}

	cmd := exec.CommandContext(ctx, "qwen-code", args...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = nil
	cmd.Env = appendHeadlessEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("qwen-code: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("qwen-code: %w", err)
	}

	return &CodeResponse{
		Result:  stdout.String(),
		Success: true,
	}, nil
}
