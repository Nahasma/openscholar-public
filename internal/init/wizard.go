package initwizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

type wizardStep int

const (
	stepWelcome     wizardStep = iota // Welcome screen
	stepProviderKey                   // Input API key for each provider
	stepBaseURL                       // OpenAI-Compatible base URL
	stepPing                          // Validate API keys (spinner)
	stepDefault                       // Select default provider
	stepCoderModel                    // Select coder agent model
	stepAgentModels                   // Optional: configure other agent models
	stepWebSearch                     // Configure web search backends
	stepSummary                       // Show config summary
	stepDone                          // Write config and finish
)

// pingDoneMsg is sent when validation completes.
type pingDoneMsg struct {
	results map[models.ModelProvider]ValidateResult
}

type wizardModel struct {
	step       wizardStep
	workingDir string
	width      int
	height     int

	// Provider configuration
	providerOrder []models.ModelProvider
	providerIdx   int                             // current provider being configured
	apiKeys       map[models.ModelProvider]string // collected API keys
	baseURLs      map[models.ModelProvider]string // for OpenAI-Compatible
	pingResults   map[models.ModelProvider]ValidateResult

	// Selection state
	defaultProvider models.ModelProvider
	coderModel      models.ModelID
	agentModels     map[config.AgentName]string

	// Web search configuration
	webSearchIdx  int // 0=brave, 1=tavily, 2=searxng
	webBraveKey   string
	webTavilyKey  string
	webSearXNGURL string

	// UI components
	textInput textinput.Model
	spinner   spinner.Model
	selectIdx int

	// State
	validProviders []models.ModelProvider // providers that passed validation
	skipAgentStep  bool

	// Output
	result *config.Config
	err    error
	done   bool
}

func newWizardModel(workingDir string) wizardModel {
	ti := textinput.New()
	ti.Placeholder = "sk-..."
	ti.Focus()
	ti.EchoMode = textinput.EchoPassword
	ti.CharLimit = 256

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

	return wizardModel{
		step:       stepWelcome,
		workingDir: workingDir,
		providerOrder: []models.ModelProvider{
			models.ProviderAnthropic,
			models.ProviderOpenAI,
			models.ProviderDeepSeek,
			models.ProviderMiniMax,
			models.ProviderGLM,
			models.ProviderSiliconFlow,
			models.ProviderOpenAICompatible,
		},
		providerIdx: 0,
		apiKeys:     make(map[models.ModelProvider]string),
		baseURLs:    make(map[models.ModelProvider]string),
		pingResults: make(map[models.ModelProvider]ValidateResult),
		agentModels: make(map[config.AgentName]string),
		textInput:   ti,
		spinner:     s,
	}
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.err = fmt.Errorf("cancelled by user")
			m.done = true
			return m, tea.Quit
		}

	case pingDoneMsg:
		m.pingResults = msg.results
		m.validProviders = nil
		for _, p := range m.providerOrder {
			if r, ok := m.pingResults[p]; ok && r.Valid {
				m.validProviders = append(m.validProviders, p)
			}
		}
		if len(m.validProviders) == 0 {
			m.step = stepProviderKey
			m.providerIdx = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = "sk-..."
			m.textInput.Focus()
			return m, textinput.Blink
		}
		m.step = stepDefault
		m.selectIdx = 0
		return m, nil

	case spinner.TickMsg:
		if m.step == stepPing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.step {
	case stepWelcome:
		return m.updateWelcome(msg)
	case stepProviderKey:
		return m.updateProviderKey(msg)
	case stepBaseURL:
		return m.updateBaseURL(msg)
	case stepPing:
		return m, nil
	case stepDefault:
		return m.updateDefault(msg)
	case stepCoderModel:
		return m.updateCoderModel(msg)
	case stepAgentModels:
		return m.updateAgentModels(msg)
	case stepWebSearch:
		return m.updateWebSearch(msg)
	case stepSummary:
		return m.updateSummary(msg)
	}

	return m, nil
}

func (m wizardModel) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" || keyMsg.String() == " " {
			m.step = stepProviderKey
			m.providerIdx = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = providerKeyPlaceholder(m.providerOrder[0])
			m.textInput.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m wizardModel) updateProviderKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			prov := m.providerOrder[m.providerIdx]
			val := strings.TrimSpace(m.textInput.Value())
			if val != "" {
				m.apiKeys[prov] = val
			}

			// If OpenAI-Compatible and key was entered, ask for base URL
			if prov == models.ProviderOpenAICompatible && val != "" {
				m.step = stepBaseURL
				m.textInput.SetValue("")
				m.textInput.Placeholder = "https://example.com/v1"
				m.textInput.EchoMode = textinput.EchoNormal
				return m, textinput.Blink
			}

			return m.advanceProvider()

		case "esc":
			// Skip this provider
			return m.advanceProvider()
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m wizardModel) updateBaseURL(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
		val := strings.TrimSpace(m.textInput.Value())
		if val != "" {
			m.baseURLs[models.ProviderOpenAICompatible] = val
		}
		m.textInput.EchoMode = textinput.EchoPassword
		return m.advanceProvider()
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m wizardModel) advanceProvider() (wizardModel, tea.Cmd) {
	m.providerIdx++
	if m.providerIdx >= len(m.providerOrder) {
		// All providers done — check if any keys were entered
		hasKey := false
		for _, k := range m.apiKeys {
			if k != "" {
				hasKey = true
				break
			}
		}
		if !hasKey {
			// No keys entered — go back to first provider
			m.providerIdx = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = providerKeyPlaceholder(m.providerOrder[0])
			return m, textinput.Blink
		}

		// Start validation
		m.step = stepPing
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg {
				results := ValidateProviders(m.apiKeys, m.baseURLs)
				return pingDoneMsg{results: results}
			},
		)
	}

	m.textInput.SetValue("")
	m.textInput.Placeholder = providerKeyPlaceholder(m.providerOrder[m.providerIdx])
	m.textInput.Focus()
	return m, textinput.Blink
}

func (m wizardModel) updateDefault(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.selectIdx > 0 {
				m.selectIdx--
			}
		case "down", "j":
			if m.selectIdx < len(m.validProviders)-1 {
				m.selectIdx++
			}
		case "enter":
			m.defaultProvider = m.validProviders[m.selectIdx]
			m.step = stepCoderModel
			m.selectIdx = 0
			return m, nil
		}
	}
	return m, nil
}

func (m wizardModel) updateCoderModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	providerModels := modelsForProvider(m.defaultProvider)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.selectIdx > 0 {
				m.selectIdx--
			}
		case "down", "j":
			if m.selectIdx < len(providerModels)-1 {
				m.selectIdx++
			}
		case "enter":
			m.coderModel = providerModels[m.selectIdx].ID
			m.step = stepWebSearch
			m.webSearchIdx = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = "BSA-xxx（留空跳过）"
			m.textInput.EchoMode = textinput.EchoPassword
			m.textInput.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m wizardModel) updateAgentModels(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Simplified: skip agent model configuration for now
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
		m.step = stepSummary
	}
	return m, nil
}

func (m wizardModel) updateSummary(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter", "y":
			m.buildConfig()
			m.step = stepDone
			m.done = true
			return m, tea.Quit
		case "esc", "n":
			// Go back to start
			m.step = stepProviderKey
			m.providerIdx = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = providerKeyPlaceholder(m.providerOrder[0])
			m.textInput.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

// webSearchSteps: 0=brave, 1=tavily, 2=searxng
var webSearchLabels = []struct {
	name        string
	placeholder string
	envHint     string
	isURL       bool
}{
	{"Brave Search", "BSA-xxx（留空跳过）", "BRAVE_API_KEY", false},
	{"Tavily", "tvly-xxx（留空跳过）", "TAVILY_API_KEY", false},
	{"SearXNG", "http://localhost:8080（留空跳过）", "SEARXNG_URL", true},
}

func (m wizardModel) updateWebSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			switch m.webSearchIdx {
			case 0:
				m.webBraveKey = val
			case 1:
				m.webTavilyKey = val
			case 2:
				m.webSearXNGURL = val
			}
			return m.advanceWebSearch()
		case "esc":
			return m.advanceWebSearch()
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m wizardModel) advanceWebSearch() (wizardModel, tea.Cmd) {
	m.webSearchIdx++
	if m.webSearchIdx >= len(webSearchLabels) {
		m.step = stepSummary
		m.selectIdx = 0
		return m, nil
	}
	ws := webSearchLabels[m.webSearchIdx]
	m.textInput.SetValue("")
	m.textInput.Placeholder = ws.placeholder
	if ws.isURL {
		m.textInput.EchoMode = textinput.EchoNormal
	} else {
		m.textInput.EchoMode = textinput.EchoPassword
	}
	m.textInput.Focus()
	return m, textinput.Blink
}

func (m *wizardModel) buildConfig() {
	providers := make(map[models.ModelProvider]config.Provider)
	for prov, key := range m.apiKeys {
		if key == "" {
			continue
		}
		p := config.Provider{APIKey: key}
		if bu, ok := m.baseURLs[prov]; ok {
			p.BaseURL = bu
		}
		providers[prov] = p
	}

	agents := map[config.AgentName]config.Agent{
		config.AgentCoder: {Model: string(m.coderModel)},
	}

	// Build web search config
	var webCfg config.WebConfig
	var backends []config.WebSearchBackendCfg

	if m.webSearXNGURL != "" {
		backends = append(backends, config.WebSearchBackendCfg{
			Name: "searxng", Enabled: true, Priority: 0, BaseURL: m.webSearXNGURL,
		})
	}
	if m.webBraveKey != "" {
		backends = append(backends, config.WebSearchBackendCfg{
			Name: "brave", Enabled: true, Priority: 1, APIKey: m.webBraveKey, MonthlyLimit: 1000,
		})
	}
	if m.webTavilyKey != "" {
		backends = append(backends, config.WebSearchBackendCfg{
			Name: "tavily", Enabled: true, Priority: 2, APIKey: m.webTavilyKey, MonthlyLimit: 1000,
		})
	}
	// Free HTML-scrape backends as fallback chain
	backends = append(backends, config.WebSearchBackendCfg{
		Name: "duckduckgo", Enabled: true, Priority: 5,
	})
	backends = append(backends, config.WebSearchBackendCfg{
		Name: "startpage", Enabled: true, Priority: 6,
	})
	backends = append(backends, config.WebSearchBackendCfg{
		Name: "bing", Enabled: true, Priority: 7,
	})
	// LLM-native search as final fallback
	backends = append(backends, config.WebSearchBackendCfg{
		Name: "anthropic", Enabled: true, Priority: 10, MaxUses: 1,
	})
	backends = append(backends, config.WebSearchBackendCfg{
		Name: "openai", Enabled: true, Priority: 11,
	})
	webCfg.SearchBackends = backends

	m.result = &config.Config{
		WorkingDir:      m.workingDir,
		Data:            config.DataConfig{Directory: ".openscholar"},
		Paths:           config.PathsConfig{Root: ".openscholar"},
		DefaultProvider: m.defaultProvider,
		Providers:       providers,
		Agents:          agents,
		Web:             webCfg,
	}
}

// View renders the wizard UI.
func (m wizardModel) View() string {
	var sb strings.Builder

	switch m.step {
	case stepWelcome:
		sb.WriteString(m.viewWelcome())
	case stepProviderKey:
		sb.WriteString(m.viewProviderKey())
	case stepBaseURL:
		sb.WriteString(m.viewBaseURL())
	case stepPing:
		sb.WriteString(m.viewPing())
	case stepDefault:
		sb.WriteString(m.viewDefault())
	case stepCoderModel:
		sb.WriteString(m.viewCoderModel())
	case stepWebSearch:
		sb.WriteString(m.viewWebSearch())
	case stepSummary:
		sb.WriteString(m.viewSummary())
	case stepDone:
		sb.WriteString(m.viewDone())
	}

	return sb.String()
}

func (m wizardModel) viewWelcome() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	subtitleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	return fmt.Sprintf(`
%s

%s

  OpenScholar 是一个 AI 驱动的学术论文写作助手。
  它可以帮助你：

  • 撰写和编辑 LaTeX 论文
  • 管理 BibTeX 参考文献
  • 检查符号一致性和格式规范
  • 投稿前全面检查

  接下来将引导你配置 AI 服务提供商的 API Key。
  你至少需要配置一个提供商才能开始使用。

%s
`,
		titleStyle.Render("  Welcome to OpenScholar! "),
		subtitleStyle.Render("  AI-Powered Academic Writing Assistant"),
		subtitleStyle.Render("  按 Enter 开始配置..."),
	)
}

func (m wizardModel) viewProviderKey() string {
	prov := m.providerOrder[m.providerIdx]
	name := models.ProviderDisplayName(prov)
	progress := fmt.Sprintf("(%d/%d)", m.providerIdx+1, len(m.providerOrder))

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Show status of previously entered keys
	var status strings.Builder
	for i := 0; i < m.providerIdx; i++ {
		p := m.providerOrder[i]
		if m.apiKeys[p] != "" {
			status.WriteString(fmt.Sprintf("  ✓ %s\n", models.ProviderDisplayName(p)))
		}
	}

	envHint := providerEnvHint(prov)

	return fmt.Sprintf(`
%s

%s
%s
  输入 %s 的 API Key（留空跳过，按 Enter 确认）：
  环境变量：%s

  %s

%s`,
		titleStyle.Render("  配置 API Key "+progress),
		status.String(),
		"",
		name,
		envHint,
		m.textInput.View(),
		dimStyle.Render("  Enter 确认  |  Esc 跳过"),
	)
}

func (m wizardModel) viewBaseURL() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	return fmt.Sprintf(`
%s

  输入 OpenAI Compatible 的 Base URL：

  %s

%s`,
		titleStyle.Render("  配置 Base URL"),
		m.textInput.View(),
		dimStyle.Render("  Enter 确认"),
	)
}

func (m wizardModel) viewPing() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n\n", titleStyle.Render("  验证 API Key")))
	sb.WriteString(fmt.Sprintf("  %s 正在验证...\n\n", m.spinner.View()))

	for _, p := range m.providerOrder {
		if m.apiKeys[p] == "" {
			continue
		}
		if r, ok := m.pingResults[p]; ok {
			if r.Valid {
				sb.WriteString(fmt.Sprintf("  ✓ %s (%dms)\n", models.ProviderDisplayName(p), r.Latency.Milliseconds()))
			} else {
				sb.WriteString(fmt.Sprintf("  ✗ %s: %s\n", models.ProviderDisplayName(p), r.Error))
			}
		} else {
			sb.WriteString(fmt.Sprintf("  ⋯ %s\n", models.ProviderDisplayName(p)))
		}
	}

	return sb.String()
}

func (m wizardModel) viewDefault() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n\n", titleStyle.Render("  选择默认 Provider")))

	for i, p := range m.validProviders {
		marker := "  "
		name := models.ProviderDisplayName(p)
		if i == m.selectIdx {
			marker = "→ "
			name = selectedStyle.Render(name)
		}
		sb.WriteString(fmt.Sprintf("  %s%s\n", marker, name))
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", dimStyle.Render("  ↑↓ 选择  |  Enter 确认")))
	return sb.String()
}

func (m wizardModel) viewCoderModel() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)

	provModels := modelsForProvider(m.defaultProvider)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n\n", titleStyle.Render("  选择默认模型")))
	sb.WriteString(fmt.Sprintf("  Provider: %s\n\n", models.ProviderDisplayName(m.defaultProvider)))

	for i, model := range provModels {
		marker := "  "
		name := fmt.Sprintf("%-25s %s", model.Name, model.ID)
		if i == m.selectIdx {
			marker = "→ "
			name = selectedStyle.Render(name)
		}
		sb.WriteString(fmt.Sprintf("  %s%s\n", marker, name))
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", dimStyle.Render("  ↑↓ 选择  |  Enter 确认")))
	return sb.String()
}

func (m wizardModel) viewWebSearch() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	ws := webSearchLabels[m.webSearchIdx]
	progress := fmt.Sprintf("(%d/%d)", m.webSearchIdx+1, len(webSearchLabels))

	// Show previously entered values
	var status strings.Builder
	if m.webSearchIdx > 0 && m.webBraveKey != "" {
		status.WriteString("  ✓ Brave Search\n")
	}
	if m.webSearchIdx > 1 && m.webTavilyKey != "" {
		status.WriteString("  ✓ Tavily\n")
	}

	return fmt.Sprintf(`
%s

%s

  联网搜索支持多源路由，优先使用免费搜索源，额度用完自动切换。
  默认已启用 DuckDuckGo（免费）和 LLM 原生搜索（兜底）。
  以下为可选的搜索 API，留空跳过：

%s  配置 %s %s
  环境变量：%s

  %s

%s`,
		titleStyle.Render("  联网搜索配置 "+progress),
		"",
		status.String(),
		ws.name,
		progress,
		ws.envHint,
		m.textInput.View(),
		dimStyle.Render("  Enter 确认  |  Esc 跳过"),
	)
}

func (m wizardModel) viewSummary() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n\n", titleStyle.Render("  配置摘要")))

	sb.WriteString("  已配置的 Provider:\n")
	for _, p := range m.providerOrder {
		if m.apiKeys[p] != "" {
			status := "✓"
			if r, ok := m.pingResults[p]; ok && !r.Valid {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("    %s %s\n", status, models.ProviderDisplayName(p)))
		}
	}

	sb.WriteString(fmt.Sprintf("\n  默认 Provider: %s\n", models.ProviderDisplayName(m.defaultProvider)))

	if model, ok := models.SupportedModels[m.coderModel]; ok {
		sb.WriteString(fmt.Sprintf("  默认模型: %s (%s)\n", model.Name, model.ID))
	}

	// Web search summary
	sb.WriteString("\n  联网搜索:\n")
	if m.webSearXNGURL != "" {
		sb.WriteString(fmt.Sprintf("    ✓ SearXNG (%s)\n", m.webSearXNGURL))
	}
	if m.webBraveKey != "" {
		sb.WriteString("    ✓ Brave Search (1000/月)\n")
	}
	if m.webTavilyKey != "" {
		sb.WriteString("    ✓ Tavily (1000/月)\n")
	}
	sb.WriteString("    ✓ DuckDuckGo / Startpage / Bing（免费 HTML 抓取兜底）\n")
	sb.WriteString("    ✓ LLM 原生搜索（最终兜底）\n")

	sb.WriteString(fmt.Sprintf("\n  配置将写入: %s\n", config.ConfigFilePath(m.workingDir)))

	sb.WriteString(fmt.Sprintf("\n%s\n", dimStyle.Render("  Enter/Y 确认保存  |  Esc/N 返回修改")))
	return sb.String()
}

func (m wizardModel) viewDone() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ✗ 配置失败: %s\n", m.err)
	}
	return "\n  ✓ 配置已保存！\n"
}

// Helpers

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

func providerEnvHint(p models.ModelProvider) string {
	switch p {
	case models.ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case models.ProviderOpenAI:
		return "OPENAI_API_KEY"
	case models.ProviderDeepSeek:
		return "DEEPSEEK_API_KEY"
	case models.ProviderMiniMax:
		return "MINIMAX_API_KEY"
	case models.ProviderGLM:
		return "GLM_API_KEY"
	case models.ProviderSiliconFlow:
		return "SILICONFLOW_API_KEY"
	case models.ProviderOpenAICompatible:
		return "OPENAI_COMPATIBLE_API_KEY"
	default:
		return ""
	}
}

func modelsForProvider(p models.ModelProvider) []models.Model {
	return models.OrderedModelsByProvider(p)
}
