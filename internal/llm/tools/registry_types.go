package tools

import (
	"github.com/openscholar/openscholar/internal/evolution"
	"github.com/openscholar/openscholar/internal/llm/tools/codeagent"
	"github.com/openscholar/openscholar/internal/llm/web"
	"github.com/openscholar/openscholar/internal/memory"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/skillbank"
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
