package tools

import (
	"context"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/evolution"
	"github.com/stretchr/testify/require"
)

type fakeEvolutionService struct {
	recorded         int
	autoEvolveCalled chan struct{}
}

func (f *fakeEvolutionService) RecordFeedback(ctx context.Context, req evolution.FeedbackRequest) (*evolution.EvolutionCase, error) {
	f.recorded++
	return &evolution.EvolutionCase{ID: "12345678-test-case"}, nil
}

func (f *fakeEvolutionService) RunEvolution(ctx context.Context) (*evolution.RefinementResult, *evolution.AnalysisResult, []evolution.EvolutionCase, error) {
	return nil, nil, nil, nil
}
func (f *fakeEvolutionService) ApplyChanges(ctx context.Context, change evolution.SkillChange, cases []evolution.EvolutionCase) error {
	return nil
}
func (f *fakeEvolutionService) AutoEvolve(ctx context.Context) error {
	close(f.autoEvolveCalled)
	return nil
}
func (f *fakeEvolutionService) SetLLMCaller(caller evolution.LLMCaller) {}
func (f *fakeEvolutionService) Rollback(ctx context.Context, skillID string) error {
	return nil
}
func (f *fakeEvolutionService) ListCases(ctx context.Context, status string, limit int) ([]evolution.EvolutionCase, error) {
	return nil, nil
}
func (f *fakeEvolutionService) DismissAll(ctx context.Context) error { return nil }
func (f *fakeEvolutionService) SkillStats(ctx context.Context, skillID string) (*evolution.SkillStatsResult, error) {
	return nil, nil
}
func (f *fakeEvolutionService) AllSkillStats(ctx context.Context) ([]evolution.SkillStatsResult, error) {
	return nil, nil
}

func TestRecordFeedbackTool_DoesNotTriggerAutoEvolve(t *testing.T) {
	fakeSvc := &fakeEvolutionService{autoEvolveCalled: make(chan struct{})}
	tool := NewRecordFeedbackTool(fakeSvc)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name:  "RecordFeedback",
		Input: `{"feedback":"bad output","user_request":"fix this","agent_output":"x","skill_id":"writing/format"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Equal(t, 1, fakeSvc.recorded)
	require.Contains(t, resp.Content, "/evolve")

	select {
	case <-fakeSvc.autoEvolveCalled:
		t.Fatal("RecordFeedback unexpectedly triggered AutoEvolve")
	case <-time.After(50 * time.Millisecond):
	}
}
