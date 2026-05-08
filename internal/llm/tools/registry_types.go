package tools

import (
	"github.com/Nahasma/openscholar-public/internal/evolution"
	"github.com/Nahasma/openscholar-public/internal/llm/tools/codeagent"
	"github.com/Nahasma/openscholar-public/internal/llm/web"
	"github.com/Nahasma/openscholar-public/internal/memory"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/plan"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

// ToolDeps holds all dependencies required to build the Coder Agent tool registry.
// It replaces the 12+ individual parameters of NewCoderRegistry.
type ToolDeps struct {
	Perms            permission.Service
	Sessions         session.Service
	Messages         message.Service
	RunAgent         AgentRunner
	AskBroker        *pubsub.Broker[ClarificationEvent]
	Plans            plan.Service
	PlanBroker       *pubsub.Broker[PlanApprovalEvent]
	KBs              *KBServices
	CallLLM          LLMCaller
	MemService       memory.Service
	Skills           skillbank.Service
	EvoService       evolution.Service
	ResearchCtrl     ResearchController
	CheckpointBroker *pubsub.Broker[CheckpointEvent]
	MCPCaller        codeagent.MCPCaller
	WebRuntime       *web.Runtime // nil = no web tools
}
