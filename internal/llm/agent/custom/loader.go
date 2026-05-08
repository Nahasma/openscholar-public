package custom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
)

// Loader discovers and loads custom agent definitions from the filesystem.
type Loader struct {
	searchPaths []string
}

// NewLoader creates a Loader that searches .openscholar/agents/ and .openscholar/modes/
// under the given project directory.
func NewLoader(projectDir string) *Loader {
	paths := []string{
		filepath.Join(projectDir, config.DefaultDataDir(), "agents"),
		filepath.Join(projectDir, config.DefaultDataDir(), "modes"),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, config.DefaultDataDir(), "agents"),
			filepath.Join(home, config.DefaultDataDir(), "modes"),
		)
	}
	return &Loader{searchPaths: paths}
}

// LoadAll returns all valid custom agent definitions found in the search paths.
// Invalid definitions are silently skipped.
func (l *Loader) LoadAll() []*AgentConfig {
	var configs []*AgentConfig
	for _, dir := range l.searchPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // directory doesn't exist — skip
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			cfg, err := Parse(data)
			if err != nil {
				continue // skip invalid definitions
			}
			cfg.FilePath = path
			configs = append(configs, cfg)
		}
	}
	return configs
}

// Load returns a single custom agent definition by name, or an error if not found.
func (l *Loader) Load(name string) (*AgentConfig, error) {
	all := l.LoadAll()
	for _, cfg := range all {
		if strings.EqualFold(cfg.Name, name) {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("custom agent %q not found in %v", name, l.searchPaths)
}
