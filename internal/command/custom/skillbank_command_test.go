package custom

import (
	"context"
	"testing"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/skillbank"
)

type skillCommandTestService struct {
	metas map[string]skillbank.SkillMeta
	body  map[string]string
}

func (s *skillCommandTestService) Search(context.Context, skillbank.QueryOptions) ([]skillbank.Skill, error) {
	return nil, nil
}
func (s *skillCommandTestService) SearchMetadata(context.Context, skillbank.QueryOptions) ([]skillbank.SkillSearchHit, error) {
	return nil, nil
}
func (s *skillCommandTestService) View(_ context.Context, id string) (*skillbank.Skill, error) {
	return &skillbank.Skill{ID: id, Instruction: s.body[id]}, nil
}
func (s *skillCommandTestService) Get(context.Context, string) (*skillbank.Skill, error) {
	return nil, nil
}
func (s *skillCommandTestService) List(context.Context, string) ([]skillbank.Skill, error) {
	return nil, nil
}
func (s *skillCommandTestService) ListMetadata(context.Context) ([]skillbank.SkillMeta, error) {
	out := make([]skillbank.SkillMeta, 0, len(s.metas))
	for _, m := range s.metas {
		out = append(out, m)
	}
	return out, nil
}
func (s *skillCommandTestService) Create(context.Context, skillbank.Skill) error         { return nil }
func (s *skillCommandTestService) Update(context.Context, string, skillbank.Skill) error { return nil }
func (s *skillCommandTestService) Delete(context.Context, string) error                  { return nil }
func (s *skillCommandTestService) RecordUsage(context.Context, string, bool) error       { return nil }
func (s *skillCommandTestService) ImportFromDir(context.Context, string) (int, []error) {
	return 0, nil
}
func (s *skillCommandTestService) Reindex(context.Context) error    { return nil }
func (s *skillCommandTestService) SetLLMCaller(skillbank.LLMCaller) {}

type staticCommand struct{ name string }

func (c staticCommand) Name() string        { return c.name }
func (c staticCommand) Description() string { return "static" }
func (c staticCommand) Execute(command.Context) command.Result {
	return command.Result{Prompt: "static"}
}

func TestLoadSkillBankCommands_RegistersOnlyUserInvocable(t *testing.T) {
	reg := command.NewRegistry()
	svc := &skillCommandTestService{
		metas: map[string]skillbank.SkillMeta{
			"workflow/use_me": {
				ID:                     "workflow/use_me",
				UserInvocable:          true,
				Description:            "run it",
				WhenToUse:              "use it now",
				AllowedTools:           []string{"View"},
				Model:                  "gpt-5.4",
				DisableModelInvocation: true,
			},
			"workflow/conditional": {
				ID:            "workflow/conditional",
				UserInvocable: true,
				Paths:         []string{"**/*.tex"},
			},
			"workflow/hidden": {ID: "workflow/hidden", UserInvocable: false},
		},
		body: map[string]string{"workflow/use_me": "hello $ARGUMENTS"},
	}

	LoadSkillBankCommands(reg, svc)

	if reg.Get("use_me") == nil {
		t.Fatalf("expected /use_me to be registered")
	}
	if reg.Get("conditional") != nil {
		t.Fatalf("expected conditional command to be deferred until path activation")
	}
	if reg.Get("hidden") != nil {
		t.Fatalf("expected /hidden to be skipped")
	}

	cmd := reg.Get("use_me")
	res := cmd.Execute(command.Context{Args: "world"})
	if res.Prompt != "hello world" {
		t.Fatalf("prompt mismatch: %q", res.Prompt)
	}
	if res.Runtime == nil {
		t.Fatalf("expected runtime override metadata")
	}
	if len(res.Runtime.AllowedTools) != 1 || res.Runtime.AllowedTools[0] != "View" {
		t.Fatalf("unexpected runtime allowed tools: %#v", res.Runtime.AllowedTools)
	}
	if res.Runtime.Model != "gpt-5.4" {
		t.Fatalf("unexpected runtime model: %q", res.Runtime.Model)
	}
	if res.Runtime.DisableModelInvocation {
		t.Fatalf("skill slash commands should not disable the user-triggered model run")
	}
	if withWhen, ok := cmd.(interface{ WhenToUse() string }); !ok || withWhen.WhenToUse() != "use it now" {
		t.Fatalf("expected WhenToUse metadata to be exposed")
	}
	if withAllowed, ok := cmd.(interface{ AllowedTools() []string }); !ok || len(withAllowed.AllowedTools()) != 1 || withAllowed.AllowedTools()[0] != "View" {
		t.Fatalf("expected AllowedTools metadata to be exposed")
	}
	if withModel, ok := cmd.(interface{ Model() string }); !ok || withModel.Model() != "gpt-5.4" {
		t.Fatalf("expected Model metadata to be exposed")
	}
	if withSpec, ok := cmd.(interface{ Spec() command.CommandSpec }); !ok {
		t.Fatalf("expected Spec provider")
	} else {
		spec := withSpec.Spec()
		if spec.Source != "skill" {
			t.Fatalf("source = %q, want skill", spec.Source)
		}
		if spec.WhenToUse != "use it now" {
			t.Fatalf("when_to_use = %q", spec.WhenToUse)
		}
	}
}

func TestLoadSkillBankCommands_SkipsNameCollision(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(staticCommand{name: "use_me"})
	svc := &skillCommandTestService{
		metas: map[string]skillbank.SkillMeta{
			"workflow/use_me": {ID: "workflow/use_me", UserInvocable: true},
		},
		body: map[string]string{"workflow/use_me": "skill"},
	}

	LoadSkillBankCommands(reg, svc)

	res := reg.Get("use_me").Execute(command.Context{})
	if res.Prompt != "static" {
		t.Fatalf("existing command should win on collision, got %#v", res)
	}
}

func TestSkillActivator_RegistersConditionalCommandOnMatchingPath(t *testing.T) {
	reg := command.NewRegistry()
	svc := &skillCommandTestService{
		metas: map[string]skillbank.SkillMeta{
			"workflow/latex_helper": {
				ID:            "workflow/latex_helper",
				UserInvocable: true,
				Description:   "latex helper",
				Paths:         []string{"**/*.tex"},
			},
			"workflow/no_match": {
				ID:            "workflow/no_match",
				UserInvocable: true,
				Paths:         []string{"**/*.md"},
			},
		},
		body: map[string]string{
			"workflow/latex_helper": "latex $ARGUMENTS",
			"workflow/no_match":     "md $ARGUMENTS",
		},
	}

	LoadSkillBankCommands(reg, svc)
	activator := NewSkillActivator(svc, reg)
	activator.OnFileToolUsage("session-1", tools.FileToolUsageEvent{
		ToolName:  "View",
		Workspace: "/tmp/workspace",
		Paths:     []string{"/tmp/workspace/sections/main.tex"},
	})

	cmd := reg.Get("latex_helper")
	if cmd == nil {
		t.Fatalf("expected conditional skill command to be activated")
	}
	if reg.Get("no_match") != nil {
		t.Fatalf("expected unmatched conditional command to stay inactive")
	}
	if got := cmd.Execute(command.Context{Args: "diagram"}).Prompt; got != "latex diagram" {
		t.Fatalf("unexpected activated prompt: %q", got)
	}

	before := len(reg.List())
	activator.OnFileToolUsage("session-1", tools.FileToolUsageEvent{
		ToolName:  "Grep",
		Workspace: "/tmp/workspace",
		Paths:     []string{"/tmp/workspace/sections/main.tex"},
	})
	after := len(reg.List())
	if after != before {
		t.Fatalf("expected idempotent activation, before=%d after=%d", before, after)
	}
}
