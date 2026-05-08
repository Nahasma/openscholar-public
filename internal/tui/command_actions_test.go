package tui

import (
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestCommandActions_NonTerminatingActionsContinueToOutput(t *testing.T) {
	tests := []struct {
		name   string
		action command.CommandAction
	}{
		{
			name:   "cd",
			action: command.CommandAction{Kind: command.CommandActionCD},
		},
		{
			name:   "research started without session",
			action: command.CommandAction{Kind: command.CommandActionResearchStart},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			registry := command.NewRegistry()
			registry.Register(staticCommand{
				name: "continuation",
				result: command.Result{
					Output:  "continued-output",
					Actions: []command.CommandAction{tt.action},
				},
			})
			m.dispatcher = command.NewDispatcher(registry)
			m.composer.input.SetValue("/continuation")

			model, _ := m.sendMessage()
			got, ok := model.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", model)
			}
			if got.composer.commandOutput != "continued-output" {
				t.Fatalf("commandOutput = %q, want continued-output", got.composer.commandOutput)
			}
			if got.composer.commandDialog.Visible {
				t.Fatal("command dialog should remain hidden for notice output")
			}
		})
	}
}

func TestCommandOutputTranscriptBlockAppendsSystemMessage(t *testing.T) {
	m := newTestModel()
	registry := command.NewRegistry()
	registry.Register(staticCommand{
		name: "debug",
		result: command.Result{
			Output:     "long debug output",
			OutputKind: command.OutputTranscriptBlock,
		},
	})
	m.dispatcher = command.NewDispatcher(registry)
	m.composer.input.SetValue("/debug")

	model, _ := m.sendMessage()
	got, ok := model.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", model)
	}
	if got.composer.commandOutput != "" {
		t.Fatalf("commandOutput = %q, want empty", got.composer.commandOutput)
	}
	if len(got.chat.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(got.chat.messages))
	}
	msg := got.chat.messages[0]
	if msg.Role != message.System {
		t.Fatalf("role = %s, want system", msg.Role)
	}
	if !strings.Contains(msg.Content().String(), "long debug output") {
		t.Fatalf("system message content = %q", msg.Content().String())
	}
}

func TestCommandOutputCommandBlockSetsComposerOnly(t *testing.T) {
	m := newTestModel()
	registry := command.NewRegistry()
	registry.Register(staticCommand{
		name: "debug",
		result: command.Result{
			Output:      "command output",
			OutputTitle: "Debug",
			OutputKind:  command.OutputCommandBlock,
		},
	})
	m.dispatcher = command.NewDispatcher(registry)
	m.composer.input.SetValue("/debug")

	model, _ := m.sendMessage()
	got, ok := model.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", model)
	}
	if !got.composer.commandDialog.Visible {
		t.Fatal("command dialog should be visible")
	}
	if got.composer.commandDialog.Title != "Debug" {
		t.Fatalf("command dialog title = %q", got.composer.commandDialog.Title)
	}
	if got.composer.commandDialog.Body != "command output" {
		t.Fatalf("command dialog body = %q", got.composer.commandDialog.Body)
	}
	if got.composer.commandOutput != "" {
		t.Fatalf("legacy commandOutput = %q, want empty", got.composer.commandOutput)
	}
	if len(got.chat.messages) != 0 {
		t.Fatalf("messages len = %d, want 0", len(got.chat.messages))
	}
}
