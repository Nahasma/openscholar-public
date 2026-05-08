package agent

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/tools"
)

func TestPreActivateToolsByIntent_ReadingActivatesTaskAndScholarOnly(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		[]tools.BaseTool{
			stubTool{name: "Task"},
			stubTool{name: "KBAdd"},
			stubTool{name: "KBQuery"},
			stubTool{name: "KBSearch"},
			stubTool{name: "KBTree"},
			stubTool{name: "KBList"},
			stubTool{name: "ScholarSearch"},
			stubTool{name: "WebSearch"},
		},
	)
	ag := &agent{registry: reg}

	ag.preActivateToolsByIntent("请搜索并阅读三篇论文，比较方法和实验结论")

	active := map[string]bool{}
	for _, tool := range reg.ActiveTools() {
		active[tool.Info().Name] = true
	}
	for _, want := range []string{"Task", "ScholarSearch"} {
		if !active[want] {
			t.Fatalf("expected reading intent to activate %s, active=%v", want, active)
		}
	}
	for _, blocked := range []string{"KBAdd", "KBQuery", "KBSearch", "KBTree", "KBList"} {
		if active[blocked] {
			t.Fatalf("generic reading intent should not activate %s, active=%v", blocked, active)
		}
	}
	if !active["WebSearch"] {
		t.Fatalf("expected explicit search intent to keep web activation too, active=%v", active)
	}
}

func TestPreActivateToolsByIntent_LocalKnowledgeBaseActivatesOnlyKBGroup(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		[]tools.BaseTool{
			stubTool{name: "Task"},
			stubTool{name: "KBAdd"},
			stubTool{name: "KBQuery"},
			stubTool{name: "KBSearch"},
			stubTool{name: "KBTree"},
			stubTool{name: "KBList"},
			stubTool{name: "ScholarSearch"},
		},
	)
	ag := &agent{registry: reg}

	ag.preActivateToolsByIntent("列出我的知识库里已上传的文档")

	active := map[string]bool{}
	for _, tool := range reg.ActiveTools() {
		active[tool.Info().Name] = true
	}
	if active["Task"] || active["ScholarSearch"] {
		t.Fatalf("knowledge-base intent should not activate Task/ScholarSearch, active=%v", active)
	}
	for _, want := range []string{"KBAdd", "KBQuery", "KBSearch", "KBTree", "KBList"} {
		if !active[want] {
			t.Fatalf("expected kb intent to activate %s, active=%v", want, active)
		}
	}
}

func TestPreActivateToolsByIntent_PublicKnowledgeBaseExplanationDoesNotActivateKB(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		[]tools.BaseTool{
			stubTool{name: "KBAdd"},
			stubTool{name: "KBQuery"},
			stubTool{name: "KBSearch"},
			stubTool{name: "KBTree"},
			stubTool{name: "KBList"},
		},
	)
	ag := &agent{registry: reg}

	ag.preActivateToolsByIntent("知识库是什么？介绍一下概念")

	active := map[string]bool{}
	for _, tool := range reg.ActiveTools() {
		active[tool.Info().Name] = true
	}
	for _, blocked := range []string{"KBAdd", "KBQuery", "KBSearch", "KBTree", "KBList"} {
		if active[blocked] {
			t.Fatalf("public knowledge-base explanation should not activate %s, active=%v", blocked, active)
		}
	}
}
