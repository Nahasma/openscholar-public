package builtin

import (
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
)

type kbTreeCmd struct{}

func (c *kbTreeCmd) Name() string { return "kb-tree" }
func (c *kbTreeCmd) Description() string {
	return "Show semantic tree, page index, or status for a KB paper"
}

func (c *kbTreeCmd) Execute(ctx command.Context) command.Result {
	if ctx.Args == "" {
		return command.Result{Output: "Usage: /kb-tree <paper_id> [auto|semantic|pages|status]"}
	}
	paperID := ctx.Args
	view := "auto"
	parts := strings.Fields(ctx.Args)
	if len(parts) > 0 {
		paperID = parts[0]
	}
	if len(parts) > 1 {
		view = parts[1]
	}
	return command.Result{
		Prompt: "Use KBTree with paper_id=" + paperID + " and view=" + view + " to inspect semantic/pages/status.",
	}
}
