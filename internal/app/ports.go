package app

import (
	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/research"
	"github.com/Nahasma/openscholar-public/internal/session"
)

// ConversationService groups the core subsystems needed by TUI, command handlers,
// and the HTTP server to drive a conversation session.
//
// Callers that previously held *App directly should depend on this interface
// when they only need conversation-level access.
type ConversationService interface {
	// AgentService returns the coder-agent service.
	AgentService() agent.Service
	// SessionService returns the session management service.
	SessionService() session.Service
	// MessageService returns the message storage service.
	MessageService() message.Service
	// PermissionService returns the permission enforcement service.
	PermissionService() permission.Service
}

// EventBrokers provides access to the application-level pub/sub brokers
// that deliver async events to the TUI and CLI subscribers.
type EventBrokers interface {
	// ClarificationEvents returns the broker for tool clarification events.
	ClarificationEvents() *pubsub.Broker[tools.ClarificationEvent]
	// CheckpointEvents returns the broker for research checkpoint events.
	CheckpointEvents() *pubsub.Broker[tools.CheckpointEvent]
}

// ResearchService provides access to the research pipeline engine.
// TUI wizards and helpers use these methods to create and inspect pipelines.
type ResearchService interface {
	// Research returns the underlying research engine, or nil if not initialised.
	Research() *research.Engine
}
