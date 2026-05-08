package builtin

import (
	"strings"

	"github.com/openscholar/openscholar/internal/command"
)

type kbHealthCmd struct{}
type kbRepairCmd struct{}
type kbReindexCmd struct{}

func (c *kbHealthCmd) Name() string        { return "kb-health" }
func (c *kbHealthCmd) Description() string { return "Run KB health checks (chunks/fts/semantic/tasks)" }
func (c *kbHealthCmd) Execute(ctx command.Context) command.Result {
	return command.Result{Prompt: "Use KBHealth to run KB health checks and report warnings."}
}

func (c *kbRepairCmd) Name() string        { return "kb-repair" }
func (c *kbRepairCmd) Description() string { return "Repair safe KB issues; dry-run by default" }
func (c *kbRepairCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)
	if args == "" {
		return command.Result{Prompt: "Use KBRepair with apply=false (dry-run)."}
	}
	return command.Result{Prompt: "Use KBRepair with args: " + args}
}

func (c *kbReindexCmd) Name() string { return "kb-reindex" }
func (c *kbReindexCmd) Description() string {
	return "Rebuild KB indexes and enqueue semantic reindex intents; dry-run by default"
}
func (c *kbReindexCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)
	if args == "" {
		return command.Result{Prompt: "Use KBReindex with apply=false (dry-run)."}
	}
	return command.Result{Prompt: "Use KBReindex with args: " + args}
}
