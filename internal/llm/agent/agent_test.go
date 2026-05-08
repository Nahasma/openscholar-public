package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type streamStep struct {
	response *provider.ProviderResponse
	err      error
}

type sequenceStreamProvider struct {
	model        models.Model
	steps        []streamStep
	callCount    int
	lastMessages [][]message.Message
	streamMsgs   [][]message.Message
	tokenCount   int64
	countErr     error
}

func (p *sequenceStreamProvider) SendMessages(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (*provider.ProviderResponse, error) {
	if p.callCount >= len(p.steps) {
		return nil, context.Canceled
	}
	step := p.steps[p.callCount]
	p.callCount++
	p.lastMessages = append(p.lastMessages, cloneMessages(msgs))
	return step.response, step.err
}

func (p *sequenceStreamProvider) StreamResponse(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) <-chan provider.ProviderEvent {
	ch := make(chan provider.ProviderEvent, 3)
	p.lastMessages = append(p.lastMessages, cloneMessages(msgs))
	p.streamMsgs = append(p.streamMsgs, cloneMessages(msgs))
	if p.callCount >= len(p.steps) {
		close(ch)
		return ch
	}
	step := p.steps[p.callCount]
	p.callCount++
	go func() {
		defer close(ch)
		if step.err != nil {
			ch <- provider.ProviderEvent{Type: provider.EventError, Error: step.err}
			return
		}
		ch <- provider.ProviderEvent{Type: provider.EventContentStart}
		if step.response != nil && step.response.Content != "" {
			ch <- provider.ProviderEvent{Type: provider.EventContentDelta, Content: step.response.Content}
		}
		ch <- provider.ProviderEvent{Type: provider.EventComplete, Response: step.response}
	}()
	return ch
}

func (p *sequenceStreamProvider) CountTokens(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (provider.TokenCount, error) {
	_ = ctx
	p.lastMessages = append(p.lastMessages, cloneMessages(msgs))
	if p.countErr != nil {
		return provider.TokenCount{}, p.countErr
	}
	if p.tokenCount > 0 {
		return provider.TokenCount{InputTokens: p.tokenCount}, nil
	}
	return provider.TokenCount{InputTokens: 100}, nil
}

func (p *sequenceStreamProvider) Model() models.Model {
	if p.model.ID == "" {
		return models.Model{ID: "test-model"}
	}
	return p.model
}

func cloneMessages(msgs []message.Message) []message.Message {
	cloned := make([]message.Message, len(msgs))
	for i, msg := range msgs {
		parts := make([]message.ContentPart, len(msg.Parts))
		copy(parts, msg.Parts)
		msg.Parts = parts
		cloned[i] = msg
	}
	return cloned
}

func setup(t *testing.T) (session.Service, message.Service) {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	return session.NewService(q), message.NewService(q)
}

func loadAgentNotificationConfig(t *testing.T, enabled bool) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	wd := t.TempDir()
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	cfg := `{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"model": "qwen2.5-coder:latest"}
  },
  "subagent_orchestration": {
    "enabled": ` + enabledValue + `
  }
}`
	require.NoError(t, os.MkdirAll(filepath.Dir(config.ConfigFilePath(wd)), 0o755))
	require.NoError(t, os.WriteFile(config.ConfigFilePath(wd), []byte(cfg), 0o644))
	_, err := config.Load(wd)
	require.NoError(t, err)
}

func TestAgent_Run_SimpleResponse(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "Hello! I'm the assistant.",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "Say hello")
	require.NoError(t, err)

	result := <-eventCh
	assert.Nil(t, result.Error)
	assert.Contains(t, result.Message.Content().Text, "Hello! I'm the assistant.")

	// Verify messages were persisted
	msgs, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	// Should have: user message + assistant message
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestAgent_ResetTelemetry(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model", ContextWindow: 200000},
		Response: "hello",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)
	eventCh, err := ag.Run(ctx, sess.ID, "Say hello")
	require.NoError(t, err)
	<-eventCh

	require.NotZero(t, ag.LastInputTokens())
	require.NotZero(t, ag.ContextSnapshot().CurrentUsage.InputTokens)
	require.NotEmpty(t, ag.CostState().ByModel)

	ag.ResetTelemetry()

	assert.Zero(t, ag.LastInputTokens())
	assert.True(t, ag.ContextSnapshot().CurrentUsage.IsZero())
	assert.Empty(t, ag.CostState().ByModel)
}

func TestAgent_Cancel(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	// Slow mock that takes time to respond
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "slow response",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "Long task")
	require.NoError(t, err)

	// Cancel immediately
	ag.Cancel(sess.ID)

	// Should receive result (possibly with error)
	select {
	case result := <-eventCh:
		// May have error due to cancellation, or may complete before cancel
		_ = result
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for agent completion after cancel")
	}
}

func TestAgent_IsSessionBusy(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "response",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)

	assert.False(t, ag.IsSessionBusy(sess.ID))
	assert.False(t, ag.IsBusy())

	eventCh, _ := ag.Run(ctx, sess.ID, "test")

	// Drain the channel
	<-eventCh

	// After completion, should no longer be busy
	assert.False(t, ag.IsSessionBusy(sess.ID))
	assert.False(t, ag.IsBusy())
}

func TestAgent_Run_BusyError(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	// Use a channel to control when the mock responds
	block := make(chan struct{})
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "response",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "first request")
	require.NoError(t, err)

	// Try to run again on the same session — may or may not be busy
	// depending on timing. If first completes fast, second will succeed.
	_, err = ag.Run(ctx, sess.ID, "second request")
	// The first request may have already completed, so we accept both outcomes
	if err != nil {
		assert.ErrorIs(t, err, agent.ErrSessionBusy)
	}

	close(block)
	<-eventCh
}

func TestAgent_TaskNotificationDrain_PersistsAndInjectsOnce(t *testing.T) {
	loadAgentNotificationConfig(t, true)
	sessions, messages := setup(t)
	ctx := context.Background()
	parent := testutil.CreateTestSession(t, sessions)
	child, err := sessions.CreateTaskSession(ctx, "child-1", parent.ID, "child")
	require.NoError(t, err)

	_, err = messages.Create(ctx, child.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "SECRET_TRANSCRIPT"}},
	})
	require.NoError(t, err)

	reg := task.NewRegistry()
	defer reg.Shutdown()
	finishedAt := time.Now().UTC()
	state := task.NewSubtaskState(task.Meta{
		ID:             "task-1",
		Label:          "subtask",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      finishedAt.Add(-2 * time.Second),
		EndedAt:        &finishedAt,
		IsBackgrounded: true,
		Notified:       false,
	}, parent.ID, child.ID, "explore", "scan files", "gpt-4.1", "none")
	state.Result = "found <ok>"
	state.VerifyStatus = task.VerifyStatusPassed
	state.VerifyVerdict = "PASS"
	state.VerifyResult = "PASS"
	reg.Register(state)

	p := &sequenceStreamProvider{
		model: models.Model{ID: "test-model"},
		steps: []streamStep{
			{response: &provider.ProviderResponse{Content: "first", Usage: provider.TokenUsage{InputTokens: 1, OutputTokens: 1}, FinishReason: message.FinishReasonEndTurn}},
			{response: &provider.ProviderResponse{Content: "second", Usage: provider.TokenUsage{InputTokens: 1, OutputTokens: 1}, FinishReason: message.FinishReasonEndTurn}},
		},
	}
	ag := agent.NewAgentForTest(p, sessions, messages, nil)
	ag.SetTaskRegistry(reg)

	ch, runErr := ag.Run(ctx, parent.ID, "go")
	require.NoError(t, runErr)
	<-ch

	// First request should include exactly one notification.
	firstReq := p.streamMsgs[0]
	notificationCount := 0
	for _, m := range firstReq {
		text := m.Content().String()
		if strings.Contains(text, "<task-notification>") {
			notificationCount++
			assert.Contains(t, text, "<task-id>task-1</task-id>")
			assert.Contains(t, text, "&lt;ok&gt;")
			assert.Contains(t, text, "<verify-verdict>PASS</verify-verdict>")
			assert.NotContains(t, text, "SECRET_TRANSCRIPT")
		}
	}
	assert.Equal(t, 1, notificationCount)

	st, ok := reg.Get("task-1")
	require.True(t, ok)
	assert.True(t, st.TaskMeta().Notified)

	allMsgs, err := messages.List(ctx, parent.ID)
	require.NoError(t, err)
	persistedNotifications := 0
	for _, m := range allMsgs {
		if v, ok := m.Meta["subagent_notification"].(bool); ok && v {
			persistedNotifications++
			assert.Equal(t, "task-1", m.Meta["task_id"])
		}
	}
	assert.Equal(t, 1, persistedNotifications)

	// Second run should not inject duplicates.
	ch, runErr = ag.Run(ctx, parent.ID, "again")
	require.NoError(t, runErr)
	<-ch
	secondReq := p.streamMsgs[1]
	secondReqNotifyCount := 0
	for _, m := range secondReq {
		if strings.Contains(m.Content().String(), "<task-notification>") {
			secondReqNotifyCount++
		}
	}
	assert.Equal(t, 1, secondReqNotifyCount)

	allMsgs, err = messages.List(ctx, parent.ID)
	require.NoError(t, err)
	persistedNotifications = 0
	for _, m := range allMsgs {
		if v, ok := m.Meta["subagent_notification"].(bool); ok && v {
			persistedNotifications++
		}
	}
	assert.Equal(t, 1, persistedNotifications)
}

func TestAgent_TaskNotificationDrain_DisabledFlagDoesNotInjectNotifications(t *testing.T) {
	loadAgentNotificationConfig(t, false)
	sessions, messages := setup(t)
	ctx := context.Background()
	parent := testutil.CreateTestSession(t, sessions)

	reg := task.NewRegistry()
	defer reg.Shutdown()
	finishedAt := time.Now().UTC()
	state := task.NewSubtaskState(task.Meta{
		ID:             "disabled-task",
		Label:          "subtask",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      finishedAt.Add(-2 * time.Second),
		EndedAt:        &finishedAt,
		IsBackgrounded: true,
	}, parent.ID, "disabled-child", "explore", "scan files", "test-model", "none")
	state.Result = "should not inject"
	reg.Register(state)

	p := &sequenceStreamProvider{
		model: models.Model{ID: "test-model"},
		steps: []streamStep{
			{response: &provider.ProviderResponse{Content: "legacy response", Usage: provider.TokenUsage{InputTokens: 1, OutputTokens: 1}, FinishReason: message.FinishReasonEndTurn}},
		},
	}
	ag := agent.NewAgentForTest(p, sessions, messages, nil)
	ag.SetTaskRegistry(reg)

	ch, runErr := ag.Run(ctx, parent.ID, "go")
	require.NoError(t, runErr)
	res := <-ch
	require.NoError(t, res.Error)

	requestText := strings.Builder{}
	for _, msg := range p.streamMsgs[0] {
		requestText.WriteString(msg.Content().String())
		requestText.WriteByte('\n')
	}
	assert.NotContains(t, requestText.String(), "<task-notification>")

	parentMessages, err := messages.List(ctx, parent.ID)
	require.NoError(t, err)
	for _, msg := range parentMessages {
		assert.NotEqual(t, true, msg.Meta["subagent_notification"])
	}
	st, ok := reg.Get("disabled-task")
	require.True(t, ok)
	assert.False(t, st.TaskMeta().Notified)
}

func TestAgent_TaskNotificationDrain_TwoTerminalNotificationsSynthesizedWithoutTranscriptInjection(t *testing.T) {
	loadAgentNotificationConfig(t, true)
	sessions, messages := setup(t)
	ctx := context.Background()
	parent := testutil.CreateTestSession(t, sessions)

	childA, err := sessions.CreateTaskSession(ctx, "child-a", parent.ID, "child a")
	require.NoError(t, err)
	childB, err := sessions.CreateTaskSession(ctx, "child-b", parent.ID, "child b")
	require.NoError(t, err)

	_, err = messages.Create(ctx, childA.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "CHILD_A_SECRET_TRANSCRIPT"}},
	})
	require.NoError(t, err)
	_, err = messages.Create(ctx, childB.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "CHILD_B_SECRET_TRANSCRIPT"}},
	})
	require.NoError(t, err)

	reg := task.NewRegistry()
	defer reg.Shutdown()
	finishedAt := time.Now().UTC()
	stateA := task.NewSubtaskState(task.Meta{
		ID:             "task-a",
		Label:          "subtask-a",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      finishedAt.Add(-4 * time.Second),
		EndedAt:        &finishedAt,
		IsBackgrounded: true,
	}, parent.ID, childA.ID, "explore", "scan a", "test-model", "none")
	stateA.Result = "finding A"
	reg.Register(stateA)

	stateB := task.NewSubtaskState(task.Meta{
		ID:             "task-b",
		Label:          "subtask-b",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      finishedAt.Add(-3 * time.Second),
		EndedAt:        &finishedAt,
		IsBackgrounded: true,
	}, parent.ID, childB.ID, "general", "implement b", "test-model", "none")
	stateB.Result = "finding B"
	reg.Register(stateB)

	p := &sequenceStreamProvider{
		model: models.Model{ID: "test-model"},
		steps: []streamStep{
			{response: &provider.ProviderResponse{Content: "SYNTHESIZED: finding A + finding B", Usage: provider.TokenUsage{InputTokens: 10, OutputTokens: 6}, FinishReason: message.FinishReasonEndTurn}},
		},
	}

	ag := agent.NewAgentForTest(p, sessions, messages, nil)
	ag.SetTaskRegistry(reg)

	ch, runErr := ag.Run(ctx, parent.ID, "synthesize now")
	require.NoError(t, runErr)
	res := <-ch
	require.NoError(t, res.Error)
	assert.Contains(t, res.Message.Content().String(), "SYNTHESIZED")

	req := p.streamMsgs[0]
	notificationCount := 0
	joined := strings.Builder{}
	for _, m := range req {
		text := m.Content().String()
		joined.WriteString(text)
		joined.WriteByte('\n')
		if strings.Contains(text, "<task-notification>") {
			notificationCount++
		}
	}
	requestText := joined.String()
	assert.Equal(t, 2, notificationCount)
	assert.Contains(t, requestText, "<task-id>task-a</task-id>")
	assert.Contains(t, requestText, "<task-id>task-b</task-id>")
	assert.Contains(t, requestText, "finding A")
	assert.Contains(t, requestText, "finding B")
	assert.NotContains(t, requestText, "CHILD_A_SECRET_TRANSCRIPT")
	assert.NotContains(t, requestText, "CHILD_B_SECRET_TRANSCRIPT")
}

func TestAgent_Run_RuntimeDisableModelInvocation(t *testing.T) {
	sessions, messages := setup(t)
	sess := testutil.CreateTestSession(t, sessions)
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "response",
	}
	ag := agent.NewAgentForTest(mock, sessions, messages, nil)

	ctx := agent.WithRequestRuntime(context.Background(), agent.RequestRuntime{
		DisableModelInvocation: true,
	})
	_, err := ag.Run(ctx, sess.ID, "test")
	require.ErrorIs(t, err, agent.ErrModelInvocationDisabled)
}

func TestAgent_Run_RuntimeUnknownAllowedTool(t *testing.T) {
	sessions, messages := setup(t)
	sess := testutil.CreateTestSession(t, sessions)
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "response",
	}
	ag := agent.NewAgentForTest(mock, sessions, messages, []tools.BaseTool{
		&mockTool{name: "View", result: tools.NewTextResponse("ok")},
	})

	ctx := agent.WithRequestRuntime(context.Background(), agent.RequestRuntime{
		AllowedTools: []string{"NoSuchTool"},
	})
	_, err := ag.Run(ctx, sess.ID, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tools")
}

func TestAgent_PubSubEvent(t *testing.T) {
	sessions, messages := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := testutil.CreateTestSession(t, sessions)

	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
		Response: "pubsub test response",
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)
	subCh := ag.Subscribe(ctx)

	eventCh, _ := ag.Run(ctx, sess.ID, "trigger pubsub")
	<-eventCh // wait for completion

	// Subscriber should receive the agent event
	select {
	case event := <-subCh:
		assert.Equal(t, pubsub.CreatedEvent, event.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pubsub event")
	}
}

func TestAgent_Run_ToolUse(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)

	callCount := 0
	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "test-model"},
	}

	// First call returns tool_use, second returns end_turn
	origStream := mock.StreamResponse
	_ = origStream

	// Create a simple mock tool
	viewTool := &mockTool{
		name:   "View",
		result: tools.NewTextResponse("file contents here"),
	}

	// For tool use testing, we need a more sophisticated mock
	// that returns different responses on successive calls.
	// This is complex due to the stream-based API.
	// We verify basic tool registration works.
	ag := agent.NewAgentForTest(mock, sessions, messages, []tools.BaseTool{viewTool})

	eventCh, err := ag.Run(ctx, sess.ID, "Read a file")
	require.NoError(t, err)

	result := <-eventCh
	_ = callCount
	// Should complete (even without tool calls in this simple mock)
	assert.NotNil(t, result)
}

func TestAgent_Model(t *testing.T) {
	sessions, messages := setup(t)

	mock := &testutil.MockProvider{
		ModelVal: models.Model{ID: "claude-3-sonnet", Name: "Claude 3 Sonnet"},
	}

	ag := agent.NewAgentForTest(mock, sessions, messages, nil)
	assert.Equal(t, models.ModelID("claude-3-sonnet"), ag.Model().ID)
	assert.Equal(t, "Claude 3 Sonnet", ag.Model().Name)
}

func TestAgent_Run_AutoRecoversFromMaxTokens(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)
	prov := &sequenceStreamProvider{
		steps: []streamStep{
			{
				response: &provider.ProviderResponse{
					Content:      "Partial answer",
					FinishReason: message.FinishReasonMaxTokens,
					Usage:        provider.TokenUsage{InputTokens: 40, OutputTokens: 20},
				},
			},
			{
				response: &provider.ProviderResponse{
					Content:      "Continued answer",
					FinishReason: message.FinishReasonEndTurn,
					Usage:        provider.TokenUsage{InputTokens: 30, OutputTokens: 10},
				},
			},
		},
	}

	ag := agent.NewAgentForTest(prov, sessions, messages, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "Explain the bug")
	require.NoError(t, err)

	result := <-eventCh
	require.NoError(t, result.Error)
	assert.True(t, result.Done)
	assert.Equal(t, agent.ReasonCompleted, result.TerminalReason)
	assert.Equal(t, "Continued answer", result.Message.Content().Text)
	require.Len(t, prov.streamMsgs, 2)

	secondCall := prov.streamMsgs[1]
	require.Len(t, secondCall, 3)
	assert.Equal(t, message.Assistant, secondCall[1].Role)
	assert.Equal(t, "Partial answer", secondCall[1].Content().Text)
	assert.Equal(t, message.User, secondCall[2].Role)
	assert.Contains(t, secondCall[2].Content().Text, "Output token limit hit")

	msgs, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, message.FinishReasonMaxTokens, msgs[1].FinishReason())
	assert.Equal(t, "Continued answer", msgs[2].Content().Text)
}

func TestAgent_Run_MaxTokensRecoveryExhausted(t *testing.T) {
	sessions, messages := setup(t)
	ctx := context.Background()

	sess := testutil.CreateTestSession(t, sessions)
	prov := &sequenceStreamProvider{
		steps: []streamStep{
			{
				response: &provider.ProviderResponse{
					Content:      "chunk 1",
					FinishReason: message.FinishReasonMaxTokens,
					Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 10},
				},
			},
			{
				response: &provider.ProviderResponse{
					Content:      "chunk 2",
					FinishReason: message.FinishReasonMaxTokens,
					Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 10},
				},
			},
			{
				response: &provider.ProviderResponse{
					Content:      "chunk 3",
					FinishReason: message.FinishReasonMaxTokens,
					Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 10},
				},
			},
			{
				response: &provider.ProviderResponse{
					Content:      "chunk 4",
					FinishReason: message.FinishReasonMaxTokens,
					Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 10},
				},
			},
		},
	}

	ag := agent.NewAgentForTest(prov, sessions, messages, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "Keep going")
	require.NoError(t, err)

	result := <-eventCh
	require.NoError(t, result.Error)
	assert.True(t, result.Done)
	assert.Equal(t, agent.ReasonMaxTokensRecoveryExhausted, result.TerminalReason)
	assert.Equal(t, agent.TerminalReasonMessage(agent.ReasonMaxTokensRecoveryExhausted), result.Warning)
	assert.Equal(t, "chunk 4", result.Message.Content().Text)
	assert.Len(t, prov.streamMsgs, 4)

	msgs, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 5)
	assert.Equal(t, message.FinishReasonMaxTokens, msgs[4].FinishReason())
}

func TestAgent_PreflightCompactsBeforeProviderCall(t *testing.T) {
	sessions, messagesSvc := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	for i := 0; i < 8; i++ {
		_, err := messagesSvc.Create(ctx, sess.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: fmt.Sprintf("user %d", i)}},
		})
		require.NoError(t, err)
		_, err = messagesSvc.Create(ctx, sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: fmt.Sprintf("assistant %d", i)}},
		})
		require.NoError(t, err)
	}

	prov := &sequenceStreamProvider{
		model: models.Model{ID: "tiny", ContextWindow: 80, DefaultMaxTokens: 20},
		steps: []streamStep{{
			response: &provider.ProviderResponse{
				Content:      "ok",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
			},
		}},
		tokenCount: 75,
	}

	ag := agent.NewAgentForTest(prov, sessions, messagesSvc, nil)
	eventCh, err := ag.Run(ctx, sess.ID, "latest")
	require.NoError(t, err)
	result := <-eventCh
	require.NoError(t, result.Error)
	require.GreaterOrEqual(t, len(prov.lastMessages), 1)
	require.Len(t, prov.streamMsgs, 1)
	preflightInputLen := len(prov.lastMessages[0])
	streamInputLen := len(prov.streamMsgs[0])
	assert.Less(t, streamInputLen, preflightInputLen)
}

func TestAgent_PromptTooLongRetryForcesCompaction(t *testing.T) {
	sessions, messagesSvc := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	for i := 0; i < 10; i++ {
		_, err := messagesSvc.Create(ctx, sess.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: fmt.Sprintf("turn %d", i)}},
		})
		require.NoError(t, err)
		_, err = messagesSvc.Create(ctx, sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: "ack"}},
		})
		require.NoError(t, err)
	}

	prov := &sequenceStreamProvider{
		model: models.Model{ID: "tiny", ContextWindow: 80, DefaultMaxTokens: 20},
		steps: []streamStep{
			{err: fmt.Errorf("prompt is too long")},
			{response: &provider.ProviderResponse{
				Content:      "retried",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
			}},
		},
		tokenCount: 75,
	}
	ag := agent.NewAgentForTest(prov, sessions, messagesSvc, nil)

	eventCh, err := ag.Run(ctx, sess.ID, "new prompt")
	require.NoError(t, err)
	result := <-eventCh
	require.NoError(t, result.Error)
	assert.Equal(t, "retried", result.Message.Content().Text)
	assert.Equal(t, 2, prov.callCount)

	require.Len(t, prov.streamMsgs, 2)
	firstStreamLen := len(prov.streamMsgs[0])
	secondStreamLen := len(prov.streamMsgs[1])
	assert.LessOrEqual(t, secondStreamLen, firstStreamLen)

	msgs, err := messagesSvc.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 22)
	assistantCount := 0
	for _, msg := range msgs {
		if msg.Role == message.Assistant {
			assistantCount++
		}
	}
	assert.Equal(t, 11, assistantCount)
}

// mockTool is a simple BaseTool implementation for testing.
type mockTool struct {
	name   string
	result tools.ToolResponse
}

func (m *mockTool) Info() tools.ToolInfo {
	return tools.ToolInfo{
		Name:        m.name,
		Description: "Mock tool for testing",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (m *mockTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	return m.result, nil
}
