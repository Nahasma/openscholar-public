package tools

import (
	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/llm/tools/codeagent"
	"github.com/Nahasma/openscholar-public/internal/llm/web"
	"github.com/Nahasma/openscholar-public/internal/memory"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
)

// KBServices holds knowledge base dependencies for tool registration.
type KBServices struct {
	KB         kb.Service
	Indexer    kb.Indexer
	CallLLM    LLMCaller
	MemService memory.Service
	WebRuntime *web.Runtime
}

// NewRegistry returns the full tool set for General / legacy usage.
func NewRegistry(perms permission.Service, kbs *KBServices) []BaseTool {
	t := []BaseTool{
		NewViewTool(perms),
		NewEditTool(perms),
		NewWriteTool(perms),
		NewBashTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewImageGenTool(perms),
		NewDiagramGenTool(perms),
		NewPaperValidateTool(perms),
		NewScholarSearchTool(perms),
	}
	if kbs != nil && kbs.KB != nil {
		t = append(t,
			NewKBAddTool(kbs.KB, kbs.Indexer),
			NewKBListTool(kbs.KB),
			NewKBTreeTool(kbs.KB),
		)
		if kbs.CallLLM != nil {
			t = append(t,
				NewKBQueryTool(kbs.KB, kbs.CallLLM, kbs.MemService),
				NewKBSearchTool(kbs.KB, kbs.CallLLM, kbs.MemService),
				NewKBHealthTool(kbs.KB),
				NewKBRepairTool(kbs.KB),
				NewKBReindexTool(kbs.KB),
			)
		}
	}
	return t
}

// NewExploreRegistry returns a read-only tool set (3 tools) for Explore Agent.
func NewExploreRegistry(perms permission.Service) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
	}
}

// NewPlanRegistry returns planning-focused read-only tools.
func NewPlanRegistry(perms permission.Service) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
	}
}

// NewVerifyRegistry returns verification-focused read-only tools.
func NewVerifyRegistry(perms permission.Service) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
	}
}

// NewResearchSearchRegistry returns a read-only noisy-search worker tool set.
// It is intentionally narrower than the general worker registry so broad
// literature/web exploration can happen in a child context without granting
// file mutation or nested Task delegation.
func NewResearchSearchRegistry(perms permission.Service, kbs *KBServices) []BaseTool {
	t := []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewScholarSearchTool(perms),
	}
	if kbs != nil && kbs.KB != nil {
		t = append(t,
			NewKBListTool(kbs.KB),
			NewKBTreeTool(kbs.KB),
		)
		if kbs.CallLLM != nil {
			t = append(t,
				NewKBQueryTool(kbs.KB, kbs.CallLLM, kbs.MemService),
				NewKBSearchTool(kbs.KB, kbs.CallLLM, kbs.MemService),
			)
		}
	}
	if kbs != nil && kbs.WebRuntime != nil {
		t = append(t,
			NewWebSearchTool(perms, kbs.WebRuntime),
			NewWebFetchTool(perms, kbs.WebRuntime),
		)
	}
	return t
}

// NewCoordinatorRegistry returns coordinator tools: read + delegation.
func NewCoordinatorRegistry(perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewTaskToolWithKB(perms, sessions, messages, runAgent, nil),
	}
}

// LeaderTools returns the Leader Agent's read-only + dispatch tool set.
func LeaderTools(perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewEditTool(perms),
		NewWriteTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewTaskToolWithKB(perms, sessions, messages, runAgent, nil),
	}
}

// ResearchLeaderTools returns the fail-closed research leader tool set.
// Research leaders orchestrate and review, but must delegate file writes to
// workers instead of using Write/Edit directly.
func ResearchLeaderTools(perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewTaskToolWithKB(perms, sessions, messages, runAgent, nil),
	}
}

// NewExperimentWorkerRegistry returns the experiment-phase worker tool set.
func NewExperimentWorkerRegistry(perms permission.Service, registry *codeagent.Registry) []BaseTool {
	return []BaseTool{
		NewViewTool(perms),
		NewGlobTool(perms),
		NewGrepTool(perms),
		NewExperimentBriefTool(),
		NewExperimentDispatchTool(registry),
		NewExperimentCollectTool(),
		NewExperimentAssessTool(),
		NewExperimentRecoverTool(),
	}
}

// NewCoderRegistry returns the Coder Agent tool set as a DeferredRegistry.
// Core tools (6 file ops) are always sent to LLM; extended tools are deferred
// and activated on-demand via ToolSearch.
func NewCoderRegistry(deps ToolDeps) *DeferredRegistry {
	core := registerCoreTools(deps)

	deferred := make([]BaseTool, 0)
	deferred = append(deferred, registerScholarTools(deps)...)
	deferred = append(deferred, NewTaskToolWithKB(deps.Perms, deps.Sessions, deps.Messages, deps.RunAgent, deps.KBs))
	deferred = append(deferred, registerKnowledgeTools(deps)...)
	deferred = append(deferred, registerResearchTools(deps)...)
	deferred = append(deferred, registerIntegrationTools(deps)...)
	deferred = append(deferred, registerWebTools(deps)...)
	deferred = append(deferred, registerDocxTools(deps)...)

	return NewDeferredRegistry(core, deferred)
}
