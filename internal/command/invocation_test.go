package command_test

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/command"
)

func TestParseInvocation_QuotedArgs(t *testing.T) {
	inv := command.ParseInvocation(`/init template "hello world" 'x y' z`, len(`/init template "hello world" 'x y' z`))
	if inv.Command != "init" {
		t.Fatalf("command=%q", inv.Command)
	}
	if inv.ArgsRaw != `template "hello world" 'x y' z` {
		t.Fatalf("argsRaw=%q", inv.ArgsRaw)
	}
	want := []string{"template", "hello world", "x y", "z"}
	if len(inv.Argv) != len(want) {
		t.Fatalf("argv=%v", inv.Argv)
	}
	for i := range want {
		if inv.Argv[i] != want[i] {
			t.Fatalf("argv[%d]=%q want %q", i, inv.Argv[i], want[i])
		}
	}
}

func TestRegistryCompleteInvocation_CommandAndSubcommand(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(specTestCommand{name: "config"})
	reg.Register(specWithSubcommands{
		name: "init",
		sub:  []command.CommandSpec{{Name: "soft"}, {Name: "show"}},
	})

	_, items := reg.CompleteInvocation("/con", len("/con"))
	if len(items) == 0 || items[0].Value != "config" {
		t.Fatalf("command completion missing config: %#v", items)
	}

	_, subItems := reg.CompleteInvocation("/init s", len("/init s"))
	if len(subItems) != 2 {
		t.Fatalf("subcommand completion=%#v", subItems)
	}
	if subItems[0].Kind != "subcommand" {
		t.Fatalf("kind=%q", subItems[0].Kind)
	}
}

func TestRegistryCompleteInvocation_SubcommandAtCursorIgnoresLaterArgs(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(specWithSubcommands{
		name: "config",
		sub:  []command.CommandSpec{{Name: "show"}},
	})

	raw := "/config s --flag"
	_, items := reg.CompleteInvocation(raw, len("/config s"))
	if len(items) != 1 || items[0].Subcommand != "show" {
		t.Fatalf("subcommand completion at cursor = %#v", items)
	}
}

func TestRegistryCompleteInvocation_EmptySubcommandSlotBeforeLaterArgs(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(specWithSubcommands{
		name: "config",
		sub:  []command.CommandSpec{{Name: "show"}, {Name: "wizard"}},
	})

	raw := "/config  --flag"
	_, items := reg.CompleteInvocation(raw, len("/config "))
	if len(items) != 2 {
		t.Fatalf("empty subcommand slot completion = %#v", items)
	}
}

func TestRegistryCompleteInvocation_AliasSubcommandUsesTypedCommandName(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(specWithSubcommands{
		name:    "config",
		aliases: []string{"settings"},
		sub:     []command.CommandSpec{{Name: "show"}},
	})

	_, items := reg.CompleteInvocation("/settings s", len("/settings s"))
	if len(items) != 1 {
		t.Fatalf("subcommand completion = %#v", items)
	}
	if items[0].CommandName != "settings" {
		t.Fatalf("CommandName = %q, want settings", items[0].CommandName)
	}
}

type specWithSubcommands struct {
	name    string
	aliases []string
	sub     []command.CommandSpec
}

func (c specWithSubcommands) Name() string        { return c.name }
func (c specWithSubcommands) Description() string { return "spec test" }
func (c specWithSubcommands) Execute(command.Context) command.Result {
	return command.Result{}
}
func (c specWithSubcommands) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:        c.name,
		Aliases:     c.aliases,
		Description: c.Description(),
		Subcommands: c.sub,
	}
}
