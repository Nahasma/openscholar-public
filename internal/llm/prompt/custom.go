package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/config"
)

const promptFileName = "prompt.md"

// LoadCustomPrompts loads user-defined prompt files from global and project directories.
// Global (~/.openscholar/prompt.md) is loaded first, then project-level (.openscholar/prompt.md).
// Returns the combined content, or empty string if no files exist.
func LoadCustomPrompts() string {
	var parts []string

	// 1. Global level: ~/.openscholar/prompt.md
	if home, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(home, ".openscholar", promptFileName)
		if content := readFileIfExists(globalPath); content != "" {
			parts = append(parts, content)
		}
	}

	// 2. Project level: <dataDir>/prompt.md (higher priority, appended last)
	projectPath := config.DataPath(promptFileName)
	if content := readFileIfExists(projectPath); content != "" {
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n")
}

// readFileIfExists reads a file and returns its trimmed content, or empty string if not found.
func readFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
