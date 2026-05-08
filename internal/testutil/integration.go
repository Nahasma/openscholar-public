package testutil

import (
	"context"
	"sync"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/bib"
	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/research"
	"github.com/Nahasma/openscholar-public/internal/session"
)

// TestServices bundles the core services that can be constructed without a
// running LLM provider or external config.  It is intentionally lighter than
// app.App so that integration tests remain fast and hermetic.
type TestServices struct {
	Sessions    session.Service
	Messages    message.Service
	Permissions permission.Service
	BibEntries  bib.Service
	KB          kb.Service
	Research    *research.Engine
}

// NewTestServices creates a fully-wired set of services backed by an
// isolated, file-based SQLite database (with all migrations applied) and a
// temporary project directory.  Cleanup is registered automatically via
// t.Cleanup.
func NewTestServices(t *testing.T) *TestServices {
	t.Helper()

	conn, q := SetupTestDB(t)

	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	bibSvc := bib.NewService(q)
	kbSvc := kb.NewService(q, conn)

	researchStore := research.NewStore(conn)
	researchBroker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(researchBroker.Shutdown)
	researchEngine := research.NewEngine(researchStore, researchBroker)

	return &TestServices{
		Sessions:    sessions,
		Messages:    messages,
		Permissions: perms,
		BibEntries:  bibSvc,
		KB:          kbSvc,
		Research:    researchEngine,
	}
}

// SequentialMockProvider is a MockProvider variant that cycles through a
// pre-configured list of responses in order, then replays the last one.
// This is useful for agent-loop tests where multiple LLM calls are expected.
type SequentialMockProvider struct {
	mu        sync.Mutex
	responses []string
	index     int
	ModelVal  models.Model
}

// NewSequentialMockProvider creates a provider that returns each entry in
// responses for successive SendMessages / StreamResponse calls.
func NewSequentialMockProvider(responses ...string) *SequentialMockProvider {
	return &SequentialMockProvider{responses: responses}
}

func (s *SequentialMockProvider) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.responses) == 0 {
		return ""
	}
	r := s.responses[s.index]
	if s.index < len(s.responses)-1 {
		s.index++
	}
	return r
}

func (s *SequentialMockProvider) SendMessages(_ context.Context, _ []message.Message, _ []tools.BaseTool) (*provider.ProviderResponse, error) {
	return &provider.ProviderResponse{
		Content:      s.current(),
		Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
		FinishReason: message.FinishReasonEndTurn,
	}, nil
}

func (s *SequentialMockProvider) StreamResponse(_ context.Context, _ []message.Message, _ []tools.BaseTool) <-chan provider.ProviderEvent {
	resp := s.current()
	ch := make(chan provider.ProviderEvent, 3)
	go func() {
		defer close(ch)
		ch <- provider.ProviderEvent{Type: provider.EventContentStart}
		ch <- provider.ProviderEvent{Type: provider.EventContentDelta, Content: resp}
		ch <- provider.ProviderEvent{
			Type:     provider.EventComplete,
			Response: &provider.ProviderResponse{Content: resp, Usage: provider.TokenUsage{InputTokens: 10, OutputTokens: 5}},
		}
	}()
	return ch
}

func (s *SequentialMockProvider) Model() models.Model { return s.ModelVal }
