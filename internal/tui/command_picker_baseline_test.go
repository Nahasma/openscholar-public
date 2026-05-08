package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/command"
)

type pickerBaselineCommand struct {
	name   string
	result command.Result
}

func (c pickerBaselineCommand) Name() string                           { return c.name }
func (c pickerBaselineCommand) Description() string                    { return "test command" }
func (c pickerBaselineCommand) Execute(command.Context) command.Result { return c.result }

func dispatchPickerKey(m *Model, msg tea.KeyMsg) *Model {
	next, _ := m.handleKeyMsg(msg)
	switch cast := next.(type) {
	case *Model:
		return cast
	case Model:
		updated := cast
		return &updated
	default:
		return m
	}
}

func TestCommandPickerBaseline_EnterTabEsc(t *testing.T) {
	makeModel := func() *Model {
		m := newTestModel()
		reg := command.NewRegistry()
		reg.Register(pickerBaselineCommand{
			name:   "baseline",
			result: command.Result{Output: "baseline-ok"},
		})
		m.dispatcher = command.NewDispatcher(reg)
		m.composer.filteredCompletions = []command.CompletionItem{
			{Kind: "command", Value: "baseline", CommandName: "baseline", Description: "test command"},
		}
		m.composer.commandPickerIdx = 0
		m.composer.commandPickerActive = true
		return m
	}

	t.Run("enter on partial completion fills and does not execute", func(t *testing.T) {
		m := makeModel()
		m.composer.input.SetValue("/bas")
		m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEnter})

		if m.composer.commandPickerActive {
			t.Fatalf("expected command picker to close after Enter")
		}
		if got := m.composer.commandOutput; got != "" {
			t.Fatalf("commandOutput = %q, want empty (no execution on Enter)", got)
		}
		if got := m.composer.input.Value(); got != "/baseline" {
			t.Fatalf("input = %q, want %q", got, "/baseline")
		}
	})

	t.Run("enter on exact completion executes once", func(t *testing.T) {
		m := makeModel()
		m.composer.input.SetValue("/baseline")
		m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEnter})

		if m.composer.commandPickerActive {
			t.Fatalf("expected command picker to close after Enter")
		}
		if got := m.composer.commandOutput; got != "baseline-ok" {
			t.Fatalf("commandOutput = %q, want baseline-ok", got)
		}
	})

	t.Run("tab fills slash command and does not execute", func(t *testing.T) {
		m := makeModel()
		m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyTab})

		if m.composer.commandPickerActive {
			t.Fatalf("expected command picker to close after Tab")
		}
		if got := m.composer.input.Value(); got != "/baseline " {
			t.Fatalf("input = %q, want %q", got, "/baseline ")
		}
		if got := m.composer.commandOutput; got != "" {
			t.Fatalf("commandOutput = %q, want empty (no execution on Tab)", got)
		}
	})

	t.Run("esc closes picker and keeps current input", func(t *testing.T) {
		m := makeModel()
		m.composer.input.SetValue("/base")
		m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEsc})

		if m.composer.commandPickerActive {
			t.Fatalf("expected command picker to close after Esc")
		}
		if got := m.composer.input.Value(); got != "/base" {
			t.Fatalf("input = %q, want unchanged on Esc", got)
		}
	})
}

func TestCommandPicker_UsesCursorForSubcommandCompletion(t *testing.T) {
	m := newTestModel()
	reg := command.NewRegistry()
	reg.Register(specPickerCommand{
		name: "config",
		subcommands: []command.CommandSpec{
			{Name: "show", Description: "show config"},
		},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/config s --flag")
	m.composer.input.Textarea.SetCursor(len("/config s"))

	var cmds []tea.Cmd
	m.syncComposerPickers(&cmds)
	if !m.composer.commandPickerActive {
		t.Fatalf("expected command picker to use cursor and show subcommand completion")
	}
	if len(m.composer.filteredCompletions) != 1 || m.composer.filteredCompletions[0].Subcommand != "show" {
		t.Fatalf("filtered completions = %#v", m.composer.filteredCompletions)
	}

	m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.composer.input.Value(); got != "/config show --flag" {
		t.Fatalf("input = %q, want cursor-local replacement", got)
	}
}

func TestCommandPicker_TabKeepsCursorAfterMiddleCommandCompletion(t *testing.T) {
	m := newTestModel()
	reg := command.NewRegistry()
	reg.Register(pickerBaselineCommand{name: "config"})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/co --flag")
	m.composer.input.Textarea.SetCursor(len("/co"))

	var cmds []tea.Cmd
	m.syncComposerPickers(&cmds)
	if !m.composer.commandPickerActive {
		t.Fatalf("expected command picker to use cursor and show command completion")
	}
	m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.composer.input.Value(); got != "/config --flag" {
		t.Fatalf("input = %q, want cursor-local command replacement", got)
	}
	if got := m.composer.input.CursorOffset(); got != len("/config ") {
		t.Fatalf("cursor offset = %d, want %d", got, len("/config "))
	}
}

func TestCommandPicker_EnterInMiddleOnlyCompletesAndDoesNotExecute(t *testing.T) {
	m := newTestModel()
	reg := command.NewRegistry()
	reg.Register(pickerBaselineCommand{
		name:   "baseline",
		result: command.Result{Output: "baseline-ok"},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/base --flag")
	m.composer.input.Textarea.SetCursor(len("/base"))
	var cmds []tea.Cmd
	m.syncComposerPickers(&cmds)
	if !m.composer.commandPickerActive {
		t.Fatalf("expected command picker active")
	}
	m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.composer.input.Value(); got != "/baseline --flag" {
		t.Fatalf("input = %q, want middle completion", got)
	}
	if got := m.composer.commandOutput; got != "" {
		t.Fatalf("commandOutput = %q, want empty", got)
	}
}

func TestCommandPicker_TabKeepsCursorAfterMiddleSubcommandCompletion(t *testing.T) {
	m := newTestModel()
	reg := command.NewRegistry()
	reg.Register(specPickerCommand{
		name: "init",
		subcommands: []command.CommandSpec{
			{Name: "template", Description: "template init"},
		},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/init t --flag")
	m.composer.input.Textarea.SetCursor(len("/init t"))

	var cmds []tea.Cmd
	m.syncComposerPickers(&cmds)
	if !m.composer.commandPickerActive {
		t.Fatalf("expected command picker to use cursor and show subcommand completion")
	}
	m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.composer.input.Value(); got != "/init template --flag" {
		t.Fatalf("input = %q, want cursor-local tab replacement", got)
	}
	if got := m.composer.input.CursorOffset(); got != len("/init template ") {
		t.Fatalf("cursor offset = %d, want %d", got, len("/init template "))
	}
}

func TestCommandPicker_InsertsSubcommandInEmptySlotBeforeArgs(t *testing.T) {
	m := newTestModel()
	reg := command.NewRegistry()
	reg.Register(specPickerCommand{
		name: "config",
		subcommands: []command.CommandSpec{
			{Name: "show", Description: "show config"},
		},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/config  --flag")
	m.composer.input.Textarea.SetCursor(len("/config "))

	var cmds []tea.Cmd
	m.syncComposerPickers(&cmds)
	if !m.composer.commandPickerActive {
		t.Fatalf("expected command picker to complete empty subcommand slot")
	}
	m = dispatchPickerKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.composer.input.Value(); got != "/config show --flag" {
		t.Fatalf("input = %q, want inserted subcommand before args", got)
	}
	if got := m.composer.input.CursorOffset(); got != len("/config show") {
		t.Fatalf("cursor offset = %d, want %d", got, len("/config show"))
	}
}

type specPickerCommand struct {
	name        string
	subcommands []command.CommandSpec
}

func (c specPickerCommand) Name() string        { return c.name }
func (c specPickerCommand) Description() string { return "spec picker command" }
func (c specPickerCommand) Execute(command.Context) command.Result {
	return command.Result{}
}
func (c specPickerCommand) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:        c.name,
		Description: c.Description(),
		Subcommands: c.subcommands,
	}
}
