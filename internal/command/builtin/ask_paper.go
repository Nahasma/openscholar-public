package builtin

import (
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
)

type askPaperCmd struct{}

func (c *askPaperCmd) Name() string { return "ask-paper" }
func (c *askPaperCmd) Description() string {
	return "Ask a question about a specific KB paper via raw-first QueryPaper"
}

func (c *askPaperCmd) Execute(ctx command.Context) command.Result {
	if ctx.Args == "" {
		return command.Result{Output: "Usage: /ask-paper <paper_id> <question>\n\nExample: /ask-paper 1706.03762 \"What is multi-head attention?\""}
	}

	parts := strings.SplitN(ctx.Args, " ", 2)
	if len(parts) < 2 {
		return command.Result{Output: "Usage: /ask-paper <paper_id> <question>"}
	}

	paperID := parts[0]
	question := strings.Trim(parts[1], "\"'")

	return command.Result{
		Prompt: "Use KBQuery (QueryPaper route) to answer this question about paper " + paperID + ": " + question,
	}
}
