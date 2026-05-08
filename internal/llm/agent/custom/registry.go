package custom

import (
	"sort"
	"strings"
)

// BuiltInAgentNames are the task agent types implemented by the runtime.
var BuiltInAgentNames = []string{"general", "explore", "experiment", "leader", "plan", "verify", "coordinator"}

// Registry exposes built-in and custom agent definitions for runtime lookup.
type Registry struct {
	loader *Loader
}

// NewRegistry creates a registry backed by the project/user agent loader.
func NewRegistry(projectDir string) *Registry {
	return &Registry{loader: NewLoader(projectDir)}
}

// Load returns a custom agent definition by name.
func (r *Registry) Load(name string) (*AgentConfig, bool) {
	if r == nil || r.loader == nil {
		return nil, false
	}
	cfg, err := r.loader.Load(strings.TrimSpace(name))
	return cfg, err == nil
}

// ListCustom returns custom agents sorted by name.
func (r *Registry) ListCustom() []*AgentConfig {
	if r == nil || r.loader == nil {
		return nil
	}
	configs := r.loader.LoadAll()
	sort.Slice(configs, func(i, j int) bool {
		return strings.ToLower(configs[i].Name) < strings.ToLower(configs[j].Name)
	})
	return configs
}

// IsBuiltIn reports whether name is handled by the built-in task runtime.
func IsBuiltIn(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, builtIn := range BuiltInAgentNames {
		if name == builtIn {
			return true
		}
	}
	return false
}
