package codeagent

import "fmt"

// priorityOrder defines the fallback order when no provider is specified.
var priorityOrder = []string{"claude", "gemini", "codex", "qwen-code", "trae"}

// Registry manages available CodeAgentProvider implementations.
type Registry struct {
	providers    map[string]CodeAgentProvider
	mcpProviders map[string]CodeAgentProvider // MCP-based providers (higher priority)
	preferred    string
	configs      map[string]ProviderConfig
}

// NewEmptyRegistry creates a registry without any built-in providers.
// Useful for testing with mock providers.
func NewEmptyRegistry() *Registry {
	return &Registry{
		providers:    make(map[string]CodeAgentProvider),
		mcpProviders: make(map[string]CodeAgentProvider),
		configs:      make(map[string]ProviderConfig),
	}
}

// NewRegistry creates a registry with all built-in providers.
func NewRegistry() *Registry {
	r := &Registry{
		providers:    make(map[string]CodeAgentProvider),
		mcpProviders: make(map[string]CodeAgentProvider),
		configs:      make(map[string]ProviderConfig),
	}
	r.Register(&claudeProvider{})
	r.Register(&geminiProvider{})
	r.Register(&codexProvider{})
	r.Register(&qwenProvider{})
	r.Register(&traeProvider{})
	return r
}

// Register adds a provider to the registry.
func (r *Registry) Register(p CodeAgentProvider) {
	r.providers[p.Name()] = p
}

// SetPreferred sets the user's preferred provider.
func (r *Registry) SetPreferred(name string) {
	r.preferred = name
}

// SetProviderConfig stores per-provider config (allowed_tools, max_turns, etc.).
func (r *Registry) SetProviderConfig(name string, cfg ProviderConfig) {
	r.configs[name] = cfg
}

// GetConfig returns the config for a provider, if any.
func (r *Registry) GetConfig(name string) (ProviderConfig, bool) {
	cfg, ok := r.configs[name]
	return cfg, ok
}

// RegisterMCPProviders registers MCP-based providers from connected MCP servers.
// MCP providers take priority over Bash subprocess providers.
func (r *Registry) RegisterMCPProviders(caller MCPCaller) {
	if caller == nil {
		return
	}
	for _, serverName := range caller.ServerNames() {
		providerName := mapServerToProvider(serverName)
		r.mcpProviders[providerName] = &mcpProvider{
			name:       providerName,
			serverName: serverName,
			caller:     caller,
		}
	}
}

// Get returns a specific provider by name, or falls back to preferred, then priority order.
// MCP providers are checked before Bash subprocess providers.
func (r *Registry) Get(name string) (CodeAgentProvider, error) {
	if name != "" {
		// Check MCP first, then Bash
		if p, ok := r.mcpProviders[name]; ok && p.Available() {
			return p, nil
		}
		p, ok := r.providers[name]
		if !ok {
			return nil, fmt.Errorf("unknown code agent: %s", name)
		}
		if !p.Available() {
			return nil, fmt.Errorf("%s is not installed", name)
		}
		return p, nil
	}

	// Try preferred provider (MCP first, then Bash)
	if r.preferred != "" {
		if p, ok := r.mcpProviders[r.preferred]; ok && p.Available() {
			return p, nil
		}
		if p, ok := r.providers[r.preferred]; ok && p.Available() {
			return p, nil
		}
	}

	// Fallback by priority: MCP first, then Bash
	for _, name := range priorityOrder {
		if p, ok := r.mcpProviders[name]; ok && p.Available() {
			return p, nil
		}
	}
	for _, name := range priorityOrder {
		if p, ok := r.providers[name]; ok && p.Available() {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no code agent available; configure MCP servers or install CLI: claude, gemini, codex, qwen-code, trae-cli")
}

// Available returns the names of all available providers (MCP + Bash).
func (r *Registry) Available() []string {
	seen := make(map[string]bool)
	var names []string
	// MCP providers first
	for _, name := range priorityOrder {
		if p, ok := r.mcpProviders[name]; ok && p.Available() {
			names = append(names, name+" (mcp)")
			seen[name] = true
		}
	}
	// Then Bash providers (skip if already available via MCP)
	for _, name := range priorityOrder {
		if seen[name] {
			continue
		}
		if p, ok := r.providers[name]; ok && p.Available() {
			names = append(names, name)
		}
	}
	return names
}
