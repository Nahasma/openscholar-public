package builtin

import "github.com/Nahasma/openscholar-public/internal/command"

type clearCmd struct{}

func (c *clearCmd) Name() string        { return "clear" }
func (c *clearCmd) Description() string { return "Clear chat history" }

func (c *clearCmd) Execute(ctx command.Context) command.Result {
	return command.Result{
		ClearChat: true,
	}
}
