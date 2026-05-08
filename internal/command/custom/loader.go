package custom

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
)

// LoadAll scans command directories and registers custom commands.
// It scans both global (~/.openscholar/commands/) and local (.openscholar/commands/).
func LoadAll(registry *command.Registry) {
	// Global commands (~/.openscholar/commands/)
	home, err := os.UserHomeDir()
	if err == nil {
		loadFromDir(registry, filepath.Join(home, config.DefaultDataDir(), "commands"))
	}

	// Local (project) commands
	loadFromDir(registry, config.DataPath("commands"))
}

func loadFromDir(registry *command.Registry, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		fm, body, err := ParseFrontmatter(string(data))
		if err != nil {
			continue // skip commands with invalid frontmatter
		}

		name := fm.Name
		if name == "" {
			// Derive name from filename
			name = strings.TrimSuffix(entry.Name(), ".md")
		}

		desc := fm.Description
		if desc == "" {
			desc = "Custom command: " + name
		}

		// Skip commands explicitly marked as non-user-invocable
		if fm.UserInvocable != nil && !*fm.UserInvocable {
			continue
		}

		registry.Register(&CustomCommand{
			name:         name,
			description:  desc,
			body:         body,
			frontmatter:  fm,
		})
	}
}
