package command_test

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/command/builtin"
)

type specTestCommand struct {
	name    string
	aliases []string
}

func (c specTestCommand) Name() string        { return c.name }
func (c specTestCommand) Description() string { return "spec test" }
func (c specTestCommand) Execute(command.Context) command.Result {
	return command.Result{Output: c.name}
}
func (c specTestCommand) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:        c.name,
		Aliases:     c.aliases,
		Description: c.Description(),
	}
}

func TestRegisterAll_ExposesBaselineCommands(t *testing.T) {
	reg := command.NewRegistry()
	builtin.RegisterAll(reg, nil)

	for _, name := range []string{
		"config",
		"init",
		"help",
		"model",
		"status",
		"memory",
		"screen",
	} {
		if got := reg.Get(name); got == nil {
			t.Fatalf("expected built-in command %q to be registered", name)
		}
	}

	if got := reg.Get("settings"); got == nil || got.Name() != "config" {
		t.Fatalf("expected /settings alias to resolve to /config")
	}

	spec, ok := reg.Spec("settings")
	if !ok {
		t.Fatalf("expected spec lookup by alias to succeed")
	}
	if spec.Name != "config" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "config")
	}
	if len(reg.ListSpecs()) == 0 {
		t.Fatalf("expected command specs to be listed")
	}
}

func TestRegistryRegister_PrunesStaleAliasesOnReregister(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(specTestCommand{name: "config", aliases: []string{"settings"}})
	if got := reg.Get("settings"); got == nil || got.Name() != "config" {
		t.Fatalf("expected initial alias to resolve to config")
	}

	reg.Register(specTestCommand{name: "config"})
	if got := reg.Get("settings"); got != nil {
		t.Fatalf("stale alias resolved after reregister: %s", got.Name())
	}
	if _, ok := reg.Spec("settings"); ok {
		t.Fatalf("stale alias spec should not resolve after reregister")
	}
}
