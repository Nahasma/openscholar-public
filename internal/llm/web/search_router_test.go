package web

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// mockBackend is a configurable mock SearchProvider for router testing.
type mockBackend struct {
	name     string
	hits     []SearchHit
	err      error
	filter   bool
	called   int
	lastOpts SearchOptions
}

func (m *mockBackend) SearchWeb(_ context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	m.called++
	m.lastOpts = opts
	if m.err != nil {
		return nil, m.err
	}
	return &SearchResult{
		Hits:    m.hits,
		Summary: "mock summary for " + query,
		Backend: m.name,
	}, nil
}

func (m *mockBackend) SupportsFilter() bool { return m.filter }

func newTestRouter(backends ...rankedBackend) *SearchRouter {
	return &SearchRouter{
		backends: backends,
		state:    NewSearchState(""),
	}
}

func TestRouter_NormalRoute(t *testing.T) {
	b := &mockBackend{
		name: "p0",
		hits: []SearchHit{{Title: "hit1", URL: "https://example.com"}},
	}
	r := newTestRouter(rankedBackend{
		Name: "p0", Provider: b, Priority: 0, CanRewrite: true,
	})

	result, err := r.SearchWeb(context.Background(), "test query", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Backend != "p0" {
		t.Errorf("expected backend p0, got %s", result.Backend)
	}
	if b.called != 1 {
		t.Errorf("expected 1 call, got %d", b.called)
	}
}

func TestRouter_QuotaSwitch(t *testing.T) {
	b0 := &mockBackend{name: "p0", hits: []SearchHit{{Title: "hit0"}}}
	b1 := &mockBackend{name: "p1", hits: []SearchHit{{Title: "hit1"}}}

	r := newTestRouter(
		rankedBackend{Name: "p0", Provider: b0, Priority: 0, Quota: QuotaConfig{MonthlyLimit: 1}},
		rankedBackend{Name: "p1", Provider: b1, Priority: 1},
	)

	// First call goes to p0
	result, err := r.SearchWeb(context.Background(), "q1", SearchOptions{})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if result.Backend != "p0" {
		t.Errorf("first call: expected p0, got %s", result.Backend)
	}

	// Second call: p0 quota exhausted → p1
	result, err = r.SearchWeb(context.Background(), "q2", SearchOptions{})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if result.Backend != "p1" {
		t.Errorf("second call: expected p1, got %s", result.Backend)
	}
}

func TestRouter_CooldownSkip(t *testing.T) {
	b0 := &mockBackend{name: "p0", hits: []SearchHit{{Title: "hit0"}}}
	b1 := &mockBackend{name: "p1", hits: []SearchHit{{Title: "hit1"}}}

	r := newTestRouter(
		rankedBackend{Name: "p0", Provider: b0, Priority: 0},
		rankedBackend{Name: "p1", Provider: b1, Priority: 1},
	)

	// Put p0 in cooldown
	r.state.SetCooldown("p0", 1*time.Hour)

	result, err := r.SearchWeb(context.Background(), "q", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Backend != "p1" {
		t.Errorf("expected p1 (p0 in cooldown), got %s", result.Backend)
	}
	if b0.called != 0 {
		t.Error("p0 should not have been called (in cooldown)")
	}
}

func TestRouter_FilterSkip(t *testing.T) {
	// b0: no filter support at all (not native, not rewrite)
	b0 := &mockBackend{name: "openai", hits: []SearchHit{{Title: "hit0"}}}
	// b1: has native filter
	b1 := &mockBackend{name: "tavily", hits: []SearchHit{{Title: "hit1"}}, filter: true}

	r := newTestRouter(
		rankedBackend{Name: "openai", Provider: b0, Priority: 0, NativeFilter: false, CanRewrite: false},
		rankedBackend{Name: "tavily", Provider: b1, Priority: 1, NativeFilter: true},
	)

	result, err := r.SearchWeb(context.Background(), "q", SearchOptions{
		AllowedDomains: []string{"arxiv.org"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Backend != "tavily" {
		t.Errorf("expected tavily (openai can't filter), got %s", result.Backend)
	}
	if b0.called != 0 {
		t.Error("openai should not be called when filter needed and it can't handle")
	}
}

func TestRouter_AllFailed(t *testing.T) {
	b := &mockBackend{
		name: "fail",
		err:  &SearchError{Provider: "fail", Kind: SearchErrTemporary, Cause: fmt.Errorf("down")},
	}

	r := newTestRouter(rankedBackend{Name: "fail", Provider: b, Priority: 0})

	_, err := r.SearchWeb(context.Background(), "q", SearchOptions{})
	if err == nil {
		t.Fatal("expected error when all backends fail")
	}
	var failureErr *SearchFailureError
	if !errors.As(err, &failureErr) {
		t.Fatalf("expected SearchFailureError, got %T", err)
	}
	if len(failureErr.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(failureErr.Attempts))
	}
	if failureErr.Attempts[0].Name != "fail" || failureErr.Attempts[0].Success {
		t.Fatalf("unexpected attempt: %+v", failureErr.Attempts[0])
	}
	if !errors.Is(failureErr, b.err) {
		t.Fatal("expected failure error to wrap last backend error")
	}
}

func TestRouter_PostFilterEmpty(t *testing.T) {
	// Backend returns results but post-filter removes all
	b0 := &mockBackend{
		name: "brave",
		hits: []SearchHit{{Title: "not-arxiv", URL: "https://example.com"}},
	}
	b1 := &mockBackend{
		name:   "tavily",
		hits:   []SearchHit{{Title: "arxiv", URL: "https://arxiv.org/123"}},
		filter: true,
	}

	r := newTestRouter(
		rankedBackend{Name: "brave", Provider: b0, Priority: 0, CanRewrite: true},
		rankedBackend{Name: "tavily", Provider: b1, Priority: 1, NativeFilter: true},
	)

	result, err := r.SearchWeb(context.Background(), "q", SearchOptions{
		AllowedDomains: []string{"arxiv.org"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Backend != "tavily" {
		t.Errorf("expected tavily (brave post-filter empty), got %s", result.Backend)
	}
}

func TestRouter_PostFilterEmptyAllFailedReturnsNoResultsCause(t *testing.T) {
	b := &mockBackend{
		name: "brave",
		hits: []SearchHit{{Title: "not-arxiv", URL: "https://example.com"}},
	}
	r := newTestRouter(rankedBackend{Name: "brave", Provider: b, Priority: 0, CanRewrite: true})

	_, err := r.SearchWeb(context.Background(), "q", SearchOptions{
		AllowedDomains: []string{"arxiv.org"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var failureErr *SearchFailureError
	if !errors.As(err, &failureErr) {
		t.Fatalf("expected SearchFailureError, got %T", err)
	}
	if len(failureErr.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(failureErr.Attempts))
	}
	if failureErr.Attempts[0].Error != "post-filter removed all results" {
		t.Fatalf("unexpected attempt: %+v", failureErr.Attempts[0])
	}
	var searchErr *SearchError
	if !errors.As(err, &searchErr) {
		t.Fatal("expected wrapped SearchError")
	}
	if searchErr.Kind != SearchErrNoResults {
		t.Fatalf("expected no_results cause, got %s", searchErr.Kind)
	}
}

func TestRouter_SupportsFilter(t *testing.T) {
	r1 := newTestRouter(
		rankedBackend{Name: "a", CanRewrite: true},
	)
	if !r1.SupportsFilter() {
		t.Error("router with rewrite backend should support filter")
	}

	r2 := newTestRouter(
		rankedBackend{Name: "b", NativeFilter: true},
	)
	if !r2.SupportsFilter() {
		t.Error("router with native filter backend should support filter")
	}

	r3 := newTestRouter(
		rankedBackend{Name: "c"},
	)
	if r3.SupportsFilter() {
		t.Error("router with no filter backends should not support filter")
	}
}

func TestRouter_ZeroConfig(t *testing.T) {
	// No env vars set, no config → should have at least DDG + anthropic
	r := BuildSearchRouter(DefaultConfig(), nil, nil, "")
	if len(r.backends) == 0 {
		t.Fatal("zero config should have at least DDG")
	}

	// DDG should be in the list
	found := false
	for _, b := range r.backends {
		if b.Name == "duckduckgo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("DDG should be auto-discovered")
	}
}

func TestRouter_AttemptsTracking(t *testing.T) {
	failB := &mockBackend{
		name: "fail",
		err:  &SearchError{Provider: "fail", Kind: SearchErrTemporary, Cause: fmt.Errorf("timeout")},
	}
	okB := &mockBackend{
		name: "ok",
		hits: []SearchHit{{Title: "hit"}},
	}

	r := newTestRouter(
		rankedBackend{Name: "fail", Provider: failB, Priority: 0},
		rankedBackend{Name: "ok", Provider: okB, Priority: 1},
	)

	result, err := r.SearchWeb(context.Background(), "q", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}
	if result.Attempts[0].Success || result.Attempts[0].Name != "fail" {
		t.Errorf("first attempt should be failed 'fail', got %+v", result.Attempts[0])
	}
	if !result.Attempts[1].Success || result.Attempts[1].Name != "ok" {
		t.Errorf("second attempt should be success 'ok', got %+v", result.Attempts[1])
	}
}

func TestRouter_AllSkippedStillReturnsAttempts(t *testing.T) {
	b0 := &mockBackend{name: "cooldown"}
	b1 := &mockBackend{name: "quota"}

	r := newTestRouter(
		rankedBackend{Name: "cooldown", Provider: b0, Priority: 0},
		rankedBackend{Name: "quota", Provider: b1, Priority: 1, Quota: QuotaConfig{MonthlyLimit: 1}},
	)
	r.state.SetCooldown("cooldown", time.Hour)
	if !r.state.Reserve("quota", QuotaConfig{MonthlyLimit: 1}) {
		t.Fatal("failed to pre-consume quota for test setup")
	}

	_, err := r.SearchWeb(context.Background(), "q", SearchOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	var failureErr *SearchFailureError
	if !errors.As(err, &failureErr) {
		t.Fatalf("expected SearchFailureError, got %T", err)
	}
	if failureErr.Cause != nil {
		t.Fatalf("expected nil cause for all-skipped case, got %v", failureErr.Cause)
	}
	if len(failureErr.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(failureErr.Attempts))
	}
	if failureErr.Attempts[0].Error != "skipped: backend in cooldown" {
		t.Fatalf("unexpected first skip reason: %q", failureErr.Attempts[0].Error)
	}
	if failureErr.Attempts[1].Error != "skipped: quota exhausted" {
		t.Fatalf("unexpected second skip reason: %q", failureErr.Attempts[1].Error)
	}
}

func TestRouter_SuccessIncludesSkipAttempts(t *testing.T) {
	unsupported := &mockBackend{name: "openai"}
	ok := &mockBackend{name: "tavily", hits: []SearchHit{{Title: "ok"}}}
	r := newTestRouter(
		rankedBackend{Name: "openai", Provider: unsupported, Priority: 0, NativeFilter: false, CanRewrite: false},
		rankedBackend{Name: "tavily", Provider: ok, Priority: 1, NativeFilter: true},
	)

	result, err := r.SearchWeb(context.Background(), "q", SearchOptions{AllowedDomains: []string{"arxiv.org"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}
	if result.Attempts[0].Error != "skipped: filter unsupported" || result.Attempts[0].Success {
		t.Fatalf("unexpected first attempt: %+v", result.Attempts[0])
	}
	if !result.Attempts[1].Success || result.Attempts[1].Name != "tavily" {
		t.Fatalf("unexpected second attempt: %+v", result.Attempts[1])
	}
}
