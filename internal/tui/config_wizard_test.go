package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	appcore "github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

func loadWizardConfig(t *testing.T) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")
	if _, err := config.Load(t.TempDir()); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func TestStartConfigWizardClearsTransientComposerState(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.composer.commandOutput = "stale"
	m.composer.commandDialog = CommandDialogState{Visible: true}
	m.composer.commandPickerActive = true
	m.composer.commandPickerIdx = 2
	m.composer.filteredCompletions = []command.CompletionItem{{Value: "config"}}
	m.composer.filePicker = picker.New(8)
	m.composer.filePicker.Active = true

	m.startConfigWizard()

	if m.state != stateConfigWizard {
		t.Fatalf("state = %v, want stateConfigWizard", m.state)
	}
	if m.composer.commandOutput != "" {
		t.Fatalf("command output should be cleared, got %q", m.composer.commandOutput)
	}
	if m.composer.commandDialog.Visible {
		t.Fatal("command dialog should be cleared")
	}
	if m.composer.commandPickerActive {
		t.Fatal("command picker should be inactive")
	}
	if m.composer.commandPickerIdx != 0 {
		t.Fatalf("commandPickerIdx = %d, want 0", m.composer.commandPickerIdx)
	}
	if len(m.composer.filteredCompletions) != 0 {
		t.Fatalf("filteredCompletions should be cleared, got %d", len(m.composer.filteredCompletions))
	}
	if m.composer.filePicker != nil && m.composer.filePicker.Active {
		t.Fatal("file picker should be reset")
	}
}

func TestConfigWizardVeryNarrowWidthDoesNotPanic(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.startConfigWizard()
	m.width = 1
	m.height = 3
	m.recalcLayout()

	_ = m.View()
}

func TestConfigWizardOverlayVeryNarrowWidthDoesNotPanic(t *testing.T) {
	loadWizardConfig(t)
	overlay := NewConfigWizardOverlay(config.Get())

	if got := overlay.View(1, 3); strings.TrimSpace(stripANSI(got)) == "" {
		t.Fatal("very narrow config wizard overlay rendered empty output")
	}
}

func TestConfigWizardOwnsKeysOverTransientComposerUI(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.startConfigWizard()
	m.composer.commandOutput = "stale output"
	m.composer.commandPickerActive = true
	m.composer.filePicker.Active = true
	m.composer.filePicker.Items = []picker.FileEntry{{RelPath: "a.md"}, {RelPath: "b.md"}}

	updated := dispatchKey(m, tea.KeyMsg{Type: tea.KeyDown})
	if updated.dlg.configWizard.idx != 1 {
		t.Fatalf("wizard idx = %d, want 1", updated.dlg.configWizard.idx)
	}
	if updated.composer.commandOutput != "stale output" {
		t.Fatalf("wizard key should not dismiss command output, got %q", updated.composer.commandOutput)
	}
	if !updated.composer.commandPickerActive {
		t.Fatal("wizard key should not route through command picker")
	}
}

func TestConfigWizardProviderMenuEnterOpensSelectedProvider(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.startConfigWizard()
	step := m.dlg.configWizard.wizard.CurrentStep()
	if step == nil {
		t.Fatal("expected provider menu step")
	}
	targetIdx := -1
	for i, opt := range step.Options {
		if opt.Value == string(models.ProviderOpenAI) {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		t.Fatal("openai option not found")
	}
	m.dlg.configWizard.idx = targetIdx

	updated := dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.dlg.configWizard.wizard.CurrentStep()
	if next == nil || next.Type != config.CWStepProviderKey {
		t.Fatalf("step type = %v, want CWStepProviderKey", next)
	}
	if next.Provider != models.ProviderOpenAI {
		t.Fatalf("selected provider = %s, want openai", next.Provider)
	}
}

func TestConfigWizardEscExitsWizard(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.startConfigWizard()

	updated := dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if updated.state != stateChat {
		t.Fatalf("state = %v, want stateChat", updated.state)
	}
	if updated.dlg.configWizard.wizard != nil {
		t.Fatal("wizard should be cleared")
	}
	if len(updated.chat.messages) != 0 {
		t.Fatalf("expected no activity without pending invocation, got %d", len(updated.chat.messages))
	}
}

func TestConfigWizardDismissRecordsPendingCommandActivity(t *testing.T) {
	loadWizardConfig(t)
	m := newTestModel()
	m.startConfigWizard()
	m.composer.pendingCommandActivity = CommandActivityState{Invocation: "/config wizard"}

	updated := dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(updated.chat.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(updated.chat.messages))
	}
	meta := updated.chat.messages[0].Meta
	if got, _ := meta["command_invocation"].(string); got != "/config wizard" {
		t.Fatalf("command_invocation = %q", got)
	}
	if got, _ := meta["command_summary"].(string); got != "Config wizard dismissed" {
		t.Fatalf("command_summary = %q", got)
	}
}

type runtimeConfigTestAgent struct {
	*layoutTestAgent
	model     models.Model
	reloads   int
	reloadErr error
}

func (a *runtimeConfigTestAgent) Model() models.Model {
	return a.model
}

func (a *runtimeConfigTestAgent) ReloadProvider() error {
	a.reloads++
	if a.reloadErr != nil {
		return a.reloadErr
	}
	cfg := config.Get()
	providerName := cfg.DefaultProvider
	modelID := ""
	if agentCfg, ok := cfg.Agents[config.AgentCoder]; ok {
		if agentCfg.Provider != "" {
			providerName = agentCfg.Provider
		}
		modelID = agentCfg.Model
	}
	providerCfg := cfg.Providers[providerName]
	if modelID == "" {
		modelID = providerCfg.Model
	}
	if modelCfg, ok := providerCfg.Models[modelID]; ok {
		a.model = config.ModelFromConfig(providerName, modelID, modelCfg)
		return nil
	}
	if ref, err := models.ResolveModelRefForProvider(providerName, modelID); err == nil && ref.Metadata != nil {
		a.model = *ref.Metadata
		return nil
	}
	a.model = models.NewCustomModel(providerName, modelID)
	return nil
}

func newRuntimeConfigTestModel(t *testing.T, dir string, modelID string) (*Model, *runtimeConfigTestAgent) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	cfg := config.Config{
		WorkingDir:      dir,
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  modelID,
			},
		},
		Agents: map[config.AgentName]config.Agent{
			config.AgentCoder: {Provider: models.ProviderOpenAI, Model: modelID},
		},
	}
	if err := config.SaveFull(dir, cfg); err != nil {
		t.Fatalf("SaveFull: %v", err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	ref, err := models.ResolveModelRefForProvider(models.ProviderOpenAI, modelID)
	if err != nil || ref.Metadata == nil {
		t.Fatalf("resolve initial model: ref=%+v err=%v", ref, err)
	}

	agent := &runtimeConfigTestAgent{
		layoutTestAgent: newLayoutTestAgent(),
		model:           *ref.Metadata,
	}
	m := newTestModel()
	m.app = &appcore.App{CoderAgent: agent}
	m.width = 80
	m.height = 24
	m.recalcLayout()
	return m, agent
}

func newCoderModelWizard(t *testing.T, modelID string) *config.ConfigWizard {
	t.Helper()
	w := config.NewConfigWizard(config.Get())
	w.Apply("continue")
	w.Apply(string(models.ProviderOpenAI))
	w.Apply(string(config.AgentCoder))
	w.Apply(string(models.ProviderOpenAI))
	w.Apply(modelID)
	w.Apply("continue")
	w.Apply("save")
	if !w.IsDone() || !w.HasChanges() {
		t.Fatalf("wizard did not finish with changes")
	}
	return w
}

func TestFinishConfigWizardAppliesRuntimeRefresh(t *testing.T) {
	m, agent := newRuntimeConfigTestModel(t, t.TempDir(), string(models.GPT41))
	_ = m.updateViewportContent()
	if !strings.Contains(stripANSI(m.chat.renderedContent), "GPT-4.1") {
		t.Fatalf("initial welcome should contain old model, got %q", stripANSI(m.chat.renderedContent))
	}

	m.state = stateConfigWizard
	m.dlg.configWizard.wizard = newCoderModelWizard(t, string(models.GPT55))
	updated, _ := m.finishConfigWizard(true)
	m = updated.(*Model)

	if agent.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", agent.reloads)
	}
	if !strings.Contains(m.composer.commandOutput, "配置已保存并生效") {
		t.Fatalf("command output = %q", m.composer.commandOutput)
	}
	if got := stripANSI(m.chat.renderedContent); !strings.Contains(got, "GPT-5.5") {
		t.Fatalf("welcome was not refreshed with new model: %q", got)
	}
	if got := stripANSI(m.renderHeaderBar()); !strings.Contains(got, "GPT-5.5") {
		t.Fatalf("header was not refreshed with new model: %q", got)
	}
	if got := stripANSI(m.renderStatusBar()); !strings.Contains(got, "GPT-5.5") {
		t.Fatalf("status was not refreshed with new model: %q", got)
	}
}

func TestOverlayConfigWizardAppliesRuntimeRefresh(t *testing.T) {
	m, agent := newRuntimeConfigTestModel(t, t.TempDir(), string(models.GPT41))
	_ = m.updateViewportContent()

	choice := ConfigWizardChoice{Wizard: newCoderModelWizard(t, string(models.GPT55)), Saved: true}
	_, _ = m.applyConfigWizardChoice(choice)

	if agent.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", agent.reloads)
	}
	if got := stripANSI(m.chat.renderedContent); !strings.Contains(got, "GPT-5.5") {
		t.Fatalf("overlay welcome was not refreshed with new model: %q", got)
	}
}

func TestRuntimeConfigRefreshDoesNotReprintFlushedIntro(t *testing.T) {
	m, _ := newRuntimeConfigTestModel(t, t.TempDir(), string(models.GPT41))
	m.chat.mainScreenIntroFlushed = true
	m.chat.renderedContent = "stale GPT-4.1 welcome"
	m.chat.contentLines = []string{"stale GPT-4.1 welcome"}

	choice := ConfigWizardChoice{Wizard: newCoderModelWizard(t, string(models.GPT55)), Saved: true}
	_, cmd := m.applyConfigWizardChoice(choice)

	if got := stripANSI(m.chat.renderedContent); strings.Contains(got, "Welcome") || strings.Contains(got, "GPT-5.5") {
		t.Fatalf("flushed intro should not be reintroduced into live transcript: %q", got)
	}
	if printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n")); strings.Contains(printed, "Welcome") || strings.Contains(printed, "GPT-5.5") {
		t.Fatalf("flushed intro should not be reprinted to native scrollback: %q", printed)
	}
}

func TestRuntimeConfigRefreshPreservesManualScrollAnchor(t *testing.T) {
	m, _ := newRuntimeConfigTestModel(t, t.TempDir(), string(models.GPT41))
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 54
	m.height = 12
	m.recalcLayout()
	m.chat.messages = []message.Message{
		{
			ID:   "user-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Please review this long note."},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "assistant-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: strings.Repeat("This assistant paragraph wraps over several terminal rows. ", 12)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	_ = m.updateViewportContent()
	blocks := m.chat.blockList.All()
	targetLine := lineForMessageStart(m, "assistant-1") + 2
	m.chat.scrollMode = ScrollManualLocked
	m.chat.viewport.SetYOffset(targetLine)
	before := components.AnchorFromDisplayLine(m.chat.heightCache, blocks, m.chat.viewport.YOffset, 1)

	choice := ConfigWizardChoice{Wizard: newCoderModelWizard(t, string(models.GPT55)), Saved: true}
	_, _ = m.applyConfigWizardChoice(choice)

	after := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
	if after.BlockID != before.BlockID || after.MsgID != before.MsgID || after.LineOffset != before.LineOffset {
		t.Fatalf("manual scroll anchor changed: before=%+v after=%+v yOffset=%d", before, after, m.chat.viewport.YOffset)
	}
}

func TestRuntimeConfigReloadFailureDoesNotInvalidateTranscriptCaches(t *testing.T) {
	m, agent := newRuntimeConfigTestModel(t, t.TempDir(), string(models.GPT41))
	agent.reloadErr = errors.New("reload failed")
	m.chat.renderedContent = "old rendered content"
	m.chat.contentLines = []string{"old rendered content"}

	output, _, applied := m.saveFullAndApplyRuntimeConfig(newCoderModelWizard(t, string(models.GPT55)).BuildConfig())
	if applied {
		t.Fatal("runtime config change should not be applied after reload failure")
	}
	if !strings.Contains(output, "运行时重载失败") {
		t.Fatalf("output = %q", output)
	}
	if m.chat.renderedContent != "old rendered content" || len(m.chat.contentLines) != 1 {
		t.Fatalf("transcript caches were invalidated on reload failure: rendered=%q lines=%v", m.chat.renderedContent, m.chat.contentLines)
	}
}

func TestResetSessionStateClearsCommandTransientState(t *testing.T) {
	m := newTestModel()
	m.composer.commandOutput = "stale output"
	m.composer.commandDialog = CommandDialogState{
		Visible:         true,
		Invocation:      "/config",
		Body:            "dialog body",
		DismissText:     "Settings dialog dismissed",
		RecordOnDismiss: true,
	}
	m.composer.pendingCommandActivity = CommandActivityState{Invocation: "/config wizard"}

	m.resetSessionState()

	if m.composer.commandOutput != "" {
		t.Fatalf("commandOutput = %q, want empty", m.composer.commandOutput)
	}
	if m.composer.commandDialog.Visible {
		t.Fatal("command dialog should be cleared")
	}
	if m.composer.pendingCommandActivity.Invocation != "" {
		t.Fatalf("pending command activity = %+v, want empty", m.composer.pendingCommandActivity)
	}
}
