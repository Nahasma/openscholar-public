package tui

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/agent"
	"github.com/openscholar/openscholar/internal/llm/models"
	llmtools "github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
	"github.com/openscholar/openscholar/internal/tui/components"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func nthVisibleOccurrence(lines []string, token string, n int) (row, col int, ok bool) {
	seen := 0
	for i, line := range lines {
		plain := stripANSI(line)
		searchFrom := 0
		for {
			idx := strings.Index(plain[searchFrom:], token)
			if idx < 0 {
				break
			}
			if seen == n {
				byteOffset := searchFrom + idx
				return i, len([]rune(plain[:byteOffset])), true
			}
			seen++
			searchFrom += idx + len(token)
		}
	}
	return -1, -1, false
}

func nthRuneOffset(text, token string, n int) (int, bool) {
	seen := 0
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], token)
		if idx < 0 {
			return -1, false
		}
		byteOffset := searchFrom + idx
		if seen == n {
			return len([]rune(text[:byteOffset])), true
		}
		seen++
		searchFrom = byteOffset + len(token)
	}
}

func assertRenderedFrameFits(t *testing.T, rendered string, wantHeight, wantWidth int) {
	t.Helper()
	if got := lineCount(rendered); got != wantHeight {
		t.Fatalf("rendered frame height=%d, want terminal height=%d:\n%s", got, wantHeight, stripANSI(rendered))
	}
	maxPhysicalWidth := components.TerminalSafeWidth(wantWidth)
	for i, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > maxPhysicalWidth {
			t.Fatalf("rendered frame line %d width=%d exceeds terminal safe width=%d: %q", i, got, maxPhysicalWidth, stripANSI(line))
		}
	}
}

type layoutTestAgent struct {
	*pubsub.Broker[agent.AgentEvent]
	model models.Model
}

func newLayoutTestAgent() *layoutTestAgent {
	return &layoutTestAgent{
		Broker: pubsub.NewBroker[agent.AgentEvent](),
		model:  models.Model{Name: "test-model", DefaultMaxTokens: 4096, ContextWindow: 128000},
	}
}

func (a *layoutTestAgent) Model() models.Model {
	return a.model
}

func (a *layoutTestAgent) SetModel(model models.Model) error {
	a.model = model
	return nil
}
func (a *layoutTestAgent) ReloadProvider() error { return nil }
func (a *layoutTestAgent) Run(context.Context, string, string, ...message.ContentPart) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent)
	close(ch)
	return ch, nil
}
func (a *layoutTestAgent) Cancel(string)                                      {}
func (a *layoutTestAgent) IsSessionBusy(string) bool                          { return false }
func (a *layoutTestAgent) IsBusy() bool                                       { return false }
func (a *layoutTestAgent) SetHookService(hooks.Service)                       {}
func (a *layoutTestAgent) SetTaskRegistry(*task.Registry)                     {}
func (a *layoutTestAgent) SetPostSamplingRegistry(agent.PostSamplingRegistry) {}
func (a *layoutTestAgent) SetCheckpointStore(agent.CheckpointStore)           {}
func (a *layoutTestAgent) SetForkedRunner(agent.ForkedRunner)                 {}
func (a *layoutTestAgent) SetSessionMemoryLoader(func(context.Context, string) (string, error)) {
}
func (a *layoutTestAgent) SetFileReadNotifier(func(string, string, string)) {}
func (a *layoutTestAgent) SetFileToolUsageNotifier(func(string, llmtools.FileToolUsageEvent)) {
}
func (a *layoutTestAgent) SetPermissionService(permission.Service) {}
func (a *layoutTestAgent) SetPlanService(plan.Service)             {}
func (a *layoutTestAgent) CostState() agent.SessionCostState {
	return agent.SessionCostState{ByModel: map[string]*agent.ModelUsage{}, Total: 0.123}
}
func (a *layoutTestAgent) LastInputTokens() int64 { return 0 }
func (a *layoutTestAgent) CompactSession(context.Context, string, string) error {
	return nil
}
func (a *layoutTestAgent) ContextSnapshot() agent.ContextSnapshot {
	return agent.ContextSnapshot{}
}
func (a *layoutTestAgent) AnalyzeContext(context.Context, string) (agent.ContextReport, error) {
	return agent.ContextReport{}, nil
}
func (a *layoutTestAgent) ResetTelemetry() {}

func testMainScreenModel(name string, provider models.ModelProvider) models.Model {
	return models.Model{
		Name:             name,
		Provider:         provider,
		DefaultMaxTokens: 4096,
		ContextWindow:    128000,
	}
}

func TestMainScreenIntroFreezesAfterFirstRenderAtSameWidth(t *testing.T) {
	m := newTestModel()
	m.width = 96
	m.height = 24
	agent := newLayoutTestAgent()
	if err := agent.SetModel(testMainScreenModel("intro-freeze-a", models.ProviderDeepSeek)); err != nil {
		t.Fatalf("set first model: %v", err)
	}
	m.app = &app.App{CoderAgent: agent}
	m.recalcLayout()

	_ = m.updateViewportContent()
	first := stripANSI(m.chat.renderedContent)
	if count := strings.Count(first, "intro-freeze-a"); count != 1 {
		t.Fatalf("first intro should contain first model once, count=%d content=%q", count, first)
	}

	if err := agent.SetModel(testMainScreenModel("intro-freeze-b", models.ProviderOpenAI)); err != nil {
		t.Fatalf("set second model: %v", err)
	}
	_ = m.updateViewportContent()

	got := stripANSI(m.chat.renderedContent)
	if strings.Contains(got, "intro-freeze-b") {
		t.Fatalf("same-width intro should remain frozen, got %q", got)
	}
	if count := strings.Count(got, "intro-freeze-a"); count != 1 {
		t.Fatalf("frozen intro should still contain first model once, count=%d content=%q", count, got)
	}
}

func TestMainScreenIntroRefreshesWhenWidthChanges(t *testing.T) {
	m := newTestModel()
	m.width = 96
	m.height = 24
	agent := newLayoutTestAgent()
	if err := agent.SetModel(testMainScreenModel("intro-width-a", models.ProviderDeepSeek)); err != nil {
		t.Fatalf("set first model: %v", err)
	}
	m.app = &app.App{CoderAgent: agent}
	m.recalcLayout()
	_ = m.updateViewportContent()

	if err := agent.SetModel(testMainScreenModel("intro-width-b", models.ProviderOpenAI)); err != nil {
		t.Fatalf("set second model: %v", err)
	}
	m.width = 108
	m.recalcLayout()
	_ = m.updateViewportContent()

	got := stripANSI(m.chat.renderedContent)
	if !strings.Contains(got, "intro-width-b") {
		t.Fatalf("width change should refresh frozen intro, got %q", got)
	}
	if strings.Contains(got, "intro-width-a") {
		t.Fatalf("width change should replace old intro model, got %q", got)
	}
}

func firstVisibleMarker(view string) string {
	return regexp.MustCompile(`anchor-\d{2}`).FindString(view)
}

func markerIndex(marker string) int {
	var idx int
	fmt.Sscanf(marker, "anchor-%02d", &idx)
	return idx
}

func lineForMessageStart(m *Model, msgID string) int {
	blocks := m.chat.blockList.All()
	for i, block := range blocks {
		if block.MsgID == msgID {
			return components.DisplayHeightBefore(m.chat.heightCache, blocks, i, 1)
		}
	}
	return 0
}

func collectPrintedBodies(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	return collectPrintedBodiesFromMsg(cmd())
}

func collectPrintedBodiesFromMsg(msg tea.Msg) []string {
	if msg == nil {
		return nil
	}
	v := reflect.ValueOf(msg)
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Struct {
		field := v.FieldByName("messageBody")
		if field.IsValid() && field.Kind() == reflect.String {
			return []string{field.String()}
		}
		return nil
	}
	if v.Kind() != reflect.Slice {
		return nil
	}

	var bodies []string
	for i := 0; i < v.Len(); i++ {
		item := v.Index(i)
		if !item.IsValid() || !item.CanInterface() {
			continue
		}
		if cmd, ok := item.Interface().(tea.Cmd); ok {
			bodies = append(bodies, collectPrintedBodies(cmd)...)
			continue
		}
		if nested, ok := item.Interface().(tea.Msg); ok {
			bodies = append(bodies, collectPrintedBodiesFromMsg(nested)...)
		}
	}
	return bodies
}

func TestUpdateViewportContent_MainScreenRendersCompletedTranscriptInViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "user-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "hello from transcript"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := strings.Join(collectPrintedBodies(cmd), "\n")
	if strings.TrimSpace(stripANSI(printed)) != "" {
		t.Fatalf("main-screen should not emit native scrollback print, got %q", printed)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "hello from transcript") {
		t.Fatalf("completed transcript should remain in natural main-screen frame, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenControllerRendersNaturalTranscriptFrame(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	controller := NewMainScreenOutputController(MainScreenResetModeFull)
	m.mainOutput = controller
	m.mainResetMode = MainScreenResetModeFull
	m.chat.messages = []message.Message{
		{
			ID:   "controller-user",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "controller completed turn"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("controller path must not emit native scrollback print, got %q", printed)
	}
	token := m.View()
	var out strings.Builder
	if _, err := controller.WrapOutput(&out).Write([]byte("bubbletea-prefix" + token + "bubbletea-suffix")); err != nil {
		t.Fatalf("controller write failed: %v", err)
	}
	rendered := stripANSI(out.String())
	if !strings.Contains(rendered, "controller completed turn") {
		t.Fatalf("controller output should render current transcript frame, got %q", rendered)
	}
	if strings.Contains(rendered, "bubbletea-prefix") || strings.Contains(rendered, "bubbletea-suffix") {
		t.Fatalf("controller should suppress Bubble Tea frame bytes, got %q", rendered)
	}
	if !strings.Contains(stripANSI(m.chat.renderedContent), "controller completed turn") {
		t.Fatalf("completed transcript should stay in natural frame content, got %q", m.chat.renderedContent)
	}
}

func TestView_MainScreenDoesNotPadIdleHeaderToViewportHeight(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()

	_ = m.updateViewportContent()
	view := stripANSI(m.View())
	if !strings.Contains(view, "OpenScholar vdev") {
		t.Fatalf("main-screen idle transcript should include intro, got %q", view)
	}
	lines := strings.Split(view, "\n")
	leadingBlank := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			break
		}
		leadingBlank++
	}
	if leadingBlank != 0 {
		t.Fatalf("main-screen idle header should render without leading viewport padding, got %d blank lines", leadingBlank)
	}
}

func TestFitRenderedHeightUsesTerminalSafeWidthAndBlankRows(t *testing.T) {
	got := fitRenderedHeight(strings.Repeat("x", 10), 3, 5)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("fitRenderedHeight line count=%d, want 3: %q", len(lines), got)
	}
	if lipgloss.Width(lines[0]) > components.TerminalSafeWidth(5) {
		t.Fatalf("first line width=%d exceeds safe width=%d: %q", lipgloss.Width(lines[0]), components.TerminalSafeWidth(5), lines[0])
	}
	if lines[1] != "" || lines[2] != "" {
		t.Fatalf("blank filler rows should be empty strings, got %#v", lines[1:])
	}
}

func TestSanitizeMainScreenFrame_NormalizesCRLFAndBoundsWidth(t *testing.T) {
	width := 10
	frame := "abc\r\n" + "\x1b[31m" + strings.Repeat("x", 32) + "\x1b[0m\r\n"
	got := sanitizeMainScreenFrame(frame, width)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("sanitized line count=%d, want 3: %q", len(lines), got)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("sanitized frame should normalize CRLF to LF, got %q", got)
	}
	maxSafe := components.TerminalSafeWidth(width)
	for i, line := range lines {
		if w := lipgloss.Width(line); w > maxSafe {
			t.Fatalf("line %d width=%d exceeds safe width=%d: %q", i, w, maxSafe, stripANSI(line))
		}
	}
}

func TestSanitizeMainScreenFrame_WidthOneReturnsSafeEmptyFrame(t *testing.T) {
	got := sanitizeMainScreenFrame("first\r\nsecond", 1)
	if got != "\n" {
		t.Fatalf("width=1 should return safe empty frame preserving structure, got %q", got)
	}
}

func TestView_MainScreenDoesNotRenderFixedHeaderWithMessages(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "user-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "message body"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()

	view := stripANSI(m.View())
	if !strings.Contains(view, "OpenScholar vdev") {
		t.Fatalf("main-screen natural frame should keep intro in transcript, got %q", view)
	}
	if !strings.Contains(view, "message body") {
		t.Fatalf("completed main-screen message should render in natural frame, got %q", view)
	}
}

func TestView_MainScreenTerminalSurfaceNoHeaderAfterFlushedAssistant(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "asst-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "assistant completed reply"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	frame := stripANSI(m.View())
	surface := printed + "\n" + frame

	if count := strings.Count(surface, "assistant completed reply"); count != 1 {
		t.Fatalf("assistant reply should appear exactly once on terminal surface, count=%d surface=%q", count, surface)
	}

	replyIdx := strings.Index(surface, "assistant completed reply")
	headerIdx := strings.Index(surface, "OpenScholar vdev")
	if headerIdx < 0 || headerIdx > replyIdx {
		t.Fatalf("intro should appear before assistant reply in natural frame, surface=%q", surface)
	}
	if !strings.Contains(frame, "What shall we build today?") {
		t.Fatalf("final frame should still include input area, got %q", frame)
	}
}

func TestUpdateViewportContent_MainScreenFlushesIntroBeforeFirstCompletedMessage(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "asst-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "first completed reply"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.TrimSpace(printed) != "" {
		t.Fatalf("main-screen should not print intro/message separately, got %q", printed)
	}
	view := stripANSI(m.View())
	introIdx := strings.Index(view, "OpenScholar vdev")
	replyIdx := strings.Index(view, "first completed reply")
	if introIdx < 0 || replyIdx < 0 || introIdx > replyIdx {
		t.Fatalf("intro should render before first completed message, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenResetRendersNewTranscript(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "old-user",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "old transcript"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	if cmd := m.updateViewportContent(); cmd != nil {
		if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); strings.TrimSpace(printed) != "" {
			t.Fatalf("main-screen should not print old transcript before reset, got %q", printed)
		}
	}
	m.resetSessionState()
	m.chat.messages = []message.Message{
		{
			ID:   "new-user",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "new transcript"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	if cmd := m.updateViewportContent(); cmd != nil {
		if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); strings.TrimSpace(printed) != "" {
			t.Fatalf("main-screen should not print new transcript after reset, got %q", printed)
		}
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "old transcript") || !strings.Contains(view, "new transcript") {
		t.Fatalf("reset should replace transcript content, got %q", view)
	}
}

func TestUpdate_LoadSessionMsg_MainScreenDoesNotFoldEarlyMessages(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 250; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("resume-%03d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("resume-%03d", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}

	next, _ := m.Update(loadSessionMsg{
		session:      session.Session{ID: "sess-resume"},
		messages:     msgs,
		toolMessages: map[string]message.Message{},
	})

	updated := next.(Model)
	view := stripANSI(updated.View())
	if strings.Contains(view, "以上 50 条消息已折叠") {
		t.Fatalf("resume should no longer fold early messages by default, got %q", view)
	}
	if !strings.Contains(view, "resume-000") || !strings.Contains(view, "resume-249") {
		t.Fatalf("resumed transcript should render as natural frame content, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenCapAfterInitialFrameDoesNotClearScrollback(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.mainOutput = NewMainScreenOutputController(MainScreenResetModeFull)
	m.mainResetMode = MainScreenResetModeFull
	m.recalcLayout()

	for i := 0; i < nonFullscreenMessageCap+nonFullscreenMessageCapStep+10; i++ {
		m.chat.messages = append(m.chat.messages, message.Message{
			ID:   fmt.Sprintf("cap-%03d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("cap-%03d", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}

	_ = m.updateViewportContent()
	firstToken := m.View()
	var firstOut strings.Builder
	if _, err := m.mainOutput.WrapOutput(&firstOut).Write([]byte(firstToken)); err != nil {
		t.Fatalf("first frame write failed: %v", err)
	}
	firstRendered := stripANSI(firstOut.String())
	if !strings.Contains(firstRendered, "cap-000") || !strings.Contains(firstRendered, "cap-259") {
		t.Fatalf("first frame should render full resumed history before cap, got %q", firstRendered)
	}

	_ = m.updateViewportContent()
	secondToken := m.View()
	var secondOut strings.Builder
	if _, err := m.mainOutput.WrapOutput(&secondOut).Write([]byte(secondToken)); err != nil {
		t.Fatalf("second frame write failed: %v", err)
	}
	stats := m.mainOutput.Stats()
	if stats.FullReset || stats.OffscreenReset || strings.Contains(secondOut.String(), "\x1b[3J") {
		t.Fatalf("cap transition must not clear native scrollback, stats=%+v output=%q", stats, secondOut.String())
	}
	secondRendered := stripANSI(secondOut.String())
	if strings.Contains(secondRendered, "cap-000") || !strings.Contains(secondRendered, "cap-259") {
		t.Fatalf("capped frame should repaint current tail only, got %q", secondRendered)
	}
}

func TestUpdateViewportContent_MainScreenDoesNotCapBeforeFirstFrameConsumed(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.mainOutput = NewMainScreenOutputController(MainScreenResetModeFull)
	m.mainResetMode = MainScreenResetModeFull
	m.recalcLayout()

	for i := 0; i < nonFullscreenMessageCap+nonFullscreenMessageCapStep+10; i++ {
		m.chat.messages = append(m.chat.messages, message.Message{
			ID:   fmt.Sprintf("cap-pending-%03d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("cap-pending-%03d", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}

	_ = m.updateViewportContent()
	_ = m.View() // stage a full-history frame, but deliberately do not consume it.
	if stats := m.mainOutput.Stats(); stats.FrameSeq == 0 || stats.ConsumedFrameSeq != 0 {
		t.Fatalf("test setup should have staged but not consumed a frame, stats=%+v", stats)
	}

	_ = m.updateViewportContent()
	token := m.View()
	var out strings.Builder
	if _, err := m.mainOutput.WrapOutput(&out).Write([]byte(token)); err != nil {
		t.Fatalf("coalesced frame write failed: %v", err)
	}
	rendered := stripANSI(out.String())
	if !strings.Contains(rendered, "cap-pending-000") || !strings.Contains(rendered, "cap-pending-259") {
		t.Fatalf("unconsumed initial frame must not allow cap yet, got %q", rendered)
	}
}

func TestBuildScreenVM_MainScreenUsesContentBackedViewportSlice(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.recalcLayout()

	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m.chat.renderedContent = strings.Join(lines, "\n")
	m.chat.contentLines = append([]string(nil), lines...)
	m.chat.scrollMode = ScrollManualLocked
	m.chat.viewport.SetContent(strings.Repeat("stale viewport line\n", 40))
	m.chat.viewport.SetYOffset(5)

	screen := m.buildScreenVM()
	end := min(5+m.chat.viewport.Height, len(lines))
	want := strings.Join(lines[5:end], "\n")
	if got := strings.TrimSpace(stripANSI(screen.Transcript)); got != want {
		t.Fatalf("transcript should come from the rendered transcript slice, got %q want %q", got, want)
	}
}

func TestBuildScreenVM_HeaderOwnershipByScreenMode(t *testing.T) {
	main := newTestModel()
	main.setScreenMode(ScreenModeMain)
	main.width = 80
	main.height = 24
	main.recalcLayout()
	mainScreen := main.buildScreenVM()
	if strings.Contains(stripANSI(mainScreen.Header), "OpenScholar vdev") || strings.TrimSpace(stripANSI(mainScreen.Header)) != "" {
		t.Fatalf("main-screen should not populate Header, got %q", stripANSI(mainScreen.Header))
	}

	full := newTestModel()
	full.setScreenMode(ScreenModeFullscreen)
	full.width = 80
	full.height = 24
	full.recalcLayout()
	fullScreen := full.buildScreenVM()
	if !strings.Contains(stripANSI(fullScreen.Header), "OpenScholar vdev") {
		t.Fatalf("fullscreen should retain fixed header, got %q", stripANSI(fullScreen.Header))
	}
}

func TestUpdateViewportContent_FullscreenDoesNotEmitNativePrintForCompletedReply(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "asst-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "fullscreen completed reply"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("fullscreen mode should not emit native scrollback print, got %q", printed)
	}
	if !strings.Contains(stripANSI(m.View()), "OpenScholar vdev") {
		t.Fatalf("fullscreen view should keep fixed header")
	}
}

func TestStatusEstimateMessagesHonorsCompactBoundary(t *testing.T) {
	m := newTestModel()
	m.chat.messages = []message.Message{
		{
			ID:   "old",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: strings.Repeat("old context ", 100)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "summary",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "[Conversation Summary]\nshort"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "new",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "new context"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	m.sessionSummaryMessageID = "summary"

	got := m.statusEstimateMessages()
	if len(got) != 2 || got[0].ID != "summary" || got[1].ID != "new" {
		t.Fatalf("status estimate should use compact boundary onward, got %+v", got)
	}
}

func TestStatusEstimateMessagesSkipsUICommandActivity(t *testing.T) {
	m := newTestModel()
	m.chat.messages = []message.Message{
		{
			ID:   "u1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "hello"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "sys1",
			Role: message.System,
			Meta: map[string]any{
				"ui_command_activity": true,
				"command_invocation":  "/config",
				"command_summary":     "Settings dialog dismissed",
			},
			Parts: []message.ContentPart{
				message.TextContent{Text: "Settings dialog dismissed"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	got := m.statusEstimateMessages()
	if len(got) != 1 || got[0].ID != "u1" {
		t.Fatalf("status estimate should skip UI-only command activity, got %+v", got)
	}
}

func TestScrollToMessage_MainScreenUsesBlockAnchors(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 40
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 30; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("msg-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("msg-%02d line A line B line C line D line E", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	targetIdx := 9
	m.scrollToMessage(targetIdx)
	if m.chat.scrollMode != ScrollManualLocked {
		t.Fatalf("scrollToMessage should lock manual mode, got %v", m.chat.scrollMode)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, fmt.Sprintf("msg-%02d", targetIdx)) {
		t.Fatalf("scrollToMessage should bring target message into visible main-screen viewport, got %q", view)
	}
}

func TestScrollToMessage_MainScreenFirstMessageVisibleWithoutFoldBanner(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 250; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("resume-%03d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("resume-%03d", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}

	next, _ := m.Update(loadSessionMsg{
		session:      session.Session{ID: "sess-resume"},
		messages:     msgs,
		toolMessages: map[string]message.Message{},
	})

	updated := next.(Model)
	updated.scrollToMessage(0)
	view := stripANSI(updated.View())
	if !strings.Contains(view, "resume-000") {
		t.Fatalf("first message should be visible after jumping to index 0, got %q", view)
	}
	if strings.Contains(view, "以上 50 条消息已折叠") {
		t.Fatalf("main-screen should not render folded-tail banner, got %q", view)
	}
}

func TestUpdate_MainScreenRendersConversationInSingleViewportTranscript(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.sessionID = "sess-1"
	m.status.isProcessing = true
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-0",
			SessionID: "sess-1",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "older prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-0",
			SessionID: "sess-1",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "older answer"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "user-1",
			SessionID: "sess-1",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	next, _ := m.Update(pubsub.Event[message.Message]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Message{
			ID:        "asst-1",
			SessionID: "sess-1",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "assistant delta"},
			},
		},
	})

	updated := next.(Model)
	view := stripANSI(updated.View())
	if !strings.Contains(view, "older prompt") || !strings.Contains(view, "older answer") {
		t.Fatalf("older completed turn should remain in natural transcript frame: %q", view)
	}
	if !strings.Contains(view, "assistant delta") {
		t.Fatalf("latest assistant content missing from viewport: %q", view)
	}
}

func TestUpdateViewportContent_MainScreenStreamingUsesSingleViewportTranscript(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.status.isProcessing = true
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := strings.Join(collectPrintedBodies(cmd), "\n")
	plainPrinted := stripANSI(printed)
	if strings.TrimSpace(plainPrinted) != "" {
		t.Fatalf("main-screen should not print completed prefix during streaming, got %q", plainPrinted)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "current prompt") || !strings.Contains(view, "tok16") {
		t.Fatalf("streaming transcript should render in natural frame: %q", view)
	}
	if !strings.Contains(view, "tok01") {
		t.Fatalf("open stream paragraph should remain live to avoid visual-line reflow duplicates, got %q", view)
	}
}

func TestRenderBlockSliceWithWidthAndOverride_CachesPureHeightForLiveTail(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.recalcLayout()

	blocks := m.buildBlocksForMessages([]message.Message{
		{
			ID:   "user-stream",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "asst-stream",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	})
	blockIdx := findAssistantTextBlock(blocks, "asst-stream")
	if blockIdx < 0 {
		t.Fatal("expected assistant stream block")
	}

	split := components.RenderStreamTailBlockSplitV2(&blocks[blockIdx], m.blockRenderContext())
	if len(split.CommittedLines) != 0 {
		t.Fatalf("open paragraph should not commit visual lines, got %+v", split.CommittedLines)
	}
	if len(split.LiveLines) != len(split.RenderedLines) {
		t.Fatalf("open paragraph should keep all rendered lines live, live=%d rendered=%d", len(split.LiveLines), len(split.RenderedLines))
	}

	_ = m.renderBlockSliceWithWidthAndOverride(blocks, m.width, blockIdx, renderOverrideLines(split.LiveLines))

	height, ok := m.chat.heightCache.Get(blocks[blockIdx].ID)
	if !ok {
		t.Fatal("expected override height cached")
	}
	if height != len(split.LiveLines) {
		t.Fatalf("assistant override height=%d, want %d live lines without boundary", height, len(split.LiveLines))
	}
}

func TestUpdateViewportContent_MainScreenResizeKeepsFlushedPrefixOutOfViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.status.isProcessing = true
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}

	firstCmd := m.updateViewportContent()
	firstPrinted := stripANSI(strings.Join(collectPrintedBodies(firstCmd), "\n"))
	if strings.TrimSpace(firstPrinted) != "" {
		t.Fatalf("initial streaming render should not print completed prefix, got %q", firstPrinted)
	}

	m.width = 24
	m.recalcLayout()

	secondCmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(secondCmd), "\n")); strings.Contains(printed, "current prompt") {
		t.Fatalf("resize should not duplicate already-flushed lines, got %q", printed)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "current prompt") || !strings.Contains(view, "tok16") {
		t.Fatalf("resize should preserve natural streaming transcript: %q", view)
	}

	m.status.isProcessing = false
	m.chat.messages[1].Parts = append(m.chat.messages[1].Parts, message.Finish{Reason: message.FinishReasonEndTurn})

	thirdCmd := m.updateViewportContent()
	thirdPrinted := stripANSI(strings.Join(collectPrintedBodies(thirdCmd), "\n"))
	if strings.TrimSpace(thirdPrinted) != "" {
		t.Fatalf("completion should not print remaining tail, got %q", thirdPrinted)
	}
	view = stripANSI(m.View())
	if !strings.Contains(view, "tok16") {
		t.Fatalf("completion should keep assistant text in natural frame, got %q", view)
	}

	m.status.isProcessing = true
	m.chat.messages = append(m.chat.messages, message.Message{
		ID:        "user-next",
		SessionID: "sess-stream",
		Role:      message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "next prompt"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})

	fourthCmd := m.updateViewportContent()
	if fourthPrinted := stripANSI(strings.Join(collectPrintedBodies(fourthCmd), "\n")); strings.Contains(fourthPrinted, "tok16") {
		t.Fatalf("next turn should not reprint prior assistant text, got %q", fourthPrinted)
	}
	if !strings.Contains(stripANSI(m.View()), "tok16") {
		t.Fatalf("next turn view should retain prior assistant in natural transcript, got %q", stripANSI(m.View()))
	}
}

func TestUpdateViewportContent_MainScreenBoundsOpenStreamParagraphAfterResize(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.status.isProcessing = true
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}

	_ = m.updateViewportContent()
	m.width = 24
	m.recalcLayout()
	resizePrinted := stripANSI(strings.Join(collectPrintedBodies(m.updateViewportContent()), "\n"))
	if strings.Contains(resizePrinted, "current prompt") {
		t.Fatalf("resize should not duplicate completed prefix, got %q", resizePrinted)
	}

	m.chat.messages[1].Parts[0] = message.TextContent{
		Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16 tok17 tok18 tok19 tok20 tok21 tok22 tok23 tok24 tok25 tok26 tok27 tok28 tok29 tok30 tok31 tok32",
	}

	cmd := m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	allPrinted := resizePrinted + "\n" + printed
	if strings.TrimSpace(allPrinted) != "" {
		t.Fatalf("resized long paragraph should not print stable prefixes, got %q", allPrinted)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "tok01") || !strings.Contains(view, "tok32") {
		t.Fatalf("post-resize natural frame should keep full stream text, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenDoesNotDuplicateStreamTailAcrossNextTurn(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-stream-dup"
	m.recalcLayout()

	tail := "请告诉我你目前的研究方向或需要帮助的任务，我们可以开始工作了。"
	m.chat.messages = []message.Message{
		{
			ID:        "asst-stream",
			SessionID: "sess-stream-dup",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "• 润色和检查论文质量（语法、引用、格式等）\n• 构建知识库，管理你读过的论文\n\n" + tail},
			},
		},
	}

	firstCmd := m.updateViewportContent()
	firstPrinted := stripANSI(strings.Join(collectPrintedBodies(firstCmd), "\n"))
	if strings.TrimSpace(firstPrinted) != "" {
		t.Fatalf("streaming paragraph should not print native scrollback, got %q", firstPrinted)
	}

	m.status.isProcessing = false
	m.chat.messages[0].Parts = append(m.chat.messages[0].Parts, message.Finish{Reason: message.FinishReasonEndTurn})
	secondCmd := m.updateViewportContent()
	secondPrinted := stripANSI(strings.Join(collectPrintedBodies(secondCmd), "\n"))
	if strings.TrimSpace(secondPrinted) != "" {
		t.Fatalf("finished stream tail should not print native scrollback, got %q", secondPrinted)
	}

	m.status.isProcessing = true
	m.chat.messages = append(m.chat.messages, message.Message{
		ID:        "user-next",
		SessionID: "sess-stream-dup",
		Role:      message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "讲点有意思的"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})
	thirdCmd := m.updateViewportContent()
	thirdPrinted := stripANSI(strings.Join(collectPrintedBodies(thirdCmd), "\n"))
	if strings.Contains(thirdPrinted, tail) {
		t.Fatalf("next turn should not reprint finished stream tail, got %q", thirdPrinted)
	}

	surface := strings.Join([]string{firstPrinted, secondPrinted, thirdPrinted, stripANSI(m.View())}, "\n")
	if count := strings.Count(surface, tail); count != 1 {
		t.Fatalf("stream tail should appear exactly once across terminal surface, count=%d surface=%q", count, surface)
	}
}

func TestUpdateViewportContent_MainScreenFlushesOnlyNewStableStreamSource(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-stream-progressive"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "asst-stream",
			SessionID: "sess-stream-progressive",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "first paragraph\n\nlive tail"},
			},
		},
	}

	firstCmd := m.updateViewportContent()
	firstPrinted := stripANSI(strings.Join(collectPrintedBodies(firstCmd), "\n"))
	if strings.TrimSpace(firstPrinted) != "" {
		t.Fatalf("first stable paragraph should not print native scrollback, got %q", firstPrinted)
	}

	m.chat.messages[0].Parts[0] = message.TextContent{
		Text: "first paragraph\n\nsecond paragraph\n\nlive tail",
	}
	secondCmd := m.updateViewportContent()
	secondPrinted := stripANSI(strings.Join(collectPrintedBodies(secondCmd), "\n"))
	if strings.TrimSpace(secondPrinted) != "" {
		t.Fatalf("second stable paragraph should not print native scrollback, got %q", secondPrinted)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "first paragraph") || !strings.Contains(view, "second paragraph") {
		t.Fatalf("stable paragraphs should render in natural frame, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenSearchBlocksNativeFlush(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "asst-search",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "search should keep this in the viewport"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	m.search.textSearch.SetWidth(m.width)
	_ = m.search.textSearch.Show()

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("active search should block native flush, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "search should keep this in the viewport") {
		t.Fatalf("search-blocked transcript should remain viewport-rendered, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenSelectionBlocksNativeFlush(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "asst-selection",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "selection should keep this in the viewport"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	m.chat.selection = TextSelection{HasRange: true}

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("active selection should block native flush, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "selection should keep this in the viewport") {
		t.Fatalf("selection-blocked transcript should remain viewport-rendered, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenKeepsThinkingAboveStreamingText(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-stream-thinking"
	m.recalcLayout()

	m.chat.messages = []message.Message{
		{
			ID:        "asst-stream",
			SessionID: "sess-stream-thinking",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "checking the arXiv origin story"},
				message.TextContent{Text: "3. arXiv 的诞生故事\n\nPaul Ginsparg built an FTP server in 1991."},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.Contains(printed, "3. arXiv") || strings.Contains(printed, "Paul Ginsparg") {
		t.Fatalf("streaming text must not flush above visible thinking, got %q", printed)
	}

	view := stripANSI(m.View())
	thinkingIdx := strings.Index(view, "Thinking")
	textIdx := strings.Index(view, "3. arXiv")
	if thinkingIdx < 0 || textIdx < 0 {
		t.Fatalf("live viewport should include thinking and streaming text, got %q", view)
	}
	if thinkingIdx > textIdx {
		t.Fatalf("thinking should render above streaming text, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenFlushesThinkingBlockedStreamOnCompletion(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-stream-thinking-complete"
	m.recalcLayout()

	m.chat.messages = []message.Message{
		{
			ID:        "asst-stream",
			SessionID: "sess-stream-thinking-complete",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "checking the arXiv origin story"},
				message.TextContent{Text: "3. arXiv 的诞生故事\n\nPaul Ginsparg built an FTP server in 1991."},
			},
		},
	}

	streamCmd := m.updateViewportContent()
	streamPrinted := stripANSI(strings.Join(collectPrintedBodies(streamCmd), "\n"))
	if strings.Contains(streamPrinted, "3. arXiv") || strings.Contains(streamPrinted, "Paul Ginsparg") {
		t.Fatalf("streaming text must not flush while thinking is visible, got %q", streamPrinted)
	}

	m.status.isProcessing = false
	m.chat.messages[0].Parts = append(m.chat.messages[0].Parts, message.Finish{Reason: message.FinishReasonEndTurn})
	doneCmd := m.updateViewportContent()
	donePrinted := stripANSI(strings.Join(collectPrintedBodies(doneCmd), "\n"))
	if strings.TrimSpace(donePrinted) != "" {
		t.Fatalf("completion must not print finished assistant text, got %q", donePrinted)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "3. arXiv 的诞生故事") || !strings.Contains(view, "Paul Ginsparg built an FTP server in 1991.") {
		t.Fatalf("completed assistant text should remain in natural frame, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenControllerKeepsCompletedThinkingTranscriptInTerminalScrollback(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-stream-thinking-controller"
	m.mainOutput = NewMainScreenOutputController(MainScreenResetModeFull)
	m.mainResetMode = MainScreenResetModeFull
	m.recalcLayout()

	m.chat.messages = []message.Message{
		{
			ID:        "asst-stream",
			SessionID: "sess-stream-thinking-controller",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "checking the arXiv origin story"},
				message.TextContent{Text: "3. arXiv 的诞生故事\n\nPaul Ginsparg built an FTP server in 1991."},
			},
		},
	}

	streamCmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(streamCmd), "\n")); printed != "" {
		t.Fatalf("controller path must not emit native flush output, got %q", printed)
	}
	streamToken := m.View()
	var streamOut strings.Builder
	if _, err := m.mainOutput.WrapOutput(&streamOut).Write([]byte(streamToken)); err != nil {
		t.Fatalf("stream write failed: %v", err)
	}
	if stats := m.mainOutput.Stats(); stats.FrameSeq == 0 {
		t.Fatalf("streaming frame should stage renderer frame stats, got %+v", stats)
	}

	m.status.isProcessing = false
	m.chat.messages[0].Parts = append(m.chat.messages[0].Parts, message.Finish{Reason: message.FinishReasonEndTurn})
	doneCmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(doneCmd), "\n")); printed != "" {
		t.Fatalf("controller completion path must not emit native flush output, got %q", printed)
	}
	doneToken := m.View()
	var doneOut strings.Builder
	if _, err := m.mainOutput.WrapOutput(&doneOut).Write([]byte(doneToken)); err != nil {
		t.Fatalf("completion write failed: %v", err)
	}
	rendered := stripANSI(doneOut.String())
	if !strings.Contains(rendered, "3. arXiv 的诞生故事") || !strings.Contains(rendered, "Paul Ginsparg built an FTP server in 1991.") {
		t.Fatalf("controller frame should render completed transcript, got %q", rendered)
	}
}

func TestUpdateViewportContent_MainScreenEmptyFlushRangeDoesNotAdvanceLiveTail(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.status.isProcessing = true
	m.sessionID = "sess-empty-flush-range"
	m.recalcLayout()

	m.chat.messages = []message.Message{
		{
			ID:        "hidden-thinking",
			SessionID: "sess-empty-flush-range",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "finished collapsed thinking only"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-empty-flush-range",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "live answer stays visible"},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); strings.TrimSpace(printed) != "" {
		t.Fatalf("empty rendered prefix should not emit native flush output, got %q", printed)
	}
	if m.chat.flushedAnchor != nil {
		t.Fatalf("empty rendered prefix must not advance flushed anchor, got %#v", m.chat.flushedAnchor)
	}
	if m.chat.liveTailAnchor != nil {
		t.Fatalf("empty rendered prefix must not advance live tail anchor, got %#v", m.chat.liveTailAnchor)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "live answer stays visible") {
		t.Fatalf("streaming message should remain live after empty prefix range, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenKeepsCompletedAssistantOrderedBeforeLiveTail(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 48
	m.height = 10
	m.status.isProcessing = true
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-complete",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "completed tool summary"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}

	cmd := m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.TrimSpace(printed) != "" {
		t.Fatalf("completed assistant text should not print native scrollback, got %q", printed)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "completed tool summary") || !strings.Contains(view, "tok16") {
		t.Fatalf("natural frame should keep completed assistant before stream tail, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenWaitsForToolResultBeforeFlush(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 12
	m.sessionID = "sess-tools"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "asst-tool",
			SessionID: "sess-tools",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "I'll inspect the file."},
				message.ToolCall{
					ID:    "tc-1",
					Name:  "Bash",
					Input: `{"command":"printf tool-output"}`,
					State: message.ToolCallQueued,
				},
				message.Finish{Reason: message.FinishReasonToolUse},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("assistant tool turn should not flush while tool is queued, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "I'll inspect the file.") || !strings.Contains(view, "Bash") {
		t.Fatalf("unstable tool turn should remain live in viewport, got %q", view)
	}

	m.chat.messages[0].Parts[1] = message.ToolCall{
		ID:    "tc-1",
		Name:  "Bash",
		Input: `{"command":"printf tool-output"}`,
		State: message.ToolCallCompleted,
	}
	cmd = m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("assistant tool turn should wait for tool result, got %q", printed)
	}

	toolMsg := message.Message{
		ID:        "tool-1",
		SessionID: "sess-tools",
		Role:      message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc-1",
				Name:       "Bash",
				Content:    "tool-output",
			},
		},
	}
	m.chat.toolMessages["tc-1"] = toolMsg
	m.chat.messages = append(m.chat.messages, toolMsg)

	cmd = m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.TrimSpace(printed) != "" {
		t.Fatalf("stable tool turn should not print native scrollback, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "tool-output") || !strings.Contains(view, "I'll inspect the file.") {
		t.Fatalf("stable tool turn should render in natural frame, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenStreamingToolTurnFinalizesAfterToolResult(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 48
	m.height = 12
	m.sessionID = "sess-stream-tool"
	m.status.isProcessing = true
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream-tool",
			SessionID: "sess-stream-tool",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "inspect the file"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream-tool",
			SessionID: "sess-stream-tool",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "I'll inspect it before answering with a careful summary."},
			},
		},
	}

	_ = m.updateViewportContent()
	m.chat.messages[1].Parts = append(m.chat.messages[1].Parts,
		message.ToolCall{
			ID:    "tc-stream",
			Name:  "Bash",
			Input: `{"command":"printf stream-tool-output"}`,
			State: message.ToolCallQueued,
		},
		message.Finish{Reason: message.FinishReasonToolUse},
	)
	m.status.isProcessing = false

	cmd := m.updateViewportContent()
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printed != "" {
		t.Fatalf("finished streaming tool turn should wait for tool result before final flush, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Bash") {
		t.Fatalf("unfinished tool turn should remain live, got %q", view)
	}

	m.chat.messages[1].Parts[1] = message.ToolCall{
		ID:    "tc-stream",
		Name:  "Bash",
		Input: `{"command":"printf stream-tool-output"}`,
		State: message.ToolCallCompleted,
	}
	toolMsg := message.Message{
		ID:        "tool-stream",
		SessionID: "sess-stream-tool",
		Role:      message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc-stream",
				Name:       "Bash",
				Content:    "stream-tool-output",
			},
		},
	}
	m.chat.toolMessages["tc-stream"] = toolMsg
	m.chat.messages = append(m.chat.messages, toolMsg)

	cmd = m.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.TrimSpace(printed) != "" {
		t.Fatalf("stable streaming tool turn should not print native scrollback, got %q", printed)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "stream-tool-output") || !strings.Contains(view, "Bash") {
		t.Fatalf("stable streaming tool turn should render in natural frame, got %q", view)
	}
}

func TestUpdateViewportContent_MainScreenManualScrollBlocksStreamingFlush(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.status.isProcessing = true
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.scrollMode = ScrollManualLocked
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printedJoined := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); printedJoined != "" {
		t.Fatalf("manual scroll should not trigger scrollback flushes, got %q", printedJoined)
	}
	if m.chat.viewport.YOffset != 0 {
		t.Fatalf("manual-scroll viewport y-offset should remain anchored, got %d", m.chat.viewport.YOffset)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "current prompt") {
		t.Fatalf("manual-scroll viewport should stay anchored at top of transcript, got %q", view)
	}
}

func TestUpdateViewportContent_FullscreenEmitsNoPrintCommands(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 40
	m.height = 10
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "full-u",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "fullscreen message"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	cmd := m.updateViewportContent()
	if printed := collectPrintedBodies(cmd); len(printed) != 0 {
		t.Fatalf("fullscreen path must not print to native scrollback, got %q", printed)
	}
}

func TestUpdate_MainScreenWindowResizePreservesManualScrollAnchor(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 52
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 18; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("anchor-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("anchor-%02d Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt.", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	m.chat.scrollMode = ScrollManualLocked
	m.chat.viewport.SetYOffset(max(0, lineForMessageStart(m, "anchor-09")-2))
	beforeAnchor := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
	if beforeAnchor.MsgID == "" {
		t.Fatalf("expected a semantic anchor before resize, got %+v view=%q", beforeAnchor, stripANSI(m.View()))
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 28, Height: 10})
	updated := next.(Model)
	afterView := stripANSI(updated.View())
	afterAnchor := components.AnchorFromDisplayLine(updated.chat.heightCache, updated.chat.blockList.All(), updated.chat.viewport.YOffset, 1)
	if afterAnchor.MsgID == "" {
		t.Fatalf("expected a semantic anchor after resize, got %+v view=%q", afterAnchor, afterView)
	}
	diff := afterAnchor.BlockIdx - beforeAnchor.BlockIdx
	if diff < 0 {
		diff = -diff
	}
	if beforeAnchor.MsgID != afterAnchor.MsgID && diff > 1 {
		t.Fatalf("manual scroll anchor should stay near the same message across resize, before=%+v after=%+v view=%q", beforeAnchor, afterAnchor, afterView)
	}
}

func TestUpdate_FullscreenWindowResizePreservesManualScrollAnchor(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 52
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 18; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("anchor-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("anchor-%02d Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt.", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen anchor setup should not emit command, got %v", cmd)
	}

	m.scrollToMessage(9)
	beforeAnchor := m.chat.virtualList.Anchor()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 28, Height: 10})
	updated := next.(Model)
	afterAnchor := updated.chat.virtualList.Anchor()
	diff := afterAnchor.BlockIdx - beforeAnchor.BlockIdx
	if diff < 0 {
		diff = -diff
	}
	if beforeAnchor.MsgID != afterAnchor.MsgID && diff > 1 {
		t.Fatalf(
			"fullscreen manual scroll anchor should stay near the same message across resize, before=%+v after=%+v",
			beforeAnchor,
			afterAnchor,
		)
	}
}

func TestUpdate_FullscreenWindowResizeEmitsNoNativePrintAndKeepsVirtualAnchor(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 52
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("resize-virtual-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("resize-virtual-%02d alpha beta gamma delta epsilon zeta eta theta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen virtual setup should not emit command, got %v", cmd)
	}

	m.scrollToMessage(10)
	before := m.chat.virtualList.Anchor()
	if before.MsgID == "" {
		t.Fatalf("expected semantic virtual anchor before resize, got %+v", before)
	}

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	updated := next.(Model)
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); strings.TrimSpace(printed) != "" {
		t.Fatalf("fullscreen resize must not print to native scrollback, got %q", printed)
	}
	after := updated.chat.virtualList.Anchor()
	if after.MsgID == "" {
		t.Fatalf("expected semantic virtual anchor after resize, got %+v", after)
	}
	diff := after.BlockIdx - before.BlockIdx
	if diff < 0 {
		diff = -diff
	}
	if before.MsgID != after.MsgID && diff > 1 {
		t.Fatalf("fullscreen virtual anchor should stay near the same message across resize, before=%+v after=%+v", before, after)
	}
}

func TestUpdate_FullscreenManualScrollAnchorUsesStableBlockID(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 52
	m.height = 14
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 18; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("anchor-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("anchor-%02d Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt.", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("stable-anchor setup should not emit command, got %v", cmd)
	}

	m.chat.scrollMode = ScrollManualLocked
	m.scrollToMessage(9)
	beforeAnchor := m.chat.virtualList.Anchor()
	if beforeAnchor.BlockID == "" || beforeAnchor.MsgID == "" {
		t.Fatalf("expected semantic anchor before prepend, got %+v", beforeAnchor)
	}

	prepended := message.Message{
		ID:   "prepended",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "prepended message should not become the manual-scroll anchor"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}
	m.chat.messages = append([]message.Message{prepended}, msgs...)
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("stable-anchor update should not emit command, got %v", cmd)
	}

	afterAnchor := m.chat.virtualList.Anchor()
	if afterAnchor.MsgID != beforeAnchor.MsgID {
		t.Fatalf("semantic anchor should keep the saved message across prepend, before=%+v after=%+v view=%q", beforeAnchor, afterAnchor, stripANSI(m.View()))
	}
}

func TestUpdate_FullscreenHeightResizeConsumesPendingAnchor(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 52
	m.height = 6
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 18; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("follow-anchor-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("follow-anchor-%02d Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt.", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen follow-up setup should not emit command, got %v", cmd)
	}

	m.chat.scrollMode = ScrollManualLocked
	maxYOffset := max(0, m.chat.viewport.TotalLineCount()-m.chat.viewport.Height)
	if maxYOffset == 0 {
		t.Fatal("expected scrollable fullscreen transcript for follow-up resize test")
	}
	m.chat.viewport.SetYOffset(max(0, maxYOffset-1))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 52, Height: 12})
	updated := next.(Model)
	if updated.chat.pendingAnchor != nil {
		t.Fatal("fullscreen resize should consume pendingAnchor during the same update")
	}

	updated.chat.messages = append(updated.chat.messages, message.Message{
		ID:   "follow-anchor-next",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "new message after fullscreen height resize"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})
	if cmd := updated.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen follow-up update should not emit command, got %v", cmd)
	}
	if updated.chat.pendingAnchor != nil {
		t.Fatal("fullscreen follow-up update should not resurrect a stale pendingAnchor")
	}
}

func TestUpdate_FullscreenVirtualFeedbackResizeKeepsFeedbackVisible(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.features.MemoryUI = true
	m.width = 56
	m.height = 12
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("feedback-resize-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("feedback-resize-%02d alpha beta gamma delta epsilon zeta eta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	m.chat.inlineErrors = []components.InlineError{{Text: "visible inline feedback"}}
	m.status.memoryToast.Push(components.MemoryToast{FilePath: "visible-memory.md", Action: "updated"})
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen virtual feedback setup should not emit command, got %v", cmd)
	}

	m.chat.scrollMode = ScrollManualLocked
	if m.chat.viewport.YOffset+m.chat.viewport.Height <= m.chat.virtualList.LastFrameHeight() {
		t.Fatalf("setup viewport should overlap appended feedback area, y=%d height=%d frameHeight=%d content=%q",
			m.chat.viewport.YOffset, m.chat.viewport.Height, m.chat.virtualList.LastFrameHeight(), stripANSI(m.chat.renderedContent))
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 12})
	updated := next.(Model)
	if updated.chat.pendingAnchor != nil {
		t.Fatal("fullscreen virtual resize should consume pendingAnchor during the same update")
	}
	view := stripANSI(updated.View())
	if !strings.Contains(view, "visible inline feedback") {
		t.Fatalf("resize should keep inline feedback visible, got %q", view)
	}
	if !strings.Contains(view, "visible-memory.md") {
		t.Fatalf("resize should keep memory toast visible, got %q", view)
	}
}

func TestUpdateViewportContent_FullscreenRespectsVirtualScrollFeatureGate(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = false
	m.width = 42
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("virt-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("virt-%02d alpha beta gamma delta epsilon zeta eta theta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen non-virtual setup should not emit command, got %v", cmd)
	}

	if m.usesVirtualTranscript() {
		t.Fatal("fullscreen mode should not use virtual transcript when VirtualScroll is disabled")
	}
	if !strings.Contains(m.chat.renderedContent, "virt-00") {
		t.Fatalf("non-virtual render should include earliest message content, got %q", m.chat.renderedContent)
	}
}

func TestUpdateViewportContent_FullscreenUsesVirtualTranscriptWhenEnabled(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 42
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("virt-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("virt-%02d alpha beta gamma delta epsilon zeta eta theta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen virtual setup should not emit command, got %v", cmd)
	}

	if !m.usesVirtualTranscript() {
		t.Fatal("fullscreen mode should use virtual transcript when VirtualScroll is enabled")
	}
	if strings.Contains(m.chat.renderedContent, "virt-00") {
		t.Fatalf("virtual render should not include earliest off-screen message content, got %q", m.chat.renderedContent)
	}
	if !strings.Contains(m.chat.renderedContent, "virt-3") {
		t.Fatalf("virtual render should still include visible tail content, got %q", m.chat.renderedContent)
	}
}

func TestUpdateViewportContent_FullscreenVirtualShowsBottomFeedback(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.features.MemoryUI = true
	m.width = 56
	m.height = 12
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("feedback-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("feedback-%02d alpha beta gamma delta epsilon zeta eta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	m.chat.inlineErrors = []components.InlineError{{Text: "visible inline feedback"}}
	m.status.memoryToast.Push(components.MemoryToast{FilePath: "visible-memory.md", Action: "updated"})
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen virtual feedback update should not emit command, got %v", cmd)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "visible inline feedback") {
		t.Fatalf("virtual viewport should include inline feedback at bottom, got %q", view)
	}
	if !strings.Contains(view, "visible-memory.md") {
		t.Fatalf("virtual viewport should include memory toast at bottom, got %q", view)
	}
	if got := lineCount(view); got > m.height {
		t.Fatalf("fullscreen view height=%d exceeds terminal height=%d:\n%s", got, m.height, view)
	}
	row, col, ok := nthVisibleOccurrence(m.chat.contentLines, "visible inline feedback", 0)
	if !ok {
		t.Fatalf("inline feedback row not found in content lines: %q", stripANSI(strings.Join(m.chat.contentLines, "\n")))
	}
	start := m.transcriptCoordForContentRow(row, col)
	end := m.transcriptCoordForContentRow(row, col+len([]rune("visible inline feedback")))
	if start.Valid || end.Valid {
		t.Fatalf("feedback rows should not bind to transcript block coords, start=%+v end=%+v", start, end)
	}
	m.chat.selection = TextSelection{
		HasRange:   true,
		StartRow:   row,
		StartCol:   col,
		EndRow:     row,
		EndCol:     col + len([]rune("visible inline feedback")),
		StartCoord: start,
		EndCoord:   end,
	}
	if got := m.extractSelectedText(); got != "visible inline feedback" {
		t.Fatalf("feedback selection should copy visible text, got %q", got)
	}
}

func TestSelectionFullscreenFeedbackRowsUseVisibleTextFallback(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = false
	m.features.MemoryUI = true
	m.width = 56
	m.height = 12
	m.recalcLayout()

	m.chat.messages = []message.Message{{
		ID:   "feedback-nonvirtual",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "normal transcript text"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}}
	m.chat.inlineErrors = []components.InlineError{{Text: "visible inline feedback"}}
	m.status.memoryToast.Push(components.MemoryToast{FilePath: "visible-memory.md", Action: "updated"})
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen feedback update should not emit command, got %v", cmd)
	}

	for _, token := range []string{"visible inline feedback", "visible-memory.md"} {
		row, col, ok := nthVisibleOccurrence(m.chat.contentLines, token, 0)
		if !ok {
			t.Fatalf("feedback token %q not found in content lines: %q", token, stripANSI(strings.Join(m.chat.contentLines, "\n")))
		}
		start := m.transcriptCoordForContentRow(row, col)
		end := m.transcriptCoordForContentRow(row, col+len([]rune(token)))
		if start.Valid || end.Valid {
			t.Fatalf("feedback token %q should not bind to transcript block coords, start=%+v end=%+v", token, start, end)
		}
		m.chat.selection = TextSelection{
			HasRange:   true,
			StartRow:   row,
			StartCol:   col,
			EndRow:     row,
			EndCol:     col + len([]rune(token)),
			StartCoord: start,
			EndCoord:   end,
		}
		if got := m.extractSelectedText(); got != token {
			t.Fatalf("feedback token %q selection should copy visible text, got %q", token, got)
		}
	}
}

func TestUpdateViewportContent_FullscreenVirtualUsesPureWarmMainCache(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 48
	m.height = 12
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 12; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("warm-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("warm-%02d alpha beta gamma delta epsilon zeta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	blocks := m.chat.blockList.All()
	if len(blocks) < 2 {
		t.Fatalf("expected multiple blocks, got %d", len(blocks))
	}
	singleBlock := m.buildBlocksForMessages([]message.Message{msgs[1]})
	rendered := m.blockRenderContext().Render(&singleBlock[0])
	if rendered == "" {
		t.Fatal("expected single block to render")
	}
	wantPureHeight := singleBlock[0].Height
	if got, ok := m.chat.heightCache.Get(blocks[1].ID); !ok || got != wantPureHeight {
		t.Fatalf("height cache should store pure block height before fullscreen transition, got %d ok=%v want %d", got, ok, wantPureHeight)
	}

	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.recalcLayout()
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("fullscreen warm-cache update should not emit command, got %v", cmd)
	}
	if strings.TrimSpace(stripANSI(m.chat.viewport.View())) == "" {
		t.Fatalf("fullscreen virtual render should keep visible content after warm main cache transition, got %q", m.chat.viewport.View())
	}
}

func TestUpdateViewportContent_FullscreenSkipsHiddenThinkingInDisplayAnchors(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 52
	m.height = 12
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "before",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "before visible content"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "asst",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ReasoningContent{Thinking: "hidden thinking should not consume scroll height"},
				message.TextContent{Text: "assistant visible content"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "after",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "after visible content"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("hidden-thinking setup should not emit command, got %v", cmd)
	}

	blocks := m.chat.blockList.All()
	foundHiddenThinking := false
	for _, block := range blocks {
		if block.Kind == components.BlockThinking {
			foundHiddenThinking = true
			if !components.IsDisplayHidden(block) {
				t.Fatalf("finished collapsed thinking should be pre-marked hidden, got %+v", block)
			}
		}
	}
	if !foundHiddenThinking {
		t.Fatal("expected a thinking block in test transcript")
	}

	total := components.DisplayTotalHeight(m.chat.heightCache, blocks, 1)
	for line := 0; line < total; line++ {
		anchor := components.AnchorFromDisplayLine(m.chat.heightCache, blocks, line, 1)
		block := m.chat.blockList.Get(anchor.BlockIdx)
		if block != nil && block.Kind == components.BlockThinking {
			t.Fatalf("display line %d should not anchor to hidden thinking block: %+v", line, anchor)
		}
	}
}

func TestClearChat_FullscreenClearsVirtualTranscriptState(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 64
	m.height = 14
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "old-user",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "old fullscreen transcript content"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("setup should not emit command, got %v", cmd)
	}
	if m.chat.blockList.Len() == 0 || !strings.Contains(stripANSI(m.chat.renderedContent), "old fullscreen transcript") {
		t.Fatalf("setup should render old transcript, blocks=%d content=%q", m.chat.blockList.Len(), m.chat.renderedContent)
	}

	if cmd := m.clearChatState(); cmd != nil {
		t.Fatalf("clear chat should not emit command, got %v", cmd)
	}
	if m.chat.blockList.Len() != 0 {
		t.Fatalf("clear chat should reset block list, got %d blocks", m.chat.blockList.Len())
	}
	if m.chat.renderedContent != "" {
		t.Fatalf("clear chat should clear rendered content, got %q", m.chat.renderedContent)
	}
	if strings.Contains(stripANSI(m.chat.viewport.View()), "old fullscreen transcript") {
		t.Fatalf("clear chat should remove stale virtual viewport content, got %q", stripANSI(m.chat.viewport.View()))
	}
	if m.chat.virtualList.Anchor() != (components.ScrollAnchor{}) {
		t.Fatalf("clear chat should reset virtual anchor, got %+v", m.chat.virtualList.Anchor())
	}
}

func TestFullscreenVirtualScroll_ScrollAndJumpKeepVisibleContent(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 48
	m.height = 16
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 32; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("nav-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("nav-%02d alpha beta gamma delta epsilon zeta eta theta iota kappa", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	assertHasTranscript := func(stage string) {
		view := stripANSI(m.chat.viewport.View())
		if strings.TrimSpace(view) == "" {
			t.Fatalf("%s should keep visible transcript content, got %q", stage, view)
		}
	}

	_ = m.applyViewportKeyScroll(tea.KeyMsg{Type: tea.KeyPgUp})
	assertHasTranscript("manual key scroll")

	m.applyViewportHalfPageUp()
	assertHasTranscript("half-page scroll")

	_ = m.executeVimAction(VimActionGotoTop)
	topView := stripANSI(m.View())
	if !strings.Contains(topView, "nav-00") {
		t.Fatalf("goto top should show first message content, got %q", topView)
	}

	_ = m.executeVimAction(VimActionGotoBottom)
	bottomView := stripANSI(m.View())
	anchorBottom := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
	if anchorBottom.MsgID != "nav-31" && !strings.Contains(bottomView, "nav-31") {
		t.Fatalf("goto bottom should land near latest message, anchor=%+v view=%q", anchorBottom, bottomView)
	}

	m.scrollToMessage(10)
	jumpView := stripANSI(m.View())
	anchorJump := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
	if anchorJump.MsgID != "nav-10" && !strings.Contains(jumpView, "nav-10") {
		t.Fatalf("search/message jump should land on nav-10, anchor=%+v view=%q", anchorJump, jumpView)
	}

	jumpYOffset := m.chat.viewport.YOffset
	jumpAnchor := m.chat.virtualList.Anchor()
	if cmd := m.updateViewportContent(); cmd != nil {
		t.Fatalf("jump repaint should not emit command, got %v", cmd)
	}
	if m.chat.viewport.YOffset != jumpYOffset {
		t.Fatalf("jump should survive repaint without snapping, before=%d after=%d anchor=%+v", jumpYOffset, m.chat.viewport.YOffset, m.chat.virtualList.Anchor())
	}
	if m.chat.virtualList.Anchor() != jumpAnchor {
		t.Fatalf("jump anchor should remain stable across repaint, before=%+v after=%+v", jumpAnchor, m.chat.virtualList.Anchor())
	}
	if !strings.Contains(stripANSI(m.View()), "nav-10") {
		t.Fatalf("jump repaint should keep target message visible, got %q", stripANSI(m.View()))
	}
}

func TestUpdate_FullscreenMouseSelectionIgnoresHeaderArea(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 20
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "msg-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "selection content"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()

	headerHeight := m.headerLayoutHeight()
	if headerHeight <= 0 {
		t.Fatalf("header height=%d, want > 0", headerHeight)
	}

	next, _ := m.Update(tea.MouseMsg{
		X:      1,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	updated := next.(Model)
	if updated.chat.selection.Active {
		t.Fatalf("click in header should not start selection: %+v", updated.chat.selection)
	}

	next, _ = updated.Update(tea.MouseMsg{
		X:      1,
		Y:      headerHeight,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	updated = next.(Model)
	if !updated.chat.selection.Active {
		t.Fatalf("click in transcript should start selection after header offset, selection=%+v", updated.chat.selection)
	}
}

func TestScreenToSelectionPos_FullscreenVirtualRecordsSemanticCoord(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 52
	m.height = 14
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("sem-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("sem-%02d alpha beta gamma delta epsilon", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	targetBlocks := m.chat.blockList.BlocksForMsg("sem-10")
	if len(targetBlocks) == 0 {
		t.Fatal("expected target block for sem-10")
	}
	m.chat.scrollMode = ScrollManualLocked
	m.chat.virtualList.SetAutoFollow(false)
	m.chat.virtualList.SetAnchor(components.AnchorForBlock(m.chat.blockList.All(), targetBlocks[0], 0))
	_ = m.updateViewportContent()

	row, col, coord, ok := m.screenToSelectionPos(3, m.headerLayoutHeight())
	if !ok {
		t.Fatal("expected transcript coordinate for first viewport row")
	}
	if row != m.chat.viewport.YOffset || col != 3 {
		t.Fatalf("content row/col mismatch row=%d col=%d viewportYOffset=%d", row, col, m.chat.viewport.YOffset)
	}
	if !coord.Valid || coord.MsgID != "sem-10" {
		t.Fatalf("expected semantic coord for sem-10, got %+v view=%q", coord, stripANSI(m.View()))
	}
	if coord.AbsLine != m.chat.virtualList.LastFrameTopLine()+coord.ContentRow {
		t.Fatalf("absolute line should include virtual frame top, got coord=%+v frameTop=%d", coord, m.chat.virtualList.LastFrameTopLine())
	}

	m.chat.selection = TextSelection{
		HasRange:   true,
		StartRow:   row,
		StartCol:   col,
		EndRow:     row,
		EndCol:     col + 4,
		StartCoord: coord,
		EndCoord:   coord,
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 36, Height: 14})
	updated := next.(Model)
	if updated.chat.selection.StartCoord.MsgID != "sem-10" {
		t.Fatalf("selection semantic coord should survive resize, got %+v", updated.chat.selection.StartCoord)
	}
	if updated.chat.selection.StartRow != updated.chat.selection.StartCoord.ContentRow {
		t.Fatalf("selection row should be rebound from semantic coord after resize, selection=%+v", updated.chat.selection)
	}
}

func TestUpdate_FullscreenInputChromeChangeRecalculatesViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 20
	m.recalcLayout()
	before := m.chat.viewport.Height

	_, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	after := m.chat.viewport.Height
	want := m.currentLayoutSnapshot().TranscriptHeight
	if after != want {
		t.Fatalf("viewport height should match current layout snapshot after input chrome change, got %d want %d", after, want)
	}
	if after != before {
		t.Fatalf("typing first character should keep fullscreen transcript height stable, before=%d after=%d", before, after)
	}
}

func TestUpdate_FullscreenNoticeClearRecalculatesViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 20
	m.recalcLayout()

	baseline := m.chat.viewport.Height
	m.setNotice(components.NoticeSuccess, "Copied selection")
	withNotice := m.chat.viewport.Height
	if withNotice != baseline {
		t.Fatalf("notice should keep fullscreen transcript height stable, baseline=%d withNotice=%d", baseline, withNotice)
	}

	next, _ := m.Update(clearCopyFeedbackMsg{})
	updated := next.(Model)
	want := updated.currentLayoutSnapshot().TranscriptHeight
	if updated.chat.viewport.Height != want {
		t.Fatalf("viewport height should match snapshot after notice clears, got %d want %d", updated.chat.viewport.Height, want)
	}
	if updated.chat.viewport.Height != withNotice {
		t.Fatalf("clearing notice should keep transcript height stable, withNotice=%d after=%d", withNotice, updated.chat.viewport.Height)
	}
}

func TestSendMessage_FullscreenInputResetRecalculatesViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.sessionID = "sess-send"
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.composer.input.SetWidth(m.width)
	m.composer.input.SetValue("line one\nline two\nline three\nline four")
	m.setNotice(components.NoticeSuccess, "old notice")
	m.recalcLayout()
	before := m.chat.viewport.Height

	next, _ := m.sendMessage()
	updated := next.(Model)
	want := updated.currentLayoutSnapshot().TranscriptHeight
	if updated.chat.viewport.Height != want {
		t.Fatalf("viewport height should match snapshot after send reset, got %d want %d", updated.chat.viewport.Height, want)
	}
	if updated.chat.viewport.Height <= before {
		t.Fatalf("send should reclaim multiline input rows, before=%d after=%d", before, updated.chat.viewport.Height)
	}
	if !updated.status.notice.IsZero() {
		t.Fatalf("send should clear old footer notice, got %+v", updated.status.notice)
	}
}

func TestSelectionSemanticCoordPreservesTextAcrossRewrap(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 74
	m.height = 16
	m.recalcLayout()
	token := "TOKENSTABLE"
	m.chat.messages = []message.Message{{
		ID:   "rewrap-1",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda " + token + " omega"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}}
	_ = m.updateViewportContent()

	row, col := -1, -1
	for i, line := range m.chat.contentLines {
		if idx := strings.Index(stripANSI(line), token); idx >= 0 {
			row, col = i, idx
			break
		}
	}
	if row < 0 {
		t.Fatalf("test token %q not found in rendered content: %q", token, stripANSI(strings.Join(m.chat.contentLines, "\n")))
	}
	start := m.transcriptCoordForContentRow(row, col)
	end := m.transcriptCoordForContentRow(row, col+len([]rune(token)))
	m.chat.selection = TextSelection{
		HasRange:   true,
		StartRow:   row,
		StartCol:   col,
		EndRow:     row,
		EndCol:     col + len([]rune(token)),
		StartCoord: start,
		EndCoord:   end,
	}
	if got := m.extractSelectedText(); got != token {
		t.Fatalf("selected text before resize = %q, want %q", got, token)
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 46, Height: 16})
	updated := next.(Model)
	if got := updated.extractSelectedText(); got != token {
		t.Fatalf("selected text after rewrap = %q, want %q; selection=%+v", got, token, updated.chat.selection)
	}
}

func TestSelectionSemanticCoordKeepsDuplicateOccurrenceAcrossRewrap(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.width = 92
	m.height = 16
	m.recalcLayout()
	token := "REPEATME"
	m.chat.messages = []message.Message{{
		ID:   "rewrap-dup",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "alpha " + token + " beta gamma delta epsilon zeta eta theta iota kappa lambda " + token + " omega"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}}
	_ = m.updateViewportContent()

	row, col, ok := nthVisibleOccurrence(m.chat.contentLines, token, 1)
	if !ok {
		t.Fatalf("second test token %q not found in rendered content: %q", token, stripANSI(strings.Join(m.chat.contentLines, "\n")))
	}
	start := m.transcriptCoordForContentRow(row, col)
	end := m.transcriptCoordForContentRow(row, col+len([]rune(token)))
	m.chat.selection = TextSelection{
		HasRange:   true,
		StartRow:   row,
		StartCol:   col,
		EndRow:     row,
		EndCol:     col + len([]rune(token)),
		StartCoord: start,
		EndCoord:   end,
	}
	if got := m.extractSelectedText(); got != token {
		t.Fatalf("selected duplicate text before resize = %q, want %q", got, token)
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 46, Height: 16})
	updated := next.(Model)
	if got := updated.extractSelectedText(); got != token {
		t.Fatalf("selected duplicate text after resize = %q, want %q; selection=%+v", got, token, updated.chat.selection)
	}
	block := updated.chat.blockList.GetByID(updated.chat.selection.StartCoord.BlockID)
	if block == nil {
		t.Fatalf("selected block %q not found", updated.chat.selection.StartCoord.BlockID)
	}
	wantOffset, ok := nthRuneOffset(flattenedRenderedText(block.Rendered), token, 1)
	if !ok {
		t.Fatalf("second token %q not found after rewrap in block %q", token, stripANSI(block.Rendered))
	}
	gotOffset := min(updated.chat.selection.StartCoord.PlainOffset, updated.chat.selection.EndCoord.PlainOffset)
	if gotOffset != wantOffset {
		t.Fatalf("selection should stay on second duplicate occurrence, got offset %d want %d selection=%+v", gotOffset, wantOffset, updated.chat.selection)
	}
}

func TestSelectionSemanticTextPreservesQuoteAndBulletMarkers(t *testing.T) {
	rendered := "  " + components.FigBlackCircle + " > quoted\n" +
		strings.Repeat(" ", components.MessagePrefixWidth()) + "• bullet\n"
	block := components.BlockVM{ID: "quoted-block", MsgID: "quoted-msg", Rendered: rendered, Height: 2}

	m := newTestModel()
	m.chat.blockList = components.NewBlockList()
	m.chat.blockList.RebuildAll([]components.BlockVM{block})
	m.chat.selection = TextSelection{
		HasRange: true,
		StartCoord: TranscriptCoord{
			Valid:       true,
			BlockID:     block.ID,
			MsgID:       block.MsgID,
			PlainOffset: 0,
		},
		EndCoord: TranscriptCoord{
			Valid:       true,
			BlockID:     block.ID,
			MsgID:       block.MsgID,
			PlainOffset: len([]rune("> quoted")),
		},
	}
	if got := m.extractSelectedText(); got != "> quoted" {
		t.Fatalf("quote marker should be preserved in semantic selection, got %q", got)
	}

	start := len([]rune("> quoted"))
	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord.PlainOffset = start
	m.chat.selection.EndCoord.PlainOffset = start + len([]rune("• bullet"))
	if got := m.extractSelectedText(); got != "• bullet" {
		t.Fatalf("bullet marker should be preserved in semantic selection, got %q", got)
	}

	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord.PlainOffset = 0
	m.chat.selection.EndCoord.PlainOffset = len([]rune("> quoted")) + len([]rune("• bullet"))
	if got := m.extractSelectedText(); got != "> quoted\n• bullet" {
		t.Fatalf("same-block multiline semantic selection should preserve newline, got %q", got)
	}

	lineBreakOffset := len([]rune("> quoted"))
	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord.PlainOffset = 0
	m.chat.selection.StartCoord.LineStart = false
	m.chat.selection.EndCoord.PlainOffset = lineBreakOffset
	m.chat.selection.EndCoord.LineStart = true
	if got := m.extractSelectedText(); got != "> quoted\n" {
		t.Fatalf("selection ending at next line start should preserve trailing newline, got %q", got)
	}

	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord.PlainOffset = lineBreakOffset
	m.chat.selection.StartCoord.LineStart = true
	m.chat.selection.EndCoord.PlainOffset = 0
	m.chat.selection.EndCoord.LineStart = false
	if got := m.extractSelectedText(); got != "> quoted\n" {
		t.Fatalf("reverse selection ending at next line start should preserve trailing newline, got %q", got)
	}

	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord.PlainOffset = lineBreakOffset
	m.chat.selection.StartCoord.LineStart = true
	m.chat.selection.EndCoord.PlainOffset = lineBreakOffset
	m.chat.selection.EndCoord.LineStart = true
	if got := m.extractSelectedText(); got != "" {
		t.Fatalf("zero-width line-start selection should be empty, got %q", got)
	}

	m.chat.selection.PlainText = "> quoted\n"
	m.chat.selection.StartCoord.PlainOffset = 0
	m.chat.selection.StartCoord.LineStart = false
	m.chat.selection.EndCoord.PlainOffset = lineBreakOffset
	m.chat.selection.EndCoord.LineStart = true
	rewrapped := block
	rewrapped.Rendered = "  " + components.FigBlackCircle + " > quoted • bullet\n"
	rewrapped.Height = 1
	m.chat.blockList.RebuildAll([]components.BlockVM{rewrapped})
	if got := m.extractSelectedText(); got != "> quoted\n" {
		t.Fatalf("line-start boundary should survive rewrap, got %q", got)
	}

	indentedRendered := "  intro\n" +
		"  " + "   indented\n"
	indented := components.BlockVM{ID: "indented-block", MsgID: "indented-msg", Rendered: indentedRendered, Height: 2}
	m.chat.blockList.RebuildAll([]components.BlockVM{indented})
	offset := len([]rune("intro"))
	m.chat.selection.PlainText = ""
	m.chat.selection.StartCoord = TranscriptCoord{
		Valid:       true,
		BlockID:     indented.ID,
		MsgID:       indented.MsgID,
		PlainOffset: offset,
		LineStart:   true,
	}
	m.chat.selection.EndCoord = TranscriptCoord{
		Valid:       true,
		BlockID:     indented.ID,
		MsgID:       indented.MsgID,
		PlainOffset: offset + len([]rune("   indented")),
	}
	if got := m.extractSelectedText(); got != "   indented" {
		t.Fatalf("indented continuation content should preserve leading spaces, got %q", got)
	}
}

func TestUpdate_FullscreenMouseWheelScrollsWhenPointerIsOnHeader(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 48
	m.height = 10
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 24; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("wheel-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("wheel-%02d alpha beta gamma delta epsilon zeta eta theta", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	m.chat.scrollMode = ScrollManualLocked
	m.chat.viewport.SetYOffset(0)
	before := m.chat.viewport.YOffset

	next, _ := m.Update(tea.MouseMsg{
		X:      1,
		Y:      0,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := next.(Model)
	if updated.chat.viewport.YOffset <= before {
		t.Fatalf("wheel on header should scroll transcript, before=%d after=%d", before, updated.chat.viewport.YOffset)
	}
	view := stripANSI(updated.View())
	if !strings.Contains(view, "wheel-") {
		t.Fatalf("wheel scroll should keep real transcript content visible, got %q", view)
	}
}

func TestUpdate_FullscreenMouseSelectionReleaseAcrossHeaderFinalizes(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 20
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "msg-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "selection content that should remain selectable across the header boundary"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()

	headerHeight := m.headerLayoutHeight()

	next, _ := m.Update(tea.MouseMsg{
		X:      1,
		Y:      headerHeight,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	updated := next.(Model)
	if !updated.chat.selection.Active {
		t.Fatalf("expected selection to start in transcript, got %+v", updated.chat.selection)
	}

	next, _ = updated.Update(tea.MouseMsg{
		X:      18,
		Y:      0,
		Button: tea.MouseButtonNone,
		Action: tea.MouseActionRelease,
	})
	updated = next.(Model)
	if updated.chat.selection.Active {
		t.Fatalf("release across header should finalize selection, got %+v", updated.chat.selection)
	}
	if !updated.chat.selection.HasRange {
		t.Fatalf("release across header should preserve selected range, got %+v", updated.chat.selection)
	}
}

func TestUpdate_WindowResizeSameSizeIsNoOp(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "m-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "same size resize no-op"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()
	before := stripANSI(m.View())

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("same-size resize should return nil command, got %v", cmd)
	}
	updated := next.(Model)
	after := stripANSI(updated.View())

	if after != before {
		t.Fatalf("same-size resize should not change rendered view, before=%q after=%q", before, after)
	}
}

func TestUpdate_FullscreenWindowResizeDoesNotEmitClearScreenRepaint(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "resize-clear",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "fullscreen effective resize repaint"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	updated := next.(Model)
	if !updated.isFullscreenMode() {
		t.Fatal("effective fullscreen resize should keep fullscreen active")
	}
	if findMsgTypeIndex(collectCmdTypeNames(cmd), "clearScreenMsg") >= 0 {
		t.Fatalf("fullscreen resize should rely on safe frame rendering, not clearScreen repaint; got %#v", collectCmdTypeNames(cmd))
	}
}

func TestUpdate_FullscreenResizeKeepsRenderedHeightWithinTerminal(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 14
	m.recalcLayout()

	var msgs []message.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, message.Message{
			ID:   fmt.Sprintf("resize-bound-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("resize-bound-%02d lorem ipsum dolor sit amet", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	m.chat.messages = msgs
	_ = m.updateViewportContent()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 32, Height: 6})
	updated := next.(Model)
	assertRenderedFrameFits(t, updated.View(), updated.height, updated.width)
}

func TestView_MainScreenFinalFrameSanitizesLongRowsWithPickerStatusFooter(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 34
	m.height = 14
	m.composer.input.SetWidth(m.width)
	m.composer.input.SetValue("/doctor")
	m.chat.renderedContent = "Error: 401 Unauthorized: " + strings.Repeat("token-expired ", 40)
	m.composer.commandPickerActive = true
	m.composer.filteredCompletions = []command.CompletionItem{
		{
			Kind:        "command",
			Value:       "doctor",
			CommandName: "doctor",
			Description: "诊断: " + strings.Repeat("全角宽字符 ", 10) + "emoji 😀😀😀 and tabs\t\tend",
		},
	}
	m.status.ctrlCPending = true
	m.recalcLayout()

	view := m.View()
	maxSafe := components.TerminalSafeWidth(m.width)
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > maxSafe {
			t.Fatalf("main-screen line %d width=%d exceeds safe width=%d: %q", i, w, maxSafe, stripANSI(line))
		}
	}
	if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\x1b[3J") {
		t.Fatalf("main-screen sanitizer should not rely on clear-screen escapes, got %q", stripANSI(view))
	}
}

func TestSanitizeMainScreenFrame_HandlesWideAndComplexGlyphs(t *testing.T) {
	width := 16
	lines := []string{
		"\x1b[32mANSI-colored-text\x1b[0m",
		"全角字符混合abcdef",
		"emoji🙂🙂🙂suffix",
		"combining e\u0301e\u0301e\u0301 tail",
		"tab\tseparated\ttext",
	}
	frame := strings.Join(lines, "\r\n")
	got := sanitizeMainScreenFrame(frame, width)
	if strings.Contains(got, "\t") {
		t.Fatalf("sanitized frame should expand raw tabs before width checks, got %q", got)
	}
	maxSafe := components.TerminalSafeWidth(width)
	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > maxSafe {
			t.Fatalf("complex line %d width=%d exceeds safe width=%d: %q", i, w, maxSafe, stripANSI(line))
		}
	}
}

func TestUpdate_FullscreenStreamingResizeRendersExactTerminalFrame(t *testing.T) {
	m := newTestModel()
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.sessionID = "sess-stream-frame"
	m.width = 120
	m.height = 16
	m.composer.input.SetWidth(m.width)
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		Label:     "response",
		StartedAt: time.Now(),
	}
	m.status.processingVerb = "Streaming"
	m.status.telemetry.LastTokenCount = 37
	m.status.telemetry.DisplayedTokenCount = 37
	m.chat.messages = []message.Message{
		{
			ID:        "stream-frame-user",
			SessionID: "sess-stream-frame",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "resize the terminal while the assistant is streaming"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "stream-frame-asst",
			SessionID: "sess-stream-frame",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: strings.Repeat("streaming frame stability ", 24)},
			},
		},
	}
	m.recalcLayout()
	_ = m.updateViewportContent()
	assertRenderedFrameFits(t, m.View(), m.height, m.width)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 7})
	updated := next.(Model)
	_ = updated.updateViewportContent()
	assertRenderedFrameFits(t, updated.View(), updated.height, updated.width)
}

func TestUpdate_FullscreenStreamingResizeMatrixHasNoDockDuplication(t *testing.T) {
	m := newTestModel()
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.features.StatusV2 = true
	m.sessionID = "sess-stream-matrix"
	m.width = 120
	m.height = 16
	m.composer.input.SetWidth(m.width)
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		Label:     "response",
		StartedAt: time.Now(),
	}
	m.status.processingVerb = "Streaming response"
	m.status.telemetry.LastTokenCount = 39
	m.status.telemetry.DisplayedTokenCount = 39
	m.chat.messages = []message.Message{
		{
			ID:        "matrix-user",
			SessionID: "sess-stream-matrix",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "resize matrix reproduction"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "matrix-asst",
			SessionID: "sess-stream-matrix",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: strings.Repeat("streaming response body ", 30)},
			},
		},
	}
	m.recalcLayout()
	_ = m.updateViewportContent()

	steps := []tea.WindowSizeMsg{
		{Width: 34, Height: 7},
		{Width: 100, Height: 12},
		{Width: 28, Height: 6},
	}
	for _, step := range steps {
		next, _ := m.Update(step)
		updated := next.(Model)
		m = &updated
		_ = m.updateViewportContent()
		view := stripANSI(m.View())
		assertRenderedFrameFits(t, m.View(), m.height, m.width)
		if strings.Count(view, "Streaming response") > 1 {
			t.Fatalf("footer/status dock should not duplicate processing label after resize %dx%d: %q", step.Width, step.Height, view)
		}
		if strings.Count(view, " IN ") > 1 || strings.Count(view, " OUT ") > 1 {
			t.Fatalf("token windows duplicated after resize %dx%d: %q", step.Width, step.Height, view)
		}
	}
}

func TestBuildScreenVM_FullscreenStatusDoesNotLeakIntoTranscript(t *testing.T) {
	m := newTestModel()
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.setScreenMode(ScreenModeFullscreen)
	m.features.VirtualScroll = true
	m.sessionID = "sess-status-frame"
	m.width = 120
	m.height = 14
	m.composer.input.SetWidth(m.width)
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		Label:     "response",
		StartedAt: time.Now(),
	}
	m.status.processingVerb = "Streaming"
	m.status.telemetry.LastTokenCount = 37
	m.status.telemetry.DisplayedTokenCount = 37
	m.chat.messages = []message.Message{
		{
			ID:        "status-frame-user",
			SessionID: "sess-status-frame",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "status ownership regression"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "status-frame-asst",
			SessionID: "sess-status-frame",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "assistant text stays in the transcript"},
			},
		},
	}
	m.recalcLayout()
	_ = m.updateViewportContent()

	screen := m.buildScreenVM()
	transcript := stripANSI(screen.Transcript)
	status := stripANSI(screen.Status)
	bottom := stripANSI(screen.Bottom)
	for _, leaked := range []string{"test-model", " IN ", " OUT "} {
		if strings.Contains(transcript, leaked) {
			t.Fatalf("status segment %q leaked into transcript region:\n%s", leaked, transcript)
		}
	}
	if !strings.Contains(bottom, "Streaming") {
		t.Fatalf("processing progress should render above fullscreen input, got %q", bottom)
	}
	if strings.Contains(status, "Streaming") {
		t.Fatalf("processing progress should not duplicate in status region, got %q", status)
	}
	for _, want := range []string{"IN "} {
		if !strings.Contains(status, want) {
			t.Fatalf("status region missing %q, got %q", want, status)
		}
	}
}

func TestRenderStatusBar_PromptOwnerHidesInterruptHint(t *testing.T) {
	m := newTestModel()
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.setScreenMode(ScreenModeFullscreen)
	m.sessionID = "sess-esc-owner"
	m.width = 100
	m.height = 14
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		StartedAt: time.Now(),
	}
	m.status.processingVerb = "Streaming"
	m.search.historySearch.SetEntries([]string{"prior prompt"})
	_ = m.search.historySearch.Show()
	m.recalcLayout()

	status := stripANSI(m.renderStatusBar())
	if strings.Contains(status, "esc to interrupt") {
		t.Fatalf("history search owns Esc, so status should hide interrupt hint: %q", status)
	}
}

func TestRenderStatusBar_RuntimeOverlayOwnsFooterDock(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.OverlayStack = true
	m.width = 100
	m.height = 14
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseStreaming,
		StartedAt: time.Now(),
	}
	m.status.processingVerb = "Streaming"
	m.overlays.Push(NewHelpOverlay())
	m.recalcLayout()

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "[help]") {
		t.Fatalf("runtime overlay should own footer dock, got %q", status)
	}
	if strings.Contains(status, "Streaming") {
		t.Fatalf("overlay owner should suppress processing metadata, got %q", status)
	}
}

func TestUpdate_WindowResizeWithVisibleHistorySearchDoesNotPanic(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.recalcLayout()
	m.search.historySearch.SetEntries([]string{"first prompt", "second prompt"})
	_ = m.search.historySearch.Show()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("resize with visible history search should not panic: %v", r)
		}
	}()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 2, Height: 10})
	updated := next.(Model)
	view := stripANSI(updated.View())
	if strings.TrimSpace(view) == "" {
		t.Fatalf("history search should remain renderable after resize: %q", view)
	}
}

func TestUpdate_WindowResizeWithVisibleTextSearchDoesNotPanic(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.recalcLayout()
	_ = m.search.textSearch.Show()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("resize with visible text search should not panic: %v", r)
		}
	}()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 2, Height: 10})
	updated := next.(Model)
	view := stripANSI(updated.View())
	if strings.TrimSpace(view) == "" {
		t.Fatalf("text search should remain renderable after resize: %q", view)
	}
}

func TestUpdateViewportContent_MainScreenKeepsCompletedStreamingReplySingleCopyAcrossTurns(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 36
	m.height = 10
	m.sessionID = "sess-stream"
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:        "user-stream",
			SessionID: "sess-stream",
			Role:      message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "current prompt"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:        "asst-stream",
			SessionID: "sess-stream",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "tok01 tok02 tok03 tok04 tok05 tok06 tok07 tok08 tok09 tok10 tok11 tok12 tok13 tok14 tok15 tok16"},
			},
		},
	}
	m.status.isProcessing = true

	firstCmd := m.updateViewportContent()
	if firstPrinted := stripANSI(strings.Join(collectPrintedBodies(firstCmd), "\n")); strings.TrimSpace(firstPrinted) != "" {
		t.Fatalf("streaming should not print completed prefix, got %q", firstPrinted)
	}

	m.status.isProcessing = false
	m.chat.messages[1].Parts = append(m.chat.messages[1].Parts, message.Finish{Reason: message.FinishReasonEndTurn})

	secondCmd := m.updateViewportContent()
	if secondPrinted := stripANSI(strings.Join(collectPrintedBodies(secondCmd), "\n")); strings.TrimSpace(secondPrinted) != "" {
		t.Fatalf("completion should not print remaining lines, got %q", secondPrinted)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "tok16") {
		t.Fatalf("completed streamed reply should remain in natural frame: %q", view)
	}

	m.status.isProcessing = true
	m.chat.messages = append(m.chat.messages, message.Message{
		ID:        "user-next",
		SessionID: "sess-stream",
		Role:      message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "next prompt"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})

	thirdCmd := m.updateViewportContent()
	if thirdPrinted := stripANSI(strings.Join(collectPrintedBodies(thirdCmd), "\n")); strings.Contains(thirdPrinted, "tok16") {
		t.Fatalf("next turn should not duplicate prior reply via scrollback flush, got %q", thirdPrinted)
	}
	view = stripANSI(m.View())
	if !strings.Contains(view, "tok16") || !strings.Contains(view, "next prompt") {
		t.Fatalf("prior reply and next prompt should share the natural transcript frame: %q", view)
	}
}

func TestView_MainScreenManualScrollUsesViewportSlice(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 10
	m.recalcLayout()

	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("marker-%02d", i))
	}
	content := strings.Join(lines, "\n")
	m.chat.renderedContent = content
	m.chat.viewport.SetContent(content)
	m.chat.scrollMode = ScrollManualLocked
	m.chat.viewport.GotoTop()

	view := stripANSI(m.View())
	if !strings.Contains(view, "marker-00") {
		t.Fatalf("manual top-of-viewport view missing early content: %q", view)
	}
	if strings.Contains(view, "marker-19") {
		t.Fatalf("manual top-of-viewport view should not include bottom content: %q", view)
	}
}

func TestUpdate_FullscreenAssistantUpdateRefreshesViewport(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.sessionID = "sess-1"
	m.recalcLayout()

	next, _ := m.Update(pubsub.Event[message.Message]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Message{
			ID:        "asst-1",
			SessionID: "sess-1",
			Role:      message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "fullscreen delta"},
			},
		},
	})

	updated := next.(Model)
	viewport := stripANSI(updated.chat.viewport.View())
	if !strings.Contains(viewport, "fullscreen delta") {
		t.Fatalf("viewport missing assistant transcript after update: %q", viewport)
	}
}

func TestRenderBlockSliceWithWidthAndOverride_UsesUnifiedBoundaryRows(t *testing.T) {
	m := newTestModel()
	m.width = 80

	blocks := []components.BlockVM{
		{ID: "u-0", MsgID: "u", Kind: components.BlockUser, Content: "U", Dirty: true},
		{ID: "a-0", MsgID: "a", Kind: components.BlockAssistantMarkdown, Content: "A", Dirty: true, Meta: components.BlockMeta{IsFirstContentBlock: true}},
		{ID: "s-0", MsgID: "s", Kind: components.BlockSystem, Content: "S", Dirty: true},
	}

	got := stripANSI(m.renderBlockSliceWithWidthAndOverride(blocks, m.width, -1, ""))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	uLine, aLine, sLine := -1, -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "U") && uLine < 0 {
			uLine = i
		}
		if strings.Contains(trimmed, "A") && aLine < 0 {
			aLine = i
		}
		if strings.Contains(trimmed, "S") && sLine < 0 {
			sLine = i
		}
	}
	if uLine < 0 || aLine < 0 || sLine < 0 {
		t.Fatalf("expected all content rows present, got %q", got)
	}
	if aLine-uLine != 2 {
		t.Fatalf("expected 1 boundary row between user and assistant, got line distance=%d output=%q", aLine-uLine, got)
	}
	if sLine-aLine != 2 {
		t.Fatalf("expected 1 boundary row between assistant and system, got line distance=%d output=%q", sLine-aLine, got)
	}
}
