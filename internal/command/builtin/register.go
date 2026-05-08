package builtin

import (
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/command"
)

// RegisterAll registers all built-in commands.
func RegisterAll(registry *command.Registry, app *app.App) {
	registry.Register(&helpCmd{registry: registry})
	registry.Register(&clearCmd{})
	registry.Register(&statusCmd{app: app})
	registry.Register(&costCmd{app: app})
	registry.Register(&contextCmd{})
	registry.Register(&compactCmd{})
	registry.Register(&planCmd{})
	registry.Register(&modelCmd{app: app})
	registry.Register(newAgentCmd())

	// Diagnostics
	registry.Register(&doctorBuiltinCmd{})

	// Knowledge base commands
	registry.Register(&kbAddCmd{})
	registry.Register(&askPaperCmd{})
	registry.Register(&kbListCmd{})
	registry.Register(&kbTreeCmd{})
	registry.Register(&kbStatsCmd{})
	registry.Register(&kbImportCmd{})
	registry.Register(&kbStatusCmd{})
	registry.Register(&kbRetryCmd{})
	registry.Register(&kbHealthCmd{})
	registry.Register(&kbRepairCmd{})
	registry.Register(&kbReindexCmd{})

	// Skill bank commands
	registry.Register(&skillImportCmd{})
	registry.Register(&skillListCmd{})
	registry.Register(&skillInstallCmd{})
	registry.Register(&skillSyncCmd{})
	registry.Register(&skillUninstallCmd{})
	registry.Register(&skillInfoCmd{})
	registry.Register(&skillInstalledCmd{})

	// Skill evolution
	registry.Register(&evolveCmd{})
	registry.Register(&skillStatsCmd{})

	// Research pipeline
	registry.Register(&researchCmd{})

	// MCP
	registry.Register(&mcpCmd{})

	// Config
	registry.Register(&configCmd{})

	// Memory
	registry.Register(&memoryCmd{})

	// Output styles
	registry.Register(&outputStyleCmd{})
	registry.Register(&screenCmd{})

	// Utility commands
	registry.Register(&cdCmd{})
	registry.Register(&initBuiltinCmd{})
	registry.Register(&resumeCmd{})
	registry.Register(&continueCmd{})
}
