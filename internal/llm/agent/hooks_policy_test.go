package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type recordedHookEvent struct {
	event hooks.Event
	input hooks.Input
}

type recordingHookService struct {
	mu     sync.Mutex
	events []recordedHookEvent
}

func (s *recordingHookService) Run(_ context.Context, event hooks.Event, input hooks.Input) error {
	s.record(event, input)
	return nil
}

func (s *recordingHookService) RunBlocking(_ context.Context, event hooks.Event, input hooks.Input) error {
	s.record(event, input)
	return nil
}

func (s *recordingHookService) RunAsync(_ context.Context, event hooks.Event, input hooks.Input) {
	s.record(event, input)
}

func (s *recordingHookService) Reload() error { return nil }

func (s *recordingHookService) record(event hooks.Event, input hooks.Input) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, recordedHookEvent{event: event, input: input})
}

func (s *recordingHookService) snapshot() []recordedHookEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedHookEvent, len(s.events))
	copy(out, s.events)
	return out
}

func TestApplyHookResultMetadata(t *testing.T) {
	input := hooks.Input{}
	applyHookResultMetadata(&input, `{
		"provider":"semantic_scholar",
		"source":"semantic_scholar",
		"error_kind":"rate_limited",
		"progress_kind":"none",
		"goal_type":"academic_download",
		"artifact":{"path":".openscholar/papers/a.pdf"},
		"artifacts":[{"path":".openscholar/papers/b.pdf"}],
		"artifact_paths":[".openscholar/papers/c.pdf"]
	}`)

	if input.Provider != "semantic_scholar" || input.Source != "semantic_scholar" {
		t.Fatalf("provider/source not applied: %#v", input)
	}
	if input.ErrorKind != "rate_limited" || input.ProgressKind != "none" || input.GoalType != "academic_download" {
		t.Fatalf("runtime metadata not applied: %#v", input)
	}
	if len(input.ArtifactPaths) != 3 {
		t.Fatalf("expected artifact paths, got %#v", input.ArtifactPaths)
	}
}

func TestRunPostToolHooksEmitsCooldownPolicyEvent(t *testing.T) {
	recorder := &recordingHookService{}
	ag := &agent{hookService: recorder}
	call := message.ToolCall{
		ID:    "tc1",
		Name:  "ScholarSearch",
		Input: `{"action":"search","query":"q1"}`,
	}
	result := message.ToolResult{
		ToolCallID: call.ID,
		Content:    "skipped",
		IsError:    true,
		Metadata:   scholarCooldownSkipMetadata(call.Input),
	}

	ag.runPostToolHooks(context.Background(), call, result, nil)

	events := recorder.snapshot()
	if len(events) != 2 {
		t.Fatalf("expected failure and provider cooldown events, got %#v", events)
	}
	if events[0].event != hooks.PostToolUseFailure {
		t.Fatalf("expected post tool failure event, got %s", events[0].event)
	}
	if events[1].event != hooks.ProviderCooldown {
		t.Fatalf("expected provider cooldown event, got %s", events[1].event)
	}
	if events[1].input.ErrorKind != "provider_cooldown" || events[1].input.Provider != "semantic_scholar" {
		t.Fatalf("expected cooldown metadata in hook input, got %#v", events[1].input)
	}
}

func TestEmitBudgetExhaustedHook(t *testing.T) {
	recorder := &recordingHookService{}
	ag := &agent{hookService: recorder}

	ag.emitBudgetExhaustedHook(context.Background(), "s1", ReasonCompactExhausted)

	events := recorder.snapshot()
	if len(events) != 1 || events[0].event != hooks.BudgetExhausted {
		t.Fatalf("expected budget exhausted hook, got %#v", events)
	}
	if events[0].input.SessionID != "s1" || events[0].input.ErrorKind != string(ReasonCompactExhausted) {
		t.Fatalf("unexpected budget hook input: %#v", events[0].input)
	}
	if events[0].input.Timestamp.IsZero() {
		t.Fatal("expected budget hook timestamp")
	}
}
