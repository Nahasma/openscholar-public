package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	agentcustom "github.com/Nahasma/openscholar-public/internal/llm/agent/custom"
	"github.com/Nahasma/openscholar-public/internal/llm/web"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

func TestSubagentProfileResolverBuiltInDefaults(t *testing.T) {
	resolver := NewSubagentProfileResolver(&config.Config{})

	tests := []struct {
		agentType  string
		wantAgent  config.AgentName
		wantMode   permission.Mode
		wantVerify string
		wantSpawn  bool
	}{
		{agentType: "general", wantAgent: config.AgentGeneral, wantMode: permission.ModeDefault, wantVerify: "none"},
		{agentType: "research", wantAgent: config.AgentGeneral, wantMode: permission.ModeDefault, wantVerify: "none"},
		{agentType: "leader", wantAgent: config.AgentLeader, wantMode: permission.ModeDefault, wantVerify: "required", wantSpawn: true},
		{agentType: "plan", wantAgent: config.AgentPlan, wantMode: permission.ModePlan, wantVerify: "required"},
		{agentType: "verify", wantAgent: config.AgentVerify, wantMode: permission.ModePlan, wantVerify: "none"},
		{agentType: "coordinator", wantAgent: config.AgentCoordinator, wantMode: permission.ModePlan, wantVerify: "required", wantSpawn: true},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			prof, err := resolver.Resolve(tt.agentType, nil)
			if err != nil {
				t.Fatalf("resolve error: %v", err)
			}
			if prof.AgentName != tt.wantAgent {
				t.Fatalf("agent=%q want %q", prof.AgentName, tt.wantAgent)
			}
			if prof.SessionMode != tt.wantMode {
				t.Fatalf("mode=%q want %q", prof.SessionMode, tt.wantMode)
			}
			if prof.VerifyPolicy != tt.wantVerify {
				t.Fatalf("verify=%q want %q", prof.VerifyPolicy, tt.wantVerify)
			}
			if prof.CanSpawnTask != tt.wantSpawn {
				t.Fatalf("can_spawn_task=%v want %v", prof.CanSpawnTask, tt.wantSpawn)
			}
		})
	}
}

func TestResearchSubagentProfileIsSearchOnly(t *testing.T) {
	resolver := NewSubagentProfileResolver(&config.Config{})
	prof, err := resolver.Resolve("research", nil)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if prof.AgentName != config.AgentGeneral {
		t.Fatalf("agent=%q want general", prof.AgentName)
	}
	if prof.CanWriteFiles {
		t.Fatal("research search worker must not be writable")
	}
	if prof.CanSpawnTask {
		t.Fatal("research search worker must not spawn nested tasks")
	}
	if prof.ResultMaxChars != researchSearchWorkerResultChars || prof.MaxTurns != researchSearchWorkerMaxTurns {
		t.Fatalf("unexpected result/turn limits: result=%d turns=%d", prof.ResultMaxChars, prof.MaxTurns)
	}
	if prof.PromptPrefix == "" || !strings.Contains(prof.PromptPrefix, "candidate_papers") || !strings.Contains(prof.PromptPrefix, "evidence_table") {
		t.Fatalf("research prompt prefix missing compressed output schema:\n%s", prof.PromptPrefix)
	}
}

func TestResearchSearchRegistryIncludesSearchToolsWithoutMutationOrTask(t *testing.T) {
	taskTool := &taskTool{
		permissions: permission.NewPermissionService(),
		kbs: &KBServices{
			WebRuntime: web.NewRuntime(nil, nil, web.DefaultConfig()),
		},
		profiles: NewSubagentProfileResolver(&config.Config{}),
	}
	resolved, err := taskTool.resolveTaskAgent("research")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	agentTools := taskTool.selectAgentToolsForResolved(context.Background(), resolved)
	seen := map[string]bool{}
	for _, tool := range agentTools {
		seen[tool.Info().Name] = true
	}
	for _, want := range []string{"View", "Glob", "Grep", "ScholarSearch", "WebSearch", "WebFetch"} {
		if !seen[want] {
			t.Fatalf("expected research worker tool %s in %v", want, seen)
		}
	}
	for _, forbidden := range []string{"Write", "Edit", "Bash", "Task", "KBAdd"} {
		if seen[forbidden] {
			t.Fatalf("research worker should not include %s in %v", forbidden, seen)
		}
	}
}

func TestSubagentProfileResolverCustomOmittedTools(t *testing.T) {
	cfg := &agentcustom.AgentConfig{Name: "custom", ToolsExplicit: false}

	legacy := NewSubagentProfileResolver(&config.Config{})
	legacyProf, err := legacy.Resolve("custom", cfg)
	if err != nil {
		t.Fatalf("legacy resolve error: %v", err)
	}
	if legacyProf.ToolsExplicit {
		t.Fatalf("expected legacy omitted tools to stay non-explicit")
	}
	if legacyProf.SessionMode != permission.ModeDefault {
		t.Fatalf("expected legacy mode default, got %q", legacyProf.SessionMode)
	}

	enabled := NewSubagentProfileResolver(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}})
	enabledProf, err := enabled.Resolve("custom", cfg)
	if err != nil {
		t.Fatalf("enabled resolve error: %v", err)
	}
	if !enabledProf.ToolsExplicit {
		t.Fatalf("expected v2 omitted tools to become explicit")
	}
	if enabledProf.SessionMode != permission.ModePlan {
		t.Fatalf("expected v2 mode plan, got %q", enabledProf.SessionMode)
	}
	if len(enabledProf.AllowedTools) != 3 || enabledProf.AllowedTools[0] != "View" || enabledProf.AllowedTools[1] != "Glob" || enabledProf.AllowedTools[2] != "Grep" {
		t.Fatalf("unexpected v2 read-only defaults: %v", enabledProf.AllowedTools)
	}
}

func TestSubagentProfileResolverAppliesEnabledProfileConfig(t *testing.T) {
	resolver := NewSubagentProfileResolver(&config.Config{
		SubagentOrchestration: config.SubagentOrchestrationConfig{
			Enabled: true,
			Profiles: map[string]config.SubagentProfileConfig{
				"general": {
					AgentName:      "plan",
					ModelTier:      "fast",
					Model:          "gpt-test",
					AllowedTools:   []string{"View", "Task"},
					DeniedTools:    []string{"Task"},
					PermissionMode: "plan",
					MaxConcurrent:  3,
					VerifyPolicy:   "required",
					ResultMaxChars: 1000,
					TimeoutSeconds: 42,
					MaxTurns:       9,
				},
			},
		},
	})

	prof, err := resolver.Resolve("general", nil)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if prof.AgentName != config.AgentPlan {
		t.Fatalf("agent name = %q, want %q", prof.AgentName, config.AgentPlan)
	}
	if prof.ModelTier != "fast" || prof.Model != "gpt-test" {
		t.Fatalf("model override not applied: tier=%q model=%q", prof.ModelTier, prof.Model)
	}
	if !prof.ToolsExplicit || len(prof.AllowedTools) != 2 || prof.AllowedTools[0] != "View" || prof.AllowedTools[1] != "Task" {
		t.Fatalf("allowed tools override not applied: explicit=%v tools=%v", prof.ToolsExplicit, prof.AllowedTools)
	}
	if len(prof.DeniedTools) != 1 || prof.DeniedTools[0] != "Task" {
		t.Fatalf("denied tools override not applied: %v", prof.DeniedTools)
	}
	if prof.SessionMode != permission.ModePlan {
		t.Fatalf("mode=%q want %q", prof.SessionMode, permission.ModePlan)
	}
	if prof.MaxConcurrent != 3 || prof.VerifyPolicy != "required" || prof.ResultMaxChars != 1000 || prof.Timeout != 42*time.Second || prof.MaxTurns != 9 {
		t.Fatalf("numeric/policy overrides not applied: %#v", prof)
	}
}

func TestSubagentProfileResolverIgnoresProfileConfigWhenDisabled(t *testing.T) {
	resolver := NewSubagentProfileResolver(&config.Config{
		SubagentOrchestration: config.SubagentOrchestrationConfig{
			Enabled: false,
			Profiles: map[string]config.SubagentProfileConfig{
				"general": {
					AllowedTools:   []string{"View"},
					PermissionMode: "plan",
					VerifyPolicy:   "required",
				},
			},
		},
	})

	prof, err := resolver.Resolve("general", nil)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if prof.ToolsExplicit {
		t.Fatalf("disabled profile config should not make tools explicit")
	}
	if prof.SessionMode != permission.ModeDefault {
		t.Fatalf("disabled profile config should not change mode, got %q", prof.SessionMode)
	}
	if prof.VerifyPolicy != "none" {
		t.Fatalf("disabled profile config should not change verify policy, got %q", prof.VerifyPolicy)
	}
}

func TestTaskProfileConfigFiltersAllowedAndDeniedTools(t *testing.T) {
	taskTool := &taskTool{
		permissions: permission.NewPermissionService(),
		profiles: NewSubagentProfileResolver(&config.Config{
			SubagentOrchestration: config.SubagentOrchestrationConfig{
				Enabled: true,
				Profiles: map[string]config.SubagentProfileConfig{
					"general": {
						AllowedTools: []string{"View", "Task"},
						DeniedTools:  []string{"Task"},
					},
				},
			},
		}),
	}

	resolved, err := taskTool.resolveTaskAgent("general")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	allowedRuntimeTools := resolved.runtimeAllowedTools()
	if len(allowedRuntimeTools) != 1 || allowedRuntimeTools[0] != "View" {
		t.Fatalf("expected runtime allowed tools to exclude denied Task, got %v", allowedRuntimeTools)
	}
	agentTools := taskTool.selectAgentToolsForResolved(context.Background(), resolved)
	if len(agentTools) != 1 || agentTools[0].Info().Name != "View" {
		var names []string
		for _, tool := range agentTools {
			names = append(names, tool.Info().Name)
		}
		t.Fatalf("expected allowed tools filtered by denied tools to leave only View, got %v", names)
	}
}
