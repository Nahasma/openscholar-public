package builtin

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
)

const (
	screenActionFullscreen = "screen:fullscreen"
	screenActionMain       = "screen:main"
	screenActionToggle     = "screen:toggle"
)

type screenCmd struct{}

func (c *screenCmd) Name() string { return "screen" }
func (c *screenCmd) Description() string {
	return "Switch TUI screen mode: /screen fullscreen|main|toggle"
}

func (c *screenCmd) Execute(ctx command.Context) command.Result {
	arg := strings.ToLower(strings.TrimSpace(ctx.Args))
	switch arg {
	case "fullscreen", "full":
		return command.Result{Action: screenActionFullscreen}
	case "main", "main-screen", "mainscreen":
		return command.Result{Action: screenActionMain}
	case "toggle":
		return command.Result{Action: screenActionToggle}
	default:
		return command.Result{
			Output: fmt.Sprintf("Usage: /screen fullscreen|main|toggle (got %q)", strings.TrimSpace(ctx.Args)),
		}
	}
}
