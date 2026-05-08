package modules

import (
	"strings"
	"testing"
)

func TestSkillOpportunityModule_Basics(t *testing.T) {
	m := NewSkillOpportunityModule()
	if m.Name() != "skill_opportunity" {
		t.Fatalf("expected module name skill_opportunity, got %q", m.Name())
	}
	if m.Priority() != 9 {
		t.Fatalf("expected priority 9, got %d", m.Priority())
	}
	content := m.Content()
	for _, want := range []string{
		"at least two signals",
		"ToolSearch",
		"SkillQuery",
		"SkillManage",
		"AskUser",
		"RecordFeedback",
		"explicit user confirmation",
		"Before create, run SkillQuery duplicate search",
		"post-run skill review",
		"Research mode write block still applies",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %q in module content:\n%s", want, content)
		}
	}
	for _, avoid := range []string{
		"one-off tasks",
		"sensitive/private",
		"rejected this opportunity",
	} {
		if !strings.Contains(content, avoid) {
			t.Fatalf("expected avoid-case %q in module content:\n%s", avoid, content)
		}
	}
}
