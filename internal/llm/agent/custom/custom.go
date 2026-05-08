package custom

import "fmt"

// AgentConfig represents a custom agent definition parsed from a Markdown file.
type AgentConfig struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	Mode           string   `yaml:"mode"`        // "agent" or "mode"
	Tools          []string `yaml:"tools"`       // tool whitelist; omitted or ["*"] = all tools, explicit [] = no tools
	Model          string   `yaml:"model"`       // optional model override
	Effort         string   `yaml:"effort"`      // optional effort hint for future providers
	Temperature    float64  `yaml:"temperature"` // model temperature; 0 = use default
	Permission     string   `yaml:"permission"`  // "auto" / "default" / "plan"
	PermissionMode string   `yaml:"permissionMode,omitempty"`
	WhenToUse      string   `yaml:"whenToUse,omitempty"`

	// Populated after parsing
	SystemPrompt  string // Markdown body (after frontmatter)
	FilePath      string `yaml:"-"` // source file path
	ToolsExplicit bool   `yaml:"-"` // true when frontmatter includes tools
}

// ToMarkdown serializes AgentConfig to Markdown format (YAML frontmatter + body).
func (c *AgentConfig) ToMarkdown() string {
	md := fmt.Sprintf("---\nname: %s\ndescription: %s\nmode: %s\ntemperature: %.1f\n",
		c.Name, c.Description, c.Mode, c.Temperature)

	if c.Permission != "" {
		md += fmt.Sprintf("permission: %s\n", c.Permission)
	}
	if c.Model != "" {
		md += fmt.Sprintf("model: %s\n", c.Model)
	}
	if c.Effort != "" {
		md += fmt.Sprintf("effort: %s\n", c.Effort)
	}
	if c.WhenToUse != "" {
		md += fmt.Sprintf("whenToUse: %s\n", c.WhenToUse)
	}
	if c.ToolsExplicit && len(c.Tools) == 0 {
		md += "tools: []\n"
	} else if len(c.Tools) > 0 {
		md += "tools:\n"
		for _, t := range c.Tools {
			md += fmt.Sprintf("  - %s\n", t)
		}
	}
	md += "---\n\n" + c.SystemPrompt
	return md
}
