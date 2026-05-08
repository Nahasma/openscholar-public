package config

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const helperTimeout = 5 * time.Second

// ExecuteKeyHelper runs an external command to retrieve an API key.
// Returns the trimmed output. Fails on timeout, empty output, or exit error.
func ExecuteKeyHelper(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, helperTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("key helper command timed out after %s: %s", helperTimeout, command)
		}
		return "", fmt.Errorf("key helper command failed: %w", err)
	}

	key := strings.TrimSpace(string(output))
	if key == "" {
		return "", fmt.Errorf("key helper command returned empty output: %s", command)
	}

	return key, nil
}
