package config

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/llm/models"
)

// CWStepType defines the type of a config wizard step.
type CWStepType int

const (
	CWStepProviderKey     CWStepType = iota // text input for API key
	CWStepProviderBaseURL                   // text input for base URL
	CWStepDefaultProvider                   // select default provider
	CWStepAgentModel                        // select model for an agent
	CWStepSummary                           // review changes
	CWStepProviderMenu                      // provider list menu
	CWStepAgentMenu                         // agent list menu
	CWStepAgentProvider                     // select provider for an agent
)

// CWInputKind defines how a step input should be rendered by the UI.
type CWInputKind int

const (
	CWInputKindText CWInputKind = iota
	CWInputKindSensitive
)

// CWOption is a selectable option in a wizard step.
type CWOption struct {
	Label   string
	Value   string
	Display *CWOptionDisplay
}

// CWOptionDisplay defines structured display metadata for menu options.
type CWOptionDisplay struct {
	Primary   string
	Secondary string
}

// CWStep describes a single step of the config wizard.
type CWStep struct {
	Type         CWStepType
	ID           string     // e.g. "provider_anthropic", "agent_coder"
	Label        string     // display question
	Options      []CWOption // for select steps
	Placeholder  string     // for text input steps
	CurrentValue string     // masked key or current model name
	InputKind    CWInputKind
	Provider     models.ModelProvider
	AgentName    AgentName
}

const (
	cwSubMenu    = 0 // browsing menu
	cwSubEditing = 1 // editing a selected item
)

// ConfigWizard is a UI-agnostic engine that walks through config editing steps.
type ConfigWizard struct {
	original      *Config
	providerOrder []models.ModelProvider
	agentOrder    []AgentName

	// Collected changes
	apiKeys        map[models.ModelProvider]string // only changed keys
	baseURLs       map[models.ModelProvider]string
	defaultProv    models.ModelProvider
	agentModels    map[AgentName]string               // changed model IDs
	agentProviders map[AgentName]models.ModelProvider // changed agent providers

	// Phase tracking: 0=providers, 1=default, 2=agents, 3=summary
	phase int

	// Sub-state for menu phases (0 and 2)
	subState         int                  // cwSubMenu or cwSubEditing
	selectedProvider models.ModelProvider // provider being edited (phase 0)
	selectedAgent    AgentName            // agent being edited (phase 2)
	agentProvChoice  models.ModelProvider // intermediate provider choice for agent model

	// For provider phase: track if openai_compatible needs base URL next
	needBaseURL bool

	done      bool
	cancelled bool
	changes   []string // human-readable change list
}

// NewConfigWizard creates a config wizard from the current running config.
func NewConfigWizard(cfg *Config) *ConfigWizard {
	providerOrder := make([]models.ModelProvider, 0, len(models.ProviderCatalog()))
	for _, spec := range models.ProviderCatalog() {
		providerOrder = append(providerOrder, spec.ID)
	}
	w := &ConfigWizard{
		original:      cfg,
		providerOrder: providerOrder,
		agentOrder: []AgentName{
			AgentCoder, AgentGeneral, AgentExplore,
			AgentPlan, AgentVerify, AgentCoordinator,
			AgentSummarizer, AgentTitle, AgentLeader,
		},
		apiKeys:        make(map[models.ModelProvider]string),
		baseURLs:       make(map[models.ModelProvider]string),
		defaultProv:    cfg.DefaultProvider,
		agentModels:    make(map[AgentName]string),
		agentProviders: make(map[AgentName]models.ModelProvider),
	}
	return w
}

// CurrentStep returns the current step descriptor, or nil if done.
func (w *ConfigWizard) CurrentStep() *CWStep {
	if w.done || w.cancelled {
		return nil
	}

	switch w.phase {
	case 0: // Provider menu
		if w.subState == cwSubEditing {
			if w.needBaseURL {
				return w.baseURLStep()
			}
			return w.providerKeyStep()
		}
		return w.providerMenuStep()

	case 1: // Default provider
		return w.defaultProviderStep()

	case 2: // Agent menu
		if w.subState == cwSubEditing {
			if w.agentProvChoice == "" {
				return w.agentProviderStep()
			}
			return w.agentModelStep()
		}
		return w.agentMenuStep()

	case 3: // Summary
		return w.summaryStep()
	}

	return nil
}

// Apply commits the value for the current step and advances.
func (w *ConfigWizard) Apply(value string) {
	step := w.CurrentStep()
	if step == nil {
		return
	}

	switch step.Type {
	case CWStepProviderMenu:
		if value == "continue" {
			// Check at least one provider is configured
			if len(w.ConfiguredProviders()) == 0 {
				return // stay on menu
			}
			w.phase = 1
			w.subState = cwSubMenu
		} else {
			w.selectedProvider = models.ModelProvider(value)
			w.subState = cwSubEditing
		}

	case CWStepProviderKey:
		value = strings.TrimSpace(value)
		if value != "" {
			w.apiKeys[step.Provider] = value
		}
		// Check if this provider needs a base URL
		if models.ProviderRequiresBaseURL(step.Provider) {
			w.needBaseURL = true
			return
		}
		w.needBaseURL = false
		w.subState = cwSubMenu

	case CWStepProviderBaseURL:
		value = strings.TrimSpace(value)
		if value != "" {
			w.baseURLs[step.Provider] = value
		}
		w.needBaseURL = false
		w.subState = cwSubMenu

	case CWStepDefaultProvider:
		w.defaultProv = models.ModelProvider(value)
		w.phase = 2
		w.subState = cwSubMenu

	case CWStepAgentMenu:
		if value == "continue" {
			w.phase = 3
		} else {
			w.selectedAgent = AgentName(value)
			w.agentProvChoice = ""
			w.subState = cwSubEditing
		}

	case CWStepAgentProvider:
		w.agentProvChoice = models.ModelProvider(value)

	case CWStepAgentModel:
		value = strings.TrimSpace(value)
		if value != "" {
			w.agentModels[step.AgentName] = value
			w.agentProviders[step.AgentName] = w.agentProvChoice
		}
		w.agentProvChoice = ""
		w.subState = cwSubMenu

	case CWStepSummary:
		// "save" → build changes and finish
		w.buildChanges()
		w.done = true
	}
}

// Back returns to the parent menu from an editing sub-state without saving.
func (w *ConfigWizard) Back() {
	switch w.phase {
	case 0:
		w.needBaseURL = false
		w.subState = cwSubMenu
	case 2:
		if w.agentProvChoice != "" {
			// Back from model select → provider select
			w.agentProvChoice = ""
		} else {
			// Back from provider select → agent menu
			w.subState = cwSubMenu
		}
	}
}

// Skip keeps the existing value and advances to the next step.
func (w *ConfigWizard) Skip() {
	step := w.CurrentStep()
	if step == nil {
		return
	}

	switch step.Type {
	case CWStepProviderMenu:
		// Skip provider menu entirely → go to default provider
		w.phase = 1
		w.subState = cwSubMenu
	case CWStepProviderKey:
		w.needBaseURL = false
		w.subState = cwSubMenu
	case CWStepProviderBaseURL:
		w.needBaseURL = false
		w.subState = cwSubMenu
	case CWStepDefaultProvider:
		// Keep existing default
		w.phase = 2
		w.subState = cwSubMenu
	case CWStepAgentMenu:
		// Skip agent menu → go to summary
		w.phase = 3
	case CWStepAgentProvider:
		w.agentProvChoice = ""
		w.subState = cwSubMenu
	case CWStepAgentModel:
		w.agentProvChoice = ""
		w.subState = cwSubMenu
	case CWStepSummary:
		// Skip summary = cancel
		w.cancelled = true
	}
}

// Cancel aborts the wizard without saving.
func (w *ConfigWizard) Cancel() {
	w.cancelled = true
}

// IsDone returns true when wizard completed (ready to save).
func (w *ConfigWizard) IsDone() bool { return w.done }

// IsCancelled returns true when wizard was cancelled.
func (w *ConfigWizard) IsCancelled() bool { return w.cancelled }

// Changes returns human-readable list of what changed.
func (w *ConfigWizard) Changes() []string {
	if len(w.changes) == 0 {
		w.buildChanges()
	}
	return w.changes
}

// HasChanges returns true if any configuration was modified.
func (w *ConfigWizard) HasChanges() bool {
	return len(w.apiKeys) > 0 || len(w.baseURLs) > 0 ||
		w.defaultProv != w.original.DefaultProvider ||
		len(w.agentModels) > 0
}

// ConfiguredProviders returns providers that have API keys (existing or newly added).
func (w *ConfigWizard) ConfiguredProviders() []models.ModelProvider {
	var result []models.ModelProvider
	seen := make(map[models.ModelProvider]bool)

	for _, prov := range w.providerOrder {
		if w.providerIsConfigured(prov) && !seen[prov] {
			seen[prov] = true
			result = append(result, prov)
		}
	}
	return result
}

func (w *ConfigWizard) providerIsConfigured(prov models.ModelProvider) bool {
	if models.ProviderRequiresBaseURL(prov) {
		return w.providerBaseURLConfigured(prov)
	}
	if _, ok := w.apiKeys[prov]; ok {
		return true
	}
	if _, ok := w.baseURLs[prov]; ok {
		return true
	}
	if p, ok := w.original.Providers[prov]; ok && !p.Disabled {
		if p.APIKey != "" || p.BaseURL != "" || p.Model != "" {
			return true
		}
	}
	return false
}

func (w *ConfigWizard) providerBaseURLConfigured(prov models.ModelProvider) bool {
	if url, ok := w.baseURLs[prov]; ok && strings.TrimSpace(url) != "" {
		return true
	}
	if p, ok := w.original.Providers[prov]; ok && !p.Disabled && strings.TrimSpace(p.BaseURL) != "" {
		return true
	}
	return false
}

// BuildConfig merges original config with wizard changes and returns a new Config.
func (w *ConfigWizard) BuildConfig() *Config {
	// Deep copy providers
	providers := make(map[models.ModelProvider]Provider)
	for k, v := range w.original.Providers {
		providers[k] = v
	}
	// Apply new API keys
	for prov, key := range w.apiKeys {
		p := providers[prov]
		p.APIKey = key
		providers[prov] = p
	}
	// Apply new base URLs
	for prov, url := range w.baseURLs {
		p := providers[prov]
		p.BaseURL = url
		providers[prov] = p
	}

	// Deep copy agents
	agents := make(map[AgentName]Agent)
	for k, v := range w.original.Agents {
		agents[k] = v
	}
	// Apply agent model and provider changes
	for name, modelID := range w.agentModels {
		a := agents[name]
		a.Model = modelID
		if prov, ok := w.agentProviders[name]; ok {
			a.Provider = prov
		}
		agents[name] = a
	}

	return &Config{
		WorkingDir:            w.original.WorkingDir,
		Data:                  w.original.Data,
		DefaultProvider:       w.defaultProv,
		ImageProvider:         w.original.ImageProvider,
		Providers:             providers,
		Agents:                agents,
		Debug:                 w.original.Debug,
		SessionLog:            w.original.SessionLog,
		PaperType:             w.original.PaperType,
		OllamaBaseURL:         w.original.OllamaBaseURL,
		VLLMBaseURL:           w.original.VLLMBaseURL,
		MCPServers:            w.original.MCPServers,
		CodeAgent:             w.original.CodeAgent,
		Harness:               w.original.Harness,
		Experiment:            w.original.Experiment,
		SubagentOrchestration: w.original.SubagentOrchestration,
		Scholar:               w.original.Scholar,
		Hooks:                 w.original.Hooks,
		Web:                   w.original.Web,
	}
}

// PhaseProgress returns phase-based progress info: (phase 1-based, total phases, phase label).
func (w *ConfigWizard) PhaseProgress() (int, int, string) {
	labels := []string{"Provider 配置", "默认 Provider", "Agent 模型配置", "配置摘要"}
	phase := w.phase
	if phase >= len(labels) {
		phase = len(labels) - 1
	}
	return phase + 1, len(labels), labels[phase]
}

// StepProgress returns (1-based current, total) for progress display.
// Kept for backward compatibility but PhaseProgress is preferred.
func (w *ConfigWizard) StepProgress() (int, int) {
	p, t, _ := w.PhaseProgress()
	return p, t
}

// --- Step builders ---

func (w *ConfigWizard) providerMenuStep() *CWStep {
	var options []CWOption

	for _, prov := range w.providerOrder {
		name := models.ProviderDisplayName(prov)
		status := "✗"
		detail := "未配置"

		// Check if has key (new or existing)
		if key, ok := w.apiKeys[prov]; ok {
			status = "✓"
			detail = maskKey(key) + " (已修改)"
		} else if p, ok := w.original.Providers[prov]; ok && p.APIKey != "" {
			status = "✓"
			detail = maskKey(p.APIKey)
		} else if url, ok := w.baseURLs[prov]; ok && strings.TrimSpace(url) != "" {
			status = "✓"
			detail = url + " (已修改)"
		} else if p, ok := w.original.Providers[prov]; ok && (p.BaseURL != "" || p.Model != "") && !p.Disabled {
			status = "✓"
			if p.BaseURL != "" {
				detail = p.BaseURL
			} else {
				detail = p.Model
			}
		}

		options = append(options, CWOption{
			Label: fmt.Sprintf("%s %-20s %s", status, name, detail),
			Value: string(prov),
			Display: &CWOptionDisplay{
				Primary:   fmt.Sprintf("%s %s", status, name),
				Secondary: detail,
			},
		})
	}

	// Add continue option
	options = append(options, CWOption{
		Label: "▶ 继续",
		Value: "continue",
	})

	return &CWStep{
		Type:    CWStepProviderMenu,
		ID:      "provider_menu",
		Label:   "选择要配置的 Provider",
		Options: options,
	}
}

func (w *ConfigWizard) providerKeyStep() *CWStep {
	prov := w.selectedProvider
	name := models.ProviderDisplayName(prov)

	currentValue := ""
	if p, ok := w.original.Providers[prov]; ok && p.APIKey != "" {
		currentValue = maskKey(p.APIKey)
	}
	// If already changed in this session, show that
	if key, ok := w.apiKeys[prov]; ok {
		currentValue = maskKey(key) + " (已修改)"
	}

	return &CWStep{
		Type:         CWStepProviderKey,
		ID:           "provider_" + string(prov),
		Label:        fmt.Sprintf("配置 %s 的 API Key", name),
		Placeholder:  providerKeyPlaceholder(prov),
		CurrentValue: currentValue,
		InputKind:    CWInputKindSensitive,
		Provider:     prov,
	}
}

func (w *ConfigWizard) baseURLStep() *CWStep {
	currentValue := ""
	prov := w.selectedProvider
	if p, ok := w.original.Providers[prov]; ok && p.BaseURL != "" {
		currentValue = p.BaseURL
	}
	if url, ok := w.baseURLs[prov]; ok && url != "" {
		currentValue = url
	}
	placeholder := "https://example.com/v1"
	if def := models.ProviderDefaultBaseURL(prov); def != "" {
		placeholder = def
	}

	return &CWStep{
		Type:         CWStepProviderBaseURL,
		ID:           "baseurl_" + string(prov),
		Label:        fmt.Sprintf("配置 %s 的 Base URL", models.ProviderDisplayName(prov)),
		Placeholder:  placeholder,
		CurrentValue: currentValue,
		Provider:     prov,
	}
}

func (w *ConfigWizard) defaultProviderStep() *CWStep {
	var options []CWOption
	for _, prov := range w.ConfiguredProviders() {
		options = append(options, CWOption{
			Label: models.ProviderDisplayName(prov),
			Value: string(prov),
		})
	}

	currentValue := models.ProviderDisplayName(w.original.DefaultProvider)

	return &CWStep{
		Type:         CWStepDefaultProvider,
		ID:           "default_provider",
		Label:        "选择默认 Provider",
		Options:      options,
		CurrentValue: currentValue,
	}
}

func (w *ConfigWizard) agentMenuStep() *CWStep {
	var options []CWOption

	for _, agentName := range w.agentOrder {
		currentModel := ""
		if a, ok := w.original.Agents[agentName]; ok && a.Model != "" {
			currentModel = a.Model
		}
		// Show updated model if changed
		if m, ok := w.agentModels[agentName]; ok {
			currentModel = m + " (已修改)"
		}
		if currentModel == "" {
			currentModel = "未配置"
		}

		options = append(options, CWOption{
			Label: fmt.Sprintf("%-14s %s", agentName, currentModel),
			Value: string(agentName),
		})
	}

	// Add continue option
	options = append(options, CWOption{
		Label: "▶ 继续",
		Value: "continue",
	})

	return &CWStep{
		Type:    CWStepAgentMenu,
		ID:      "agent_menu",
		Label:   "选择要配置的 Agent",
		Options: options,
	}
}

func (w *ConfigWizard) agentProviderStep() *CWStep {
	var options []CWOption
	for _, prov := range w.ConfiguredProviders() {
		options = append(options, CWOption{
			Label: models.ProviderDisplayName(prov),
			Value: string(prov),
		})
	}

	// Determine current provider for this agent
	currentProv := w.defaultProv
	if a, ok := w.original.Agents[w.selectedAgent]; ok && a.Provider != "" {
		currentProv = a.Provider
	}

	return &CWStep{
		Type:         CWStepAgentProvider,
		ID:           "agent_provider_" + string(w.selectedAgent),
		Label:        fmt.Sprintf("为 %s Agent 选择 Provider", w.selectedAgent),
		Options:      options,
		CurrentValue: models.ProviderDisplayName(currentProv),
		AgentName:    w.selectedAgent,
	}
}

func (w *ConfigWizard) agentModelStep() *CWStep {
	agentName := w.selectedAgent
	agentProv := w.agentProvChoice

	// Current model for this agent
	currentModel := ""
	if a, ok := w.original.Agents[agentName]; ok && a.Model != "" {
		currentModel = a.Model
	}

	// Build model options from the selected provider
	var options []CWOption
	for _, opt := range ModelOptionsForProvider(w.original, agentProv, nil) {
		m := opt.Model
		options = append(options, CWOption{
			Label: fmt.Sprintf("%-25s %s", m.Name, m.ID),
			Value: string(m.ID),
		})
	}
	// For dynamic providers, show current as the only option
	if len(options) == 0 && currentModel != "" {
		options = append(options, CWOption{
			Label: currentModel,
			Value: currentModel,
		})
	}

	return &CWStep{
		Type:         CWStepAgentModel,
		ID:           "agent_" + string(agentName),
		Label:        fmt.Sprintf("配置 %s Agent 的模型 (provider: %s)", agentName, models.ProviderDisplayName(agentProv)),
		Options:      options,
		CurrentValue: currentModel,
		AgentName:    agentName,
	}
}

func (w *ConfigWizard) summaryStep() *CWStep {
	w.buildChanges()
	label := "配置摘要"
	if !w.HasChanges() {
		label = "未做任何更改"
	}
	return &CWStep{
		Type:  CWStepSummary,
		ID:    "summary",
		Label: label,
	}
}

func (w *ConfigWizard) buildChanges() {
	w.changes = nil

	for prov, key := range w.apiKeys {
		name := models.ProviderDisplayName(prov)
		if _, ok := w.original.Providers[prov]; ok {
			w.changes = append(w.changes, fmt.Sprintf("更新 %s API Key → %s", name, maskKey(key)))
		} else {
			w.changes = append(w.changes, fmt.Sprintf("新增 %s API Key → %s", name, maskKey(key)))
		}
	}
	for prov, url := range w.baseURLs {
		name := models.ProviderDisplayName(prov)
		w.changes = append(w.changes, fmt.Sprintf("设置 %s Base URL → %s", name, url))
	}
	if w.defaultProv != w.original.DefaultProvider {
		w.changes = append(w.changes, fmt.Sprintf("默认 Provider: %s → %s",
			models.ProviderDisplayName(w.original.DefaultProvider),
			models.ProviderDisplayName(w.defaultProv)))
	}
	for name, modelID := range w.agentModels {
		old := ""
		if a, ok := w.original.Agents[name]; ok {
			old = a.Model
		}
		if old != modelID {
			provLabel := ""
			if prov, ok := w.agentProviders[name]; ok {
				provLabel = fmt.Sprintf(" (%s)", models.ProviderDisplayName(prov))
			}
			w.changes = append(w.changes, fmt.Sprintf("%s Agent 模型: %s → %s%s", name, old, modelID, provLabel))
		}
	}
}

// --- Helpers ---

func maskKey(key string) string {
	return MaskSecret(key)
}

// ProviderKeyPlaceholder returns a placeholder hint for API key input.
func ProviderKeyPlaceholder(p models.ModelProvider) string {
	return providerKeyPlaceholder(p)
}

func providerKeyPlaceholder(p models.ModelProvider) string {
	switch p {
	case models.ProviderAnthropic:
		return "sk-ant-..."
	case models.ProviderOpenAI:
		return "sk-..."
	case models.ProviderDeepSeek:
		return "sk-..."
	case models.ProviderMiniMax:
		return "sk-cp-..."
	case models.ProviderGLM:
		return "API Key"
	case models.ProviderSiliconFlow:
		return "sk-..."
	case models.ProviderOpenAICompatible:
		return "API Key"
	default:
		return "API Key"
	}
}

// ProviderEnvHint returns the environment variable name for a provider.
func ProviderEnvHint(p models.ModelProvider) string {
	return providerEnvHint(p)
}

func providerEnvHint(p models.ModelProvider) string {
	return providerEnvKey(p)
}
