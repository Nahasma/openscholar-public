package builtin

import (
	"github.com/Nahasma/openscholar-public/internal/command"
)

type kbListCmd struct{}

func (c *kbListCmd) Name() string { return "kb-list" }
func (c *kbListCmd) Description() string {
	return "List KB papers with content/FTS/semantic-task status"
}

func (c *kbListCmd) Execute(ctx command.Context) command.Result {
	return command.Result{
		Prompt: "List KB papers with metadata and separate content, fts, semantic_tree, and active_task status using KBList.",
	}
}
