package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type screenStaticCommand struct {
	name   string
	result command.Result
}

func (c screenStaticCommand) Name() string        { return c.name }
func (c screenStaticCommand) Description() string { return c.name }
func (c screenStaticCommand) Execute(_ command.Context) command.Result {
	return c.result
}

func collectCmdTypeNames(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	var names []string

	var walkMsg func(tea.Msg)
	var walkCmd func(tea.Cmd)

	walkCmd = func(c tea.Cmd) {
		if c == nil {
			return
		}
		walkMsg(c())
	}

	walkMsg = func(msg tea.Msg) {
		if msg == nil {
			return
		}
		v := reflect.ValueOf(msg)
		if v.IsValid() && v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i)
				if !item.IsValid() || !item.CanInterface() {
					continue
				}
				if nestedCmd, ok := item.Interface().(tea.Cmd); ok {
					walkCmd(nestedCmd)
					continue
				}
				if nestedMsg, ok := item.Interface().(tea.Msg); ok {
					walkMsg(nestedMsg)
				}
			}
			return
		}
		names = append(names, fmt.Sprintf("%T", msg))
	}

	walkCmd(cmd)
	return names
}

func findMsgTypeIndex(types []string, contains string) int {
	for i, name := range types {
		if strings.Contains(name, contains) {
			return i
		}
	}
	return -1
}

func topLevelCmdTypeName(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	return fmt.Sprintf("%T", cmd())
}

func TestSwitchScreenModeSameModeNoOp(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)

	changed, cmd := m.switchScreenMode(ScreenModeMain)
	if changed {
		t.Fatal("switchScreenMode should be no-op for same mode")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd for same-mode no-op, got %v", cmd)
	}
}

func TestSwitchScreenModeMainToFullscreenOrder(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.setScreenMode(ScreenModeMain)

	changed, cmd := m.switchScreenMode(ScreenModeFullscreen)
	if !changed {
		t.Fatal("expected mode change")
	}
	if m.isFullscreenMode() {
		t.Fatal("fullscreen should not become active until terminal transition is applied")
	}
	if m.screenMode != ScreenModeFullscreen {
		t.Fatalf("screenMode = %q, want fullscreen target", m.screenMode)
	}
	if !strings.Contains(topLevelCmdTypeName(cmd), "sequenceMsg") {
		t.Fatalf("expected ordered sequence command, got %s", topLevelCmdTypeName(cmd))
	}

	types := collectCmdTypeNames(cmd)
	enterIdx := findMsgTypeIndex(types, "enterAltScreenMsg")
	enableIdx := findMsgTypeIndex(types, "enableMouseCellMotionMsg")
	applyIdx := findMsgTypeIndex(types, "tui.screenModeAppliedMsg")
	if enterIdx < 0 || enableIdx < 0 {
		t.Fatalf("expected enter/enable commands, got %#v", types)
	}
	if enterIdx > enableIdx {
		t.Fatalf("expected EnterAltScreen before EnableMouseCellMotion, got %#v", types)
	}
	if applyIdx < 0 || enableIdx > applyIdx {
		t.Fatalf("expected screenModeAppliedMsg after terminal commands, got %#v", types)
	}
}

func TestSwitchScreenModeFullscreenToMainOrderAndCleanup(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.setScreenMode(ScreenModeFullscreen)
	m.chat.selection = TextSelection{Active: true, HasRange: true, StartRow: 1, EndRow: 2}

	changed, cmd := m.switchScreenMode(ScreenModeMain)
	if !changed {
		t.Fatal("expected mode change")
	}
	if !m.isFullscreenMode() {
		t.Fatal("fullscreen should remain active until terminal transition is applied")
	}
	if m.screenMode != ScreenModeMain {
		t.Fatalf("screenMode = %q, want main target", m.screenMode)
	}
	if m.chat.selection.Active || m.chat.selection.HasRange {
		t.Fatal("selection state should be cleared when leaving fullscreen")
	}
	if !strings.Contains(topLevelCmdTypeName(cmd), "sequenceMsg") {
		t.Fatalf("expected ordered sequence command, got %s", topLevelCmdTypeName(cmd))
	}

	types := collectCmdTypeNames(cmd)
	disableIdx := findMsgTypeIndex(types, "disableMouseMsg")
	exitIdx := findMsgTypeIndex(types, "exitAltScreenMsg")
	applyIdx := findMsgTypeIndex(types, "tui.screenModeAppliedMsg")
	if disableIdx < 0 || exitIdx < 0 {
		t.Fatalf("expected disable/exit commands, got %#v", types)
	}
	if disableIdx > exitIdx {
		t.Fatalf("expected DisableMouse before ExitAltScreen, got %#v", types)
	}
	if applyIdx < 0 || exitIdx > applyIdx {
		t.Fatalf("expected screenModeAppliedMsg after terminal commands, got %#v", types)
	}
}

func TestUpdate_ScreenModeAppliedMsgActivatesMode(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.screenMode = ScreenModeFullscreen

	next, cmd := m.Update(screenModeAppliedMsg{
		target:    ScreenModeFullscreen,
		prevWidth: 80,
	})
	updated := next.(Model)
	if !updated.isFullscreenMode() {
		t.Fatal("fullscreen should be active after screenModeAppliedMsg")
	}
	if !updated.terminalState.AltScreen || !updated.terminalState.Mouse {
		t.Fatalf("terminal state not updated: %+v", updated.terminalState)
	}
	if findMsgTypeIndex(collectCmdTypeNames(cmd), "clearScreenMsg") >= 0 {
		t.Fatalf("screenModeAppliedMsg entering fullscreen should not rely on clearScreen repaint, got %#v", collectCmdTypeNames(cmd))
	}
}

func TestUpdate_ScreenModeAppliedMainForcesReturnMainReset(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()
	controller := NewMainScreenOutputController(MainScreenResetModeFull)
	m.mainOutput = controller
	m.mainResetMode = MainScreenResetModeFull
	m.setScreenMode(ScreenModeFullscreen)
	m.screenMode = ScreenModeMain
	m.chat.messages = []message.Message{
		{
			ID:   "return-main-msg",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "return main transcript"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}

	next, _ := m.Update(screenModeAppliedMsg{target: ScreenModeMain, prevWidth: 80})
	updated := next.(Model)
	if updated.isFullscreenMode() {
		t.Fatal("main-screen should be active after applying main mode")
	}
	if updated.chat.mainScreenFrame == nil || updated.chat.mainScreenFrame.Reason != "return-main" {
		t.Fatalf("return-main should schedule main-screen reset, got %+v", updated.chat.mainScreenFrame)
	}
	_ = updated.updateViewportContent()
	token := updated.View()
	var out strings.Builder
	if _, err := controller.WrapOutput(&out).Write([]byte(token)); err != nil {
		t.Fatalf("controller write failed: %v", err)
	}
	if !strings.Contains(out.String(), "\x1b[2J\x1b[3J\x1b[H") {
		t.Fatalf("return-main frame should full reset through controller, got %q", out.String())
	}
	out.Reset()
	token = updated.View()
	if _, err := controller.WrapOutput(&out).Write([]byte(token)); err != nil {
		t.Fatalf("controller second write failed: %v", err)
	}
	if strings.Contains(out.String(), "\x1b[3J") {
		t.Fatalf("return-main reset must be one-shot, got second frame %q", out.String())
	}
}

func TestSendMessageDispatchesScreenAction(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()

	reg := command.NewRegistry()
	reg.Register(screenStaticCommand{
		name:   "screen",
		result: command.Result{Action: "screen:fullscreen"},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/screen fullscreen")

	next, _ := m.sendMessage()
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if updated.isFullscreenMode() {
		t.Fatal("fullscreen should not be active before terminal transition is applied")
	}
	if updated.screenMode != ScreenModeFullscreen {
		t.Fatalf("screenMode = %q, want fullscreen target", updated.screenMode)
	}
	if !strings.Contains(updated.composer.commandOutput, "Switched screen mode to fullscreen") {
		t.Fatalf("unexpected command output: %q", updated.composer.commandOutput)
	}
}

func TestSendMessageScreenActionSameModeNoOp(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)

	reg := command.NewRegistry()
	reg.Register(screenStaticCommand{
		name:   "screen",
		result: command.Result{Action: "screen:main"},
	})
	m.dispatcher = command.NewDispatcher(reg)
	m.composer.input.SetValue("/screen main")

	next, cmd := m.sendMessage()
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd for same mode action, got %v", cmd)
	}
	if !strings.Contains(updated.composer.commandOutput, "already main") {
		t.Fatalf("expected no-op output, got %q", updated.composer.commandOutput)
	}
}

func TestInit_MainScreenDoesNotEmitClearScreen(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.app = &app.App{}

	types := collectCmdTypeNames(m.Init())
	if findMsgTypeIndex(types, "clearScreenMsg") >= 0 {
		t.Fatalf("main-screen init should not emit clearScreenMsg, got %#v", types)
	}
}

func TestUpdate_MainScreenWindowResizeDoesNotEmitClearScreen(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	updated := next.(Model)
	if updated.isFullscreenMode() {
		t.Fatal("main-screen effective resize should stay in main-screen mode")
	}
	types := collectCmdTypeNames(cmd)
	if findMsgTypeIndex(types, "clearScreenMsg") >= 0 {
		t.Fatalf("main-screen effective resize should not emit clearScreenMsg, got %#v", types)
	}
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("main-screen effective resize should not switch to viewport-owned transcript mode")
	}
}

func TestUpdate_MainScreenWindowHeightResizeEntersViewportOwned(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	updated := next.(Model)
	if updated.width != 80 || updated.height != 20 {
		t.Fatalf("height resize should update dimensions, got %dx%d", updated.width, updated.height)
	}
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("height-only main-screen resize should not switch to viewport-owned transcript mode")
	}
	if findMsgTypeIndex(collectCmdTypeNames(cmd), "clearScreenMsg") >= 0 {
		t.Fatalf("main-screen height resize should not emit clearScreenMsg, got %#v", collectCmdTypeNames(cmd))
	}
}

func TestUpdate_MainScreenWindowResizeSameSizeIsNoOp(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("same-size resize should return nil command, got %v", cmd)
	}
	updated := next.(Model)
	if updated.width != 80 || updated.height != 24 {
		t.Fatalf("same-size resize should keep dimensions unchanged, got %dx%d", updated.width, updated.height)
	}
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("same-size resize should not switch transcript ownership mode")
	}
	if updated.chat.mainScreenFrame != nil && updated.chat.mainScreenFrame.PendingReset {
		t.Fatal("same-size resize should not schedule visible reset")
	}
}

func TestView_MainScreenResizeRendersStagedToken(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "reset-user",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "resize-visible-reset"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	updated := next.(Model)
	updated.mainOutput = NewMainScreenOutputController(MainScreenResetModeFull)
	first := updated.View()
	if !strings.Contains(first, mainScreenFrameTokenPrefix) {
		t.Fatalf("main-screen output bridge should stage frame token, got %q", first)
	}
	second := updated.View()
	if first == second {
		t.Fatalf("consecutive staged frames should use distinct tokens, got %q", second)
	}
}

func TestSwitchScreenMode_FullscreenToMainClearsMainScreenViewportOwnedFlag(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.chat.mainScreenViewportOwned = true
	m.chat.mainScreenOwnedStart = &messageSliceAnchor{MessageID: "old", Idx: 0}
	m.chat.mainScreenOwnedStream = &streamLineFlushState{MessageID: "old"}
	m.chat.mainScreenFrame = &mainScreenFrameState{PendingReset: true}

	changed, _ := m.switchScreenMode(ScreenModeMain)
	if !changed {
		t.Fatal("expected mode change when switching fullscreen -> main")
	}
	if m.chat.mainScreenViewportOwned {
		t.Fatal("switching screen mode should clear main-screen viewport-owned state")
	}
	if m.chat.mainScreenOwnedStart != nil || m.chat.mainScreenOwnedStream != nil || m.chat.mainScreenFrame != nil {
		t.Fatal("switching screen mode should clear resize-owned main-screen state")
	}
}

func TestSwitchScreenMode_MainToFullscreenClearsMainScreenViewportOwnedFlag(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.chat.mainScreenViewportOwned = true
	m.chat.mainScreenOwnedStart = &messageSliceAnchor{MessageID: "old", Idx: 0}
	m.chat.mainScreenOwnedStream = &streamLineFlushState{MessageID: "old"}
	m.chat.mainScreenFrame = &mainScreenFrameState{PendingReset: true}

	changed, _ := m.switchScreenMode(ScreenModeFullscreen)
	if !changed {
		t.Fatal("expected mode change when switching main -> fullscreen")
	}
	if m.chat.mainScreenViewportOwned {
		t.Fatal("switching into fullscreen should clear main-screen viewport-owned state")
	}
	if m.chat.mainScreenOwnedStart != nil || m.chat.mainScreenOwnedStream != nil || m.chat.mainScreenFrame != nil {
		t.Fatal("switching into fullscreen should clear resize-owned main-screen state")
	}
}
