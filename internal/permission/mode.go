package permission

import (
	"context"
	"strings"
)

// ModeState tracks current and pre-plan mode state for a session.
type ModeState struct {
	Mode        Mode
	PrePlanMode Mode
}

// PlanFileResolver validates whether a path is the active plan file for a session.
type PlanFileResolver interface {
	IsPlanFile(ctx context.Context, sessionID string, path string) bool
}

// Mode represents a permission evaluation mode.
type Mode string

const (
	ModeDefault  Mode = ""         // Interactive: ask user for each action
	ModeAuto     Mode = "auto"     // Auto-approve all actions for this session
	ModePlan     Mode = "plan"     // Read-only: block write operations, allow reads
	ModeResearch Mode = "research" // Research pipeline: auto-approve + research features
	ModeRestore  Mode = "restore"  // Internal: restore pre-plan mode when approving a plan
)

type modeContextKey struct{}

// WithMode injects a permission mode into the context.
func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, modeContextKey{}, mode)
}

// CurrentMode extracts the permission mode from context, defaults to ModeDefault.
func CurrentMode(ctx context.Context) Mode {
	if m, ok := ctx.Value(modeContextKey{}).(Mode); ok {
		return m
	}
	return ModeDefault
}

// IsReadOnlyMode returns true if the current mode only allows read operations.
func IsReadOnlyMode(mode Mode) bool {
	return mode == ModePlan
}

// IsWriteLikeTool returns true if the tool+action combination represents a write operation.
// In Plan mode, these operations are blocked.
func IsWriteLikeTool(toolName string, action string) bool {
	switch toolName {
	case "Edit", "Write":
		return true
	case "ScholarSearch":
		return strings.EqualFold(strings.TrimSpace(action), "download")
	case "Bash":
		return isBashWriteCommand(action)
	case "NotebookEdit":
		return true
	}
	return false
}

// readOnlyBashPrefixes are bash command prefixes that are considered read-only.
var readOnlyBashPrefixes = []string{
	"ls", "cat", "head", "tail", "less", "more", "wc",
	"find", "grep", "rg", "ag", "ack",
	"git status", "git log", "git diff", "git show", "git branch", "git tag", "git remote",
	"git blame", "git stash list",
	"go list", "go doc", "go vet", "go version", "go env",
	"pwd", "echo", "date", "whoami", "hostname", "uname",
	"file", "stat", "du", "df", "which", "type", "whereis",
	"tree", "hexdump", "xxd", "od",
	"python --version", "python3 --version", "node --version", "npm --version",
	"make -n", // dry run
	"latexmk -n",
	"sqlc version",
}

// isBashWriteCommand checks if a bash command is a write operation.
// Returns true if the command is NOT recognized as read-only.
func isBashWriteCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true // empty command is suspicious
	}

	if strings.Contains(command, "$(") || strings.Contains(command, "`") ||
		strings.Contains(command, "<(") || strings.Contains(command, ">(") {
		return true
	}

	// Any command with output redirection is a write operation
	if containsRedirect(command) {
		return true
	}
	if isDangerousFindCommand(command) || hasGitDiffOutputFlag(command) {
		return true
	}

	for _, prefix := range readOnlyBashPrefixes {
		if command == prefix || len(command) > len(prefix) && command[:len(prefix)+1] == prefix+" " {
			return false // recognized read-only command
		}
	}

	// Check additional safe patterns
	// "go test" read-only (doesn't modify source)
	if len(command) >= 7 && command[:7] == "go test" {
		return false
	}

	// Default: assume write operation (safer)
	return true
}

// bashWriteIndicators are substrings that indicate a bash command performs writes.
var bashWriteIndicators = []string{
	"tee ",  // tee writes to files
	"tee\t", // tee with tab
	"| tee", // piped to tee
}

// containsRedirect checks if a command contains shell output redirection
// or other write indicators like tee.
func containsRedirect(command string) bool {
	// Check for write indicators (tee, etc.)
	for _, indicator := range bashWriteIndicators {
		if strings.Contains(command, indicator) {
			return true
		}
	}

	// Check for > or >> not inside quotes
	inSingle := false
	inDouble := false
	for i := 0; i < len(command); i++ {
		switch command[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '>':
			if !inSingle && !inDouble {
				return true
			}
		case '<':
			if !inSingle && !inDouble {
				return true
			}
		}
	}
	return false
}

func isDangerousFindCommand(command string) bool {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 || fields[0] != "find" {
		return false
	}
	for _, f := range fields[1:] {
		if f == "-delete" || f == "-exec" || f == "-ok" {
			return true
		}
	}
	return false
}

func hasGitDiffOutputFlag(command string) bool {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) < 2 || fields[0] != "git" || fields[1] != "diff" {
		return false
	}
	for _, f := range fields[2:] {
		if f == "--output" || strings.HasPrefix(f, "--output=") {
			return true
		}
	}
	return false
}

var sideEffectActionVerbs = []string{
	"write", "edit", "create", "delete", "download", "install", "sync", "uninstall", "export", "patch",
}

func isSideEffectAction(action string) bool {
	a := strings.ToLower(strings.TrimSpace(action))
	if a == "" {
		return false
	}
	for _, v := range sideEffectActionVerbs {
		if a == v {
			return true
		}
	}
	return false
}

// String returns the display name for a mode.
func (m Mode) String() string {
	switch m {
	case ModeDefault:
		return "default"
	case ModeAuto:
		return "auto"
	case ModePlan:
		return "plan"
	case ModeResearch:
		return "research"
	case ModeRestore:
		return "restore"
	default:
		return string(m)
	}
}

// AllModes returns all available modes in cycling order.
func AllModes() []Mode {
	return []Mode{ModeDefault, ModeAuto, ModePlan, ModeResearch}
}

// NextMode returns the next mode in the cycling sequence.
func NextMode(current Mode) Mode {
	modes := AllModes()
	for i, m := range modes {
		if m == current {
			return modes[(i+1)%len(modes)]
		}
	}
	return ModeDefault
}
