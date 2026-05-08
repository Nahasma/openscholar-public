// Package web provides the Web Runtime layer for WebSearch and WebFetch tools.
// It abstracts provider-specific search backends and local fetch pipelines,
// keeping the tools package free of provider imports.
package web

import (
	"context"
	"fmt"
	"time"
)

// --- Search types ---

// SearchProvider abstracts server-side web search capability.
// Anthropic uses web_search_20250305 server tool; OpenAI uses web_search_options.
type SearchProvider interface {
	// SearchWeb executes a web search query and returns structured results.
	SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
	// SupportsFilter reports whether this backend supports domain filtering.
	SupportsFilter() bool
}

// SearchOptions configures a web search request.
type SearchOptions struct {
	AllowedDomains []string // only include results from these domains
	BlockedDomains []string // never include results from these domains
	MaxUses        int      // max number of searches (default 8)
}

// SearchHit is a single search result with title and URL.
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"` // 搜索结果摘要片段
}

// SearchResult holds the complete web search output.
type SearchResult struct {
	Hits     []SearchHit       `json:"hits"`
	Summary  string            `json:"summary"`
	Duration float64           `json:"duration_seconds"`
	Backend  string            `json:"backend"`            // "anthropic" / "openai" / "brave" / "tavily" / "searxng" / "duckduckgo"
	Attempts []ProviderAttempt `json:"attempts,omitempty"` // 路由尝试记录
	Policy   PolicyMetadata    `json:"policy,omitempty"`
}

// --- Fetch types ---

// FetchResult holds the output of a successful URL fetch.
type FetchResult struct {
	Content     string         `json:"content"`      // processed content (markdown or summary)
	Bytes       int            `json:"bytes"`        // raw response size
	Code        int            `json:"code"`         // HTTP status code
	CodeText    string         `json:"code_text"`    // HTTP status text
	ContentType string         `json:"content_type"` // response content-type
	Duration    float64        `json:"duration_seconds"`
	URL         string         `json:"url"`
	CacheHit    bool           `json:"cache_hit"`
	Policy      PolicyMetadata `json:"policy,omitempty"`
}

type FetchErrorKind string

const (
	FetchErrHTTPStatus FetchErrorKind = "http_status"
	FetchErrNotFound   FetchErrorKind = "not_found"
	FetchErrForbidden  FetchErrorKind = "forbidden"
	FetchErrServer     FetchErrorKind = "server_error"
	FetchErrTooLarge   FetchErrorKind = "too_large"
	FetchErrTimeout    FetchErrorKind = "timeout"
	FetchErrTLS        FetchErrorKind = "tls_error"
	FetchErrEOF        FetchErrorKind = "eof"
	FetchErrRead       FetchErrorKind = "read_error"
	FetchErrValidation FetchErrorKind = "validation_failed"
)

type FetchError struct {
	Kind        FetchErrorKind
	Message     string
	StatusCode  int
	Provider    string
	Source      string
	Recoverable bool
	Cause       error
}

func (e *FetchError) Error() string {
	if e == nil {
		return "fetch failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return fmt.Sprintf("fetch failed [%s]: %v", e.Kind, e.Cause)
	}
	return fmt.Sprintf("fetch failed [%s]", e.Kind)
}

func (e *FetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RedirectResult is returned when a cross-host redirect is detected.
// The caller should fetch the redirect URL with a new WebFetch call.
type RedirectResult struct {
	OriginalURL string `json:"original_url"`
	RedirectURL string `json:"redirect_url"`
	StatusCode  int    `json:"status_code"`
}

// --- LLM Caller ---

// LLMCaller is a function type for making simple LLM calls.
// Used by WebFetch for content summarization with a small/fast model.
type LLMCaller func(ctx context.Context, prompt string) (string, error)

// --- Cache entry ---

// CacheEntry stores fetched content for the URL cache.
type CacheEntry struct {
	Content     string
	ContentType string
	CodeText    string
	Bytes       int
	Code        int
	Size        int // byte size for LRU eviction tracking
	CreatedAt   time.Time
}

// --- Configuration ---

// Config holds web tool configuration with sensible defaults.
type Config struct {
	SearchMaxUses        int                   `json:"searchMaxUses"`            // default 8
	FetchCacheTTLMinutes int                   `json:"fetchCacheTtlMinutes"`     // default 15
	FetchMaxResponseMB   int                   `json:"fetchMaxResponseMB"`       // default 10
	FetchMaxMarkdownLen  int                   `json:"fetchMaxMarkdownChars"`    // default 100000
	OpenAISearchModel    string                `json:"openaiSearchModel"`        // default "gpt-4o-search-preview"
	SearchBackends       []SearchBackendConfig `json:"searchBackends,omitempty"` // 多搜索源配置
	URLPolicy            URLPolicyConfig       `json:"urlPolicy,omitempty"`
	WebsitePolicy        WebsitePolicyConfig   `json:"websitePolicy,omitempty"`
	Proxy                ProxyConfig           `json:"proxy,omitempty"`
}

type URLPolicyConfig struct {
	DNSFailMode            string `json:"dnsFailMode,omitempty"`
	BlockMetadataServices  bool   `json:"blockMetadataServices,omitempty"`
	BlockUserinfo          bool   `json:"blockUserinfo,omitempty"`
	BlockURLSecrets        bool   `json:"blockURLSecrets,omitempty"`
	SkipCacheWhenHasSecret bool   `json:"skipCacheWhenHasSecret,omitempty"`
}

type WebsitePolicyConfig struct {
	Mode          string              `json:"mode,omitempty"`
	DefaultAction string              `json:"defaultAction,omitempty"`
	Rules         []WebsitePolicyRule `json:"rules,omitempty"`
}

type WebsitePolicyRule struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
}

type ProxyConfig struct {
	Mode            string   `json:"mode,omitempty"` // environment | direct | explicit
	URL             string   `json:"url,omitempty"`
	FakeIPCIDRs     []string `json:"fakeIPCIDRs,omitempty"`
	AllowLocalProxy bool     `json:"allowLocalProxy,omitempty"`
}

// SearchBackendConfig 描述单个搜索源的配置。
type SearchBackendConfig struct {
	Name         string `json:"name"`         // "searxng", "brave", "tavily", "duckduckgo"
	Enabled      bool   `json:"enabled"`      // 是否启用
	Priority     int    `json:"priority"`     // 越小越优先
	APIKey       string `json:"apiKey"`       // 空 = 从环境变量读取
	BaseURL      string `json:"baseURL"`      // SearXNG 实例地址
	MonthlyLimit int    `json:"monthlyLimit"` // 0 = 无限
	DailyLimit   int    `json:"dailyLimit"`   // 0 = 无日限
	MaxUses      int    `json:"maxUses"`      // Anthropic: 单次搜索内部最大搜索数
}

// DefaultConfig returns configuration with CC-aligned defaults.
func DefaultConfig() Config {
	return Config{
		SearchMaxUses:        8,
		FetchCacheTTLMinutes: 15,
		FetchMaxResponseMB:   10,
		FetchMaxMarkdownLen:  100_000,
		OpenAISearchModel:    "gpt-4o-search-preview",
		URLPolicy: URLPolicyConfig{
			DNSFailMode:            "closed",
			BlockMetadataServices:  true,
			BlockUserinfo:          true,
			BlockURLSecrets:        true,
			SkipCacheWhenHasSecret: true,
		},
		WebsitePolicy: WebsitePolicyConfig{
			Mode:          "blocklist",
			DefaultAction: "allow",
		},
		Proxy: ProxyConfig{
			Mode:            "environment",
			FakeIPCIDRs:     []string{"198.18.0.0/15"},
			AllowLocalProxy: true,
		},
	}
}

// --- Runtime ---

// Runtime is the unified Web tool backend.
// Tools (WebSearch/WebFetch) call into Runtime; Runtime delegates to
// provider-specific search backends and the local fetch pipeline.
type Runtime struct {
	search        SearchProvider
	callLLM       LLMCaller
	config        Config
	cache         *Cache // initialised by NewRuntime
	urlPolicy     *URLPolicy
	websitePolicy *WebsitePolicy
}

// NewRuntime creates a Web Runtime. search may be nil (no search provider).
func NewRuntime(search SearchProvider, callLLM LLMCaller, cfg Config) *Runtime {
	def := DefaultConfig()
	if cfg.SearchMaxUses == 0 {
		cfg.SearchMaxUses = def.SearchMaxUses
	}
	if cfg.FetchCacheTTLMinutes == 0 {
		cfg.FetchCacheTTLMinutes = def.FetchCacheTTLMinutes
	}
	if cfg.FetchMaxResponseMB == 0 {
		cfg.FetchMaxResponseMB = def.FetchMaxResponseMB
	}
	if cfg.FetchMaxMarkdownLen == 0 {
		cfg.FetchMaxMarkdownLen = def.FetchMaxMarkdownLen
	}
	if cfg.OpenAISearchModel == "" {
		cfg.OpenAISearchModel = def.OpenAISearchModel
	}
	if cfg.URLPolicy.DNSFailMode == "" {
		cfg.URLPolicy = def.URLPolicy
	}
	if cfg.WebsitePolicy.Mode == "" {
		cfg.WebsitePolicy = def.WebsitePolicy
	}
	if cfg.Proxy.Mode == "" {
		cfg.Proxy.Mode = def.Proxy.Mode
	}
	if len(cfg.Proxy.FakeIPCIDRs) == 0 {
		cfg.Proxy.FakeIPCIDRs = append([]string(nil), def.Proxy.FakeIPCIDRs...)
	}
	ttl := time.Duration(cfg.FetchCacheTTLMinutes) * time.Minute
	maxSize := cfg.FetchMaxResponseMB * 1024 * 1024

	return &Runtime{
		search:        search,
		callLLM:       callLLM,
		config:        cfg,
		cache:         NewCache(maxSize, ttl),
		urlPolicy:     NewURLPolicy(cfg.URLPolicy),
		websitePolicy: NewWebsitePolicy(cfg.WebsitePolicy),
	}
}

// HasSearchProvider returns true if a search backend is available.
func (r *Runtime) HasSearchProvider() bool {
	return r.search != nil
}

// SearchProvider returns the underlying search provider (may be nil).
func (r *Runtime) GetSearchProvider() SearchProvider {
	return r.search
}

// Config returns the runtime configuration.
func (r *Runtime) GetConfig() Config {
	return r.config
}

// Cache returns the fetch cache.
func (r *Runtime) GetCache() *Cache {
	return r.cache
}

// CallLLM returns the LLM caller for content summarization.
func (r *Runtime) GetLLMCaller() LLMCaller {
	return r.callLLM
}

// SetLLMCaller updates the runtime summarizer caller (used on provider reload/model switch).
func (r *Runtime) SetLLMCaller(callLLM LLMCaller) {
	r.callLLM = callLLM
}

// SetSearchProvider updates the runtime search provider (used on provider reload/model switch).
func (r *Runtime) SetSearchProvider(search SearchProvider) {
	r.search = search
}
