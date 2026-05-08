package custom

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse extracts an AgentConfig from a Markdown file with YAML frontmatter.
// Format:
//
//	---
//	name: reviewer
//	description: "Paper review mode"
//	...
//	---
//	<system prompt body>
func Parse(data []byte) (*AgentConfig, error) {
	// Split on "---" delimiter
	parts := bytes.SplitN(data, []byte("---"), 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid agent definition: missing frontmatter delimiters (---)")
	}

	var raw map[string]any
	if err := yaml.Unmarshal(parts[1], &raw); err != nil {
		return nil, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}
	clean := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "tools" || k == "allowed-tools" {
			continue
		}
		clean[k] = v
	}
	cleanYAML, err := yaml.Marshal(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}
	var cfg AgentConfig
	if err := yaml.Unmarshal(cleanYAML, &cfg); err != nil {
		return nil, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}
	if cfg.WhenToUse == "" {
		cfg.WhenToUse = asString(raw, "when_to_use", "when-to-use", "whenToUse")
	}
	if cfg.Permission == "" {
		cfg.Permission = asString(raw, "permission_mode", "permission-mode", "permissionMode")
	}
	if cfg.Model == "" {
		cfg.Model = asString(raw, "model")
	}
	if cfg.Effort == "" {
		cfg.Effort = asString(raw, "effort")
	}
	if _, ok := raw["tools"]; ok {
		cfg.ToolsExplicit = true
		cfg.Tools = asStringSlice(raw, "tools")
	}
	if _, ok := raw["allowed-tools"]; ok {
		cfg.ToolsExplicit = true
		if cfg.Tools == nil {
			cfg.Tools = asStringSlice(raw, "allowed-tools")
		}
	}
	if cfg.Permission == "" {
		cfg.Permission = strings.TrimSpace(cfg.PermissionMode)
	}

	cfg.SystemPrompt = string(bytes.TrimSpace(parts[2]))

	if cfg.Name == "" {
		return nil, fmt.Errorf("agent definition must have a 'name' field")
	}

	return &cfg, nil
}

func asStringSlice(m map[string]any, keys ...string) []string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch tv := v.(type) {
		case []any:
			out := make([]string, 0, len(tv))
			for _, item := range tv {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		case []string:
			out := make([]string, 0, len(tv))
			for _, s := range tv {
				if strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		case string:
			if strings.TrimSpace(tv) == "" {
				return nil
			}
			parts := strings.Split(tv, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				if s := strings.TrimSpace(part); s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func asString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
