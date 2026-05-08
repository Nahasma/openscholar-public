package app

import (
	"github.com/openscholar/openscholar/internal/llm/agent"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	"github.com/openscholar/openscholar/internal/session"
)

// Verify at compile time that *App satisfies the narrow interfaces.
var _ ConversationService = (*App)(nil)
var _ EventBrokers = (*App)(nil)
var _ ResearchService = (*App)(nil)

// --- ConversationService ---

// AgentService returns the coder-agent service.
func (a *App) AgentService() agent.Service { return a.CoderAgent }

// SessionService returns the session management service.
func (a *App) SessionService() session.Service { return a.Sessions }

// MessageService returns the message storage service.
func (a *App) MessageService() message.Service { return a.Messages }

// PermissionService returns the permission enforcement service.
func (a *App) PermissionService() permission.Service { return a.Permissions }

// --- EventBrokers ---

// ClarificationEvents returns the pub/sub broker for tool clarification events.
func (a *App) ClarificationEvents() *pubsub.Broker[tools.ClarificationEvent] {
	return a.ClarificationBroker
}

// CheckpointEvents returns the pub/sub broker for research checkpoint events.
func (a *App) CheckpointEvents() *pubsub.Broker[tools.CheckpointEvent] {
	return a.CheckpointBroker
}

// --- ResearchService ---

// Research returns the research pipeline engine, or nil if not initialised.
func (a *App) Research() *research.Engine { return a.ResearchEngine }
