package builtin

import (
	"fmt"
	"os"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
)

type statusCmd struct {
	app *app.App
}

func (c *statusCmd) Name() string        { return "status" }
func (c *statusCmd) Description() string { return "Show current model, session, and directory" }

func (c *statusCmd) Execute(ctx command.Context) command.Result {
	var sb strings.Builder
	model := ctx.App.CoderAgent.Model()
	sb.WriteString(fmt.Sprintf("Model:     %s (%s)\n", model.Name, model.ID))
	sb.WriteString(fmt.Sprintf("Provider:  %s\n", model.Provider))

	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("Directory: %s\n", cwd))

	if ctx.SessionID != "" {
		sb.WriteString(fmt.Sprintf("Session:   %s\n", ctx.SessionID))
	} else {
		sb.WriteString("Session:   (none)\n")
	}

	// KB indexer status
	if ctx.App.KBIndexer != nil {
		sb.WriteString("KB Index:  available\n")
	} else {
		sb.WriteString("KB Index:  unavailable (run /doctor)\n")
	}

	return command.Result{Output: sb.String()}
}
