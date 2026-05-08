package builtin

import "github.com/Nahasma/openscholar-public/internal/command"

type compactCmd struct{}

func (c *compactCmd) Name() string        { return "compact" }
func (c *compactCmd) Description() string { return "Compress conversation history" }

func (c *compactCmd) Execute(ctx command.Context) command.Result {
	if ctx.SessionID == "" {
		return command.Result{Output: "No active session."}
	}
	return command.Result{Action: "compact"}
}
