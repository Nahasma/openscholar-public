package builtin

import (
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/command"
)

func TestHelpOverviewUsesCommandSpecs(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(&configCmd{})
	reg.Register(&initBuiltinCmd{})
	cmd := &helpCmd{registry: reg}

	res := cmd.Execute(command.Context{})
	if res.OutputKind != command.OutputTranscriptBlock {
		t.Fatalf("OutputKind = %q, want transcript block", res.OutputKind)
	}
	for _, want := range []string{
		"Slash Commands",
		"setup:",
		"/config [show|wizard|debug|providers|agents]",
		"[builtin]",
		"Custom Commands And SkillBank",
		"allowed_tools/allowed-tools",
	} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("help overview missing %q:\n%s", want, res.Output)
		}
	}
}

func TestHelpCommandDetailShowsAliasesAndSubcommands(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(&configCmd{})
	cmd := &helpCmd{registry: reg}

	res := cmd.Execute(command.Context{Args: "settings"})
	for _, want := range []string{
		"Command: /config",
		"Aliases: /settings",
		"Usage: /config [show|wizard|debug|providers|agents]",
		"/config debug",
		"Source: builtin",
	} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("help detail missing %q:\n%s", want, res.Output)
		}
	}
}
