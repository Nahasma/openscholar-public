package builtin

import (
	"github.com/openscholar/openscholar/internal/command"
)

type kbStatsCmd struct{}

func (c *kbStatsCmd) Name() string        { return "kb-stats" }
func (c *kbStatsCmd) Description() string { return "Show knowledge base usage statistics and paper relations" }

func (c *kbStatsCmd) Execute(ctx command.Context) command.Result {
	return command.Result{
		Prompt: "Show knowledge base statistics: total papers, total nodes, total queries, top accessed papers, and discovered paper relations. Use the available KB tools to gather this information.",
	}
}
