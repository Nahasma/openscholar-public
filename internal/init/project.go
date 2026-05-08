package initwizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

const projectPromptFileName = "prompt.md"

func ProjectPromptPath(workingDir string) string {
	return filepath.Join(workingDir, ".openscholar", projectPromptFileName)
}

func EnsureProjectPrompt(workingDir string) (created bool, path string, preview string, err error) {
	path = ProjectPromptPath(workingDir)
	if data, readErr := os.ReadFile(path); readErr == nil {
		return false, path, previewText(string(data), 12), nil
	} else if !os.IsNotExist(readErr) {
		return false, path, "", fmt.Errorf("read project prompt: %w", readErr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, path, "", fmt.Errorf("create prompt directory: %w", err)
	}

	content := defaultProjectPrompt(filepath.Base(workingDir))
	if err := fileop.WriteFileAtomic(path, []byte(content), 0o644); err != nil {
		return false, path, "", fmt.Errorf("write project prompt: %w", err)
	}
	return true, path, previewText(content, 12), nil
}

func defaultProjectPrompt(projectName string) string {
	if strings.TrimSpace(projectName) == "" || projectName == "." || projectName == string(filepath.Separator) {
		projectName = "project"
	}
	return fmt.Sprintf(`# Project Prompt

Project: %s

## Objectives
- Define the primary research goal.
- Track constraints and deliverables.

## Context
- Domain:
- Audience:
- Deadline:

## Working Rules
- Keep outputs reproducible.
- Record assumptions before major changes.
- Prefer incremental validation over large rewrites.
`, projectName)
}

func previewText(s string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
}
