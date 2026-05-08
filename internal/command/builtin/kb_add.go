package builtin

import (
	"github.com/Nahasma/openscholar-public/internal/command"
)

type kbAddCmd struct{}

func (c *kbAddCmd) Name() string { return "kb-add" }
func (c *kbAddCmd) Description() string {
	return "Add a document to the knowledge base for raw-first search"
}

func (c *kbAddCmd) Execute(ctx command.Context) command.Result {
	if ctx.Args == "" {
		return command.Result{Output: "Usage: /kb-add <file_path>\n\nExample: /kb-add ~/papers/attention.pdf"}
	}
	return command.Result{
		Prompt: "Please add this document to the knowledge base with raw-first indexing: " + ctx.Args,
	}
}
