package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/config"
)

func loadPromptTestConfig(t *testing.T, orchestrationEnabled bool) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)
	wd := t.TempDir()
	cfg := `{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"model": "qwen2.5-coder:latest"}
  },
  "subagent_orchestration": {
    "enabled": ` + map[bool]string{true: "true", false: "false"}[orchestrationEnabled] + `
  }
}`
	if err := os.MkdirAll(filepath.Dir(config.ConfigFilePath(wd)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(config.ConfigFilePath(wd), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(wd); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func TestGetAgentPrompt_LeaderIsDistinctFromCoordinator(t *testing.T) {
	leaderPrompt := GetAgentPrompt(config.AgentLeader)
	coordinatorPrompt := GetAgentPrompt(config.AgentCoordinator)

	if !strings.Contains(leaderPrompt, "Project Leader") {
		t.Fatalf("expected leader prompt to use leader module, got: %s", leaderPrompt)
	}
	if strings.Contains(leaderPrompt, "ResearchControl(") {
		t.Fatalf("leader prompt should not reference unavailable ResearchControl tool, got: %s", leaderPrompt)
	}
	if !strings.Contains(leaderPrompt, `评估/验收类任务：agent_type="verify"`) {
		t.Fatalf("leader prompt should route evaluation through verify worker, got: %s", leaderPrompt)
	}
	if strings.Contains(leaderPrompt, "Evaluator Agent") {
		t.Fatalf("leader prompt should not steer evaluation through legacy evaluator agent, got: %s", leaderPrompt)
	}
	if !strings.Contains(coordinatorPrompt, "Coordinator agent") {
		t.Fatalf("expected coordinator prompt to use coordinator module, got: %s", coordinatorPrompt)
	}
	if leaderPrompt == coordinatorPrompt {
		t.Fatal("expected leader and coordinator prompts to differ")
	}
}

func TestGetAgentPrompt_CoderIncludesSkillOpportunityAndDeferredSkillTools(t *testing.T) {
	p := GetAgentPrompt(config.AgentCoder)
	for _, want := range []string{
		"Skill Opportunity Detection",
		"at least two signals",
		"ToolSearch",
		"SkillQuery",
		"SkillManage",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("expected coder prompt to include %q", want)
		}
	}
}

func TestGetAgentPrompt_CoderTaskGuidanceStaysInLegacyPollingMode(t *testing.T) {
	loadPromptTestConfig(t, false)
	p := GetAgentPrompt(config.AgentCoder)

	for _, want := range []string{
		"TaskV2 Map-Reduce",
		"Task read/list",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("expected coder prompt to include %q", want)
		}
	}

	for _, unwanted := range []string{
		"<task-notification>",
		"task notification",
		"wait for notification",
		"no-poll",
		"no poll",
		"do not poll",
		"don't poll",
		"不要轮询",
		"不轮询",
	} {
		if strings.Contains(strings.ToLower(p), strings.ToLower(unwanted)) {
			t.Fatalf("coder prompt should not claim notification/no-poll semantics yet; found %q", unwanted)
		}
	}
}

func TestGetAgentPrompt_CoderTaskGuidanceUsesV2NotificationModeWhenEnabled(t *testing.T) {
	loadPromptTestConfig(t, true)
	p := GetAgentPrompt(config.AgentCoder)

	for _, want := range []string{
		"Subagent Orchestration V2",
		"<task-notification>",
		"Do not sleep, poll Task list/read",
		"Do not read child transcript/debug_transcript",
		"Writable workers require write_set",
		"Verification should be fresh and independent",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("expected coder prompt to include %q", want)
		}
	}
}

func TestGetAgentPrompt_CoderResearchModePrefersTaskV2LeaderVerifyPipeline(t *testing.T) {
	loadPromptTestConfig(t, false)
	p := GetAgentPrompt(config.AgentCoder)

	if !strings.Contains(p, "TaskV2 + leader/verify + ResearchPipeline") {
		t.Fatalf("expected research mode guidance to prefer TaskV2 leader/verify pipeline, got: %s", p)
	}
	if strings.Contains(p, "新流程优先使用 ResearchPipeline/ResearchTask/ResearchMessage") {
		t.Fatalf("research mode guidance should not prefer legacy research facade tools, got: %s", p)
	}
}
