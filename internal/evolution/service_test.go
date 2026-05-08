package evolution

import (
	"context"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/skillbank"
	"github.com/stretchr/testify/require"
)

func TestRunEvolution_ErrWhenLLMCallerNotConfigured(t *testing.T) {
	svc := NewService(setupStore(t), nil, nil)

	_, _, _, err := svc.RunEvolution(context.Background())
	require.ErrorIs(t, err, ErrLLMCallerNotConfigured)
}

func TestRunEvolution_NoPendingCasesAfterSetLLMCaller(t *testing.T) {
	svc := NewService(setupStore(t), nil, nil)
	svc.SetLLMCaller(func(ctx context.Context, prompt string) (string, error) {
		return `{"action":"no_change_needed","summary":"ok","changes":[]}`, nil
	})

	result, analysis, cases, err := svc.RunEvolution(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "no_change_needed", result.Action)
	require.Nil(t, analysis)
	require.Nil(t, cases)
}

func TestRunEvolution_UsesInjectedLLMCaller(t *testing.T) {
	conn := setupTestConn(t)
	store := NewStore(conn)
	require.NoError(t, store.CreateCase(&EvolutionCase{
		SessionID:   "s1",
		UserRequest: "write related work",
		Feedback:    "citation style was wrong",
	}))

	svc := NewService(store, skillbank.NewService(conn, t.TempDir(), nil), nil)
	calls := 0
	analysisJSON := `{"failure_patterns":[],"recommendations":[],"summary":"style feedback"}`
	svc.SetLLMCaller(func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls < 3 {
			return analysisJSON, nil
		}
		return `{"action":"no_change_needed","summary":"no safe change","changes":[]}`, nil
	})

	result, analysis, cases, err := svc.RunEvolution(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, analysis)
	require.Len(t, cases, 1)
	require.Equal(t, "no_change_needed", result.Action)
	require.Equal(t, 3, calls)
}

func TestAutoEvolve_Disabled(t *testing.T) {
	svc := NewService(setupStore(t), nil, nil)

	err := svc.AutoEvolve(context.Background())
	require.ErrorIs(t, err, ErrAutoEvolveDisabled)
}
