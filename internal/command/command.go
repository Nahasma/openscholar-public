package command

import (
	"context"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/app"
)

// Context carries the runtime state needed by commands.
type Context struct {
	App         *app.App
	SessionID   string
	Args        string
	ExecContext context.Context
}

// Result is the outcome of executing a command.
type Result struct {
	Output      string // text to display to the user
	OutputKind  OutputKind
	OutputTitle string
	// OutputDismissText is recorded as UI-only command activity when a command
	// dialog (OutputCommandBlock) is dismissed.
	OutputDismissText string
	// SuppressDismissRecord skips UI-only command activity when the command
	// dialog is dismissed.
	SuppressDismissRecord bool
	ClearChat             bool   // whether to clear chat history
	Prompt                string // if non-empty, send this as a user prompt to the agent
	Runtime               *RuntimeOverride
	Error                 error
	Action                string // if non-empty, trigger an interactive UI action (e.g. "model-select")
	Actions               []CommandAction
	SessionID             string // if non-empty, caller should adopt this session before follow-up work
}

// OutputKind describes the UI surface a command result should use.
type OutputKind string

const (
	OutputNotice          OutputKind = "notice"
	OutputCommandBlock    OutputKind = "command_block"
	OutputTranscriptBlock OutputKind = "transcript_block"
)

func (r Result) EffectiveOutputKind() OutputKind {
	if r.OutputKind == "" {
		return OutputNotice
	}
	return r.OutputKind
}

// CommandActionKind identifies a typed command action.
type CommandActionKind string

const (
	CommandActionModelSelect    CommandActionKind = "model-select"
	CommandActionSessionResume  CommandActionKind = "session-resume"
	CommandActionCD             CommandActionKind = "cd"
	CommandActionInitWizard     CommandActionKind = "init-wizard"
	CommandActionConfigWizard   CommandActionKind = "config-wizard"
	CommandActionMemorySelector CommandActionKind = "memory-selector"
	CommandActionCompact        CommandActionKind = "compact"
	CommandActionResearchStart  CommandActionKind = "research-started"
	CommandActionPlanEnter      CommandActionKind = "plan-enter"
	CommandActionScreen         CommandActionKind = "screen"
)

// CommandAction is the typed representation of a command-side UI/runtime action.
type CommandAction struct {
	Kind   CommandActionKind
	Target string
}

// NormalizedActions merges typed actions and the legacy Action string adapter.
func (r Result) NormalizedActions() []CommandAction {
	actions := make([]CommandAction, 0, len(r.Actions)+1)
	actions = append(actions, r.Actions...)
	actions = append(actions, LegacyActionToCommandActions(r.Action)...)

	seen := make(map[CommandAction]struct{}, len(actions))
	out := make([]CommandAction, 0, len(actions))
	for _, action := range actions {
		if action.Kind == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}

// LegacyActionToCommandActions adapts legacy Result.Action values to typed actions.
func LegacyActionToCommandActions(action string) []CommandAction {
	switch {
	case action == "":
		return nil
	case action == string(CommandActionModelSelect):
		return []CommandAction{{Kind: CommandActionModelSelect}}
	case action == string(CommandActionSessionResume):
		return []CommandAction{{Kind: CommandActionSessionResume}}
	case len(action) > len("session-resume:") && action[:len("session-resume:")] == "session-resume:":
		return []CommandAction{{Kind: CommandActionSessionResume, Target: strings.TrimSpace(action[len("session-resume:"):])}}
	case action == string(CommandActionCD):
		return []CommandAction{{Kind: CommandActionCD}}
	case action == string(CommandActionInitWizard):
		return []CommandAction{{Kind: CommandActionInitWizard}}
	case action == string(CommandActionConfigWizard):
		return []CommandAction{{Kind: CommandActionConfigWizard}}
	case action == string(CommandActionMemorySelector):
		return []CommandAction{{Kind: CommandActionMemorySelector}}
	case action == string(CommandActionCompact):
		return []CommandAction{{Kind: CommandActionCompact}}
	case action == string(CommandActionResearchStart):
		return []CommandAction{{Kind: CommandActionResearchStart}}
	case action == string(CommandActionPlanEnter):
		return []CommandAction{{Kind: CommandActionPlanEnter}}
	case len(action) > len("screen:") && action[:len("screen:")] == "screen:":
		return []CommandAction{{Kind: CommandActionScreen, Target: action[len("screen:"):]}}
	default:
		return nil
	}
}

// CommandSpec describes command metadata for help and picker UIs.
type CommandSpec struct {
	Name            string
	Aliases         []string
	Category        string
	Source          string
	Description     string
	ArgumentHint    string
	Usage           string
	Subcommands     []CommandSpec
	WhenToUse       string
	CanRunWhileBusy bool
	Exposure        string
	UserInvocable   bool
	ModelInvocable  bool
	Hidden          bool
}

// SpecProvider optionally exposes detailed command metadata.
type SpecProvider interface {
	Spec() CommandSpec
}

// RuntimeOverride carries per-request execution constraints for command-triggered runs.
type RuntimeOverride struct {
	AllowedTools           []string // empty = no additional restriction
	Model                  string   // empty = use current agent model
	DisableModelInvocation bool
}

// Command is the interface that all slash commands implement.
type Command interface {
	Name() string
	Description() string
	Execute(ctx Context) Result
}
