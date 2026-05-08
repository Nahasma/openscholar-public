package command_test

import (
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/command/builtin"
)

func TestDispatcher_BaselineConfigInitAndUnknown(t *testing.T) {
	reg := command.NewRegistry()
	builtin.RegisterAll(reg, nil)
	dispatcher := command.NewDispatcher(reg)

	tests := []struct {
		name          string
		input         string
		wantAction    string
		wantTitle     string
		wantOutputSub string
	}{
		{
			name:          "config shows settings center",
			input:         "/config",
			wantTitle:     "Settings Center",
			wantOutputSub: "/config wizard",
		},
		{
			name:          "settings alias shows settings center",
			input:         "/settings",
			wantTitle:     "Settings Center",
			wantOutputSub: "/config wizard",
		},
		{
			name:          "config show returns current config text when not loaded",
			input:         "/config show",
			wantOutputSub: "配置未加载",
		},
		{
			name:          "init shows init center",
			input:         "/init",
			wantOutputSub: "Init Center",
		},
		{
			name:       "init profile aliases soft",
			input:      "/init profile",
			wantAction: "init-wizard",
		},
		{
			name:          "unknown command returns help hint",
			input:         "/does-not-exist",
			wantOutputSub: "Unknown command: /does-not-exist. Type /help for available commands.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispatcher.Dispatch(command.Context{}, tt.input)
			if tt.wantAction != "" && got.Action != tt.wantAction {
				t.Fatalf("action = %q, want %q", got.Action, tt.wantAction)
			}
			if tt.wantTitle != "" && got.OutputTitle != tt.wantTitle {
				t.Fatalf("output title = %q, want %q", got.OutputTitle, tt.wantTitle)
			}
			if tt.wantOutputSub != "" && !strings.Contains(got.Output, tt.wantOutputSub) {
				t.Fatalf("output = %q, want substring %q", got.Output, tt.wantOutputSub)
			}
		})
	}
}

type argsEchoCmd struct{}

func (argsEchoCmd) Name() string        { return "echoargs" }
func (argsEchoCmd) Description() string { return "echo args" }
func (argsEchoCmd) Execute(ctx command.Context) command.Result {
	return command.Result{Output: ctx.Args}
}

func TestDispatcher_PreservesQuotedRawArgs(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(argsEchoCmd{})
	dispatcher := command.NewDispatcher(reg)
	got := dispatcher.Dispatch(command.Context{}, `/echoargs "a b" c`)
	if got.Output != `"a b" c` {
		t.Fatalf("output=%q", got.Output)
	}
}
