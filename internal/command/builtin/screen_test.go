package builtin

import (
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/command"
)

func TestScreenCmdActions(t *testing.T) {
	cmd := &screenCmd{}
	tests := []struct {
		name       string
		args       string
		wantAction string
	}{
		{name: "fullscreen", args: "fullscreen", wantAction: screenActionFullscreen},
		{name: "main", args: "main", wantAction: screenActionMain},
		{name: "toggle", args: "toggle", wantAction: screenActionToggle},
		{name: "main-screen alias", args: "main-screen", wantAction: screenActionMain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmd.Execute(command.Context{Args: tt.args})
			if got.Action != tt.wantAction {
				t.Fatalf("Action = %q, want %q", got.Action, tt.wantAction)
			}
			if got.Output != "" {
				t.Fatalf("Output = %q, want empty for action", got.Output)
			}
		})
	}
}

func TestScreenCmdInvalidArgs(t *testing.T) {
	cmd := &screenCmd{}
	got := cmd.Execute(command.Context{Args: "bad"})
	if got.Action != "" {
		t.Fatalf("Action = %q, want empty", got.Action)
	}
	if !strings.Contains(got.Output, "Usage: /screen") {
		t.Fatalf("expected usage output, got: %q", got.Output)
	}
}
