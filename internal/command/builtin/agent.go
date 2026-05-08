package builtin

import (
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/agent/custom"
)

type agentCmd struct {
	loader *custom.Loader
}

func newAgentCmd() *agentCmd {
	return &agentCmd{
		loader: custom.NewLoader(config.WorkingDirectory()),
	}
}

func (c *agentCmd) Name() string        { return "agent" }
func (c *agentCmd) Description() string { return "List or switch custom agents" }

func (c *agentCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)

	if args == "" || args == "--all" || args == "list" || args == "list --all" {
		configs := c.loader.LoadAll()
		showAll := args == "--all" || strings.Contains(args, "--all")
		if len(configs) == 0 && !showAll {
			return command.Result{
				Output: "No custom agents found.\nCreate .md files in .openscholar/agents/ or .openscholar/modes/.\nUse YAML front matter with name and description, then put the system prompt after the closing ---.",
			}
		}

		var sb strings.Builder
		sb.WriteString("Available agents:\n\n")
		if showAll {
			sb.WriteString("Built-in:\n")
			for _, name := range custom.BuiltInAgentNames {
				sb.WriteString(fmt.Sprintf("  %-15s built-in runtime agent\n", name))
			}
			if len(configs) > 0 {
				sb.WriteString("\nCustom:\n")
			}
		}
		for _, cfg := range configs {
			parts := []string{cfg.Description}
			if cfg.Model != "" {
				parts = append(parts, "model="+cfg.Model)
			}
			if cfg.Permission != "" {
				parts = append(parts, "permission="+cfg.Permission)
			}
			sb.WriteString(fmt.Sprintf("  %-15s %s\n", cfg.Name, strings.Join(nonEmpty(parts), " | ")))
		}
		sb.WriteString("\nUsage: /agent <name> to switch, or Task agent_type=<name> to run as a sub-agent.")
		return command.Result{Output: sb.String()}
	}

	// Switch to the specified agent
	cfg, err := c.loader.Load(args)
	if err != nil {
		return command.Result{Error: err}
	}

	// Inject the custom agent's system prompt into the conversation
	return command.Result{
		Output: fmt.Sprintf("Switched to agent: %s — %s", cfg.Name, cfg.Description),
		Prompt: cfg.SystemPrompt,
	}
}

func nonEmpty(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
