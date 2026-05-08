package builtin

import (
	"github.com/openscholar/openscholar/internal/command"
)

type memoryCmd struct{}

func (c *memoryCmd) Name() string        { return "memory" }
func (c *memoryCmd) Description() string { return "Browse and manage memory files" }

func (c *memoryCmd) Execute(_ command.Context) command.Result {
	return command.Result{Action: "memory-selector"}
}
