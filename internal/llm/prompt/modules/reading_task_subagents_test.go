package modules

import (
	"strings"
	"testing"
)

func TestCrossReadingModule_TaskV2ReaderGuidance(t *testing.T) {
	content := NewCrossReadingModule().Content()
	for _, want := range []string{
		"TaskV2 Map-Reduce",
		`Task action=create with agent_type="reader"`,
		"max 5 concurrent workers",
		"max 3 concurrent workers",
		"Task read/list",
		"paper_id",
		"title",
		"coverage",
		"evidence (with page/node refs)",
		"claims",
		"method",
		"results",
		"limitations",
		"uncertainties",
		"not_covered",
		"may use ONLY:",
		"View",
		"KBList",
		"KBTree",
		"KBQuery",
		"KBSearch",
		"MUST NOT use:",
		"Write, Edit, Bash",
		"ScholarSearch",
		"KBAdd",
		"rate_limited/provider_cooldown",
		"do NOT spawn search workers",
		"basic/degraded/summary-only index quality",
		"never claim full-paper reading",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %q in cross reading prompt:\n%s", want, content)
		}
	}
}

func TestToolsAndBaseReadingGuidance(t *testing.T) {
	toolsContent := NewToolsModule().Content()
	for _, want := range []string{
		"需要系统化阅读",
		"Reader Agent",
		"最多 5 并发",
		"最多 3 并发",
		"rate_limited/provider_cooldown",
	} {
		if !strings.Contains(toolsContent, want) {
			t.Fatalf("expected %q in tools prompt:\n%s", want, toolsContent)
		}
	}

	baseContent := NewBasePromptModule().Content()
	for _, want := range []string{
		"In non-research mode, for systematic reading",
		"degraded",
		"not_covered",
		"never claim full-paper reading",
	} {
		if !strings.Contains(baseContent, want) {
			t.Fatalf("expected %q in base prompt:\n%s", want, baseContent)
		}
	}
}

func TestScholarAndSubagentPromptsRequireResearchSearchDelegation(t *testing.T) {
	scholar := NewScholarSearchModule().Content()
	for _, want := range []string{
		"长调研 / Broad Search 委派规则",
		`Task] action="create" agent_type="research"`,
		"candidate_papers",
		"evidence_table",
		"预计需要超过 2 次 ScholarSearch/WebSearch/KBSearch",
		"通用路由决策表",
		"公共教学/常识解释：先直接回答",
		"用户本地材料",
	} {
		if !strings.Contains(scholar, want) {
			t.Fatalf("expected %q in scholar prompt:\n%s", want, scholar)
		}
	}

	orch := NewSubagentOrchestrationModule(true).Content()
	for _, want := range []string{
		`agent_type="research"`,
		"compressed candidate papers",
		"evidence tables",
		"do not paste raw search dumps",
	} {
		if !strings.Contains(orch, want) {
			t.Fatalf("expected %q in subagent prompt:\n%s", want, orch)
		}
	}
}
