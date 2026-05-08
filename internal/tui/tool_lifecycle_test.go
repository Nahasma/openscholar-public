package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

func expectedChatHeightForCurrentLayout(t *testing.T, m Model) int {
	t.Helper()
	m.recalcLayout()
	return m.chat.viewport.Height
}

func TestUpdate_ToolLifecycleQueuedShowsBatchProgress(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.status.isProcessing = true

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			ToolLifecycle: &agent.ToolLifecycleEvent{
				ToolCallID: "tc1",
				ToolName:   "ScholarSearch",
				Input:      `{"query":"multi-agent systems"}`,
				State:      message.ToolCallQueued,
				Order:      0,
				Total:      3,
			},
		},
	})

	updated := next.(Model)
	if updated.status.processing.Phase != PhaseToolQueued {
		t.Fatalf("phase=%v, want PhaseToolQueued", updated.status.processing.Phase)
	}
	if !strings.Contains(updated.status.processing.Label, "0/3") {
		t.Fatalf("queued label=%q, want progress count", updated.status.processing.Label)
	}
}

func TestUpdate_ToolLifecycleRunningShowsIntent(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.status.isProcessing = true

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			ToolLifecycle: &agent.ToolLifecycleEvent{
				ToolCallID: "tc1",
				ToolName:   "ScholarSearch",
				Input:      `{"query":"marl 2026"}`,
				State:      message.ToolCallRunning,
				Order:      0,
				Total:      3,
			},
		},
	})

	updated := next.(Model)
	if updated.status.processing.Phase != PhaseToolRunning {
		t.Fatalf("phase=%v, want PhaseToolRunning", updated.status.processing.Phase)
	}
	if !strings.Contains(strings.ToLower(updated.status.processing.Label), "marl") {
		t.Fatalf("running label=%q, want query intent", updated.status.processing.Label)
	}
}

func TestUpdate_AssistantToolRunningDetailRendersAboveInput(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now(),
	}
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[message.Message]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Message{
			ID:        "asst-1",
			SessionID: "sess-1",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{
					ID:    "tc-read",
					Name:  "Read",
					Input: `{"file_path":"README.md"}`,
					State: message.ToolCallRunning,
				},
			},
		},
	})

	updated := next.(Model)
	if updated.status.processing.Phase != PhaseToolRunning {
		t.Fatalf("phase=%v, want PhaseToolRunning", updated.status.processing.Phase)
	}
	status := stripANSI(updated.renderStatusBar())
	if strings.Contains(status, "Running tools") {
		t.Fatalf("status should not own running detail: %q", status)
	}
	bottom := stripANSI(updated.buildBottomSection())
	if !strings.Contains(bottom, "Running tools") {
		t.Fatalf("bottom section should render running detail above input: %q", bottom)
	}
	footer := stripANSI(updated.buildPromptAreaVM().Footer)
	if strings.Contains(footer, "Read") || strings.Contains(footer, "Running tools") {
		t.Fatalf("footer should stay input-local: %q", footer)
	}
}

func TestRenderBottomSection_NormalWidthQueuedProcessingRendersAboveInput(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now(),
	}
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			ToolLifecycle: &agent.ToolLifecycleEvent{
				ToolCallID: "tc1",
				ToolName:   "ScholarSearch",
				Input:      `{"query":"queued bottom status regression"}`,
				State:      message.ToolCallQueued,
				Order:      0,
				Total:      2,
			},
		},
	})

	updated := next.(Model)
	status := stripANSI(updated.renderStatusBar())
	if strings.TrimSpace(status) == "" {
		t.Fatal("status bar should stay visible to carry stable metadata")
	}
	if strings.Contains(status, "Queued tools") || strings.Contains(status, "ScholarSearch") {
		t.Fatalf("status row should not carry queued detail: %q", status)
	}

	footer := stripANSI(updated.buildPromptAreaVM().Footer)
	if strings.Contains(footer, "Queued tools") || strings.Contains(footer, "ScholarSearch") {
		t.Fatalf("footer should stay input-local, got %q", footer)
	}

	bottom := stripANSI(updated.buildBottomSection())
	if !strings.Contains(bottom, "ScholarSearch") && !strings.Contains(bottom, "Queued tools") {
		t.Fatalf("bottom section should render queued detail above input, got %q", bottom)
	}
}

func TestRenderBottomSection_ThinkingProgressRailSitsImmediatelyAboveInput(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now().Add(-2 * time.Second),
	}
	m.status.processingVerb = "Thinking"
	m.recalcLayout()

	bottom := stripANSI(m.buildBottomSection())
	lines := strings.Split(bottom, "\n")
	progressIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Thinking") {
			progressIdx = i
			break
		}
	}
	if progressIdx < 0 {
		t.Fatalf("thinking progress rail missing from bottom section: %q", bottom)
	}
	if progressIdx+1 >= len(lines) || !strings.Contains(lines[progressIdx+1], "──") {
		t.Fatalf("thinking rail should sit immediately above input separator, bottom=%q", bottom)
	}
	if status := stripANSI(m.renderStatusBar()); strings.Contains(status, "Thinking") {
		t.Fatalf("thinking progress should not duplicate in status row: %q", status)
	}
}

func TestView_MainScreenProcessingLabelRenderedOnce(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		StartedAt: time.Now().Add(-2 * time.Second),
	}
	m.status.processingVerb = "Streaming response"
	m.recalcLayout()

	view := stripANSI(m.View())
	if count := strings.Count(view, "Streaming response"); count != 1 {
		t.Fatalf("processing label should render exactly once, count=%d view=%q", count, view)
	}
}

func TestUpdate_ToolLifecycleFinishedDoesNotLeakSummaryIntoStatusOrFooter(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			ToolLifecycle: &agent.ToolLifecycleEvent{
				ToolCallID:    "tc1",
				ToolName:      "Glob",
				Input:         `{"pattern":"*.go"}`,
				State:         message.ToolCallCompleted,
				Order:         0,
				Total:         1,
				ResultSummary: "/tmp/project/**/*.go",
			},
		},
	})

	updated := next.(Model)
	if !updated.status.notice.IsZero() {
		t.Fatalf("tool completion should not create a transient notice: %+v", updated.status.notice)
	}

	status := stripANSI(updated.renderStatusBar())
	if strings.Contains(status, "Glob:") {
		t.Fatalf("tool summary leaked into status bar: %q", status)
	}

	footer := stripANSI(updated.buildPromptAreaVM().Footer)
	if strings.Contains(footer, "Glob:") || strings.Contains(footer, "/tmp/project") {
		t.Fatalf("tool summary leaked into footer: %q", footer)
	}
}

func TestRenderBottomSection_TransientNoticeRendersInFooterDock(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 24
	m.recalcLayout()
	m.setNotice(components.NoticeSuccess, "Copied selection ✓")

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "Copied selection") {
		t.Fatalf("transient notice missing from unified footer dock: %q", status)
	}

	footer := stripANSI(m.buildPromptAreaVM().Footer)
	if strings.Contains(footer, "Copied selection") {
		t.Fatalf("transient notice should not duplicate in input footer: %q", footer)
	}
}

func TestRenderBottomSection_CtrlCPendingBelongsToStatusBar(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 24
	m.status.ctrlCPending = true
	m.recalcLayout()

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "Press Ctrl+C again to exit") {
		t.Fatalf("ctrl+c hint missing from status bar: %q", status)
	}

	footer := stripANSI(m.buildPromptAreaVM().Footer)
	if strings.Contains(strings.ToLower(footer), "ctrl+c") {
		t.Fatalf("ctrl+c hint should not render in footer: %q", footer)
	}
}

func TestBuildBottomSection_StatusV2DoesNotRenderInlineStatus(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 24
	m.status.mode = "auto"
	m.status.notice = components.TransientNotice{}
	m.recalcLayout()

	bottom := stripANSI(m.buildBottomSection())
	lines := strings.Split(bottom, "\n")
	if len(lines) < 1 {
		t.Fatalf("bottom section too short for input: %q", bottom)
	}
	if strings.Contains(bottom, "accept edits on") || strings.Contains(bottom, "auto mode") {
		t.Fatalf("bottom section should not render mode metadata: %q", bottom)
	}
	status := stripANSI(m.renderStandaloneStatusBar())
	if !strings.Contains(status, "auto mode") {
		t.Fatalf("expected standalone status bar mode metadata, got %q", status)
	}
	if strings.Contains(bottom, strings.TrimSpace(status)) {
		t.Fatalf("bottom section should not inline status row, got %q", bottom)
	}
}

func TestUpdate_ToolLifecycleRecalcLayoutWhenStatusBarHeightChanges(t *testing.T) {
	m := newTestModel()
	m.features.LayoutRegion = true
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now(),
	}
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			ToolLifecycle: &agent.ToolLifecycleEvent{
				ToolCallID: "tc1",
				ToolName:   "ScholarSearch",
				Input:      `{"query":"layout transition"}`,
				State:      message.ToolCallRunning,
				Order:      0,
				Total:      1,
			},
		},
	})

	updated := next.(Model)
	want := expectedChatHeightForCurrentLayout(t, updated)
	if updated.chat.viewport.Height != want {
		t.Fatalf("viewport height=%d, want %d after lifecycle update", updated.chat.viewport.Height, want)
	}
}

func TestUpdate_AssistantPhaseTransitionRecalcLayout(t *testing.T) {
	m := newTestModel()
	m.features.LayoutRegion = true
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now(),
	}
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[message.Message]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Message{
			ID:        "asst-1",
			SessionID: "sess-1",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "streaming content"},
			},
		},
	})

	updated := next.(Model)
	if updated.status.processing.Phase != PhaseStreaming {
		t.Fatalf("phase=%v, want PhaseStreaming", updated.status.processing.Phase)
	}
	want := expectedChatHeightForCurrentLayout(t, updated)
	if updated.chat.viewport.Height != want {
		t.Fatalf("viewport height=%d, want %d after assistant phase transition", updated.chat.viewport.Height, want)
	}
}

func TestUpdate_AgentPhaseEventsRecalcLayout(t *testing.T) {
	m := newTestModel()
	m.features.LayoutRegion = true
	m.sessionID = "sess-1"
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		StartedAt: time.Now(),
	}
	m.recalcLayout()

	// compacting: status bar appears
	nextCompacting, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			Type:      agent.AgentEventTypeCompacting,
		},
	})
	afterCompacting := nextCompacting.(Model)
	if afterCompacting.status.processing.Phase != PhaseCompacting {
		t.Fatalf("phase=%v, want PhaseCompacting", afterCompacting.status.processing.Phase)
	}
	if got, want := afterCompacting.chat.viewport.Height, expectedChatHeightForCurrentLayout(t, afterCompacting); got != want {
		t.Fatalf("compacting viewport height=%d, want %d", got, want)
	}

	// compact done: phase returns to thinking
	nextCompactDone, _ := afterCompacting.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			Type:      agent.AgentEventTypeCompactDone,
		},
	})
	afterCompactDone := nextCompactDone.(Model)
	if afterCompactDone.status.processing.Phase != PhaseThinking {
		t.Fatalf("phase=%v, want PhaseThinking", afterCompactDone.status.processing.Phase)
	}
	if got, want := afterCompactDone.chat.viewport.Height, expectedChatHeightForCurrentLayout(t, afterCompactDone); got != want {
		t.Fatalf("compact_done viewport height=%d, want %d", got, want)
	}

	// error: processing ends, rail/status should clear and layout is recomputed.
	nextError, _ := afterCompactDone.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			Error:     errors.New("boom"),
		},
	})
	afterError := nextError.(Model)
	if afterError.status.isProcessing {
		t.Fatalf("isProcessing=%v, want false", afterError.status.isProcessing)
	}
	if got, want := afterError.chat.viewport.Height, expectedChatHeightForCurrentLayout(t, afterError); got != want {
		t.Fatalf("error viewport height=%d, want %d", got, want)
	}

	// done path also recomputes layout from active processing.
	afterError.status.isProcessing = true
	afterError.status.processing = ProcessingState{
		Phase:     PhaseToolRunning,
		StartedAt: time.Now(),
	}
	afterError.recalcLayout()
	nextDone, _ := afterError.Update(pubsub.Event[agent.AgentEvent]{
		Type: pubsub.CreatedEvent,
		Payload: agent.AgentEvent{
			SessionID: "sess-1",
			Done:      true,
		},
	})
	doneModel := nextDone.(Model)
	if doneModel.status.isProcessing {
		t.Fatalf("isProcessing=%v, want false after done", doneModel.status.isProcessing)
	}
	if got, want := doneModel.chat.viewport.Height, expectedChatHeightForCurrentLayout(t, doneModel); got != want {
		t.Fatalf("done viewport height=%d, want %d", got, want)
	}
}
