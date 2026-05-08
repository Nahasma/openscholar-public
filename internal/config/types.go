package config

import (
	"strings"

	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/models"
)

type AgentName string

const (
	AgentCoder       AgentName = "coder"
	AgentSummarizer  AgentName = "summarizer"
	AgentTask        AgentName = "task"
	AgentTitle       AgentName = "title"
	AgentGeneral     AgentName = "general"
	AgentExplore     AgentName = "explore"
	AgentLeader      AgentName = "leader"
	AgentPlan        AgentName = "plan"
	AgentVerify      AgentName = "verify"
	AgentCoordinator AgentName = "coordinator"
)

type Agent struct {
	Provider  models.ModelProvider `json:"provider,omitempty"`
	Model     string               `json:"model,omitempty"`
	MaxTokens int64                `json:"maxTokens,omitempty"`
}

type Provider struct {
	APIKey   string                 `json:"apiKey,omitempty"`
	BaseURL  string                 `json:"baseURL,omitempty"`
	Profile  string                 `json:"profile,omitempty"`  // legacy | token-plan | custom
	AuthMode string                 `json:"authMode,omitempty"` // anthropic_x_api_key | bearer | auto
	Model    string                 `json:"model,omitempty"`
	Models   map[string]ModelConfig `json:"models,omitempty"`
	Kind     string                 `json:"kind,omitempty"`
	Disabled bool                   `json:"disabled,omitempty"`
}

type ModelConfig struct {
	Name              string `json:"name,omitempty"`
	ContextWindow     int64  `json:"contextWindow,omitempty"`
	DefaultMaxTokens  int64  `json:"defaultMaxTokens,omitempty"`
	SupportsTools     *bool  `json:"supportsTools,omitempty"`
	SupportsReasoning *bool  `json:"supportsReasoning,omitempty"`
	SupportsVision    *bool  `json:"supportsVision,omitempty"`
}

// PaperType represents the type of academic paper being written.
type PaperType string

const (
	PaperTypeSurvey   PaperType = "survey"
	PaperTypeResearch PaperType = "research"
	PaperTypePosition PaperType = "position"
	PaperTypeThesis   PaperType = "thesis"
)

// MCPServerConfig defines an external MCP server connection.
type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"` // stdio mode
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"` // SSE mode
	Env     []string `json:"env,omitempty"`
}

// CodeAgentConfig configures external AI coding agent preferences.
type CodeAgentConfig struct {
	Preferred      string                             `json:"preferred,omitempty"`
	TimeoutSeconds int                                `json:"timeout_seconds,omitempty"`
	Providers      map[string]CodeAgentProviderConfig `json:"providers,omitempty"`
}

// CodeAgentProviderConfig holds per-provider settings for code agent CLIs.
type CodeAgentProviderConfig struct {
	AllowedTools string `json:"allowed_tools,omitempty"`
	MaxTurns     int    `json:"max_turns,omitempty"`
}

// HarnessConfig Phase 8 (v3.5) Harness 工程升级配置。
// 所有字段都有合理默认值，未配置时保持保守行为。
type HarnessConfig struct {
	// Step 1: 上下文压缩
	MicroCompactEnabled         bool    `json:"micro_compact_enabled"`
	MicroCompactKeepRecent      int     `json:"micro_compact_keep_recent"`
	AutoCompactThresholdRatio   float64 `json:"auto_compact_threshold_ratio"`
	AutoCompactBufferTokens     int     `json:"auto_compact_buffer_tokens"`
	CompactBlockingBufferTokens int     `json:"compact_blocking_buffer_tokens"`

	// Step 2: Prompt 分层
	PromptTierEnabled bool `json:"prompt_tier_enabled"`

	// Step 3: 异步工具
	AsyncToolsEnabled bool     `json:"async_tools_enabled"`
	AsyncToolsList    []string `json:"async_tools_list"`

	// Step 4: Lint 守护
	LintGuardEnabled bool `json:"lint_guard_enabled"`
	LintGuardLatex   bool `json:"lint_guard_latex"`
	LintGuardGo      bool `json:"lint_guard_go"`

	// Step 7: Worktree 隔离
	WorktreeEnabled bool `json:"worktree_enabled"`

	// Step 8: 事件审计日志
	EventJournalEnabled bool `json:"event_journal_enabled"`
}

// DomainProfile captures discipline-specific parameters for experiment orchestration.
type DomainProfile struct {
	Name               string             `json:"name,omitempty"`
	Languages          []string           `json:"languages,omitempty"`
	Deliverables       []string           `json:"deliverables,omitempty"`
	AcceptCriteria     []string           `json:"accept_criteria,omitempty"`
	MetricArtifactPath string             `json:"metric_artifact_path,omitempty"`
	ReportArtifactPath string             `json:"report_artifact_path,omitempty"`
	MetricExample      string             `json:"metric_example,omitempty"`
	NodeProgression    []string           `json:"node_progression,omitempty"`
	RootNodeType       string             `json:"root_node_type,omitempty"`
	Stages             []StageDescription `json:"stages,omitempty"`
}

// StageDescription defines a single stage in the tree search experiment flow.
type StageDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ExperimentConfig Phase 9 实验编排配置
type ExperimentConfig struct {
	OrchestratorEnabled  bool           `json:"orchestrator_enabled"`
	StructuredFirst      bool           `json:"structured_first"`
	DefaultTimeoutSec    int            `json:"default_timeout_sec"`
	MaxDebugRetries      int            `json:"max_debug_retries"`
	MaxAgentReplans      int            `json:"max_agent_replans"`
	StreamMonitorEnabled bool           `json:"stream_monitor_enabled"`
	IdleTimeoutSec       int            `json:"idle_timeout_sec"`
	WorkspaceVersion     string         `json:"workspace_version"`
	Domain               string         `json:"domain,omitempty"`
	DomainOverride       *DomainProfile `json:"domain_override,omitempty"`
}

type SubagentOrchestrationConfig struct {
	Enabled               bool                             `json:"enabled,omitempty"`
	GlobalMaxConcurrent   int                              `json:"global_max_concurrent,omitempty"`
	MaxNestedDepth        int                              `json:"max_nested_depth,omitempty"`
	DefaultResultMaxChars int                              `json:"default_result_max_chars,omitempty"`
	NotificationMaxChars  int                              `json:"notification_max_chars,omitempty"`
	Profiles              map[string]SubagentProfileConfig `json:"profiles,omitempty"`
}

type SubagentProfileConfig struct {
	AgentType         string   `json:"agent_type,omitempty"`
	AgentName         string   `json:"agent_name,omitempty"`
	WhenToUse         string   `json:"when_to_use,omitempty"`
	WhenNotToUse      string   `json:"when_not_to_use,omitempty"`
	ModelTier         string   `json:"model_tier,omitempty"`
	Model             string   `json:"model,omitempty"`
	AllowedTools      []string `json:"allowed_tools,omitempty"`
	DeniedTools       []string `json:"denied_tools,omitempty"`
	PermissionMode    string   `json:"permission_mode,omitempty"`
	MaxConcurrent     int      `json:"max_concurrent,omitempty"`
	DefaultBackground bool     `json:"default_background,omitempty"`
	CanSpawnTask      bool     `json:"can_spawn_task,omitempty"`
	CanWriteFiles     bool     `json:"can_write_files,omitempty"`
	RequireWriteSet   bool     `json:"require_write_set,omitempty"`
	VerifyPolicy      string   `json:"verify_policy,omitempty"`
	ResultMaxChars    int      `json:"result_max_chars,omitempty"`
	TimeoutSeconds    int      `json:"timeout_seconds,omitempty"`
	MaxTurns          int      `json:"max_turns,omitempty"`
}

type Config struct {
	WorkingDir            string                            `json:"wd,omitempty"`
	Data                  DataConfig                        `json:"data"`
	Paths                 PathsConfig                       `json:"paths,omitempty"`
	DefaultProvider       models.ModelProvider              `json:"defaultProvider,omitempty"`
	ImageProvider         string                            `json:"imageProvider,omitempty"`
	Providers             map[models.ModelProvider]Provider `json:"providers,omitempty"`
	Agents                map[AgentName]Agent               `json:"agents,omitempty"`
	Debug                 bool                              `json:"debug,omitempty"`
	SessionLog            bool                              `json:"sessionLog,omitempty"`
	PaperType             PaperType                         `json:"paper_type,omitempty"`
	OllamaBaseURL         string                            `json:"ollama_base_url,omitempty"`
	VLLMBaseURL           string                            `json:"vllm_base_url,omitempty"`
	MCPServers            []MCPServerConfig                 `json:"mcp_servers,omitempty"`
	CodeAgent             *CodeAgentConfig                  `json:"code_agent,omitempty"`
	Harness               HarnessConfig                     `json:"harness,omitempty"`
	Experiment            ExperimentConfig                  `json:"experiment,omitempty"`
	SubagentOrchestration SubagentOrchestrationConfig       `json:"subagent_orchestration,omitempty"`
	Scholar               ScholarConfig                     `json:"scholar,omitempty"`
	Hooks                 []hooks.HookConfig                `json:"hooks,omitempty"`
	Web                   WebConfig                         `json:"web,omitempty"`
}

// WebConfig holds web search and fetch configuration.
type WebConfig struct {
	SearchMaxUses        int                   `json:"searchMaxUses,omitempty"`
	FetchCacheTTLMinutes int                   `json:"fetchCacheTtlMinutes,omitempty"`
	FetchMaxResponseMB   int                   `json:"fetchMaxResponseMB,omitempty"`
	FetchMaxMarkdownLen  int                   `json:"fetchMaxMarkdownChars,omitempty"`
	OpenAISearchModel    string                `json:"openaiSearchModel,omitempty"`
	SearchBackends       []WebSearchBackendCfg `json:"searchBackends,omitempty"`
	URLPolicy            WebURLPolicyConfig    `json:"urlPolicy,omitempty"`
	WebsitePolicy        WebWebsitePolicy      `json:"websitePolicy,omitempty"`
	Proxy                WebProxyConfig        `json:"proxy,omitempty"`
}

type WebProxyConfig struct {
	Mode            string   `json:"mode,omitempty"`
	URL             string   `json:"url,omitempty"`
	FakeIPCIDRs     []string `json:"fakeIPCIDRs,omitempty"`
	AllowLocalProxy *bool    `json:"allowLocalProxy,omitempty"`
}

type WebURLPolicyConfig struct {
	DNSFailMode            string `json:"dnsFailMode,omitempty"`
	BlockMetadataServices  *bool  `json:"blockMetadataServices,omitempty"`
	BlockUserinfo          *bool  `json:"blockUserinfo,omitempty"`
	BlockURLSecrets        *bool  `json:"blockURLSecrets,omitempty"`
	SkipCacheWhenHasSecret *bool  `json:"skipCacheWhenHasSecret,omitempty"`
}

type WebWebsitePolicy struct {
	Mode          string                 `json:"mode,omitempty"`
	DefaultAction string                 `json:"defaultAction,omitempty"`
	Rules         []WebWebsitePolicyRule `json:"rules,omitempty"`
}

type WebWebsitePolicyRule struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
}

// WebSearchBackendCfg describes a single search backend configuration.
type WebSearchBackendCfg struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	Priority     int    `json:"priority"`
	APIKey       string `json:"apiKey,omitempty"`
	BaseURL      string `json:"baseURL,omitempty"`
	MonthlyLimit int    `json:"monthlyLimit,omitempty"`
	DailyLimit   int    `json:"dailyLimit,omitempty"`
	MaxUses      int    `json:"maxUses,omitempty"`
}

// ScholarConfig holds API keys and settings for academic search providers.
type ScholarConfig struct {
	ContactEmail       string `json:"contactEmail,omitempty"`
	OpenAlexAPIKey     string `json:"openAlexApiKey,omitempty"`
	NCBIAPIKey         string `json:"ncbiApiKey,omitempty"`
	CoreAPIKey         string `json:"coreApiKey,omitempty"`
	PatentsViewAPIKey  string `json:"patentsViewApiKey,omitempty"`
	SemanticScholarKey string `json:"semanticScholarApiKey,omitempty"`
}

type DataConfig struct {
	Directory string `json:"directory,omitempty"`
}

type PathsConfig struct {
	Root       string `json:"root,omitempty"`
	Config     string `json:"config,omitempty"`
	State      string `json:"state,omitempty"`
	Extensions string `json:"extensions,omitempty"`
	Cache      string `json:"cache,omitempty"`
	Logs       string `json:"logs,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
}

const (
	defaultDataDirectory = ".openscholar"
	configRelativePath   = ".openscholar/config.json"
)

// GetPaperType returns the configured paper type, defaulting to "research".
func (c *Config) GetPaperType() PaperType {
	if c.PaperType == "" {
		return PaperTypeResearch
	}
	return c.PaperType
}

// ParseAgentName converts a string to an AgentName, returning false if invalid.
func ParseAgentName(s string) (AgentName, bool) {
	switch AgentName(strings.ToLower(s)) {
	case AgentCoder:
		return AgentCoder, true
	case AgentSummarizer:
		return AgentSummarizer, true
	case AgentTask:
		return AgentTask, true
	case AgentTitle:
		return AgentTitle, true
	case AgentGeneral:
		return AgentGeneral, true
	case AgentExplore:
		return AgentExplore, true
	case AgentLeader:
		return AgentLeader, true
	case AgentPlan:
		return AgentPlan, true
	case AgentVerify:
		return AgentVerify, true
	case AgentCoordinator:
		return AgentCoordinator, true
	}
	return "", false
}
