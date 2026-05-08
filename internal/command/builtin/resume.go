package builtin

import (
	"strings"

	"github.com/openscholar/openscholar/internal/command"
)

type resumeCmd struct{}

func (c *resumeCmd) Name() string        { return "resume" }
func (c *resumeCmd) Description() string { return "Resume a previous session" }
func (c *resumeCmd) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:         c.Name(),
		Description:  c.Description(),
		ArgumentHint: "[session id | title | search]",
	}
}
func (c *resumeCmd) Execute(ctx command.Context) command.Result {
	selector := strings.TrimSpace(ctx.Args)
	if selector == "" {
		return command.Result{Action: "session-resume"}
	}
	return command.Result{Action: "session-resume:" + selector}
}

type continueCmd struct{}

func (c *continueCmd) Name() string        { return "continue" }
func (c *continueCmd) Description() string { return "Resume the latest session in this project" }
func (c *continueCmd) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:         c.Name(),
		Description:  c.Description(),
		ArgumentHint: "[session id | title | search]",
	}
}
func (c *continueCmd) Execute(ctx command.Context) command.Result {
	selector := strings.TrimSpace(ctx.Args)
	if selector == "" {
		return command.Result{Action: "session-resume:__latest__"}
	}
	return command.Result{Action: "session-resume:" + selector}
}
