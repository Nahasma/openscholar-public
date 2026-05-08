package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/openscholar/openscholar/internal/permission"
)

// ToolBrief is a compact tool summary for system prompt injection.
type ToolBrief struct {
	Name        string
	Description string
	Metadata    DeferredToolMetadata
}

type DeferredToolMetadata struct {
	Domain          string
	CostTier        string
	RequiresIntent  string
	ActivationScope string
}

type DeferredActivation struct {
	Scope string
}

const (
	IntentLocalKB = "local_kb"

	ActivationScopeSession = "session"
	ActivationScopeTurn    = "turn"
)

// DeferredRegistry manages core (always-active) and deferred (on-demand) tools.
// Core tools are sent to the LLM on every request. Deferred tools are only
// activated when the LLM calls ToolSearch to find and activate them.
type DeferredRegistry struct {
	core     []BaseTool
	deferred map[string]BaseTool
	active   map[string]DeferredActivation
	metadata map[string]DeferredToolMetadata
	mu       sync.RWMutex

	toolSearch *toolSearchTool // always included in active tools
}

func (r *DeferredRegistry) metadataFor(name string) DeferredToolMetadata {
	md, ok := r.metadata[name]
	if ok && (md.RequiresIntent != "" || md.ActivationScope != "" || md.Domain != "" || md.CostTier != "") {
		return md
	}
	return DefaultDeferredToolMetadata(name)
}

// NewDeferredRegistry creates a registry with core and deferred tools.
// A ToolSearch tool is automatically added to the core set.
func NewDeferredRegistry(core []BaseTool, deferred []BaseTool) *DeferredRegistry {
	deferredMap := make(map[string]BaseTool, len(deferred))
	for _, t := range deferred {
		deferredMap[t.Info().Name] = t
	}

	r := &DeferredRegistry{
		core:     core,
		deferred: deferredMap,
		active:   make(map[string]DeferredActivation),
		metadata: make(map[string]DeferredToolMetadata, len(deferred)),
	}
	for _, t := range deferred {
		name := t.Info().Name
		r.metadata[name] = DefaultDeferredToolMetadata(name)
	}

	r.toolSearch = &toolSearchTool{registry: r}
	return r
}

// ActiveTools returns core tools + ToolSearch + currently activated deferred tools,
// sorted by name for stable tool schema ordering (improves prompt cache hit rate).
func (r *DeferredRegistry) ActiveTools() []BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]BaseTool, 0, len(r.core)+1+len(r.active))
	result = append(result, r.core...)
	result = append(result, r.toolSearch)

	for name := range r.active {
		if t, ok := r.deferred[name]; ok {
			result = append(result, t)
		}
	}

	// Sort by tool name to ensure a stable ordering across calls.
	// Stable tool schema ordering is required for Anthropic prompt cache to hit
	// on the tool block (cache_control is applied to the last tool).
	sort.Slice(result, func(i, j int) bool {
		return result[i].Info().Name < result[j].Info().Name
	})

	return result
}

// AllTools returns all tools (core + deferred), for use in tool execution lookup.
func (r *DeferredRegistry) AllTools() []BaseTool {
	result := make([]BaseTool, 0, len(r.core)+1+len(r.deferred))
	result = append(result, r.core...)
	result = append(result, r.toolSearch)
	for _, t := range r.deferred {
		result = append(result, t)
	}
	return result
}

// RegisterDeferred adds a tool to the deferred set at runtime.
// Use this to register tools that depend on services not yet available at registry creation time.
func (r *DeferredRegistry) RegisterDeferred(tool BaseTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Info().Name
	r.deferred[name] = tool
	r.metadata[name] = DefaultDeferredToolMetadata(name)
}

// FindTool looks up a tool by name from all tools (core + deferred) without
// side effects. It does NOT activate deferred tools — use ActivateTool for that.
func (r *DeferredRegistry) FindTool(name string) (BaseTool, bool) {
	// Check core tools
	for _, t := range r.core {
		if t.Info().Name == name {
			return t, true
		}
	}

	// Check ToolSearch
	if name == r.toolSearch.Info().Name {
		return r.toolSearch, true
	}

	// Check deferred tools (lookup only, no activation)
	r.mu.RLock()
	t, ok := r.deferred[name]
	r.mu.RUnlock()
	if ok {
		return t, true
	}

	return nil, false
}

func (r *DeferredRegistry) ToolEligible(ctx context.Context, name string, query string) bool {
	r.mu.RLock()
	_, isDeferred := r.deferred[name]
	md := r.metadataFor(name)
	r.mu.RUnlock()
	if !isDeferred {
		return true
	}
	return deferredToolEligibleByMetadata(md, name, ctx, query)
}

// ActivateTool explicitly marks a deferred tool as active so it appears in
// ActiveTools(). Returns an error if the tool is not in the deferred set.
func (r *DeferredRegistry) ActivateTool(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.deferred[name]; !ok {
		return fmt.Errorf("deferred tool not found: %s", name)
	}
	md := r.metadataFor(name)
	if !deferredToolEligibleByMetadata(md, name, ctx, "") {
		return fmt.Errorf("deferred tool not eligible: %s", name)
	}
	r.active[name] = DeferredActivation{Scope: md.ActivationScope}
	return nil
}

// Search finds deferred tools matching a query and activates them.
func (r *DeferredRegistry) Search(query string, maxResults int) []BaseTool {
	return r.SearchFiltered(context.Background(), query, maxResults, nil)
}

// SearchFiltered finds deferred tools matching a query, applies an optional
// predicate, and activates only the returned tools.
func (r *DeferredRegistry) SearchFiltered(ctx context.Context, query string, maxResults int, allow func(BaseTool) bool) []BaseTool {
	if maxResults <= 0 {
		maxResults = 3
	}

	query = strings.ToLower(query)
	type scored struct {
		tool  BaseTool
		score int
	}

	var matches []scored
	for name, t := range r.deferred {
		md := r.metadataFor(name)
		if !deferredToolEligibleByMetadata(md, name, ctx, query) {
			continue
		}
		if allow != nil && !allow(t) {
			continue
		}
		info := t.Info()
		score := 0

		nameLower := strings.ToLower(name)
		descLower := strings.ToLower(info.Description)

		// Exact name match
		if strings.Contains(query, nameLower) || strings.Contains(nameLower, query) {
			score += 10
		}

		// Keyword matching in query vs name+description
		words := strings.Fields(query)
		for _, word := range words {
			if strings.Contains(nameLower, word) {
				score += 5
			}
			if strings.Contains(descLower, word) {
				score += 2
			}
		}

		if score > 0 {
			matches = append(matches, scored{tool: t, score: score})
		}
	}

	// Sort by score descending (simple insertion sort for small N)
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].score > matches[j-1].score; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	// Activate and return top results
	var result []BaseTool
	for i, m := range matches {
		if i >= maxResults {
			break
		}
		_ = r.ActivateTool(ctx, m.tool.Info().Name)
		result = append(result, m.tool)
	}

	return result
}

// DeferredToolNames returns brief info about all deferred tools.
func (r *DeferredRegistry) DeferredToolNames() []ToolBrief {
	briefs := make([]ToolBrief, 0, len(r.deferred))
	for _, t := range r.deferred {
		info := t.Info()
		briefs = append(briefs, ToolBrief{
			Name:        info.Name,
			Description: info.Description,
			Metadata:    r.metadataFor(info.Name),
		})
	}
	return briefs
}

// ActivateByNames activates deferred tools by exact name.
// Used for intent-based pre-activation to skip ToolSearch iterations.
func (r *DeferredRegistry) ActivateByNames(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		if _, ok := r.deferred[name]; ok {
			md := r.metadataFor(name)
			r.active[name] = DeferredActivation{Scope: md.ActivationScope}
		}
	}
}

func (r *DeferredRegistry) BeginTurn() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, activation := range r.active {
		scope := activation.Scope
		if scope == "" {
			scope = ActivationScopeSession
		}
		if scope == ActivationScopeTurn {
			delete(r.active, name)
		}
	}
}

// ActiveToolCount returns the number of currently active tools (core + ToolSearch + activated deferred).
func (r *DeferredRegistry) ActiveToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.core) + 1 + len(r.active)
}

// --- ToolSearch Tool ---

type toolSearchTool struct {
	registry *DeferredRegistry
}

type toolSearchParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (t *toolSearchTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ToolSearch",
		Description: "Search and activate specialized tools. Use when you need capabilities beyond basic file operations " +
			"(e.g., web search, fetch URL content, image generation, knowledge base, paper validation, scholar search, sub-agent tasks). " +
			"Do not activate knowledge-base tools for public/common explanations unless the user explicitly asks about local or added papers. " +
			"Returns matching tool definitions that become callable in subsequent requests.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language description of what you need, or exact tool name",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum tools to return (default: 3)",
				},
			},
			"required": []string{"query"},
		},
		Required: []string{"query"},
	}
}

func (t *toolSearchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params toolSearchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	// Fast path: "select:Tool1,Tool2" directly activates named tools without scoring.
	if strings.HasPrefix(params.Query, "select:") {
		namesStr := strings.TrimPrefix(params.Query, "select:")
		names := strings.Split(namesStr, ",")
		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}

		var activated []BaseTool
		for _, name := range names {
			if tool, ok := t.registry.deferred[name]; ok {
				md := t.registry.metadataFor(name)
				if !deferredToolEligibleByMetadata(md, name, ctx, params.Query) {
					continue
				}
				if !toolAllowedForSearchContext(ctx, tool.Info().Name) {
					continue
				}
				_ = t.registry.ActivateTool(ctx, name)
				activated = append(activated, tool)
			}
		}

		if len(activated) == 0 {
			return NewTextResponse(fmt.Sprintf("No tools found for select query %q. Use ToolSearch with a keyword query to discover available tools.", params.Query)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Activated %d tool(s) by direct selection:\n\n", len(activated))
		for _, tool := range activated {
			info := tool.Info()
			fmt.Fprintf(&sb, "**%s** — %s\n", info.Name, truncateDesc(info.Description, 120))
		}
		sb.WriteString("\nThese tools are now available for use.")
		return NewTextResponse(sb.String()), nil
	}

	matched := t.registry.SearchFiltered(ctx, params.Query, params.MaxResults, func(tool BaseTool) bool {
		return toolAllowedForSearchContext(ctx, tool.Info().Name)
	})

	if len(matched) == 0 {
		// Return available tool names to help LLM refine search
		var sb strings.Builder
		sb.WriteString("No tools matched your query. Available specialized tools:\n")
		for _, brief := range t.registry.DeferredToolNames() {
			if !deferredToolEligibleByMetadata(brief.Metadata, brief.Name, ctx, params.Query) {
				continue
			}
			if !toolAllowedForSearchContext(ctx, brief.Name) {
				continue
			}
			fmt.Fprintf(&sb, "- %s: %s\n", brief.Name, truncateDesc(brief.Description, 80))
		}
		return NewTextResponse(sb.String()), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Activated %d tool(s):\n\n", len(matched))
	for _, tool := range matched {
		info := tool.Info()
		// Check runtime availability if the tool supports it
		if checker, ok := tool.(AvailabilityChecker); ok {
			if avail, reason := checker.Available(); !avail {
				fmt.Fprintf(&sb, "**%s** [degraded: %s] — %s\n", info.Name, reason, truncateDesc(info.Description, 100))
				continue
			}
		}
		fmt.Fprintf(&sb, "**%s** — %s\n", info.Name, truncateDesc(info.Description, 120))
	}
	sb.WriteString("\nThese tools are now available for use.")

	return NewTextResponse(sb.String()), nil
}

func toolAllowedForSearchContext(ctx context.Context, toolName string) bool {
	if IsResearchMode(ctx) && !ResearchToolAllowed(ctx, toolName) {
		return false
	}
	if permission.CurrentMode(ctx) == permission.ModePlan && !PlanModeToolAllowed(toolName) {
		return false
	}
	return true
}

func DefaultDeferredToolMetadata(toolName string) DeferredToolMetadata {
	md := DeferredToolMetadata{
		Domain:          "general",
		CostTier:        "standard",
		RequiresIntent:  "",
		ActivationScope: ActivationScopeSession,
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "kbadd", "kbtree", "kbsearch", "kblist", "kbquery", "kbhealth", "kbrepair", "kbreindex",
		"kb_add", "kb_tree", "kb_search", "kb_list", "kb_query", "kb_health", "kb_repair", "kb_reindex":
		md.Domain = "kb"
		md.CostTier = "high"
		md.RequiresIntent = IntentLocalKB
		md.ActivationScope = ActivationScopeTurn
	}
	return md
}

type localKBIntentContextKey struct{}

func WithLocalKBIntent(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, localKBIntentContextKey{}, enabled)
}

func LocalKBIntentFromContext(ctx context.Context) bool {
	enabled, _ := ctx.Value(localKBIntentContextKey{}).(bool)
	return enabled
}

func TextHasLocalKBIntent(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	markers := []string{
		"我的知识库", "本地知识库", "知识库里", "知识库里的", "知识库中", "知识库中的",
		"本地论文", "已添加", "已上传", "库里", "这篇 pdf", "这篇论文",
		"local kb", "my papers", "uploaded", "this pdf", "this paper", "in my library", "my library",
	}
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func deferredToolEligibleByMetadata(md DeferredToolMetadata, toolName string, ctx context.Context, query string) bool {
	requires := strings.TrimSpace(md.RequiresIntent)
	if requires == "" {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "kbadd", "kbtree", "kbsearch", "kblist", "kbquery", "kbhealth", "kbrepair", "kbreindex",
			"kb_add", "kb_tree", "kb_search", "kb_list", "kb_query", "kb_health", "kb_repair", "kb_reindex":
			requires = IntentLocalKB
		}
	}
	switch requires {
	case "", "none":
		return true
	case IntentLocalKB:
		return LocalKBIntentFromContext(ctx) || TextHasLocalKBIntent(query)
	default:
		return true
	}
}

func truncateDesc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
