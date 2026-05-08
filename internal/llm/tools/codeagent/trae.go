package codeagent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type traeProvider struct{}

func (p *traeProvider) Name() string { return "trae" }

func (p *traeProvider) Available() bool {
	_, err := exec.LookPath("trae-cli")
	return err == nil
}

func (p *traeProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"run", req.Prompt}

	cmd := exec.CommandContext(ctx, "trae-cli", args...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = nil
	cmd.Env = appendHeadlessEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("trae: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("trae: %w", err)
	}

	return &CodeResponse{
		Result:  stdout.String(),
		Success: true,
	}, nil
}
