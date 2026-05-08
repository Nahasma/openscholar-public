package web

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sort"
	"time"
)

// SearchRouter 包装多个 SearchProvider backend，按优先级路由。
// 实现 SearchProvider 接口，对 Runtime 完全透明。
type SearchRouter struct {
	backends []rankedBackend
	state    *SearchState
}

type rankedBackend struct {
	Name         string
	Provider     SearchProvider
	Quota        QuotaConfig
	Priority     int
	NativeFilter bool // true=Anthropic/Tavily; false=需要 rewrite+post-filter
	CanRewrite   bool // true=Brave/SearXNG/DDG; false=OpenAI
	MaxUses      int  // 覆盖 opts.MaxUses（如 Anthropic maxUses=1），0=不覆盖
}

// SearchWeb 实现 SearchProvider 接口。路由逻辑:
//
//  1. 按 priority 遍历 backends
//  2. 跳过 cooldown / quota exhausted 的
//  3. domain filter 处理:
//     - NativeFilter=true → 直接传 opts
//     - CanRewrite=true → query rewrite + 清空 opts 的 filter + post-filter
//     - 都不支持且有 filter → skip
//  4. 调用 backend.SearchWeb()
//  5. 成功 → RecordSuccess → 返回; 失败 → SetCooldown → 下一个
//  6. post-filter 后结果为空 → 视为 no_results → 下一个
func (r *SearchRouter) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	hasFilter := len(opts.AllowedDomains) > 0 || len(opts.BlockedDomains) > 0
	var attempts []ProviderAttempt
	var lastErr error

	for _, b := range r.backends {
		// Skip cooldown
		if r.state.InCooldown(b.Name) {
			slog.Debug("search router: skipping backend (cooldown)", "backend", b.Name)
			attempts = append(attempts, ProviderAttempt{
				Name:     b.Name,
				Success:  false,
				Duration: 0,
				Error:    "skipped: backend in cooldown",
			})
			continue
		}

		// Atomically reserve quota (check + increment in one step)
		if !r.state.Reserve(b.Name, b.Quota) {
			slog.Debug("search router: skipping backend (quota)", "backend", b.Name)
			attempts = append(attempts, ProviderAttempt{
				Name:     b.Name,
				Success:  false,
				Duration: 0,
				Error:    "skipped: quota exhausted",
			})
			continue
		}

		// Determine filter strategy
		actualQuery := query
		actualOpts := opts
		needPostFilter := false

		// Apply per-backend MaxUses override (e.g. Anthropic maxUses=1)
		if b.MaxUses > 0 {
			actualOpts.MaxUses = b.MaxUses
		}

		if hasFilter {
			if b.NativeFilter {
				// Native filter: pass opts as-is
			} else if b.CanRewrite {
				// Query rewrite + post-filter
				actualQuery = RewriteQueryWithFilters(query, opts.AllowedDomains, opts.BlockedDomains)
				actualOpts = SearchOptions{MaxUses: actualOpts.MaxUses} // clear domain filters
				needPostFilter = true
			} else {
				// Cannot handle filters → skip
				slog.Debug("search router: skipping backend (no filter support)", "backend", b.Name)
				attempts = append(attempts, ProviderAttempt{
					Name:     b.Name,
					Success:  false,
					Duration: 0,
					Error:    "skipped: filter unsupported",
				})
				continue
			}
		}

		start := time.Now()
		result, err := b.Provider.SearchWeb(ctx, actualQuery, actualOpts)
		duration := time.Since(start).Seconds()

		if err != nil {
			attempt := ProviderAttempt{
				Name:     b.Name,
				Success:  false,
				Duration: duration,
				Error:    attemptErrorSummary(err),
			}
			attempts = append(attempts, attempt)
			lastErr = err

			// Handle error: set cooldown only for provider-health errors
			var searchErr *SearchError
			if errors.As(err, &searchErr) {
				switch searchErr.Kind {
				case SearchErrNoResults:
					// no_results is query-specific, not provider health — rotate but no cooldown
					slog.Debug("search router: no results, trying next",
						"backend", b.Name)
					continue
				case SearchErrRateLimited, SearchErrTemporary, SearchErrQuotaExhausted:
					// Provider-health issues — set cooldown + persist
					cooldown := 30 * time.Second
					if searchErr.RetryAfter > 0 {
						cooldown = searchErr.RetryAfter
					}
					r.state.SetCooldown(b.Name, cooldown)
					_ = r.state.Save() // persist cooldown to survive restart
					slog.Debug("search router: backend failed, rotating",
						"backend", b.Name, "error", searchErr.Kind, "cooldown", cooldown)
					continue
				case SearchErrAuth:
					// Auth errors: set long cooldown to avoid repeated 401 requests + persist
					r.state.SetCooldown(b.Name, 10*time.Minute)
					_ = r.state.Save()
					slog.Warn("search router: backend auth error, disabling temporarily",
						"backend", b.Name)
					continue
				}
			}
			continue
		}

		// Post-filter if needed
		if needPostFilter && result != nil && len(result.Hits) > 0 {
			result.Hits = PostFilterHits(result.Hits, opts.AllowedDomains, opts.BlockedDomains)
			if len(result.Hits) == 0 {
				// Post-filter removed everything → try next
				lastErr = &SearchError{
					Provider: b.Name,
					Kind:     SearchErrNoResults,
					Cause:    errors.New("post-filter removed all results"),
				}
				attempts = append(attempts, ProviderAttempt{
					Name:     b.Name,
					Success:  false,
					Duration: duration,
					Error:    "post-filter removed all results",
				})
				continue
			}
		}

		// Success
		r.state.RecordSuccess(b.Name)
		_ = r.state.Save() // best-effort persist

		attempts = append(attempts, ProviderAttempt{
			Name:     b.Name,
			Success:  true,
			Duration: duration,
		})

		if result != nil {
			result.Attempts = attempts
		}
		return result, nil
	}

	// All backends failed
	msg := "all search backends failed"
	if lastErr != nil {
		return nil, &SearchFailureError{
			Message:  msg,
			Attempts: attempts,
			Cause:    lastErr,
		}
	}
	return nil, &SearchFailureError{
		Message:  "no search backend available",
		Attempts: attempts,
	}
}

func attemptErrorSummary(err error) string {
	var searchErr *SearchError
	if errors.As(err, &searchErr) {
		return string(searchErr.Kind)
	}
	return "search request failed"
}

// SupportsFilter 只要任何一个 backend 支持 filter 就返回 true。
func (r *SearchRouter) SupportsFilter() bool {
	for _, b := range r.backends {
		if b.NativeFilter || b.CanRewrite {
			return true
		}
	}
	return false
}

// Stats 返回所有 backend 的用量统计。
func (r *SearchRouter) Stats() map[string]ProviderState {
	return r.state.Stats()
}

// BuildSearchRouter 根据配置和环境变量构建 SearchRouter。
//
// 构建流程:
//  1. 读取 Config.SearchBackends 用户配置
//  2. 如果用户没配置任何 backend → 自动发现环境变量
//  3. Anthropic/OpenAI 原生搜索作为最低优先级兜底
//  4. 按 priority 排序
//  5. 加载持久化状态
func BuildSearchRouter(
	cfg Config,
	anthropicBackend SearchProvider,
	openaiBackend SearchProvider,
	statePath string,
) *SearchRouter {
	state := NewSearchState(statePath)
	_ = state.Load() // best-effort load

	var backends []rankedBackend

	// Track explicitly disabled backends so we don't re-add them as fallbacks.
	explicitlyDisabled := make(map[string]bool)

	if len(cfg.SearchBackends) > 0 {
		// Record which names the user explicitly disabled
		for _, c := range cfg.SearchBackends {
			if !c.Enabled {
				explicitlyDisabled[c.Name] = true
			}
		}
		// User-configured backends
		backends = buildFromConfig(cfg.SearchBackends)
		// Inject providers into config-declared LLM backend entries
		for i := range backends {
			switch backends[i].Name {
			case "anthropic":
				backends[i].Provider = anthropicBackend
			case "openai":
				backends[i].Provider = openaiBackend
			}
		}
		// Remove entries where provider is nil (configured but not available)
		backends = filterAvailable(backends)
	} else {
		// Auto-discover from environment variables
		backends = autoDiscoverBackends()
	}

	// Add LLM-native backends as low-priority fallbacks.
	// Only add if not already configured via SearchBackends and not explicitly disabled.
	anthropicConfigured := hasBackendNamed(backends, "anthropic")
	openaiConfigured := hasBackendNamed(backends, "openai")

	if anthropicBackend != nil && !anthropicConfigured && !explicitlyDisabled["anthropic"] {
		backends = append(backends, rankedBackend{
			Name:         "anthropic",
			Provider:     anthropicBackend,
			Quota:        QuotaConfig{},
			Priority:     10,
			NativeFilter: true,
			MaxUses:      1, // ADR-4: restrict Anthropic fallback cost
		})
	}

	if openaiBackend != nil && !openaiConfigured && !explicitlyDisabled["openai"] {
		backends = append(backends, rankedBackend{
			Name:     "openai",
			Provider: openaiBackend,
			Quota:    QuotaConfig{},
			Priority: 11,
		})
	}

	applyPolicyHTTPClients(backends, cfg)

	// Sort by priority
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Priority < backends[j].Priority
	})

	return &SearchRouter{
		backends: backends,
		state:    state,
	}
}

// buildFromConfig 根据用户配置创建 backends。
func buildFromConfig(cfgs []SearchBackendConfig) []rankedBackend {
	var backends []rankedBackend
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		b, ok := createBackendFromConfig(cfg)
		if ok {
			backends = append(backends, b)
		}
	}
	return backends
}

// createBackendFromConfig 根据单个配置创建 backend。
func createBackendFromConfig(cfg SearchBackendConfig) (rankedBackend, bool) {
	quota := QuotaConfig{
		MonthlyLimit: cfg.MonthlyLimit,
		DailyLimit:   cfg.DailyLimit,
	}

	switch cfg.Name {
	case "searxng":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("SEARXNG_URL")
		}
		if baseURL == "" {
			return rankedBackend{}, false
		}
		return rankedBackend{
			Name:       "searxng",
			Provider:   NewSearXNGSearch(baseURL),
			Quota:      quota,
			Priority:   cfg.Priority,
			CanRewrite: true,
		}, true

	case "brave":
		key := resolveKey(cfg)
		if key == "" {
			return rankedBackend{}, false
		}
		return rankedBackend{
			Name:       "brave",
			Provider:   NewBraveSearch(key),
			Quota:      quota,
			Priority:   cfg.Priority,
			CanRewrite: true,
		}, true

	case "tavily":
		key := resolveKey(cfg)
		if key == "" {
			return rankedBackend{}, false
		}
		return rankedBackend{
			Name:         "tavily",
			Provider:     NewTavilySearch(key),
			Quota:        quota,
			Priority:     cfg.Priority,
			NativeFilter: true,
		}, true

	case "duckduckgo":
		return rankedBackend{
			Name:       "duckduckgo",
			Provider:   NewDuckDuckGoSearch(),
			Quota:      quota,
			Priority:   cfg.Priority,
			CanRewrite: true,
		}, true

	case "startpage":
		return rankedBackend{
			Name:       "startpage",
			Provider:   NewStartpageSearch(),
			Quota:      quota,
			Priority:   cfg.Priority,
			CanRewrite: true,
		}, true

	case "bing":
		return rankedBackend{
			Name:       "bing",
			Provider:   NewBingSearch(),
			Quota:      quota,
			Priority:   cfg.Priority,
			CanRewrite: true,
		}, true

	case "anthropic":
		// Anthropic/OpenAI backends are injected externally via BuildSearchRouter;
		// config entry only controls priority/maxUses/enabled.
		// Return a placeholder — BuildSearchRouter will match by name and apply settings.
		return rankedBackend{
			Name:         "anthropic",
			Priority:     cfg.Priority,
			NativeFilter: true,
			MaxUses:      cfg.MaxUses,
			Quota:        quota,
			// Provider set to nil — filled later by BuildSearchRouter
		}, true

	case "openai":
		return rankedBackend{
			Name:     "openai",
			Priority: cfg.Priority,
			Quota:    quota,
			// Provider set to nil — filled later by BuildSearchRouter
		}, true

	default:
		slog.Warn("search router: unknown backend", "name", cfg.Name)
		return rankedBackend{}, false
	}
}

// filterAvailable removes backends with nil Provider.
func filterAvailable(backends []rankedBackend) []rankedBackend {
	result := make([]rankedBackend, 0, len(backends))
	for _, b := range backends {
		if b.Provider != nil {
			result = append(result, b)
		}
	}
	return result
}

// hasBackendNamed checks if any backend in the list has the given name.
func hasBackendNamed(backends []rankedBackend, name string) bool {
	for _, b := range backends {
		if b.Name == name {
			return true
		}
	}
	return false
}

// autoDiscoverBackends 从环境变量自动发现可用的搜索 backend。
func autoDiscoverBackends() []rankedBackend {
	var backends []rankedBackend

	// SearXNG (priority 0)
	if searxngURL := os.Getenv("SEARXNG_URL"); searxngURL != "" {
		backends = append(backends, rankedBackend{
			Name:       "searxng",
			Provider:   NewSearXNGSearch(searxngURL),
			Priority:   0,
			CanRewrite: true,
		})
	}

	// Brave (priority 1, 1000/month)
	if braveKey := os.Getenv("BRAVE_API_KEY"); braveKey != "" {
		backends = append(backends, rankedBackend{
			Name:       "brave",
			Provider:   NewBraveSearch(braveKey),
			Quota:      QuotaConfig{MonthlyLimit: 1000},
			Priority:   1,
			CanRewrite: true,
		})
	}

	// Tavily (priority 2, 1000/month)
	if tavilyKey := os.Getenv("TAVILY_API_KEY"); tavilyKey != "" {
		backends = append(backends, rankedBackend{
			Name:         "tavily",
			Provider:     NewTavilySearch(tavilyKey),
			Quota:        QuotaConfig{MonthlyLimit: 1000},
			Priority:     2,
			NativeFilter: true,
		})
	}

	// DuckDuckGo (always available, priority 5)
	backends = append(backends, rankedBackend{
		Name:       "duckduckgo",
		Provider:   NewDuckDuckGoSearch(),
		Priority:   5,
		CanRewrite: true,
	})

	// Startpage — Google results via privacy proxy (priority 6)
	backends = append(backends, rankedBackend{
		Name:       "startpage",
		Provider:   NewStartpageSearch(),
		Priority:   6,
		CanRewrite: true,
	})

	// Bing — free HTML scrape fallback (priority 7)
	backends = append(backends, rankedBackend{
		Name:       "bing",
		Provider:   NewBingSearch(),
		Priority:   7,
		CanRewrite: true,
	})

	return backends
}

// resolveKey 解析 API key：配置文件优先，回退到环境变量。
func resolveKey(cfg SearchBackendConfig) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	switch cfg.Name {
	case "brave":
		return os.Getenv("BRAVE_API_KEY")
	case "tavily":
		return os.Getenv("TAVILY_API_KEY")
	}
	return ""
}

func applyPolicyHTTPClients(backends []rankedBackend, cfg Config) {
	for _, b := range backends {
		switch p := b.Provider.(type) {
		case *SearXNGBackend:
			p.client = newPolicyHTTPClient(5*time.Second, cfg, PurposeSearch)
		case *BraveSearchBackend:
			p.client = newPolicyHTTPClient(5*time.Second, cfg, PurposeSearch)
		case *TavilySearchBackend:
			p.client = newPolicyHTTPClient(6*time.Second, cfg, PurposeSearch)
		case *DuckDuckGoBackend:
			p.client = newPolicyHTTPClient(4*time.Second, cfg, PurposeSearch)
		case *StartpageBackend:
			p.client = newPolicyHTTPClient(5*time.Second, cfg, PurposeSearch)
		case *BingSearchBackend:
			p.client = newPolicyHTTPClient(5*time.Second, cfg, PurposeSearch)
		}
	}
}
