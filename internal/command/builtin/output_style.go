package builtin

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/outputstyle"
)

type outputStyleCmd struct {
	manager *outputstyle.Manager
}

func (c *outputStyleCmd) Name() string        { return "output-style" }
func (c *outputStyleCmd) Description() string { return "Manage output styles (/output-style [name|none])" }

func (c *outputStyleCmd) Execute(ctx command.Context) command.Result {
	// nil guard — manager not wired yet
	if c.manager == nil {
		return command.Result{Output: "output-style: manager not initialised"}
	}

	arg := strings.TrimSpace(ctx.Args)

	switch {
	case arg == "":
		return c.listStyles()
	case arg == "none":
		c.manager.Clear()
		return command.Result{Output: "Output style cleared."}
	default:
		return c.activateStyle(arg)
	}
}

func (c *outputStyleCmd) listStyles() command.Result {
	styles := c.manager.List()
	if len(styles) == 0 {
		return command.Result{Output: "No output styles available.\n\nPlace .md files in .openscholar/output-styles/ (project) or ~/.openscholar/output-styles/ (user)."}
	}

	active := c.manager.GetActive()

	var sb strings.Builder
	sb.WriteString("Available output styles:\n\n")
	for _, s := range styles {
		marker := "  "
		if active != nil && active.Name == s.Name {
			marker = "→ "
		}
		desc := s.Description
		if desc == "" {
			desc = "(no description)"
		}
		sb.WriteString(fmt.Sprintf("%s%-20s [%s]  %s\n", marker, s.Name, s.Source, desc))
	}
	sb.WriteString("\nUsage: /output-style <name>   — activate a style\n")
	sb.WriteString("       /output-style none      — clear active style\n")
	return command.Result{Output: sb.String()}
}

func (c *outputStyleCmd) activateStyle(name string) command.Result {
	if err := c.manager.SetActive(name); err != nil {
		return command.Result{Output: fmt.Sprintf("output-style: %v\nUse /output-style to list available styles.", err)}
	}
	active := c.manager.GetActive()
	desc := ""
	if active.Description != "" {
		desc = fmt.Sprintf(" — %s", active.Description)
	}
	return command.Result{Output: fmt.Sprintf("Output style set to %q%s.", active.Name, desc)}
}
