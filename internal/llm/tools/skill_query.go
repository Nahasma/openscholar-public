package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/openscholar/openscholar/internal/skillbank"
)

type skillQueryTool struct {
	skills skillbank.Service
}

type skillQueryParams struct {
	Mode            string   `json:"mode,omitempty"`
	ID              string   `json:"id,omitempty"`
	Query           string   `json:"query"`
	Category        string   `json:"category,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	IncludeExplicit bool     `json:"include_explicit,omitempty"`
	IncludeDisabled bool     `json:"include_disabled,omitempty"`
	Exposure        string   `json:"exposure,omitempty"`
	Path            string   `json:"path,omitempty"`
}

// NewSkillQueryTool creates a deferred tool for searching the skill bank.
func NewSkillQueryTool(skills skillbank.Service) BaseTool {
	return &skillQueryTool{skills: skills}
}

func (t *skillQueryTool) Info() ToolInfo {
	return ToolInfo{
		Name: "SkillQuery",
		Description: "Search skill metadata/snippets from the skill catalog, then load full instructions for a specific skill id. " +
			"Skills cover writing, research, memory, tool usage, and workflow patterns.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language description of the skill you need in search mode, or a fallback resolver in view mode.",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Optional mode: 'search' (default) returns metadata hits; 'view' returns the full skill by id.",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "Skill id for view mode, e.g. 'writing/apa_citation'.",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "Optional category filter: memory, writing, research, tool_usage, domain, workflow, prompt, debug",
				},
				"tags": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional tag filter (any-match)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum results to return (default: 5)",
				},
				"exposure": map[string]any{
					"type":        "string",
					"description": "Optional exposure filter. Model-facing search can only return model-invocable skills.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional path filter for skills with path activation patterns.",
				},
			},
			"required": []string{},
		},
		Required: []string{},
	}
}

func (t *skillQueryTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params skillQueryParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "search"
	}
	if params.ID != "" {
		mode = "view"
	}

	if mode == "view" {
		id := strings.TrimSpace(params.ID)
		if id == "" && params.Query != "" {
			hits, err := t.skills.SearchMetadata(ctx, skillbank.QueryOptions{
				Query:    params.Query,
				Category: params.Category,
				Tags:     params.Tags,
				Limit:    2,
			})
			if err != nil {
				return NewTextErrorResponse(fmt.Sprintf("Skill search failed: %v", err)), nil
			}
			filtered := make([]skillbank.SkillSearchHit, 0, len(hits))
			for _, hit := range hits {
				if hit.Meta.IsModelInvocable() && matchesSkillMetaFilters(hit.Meta, params) {
					filtered = append(filtered, hit)
				}
			}
			hits = filtered
			if len(hits) == 0 {
				return NewTextResponse("No skill matched for view mode."), nil
			}
			if len(hits) > 1 {
				return NewTextErrorResponse("view mode query was ambiguous; run search mode first and then view by explicit id."), nil
			}
			id = hits[0].Meta.ID
		}
		if id == "" {
			return NewTextErrorResponse("view mode requires id (or query to resolve a single skill)."), nil
		}
		skill, err := t.skills.View(ctx, id)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Skill view failed: %v", err)), nil
		}
		if !skill.IsModelInvocable() || !matchesSkillMetaFilters(skillToMeta(*skill), params) {
			return NewTextErrorResponse(fmt.Sprintf("Skill %s is not model-invocable under current filters.", id)), nil
		}
		_ = t.skills.RecordUsage(ctx, skill.ID, true)
		var sb strings.Builder
		fmt.Fprintf(&sb, "## %s [%s]\n", skill.Name, skill.ID)
		fmt.Fprintf(&sb, "**Category**: %s | **Tags**: %s | **Usage**: %d\n",
			skill.Category, strings.Join(skill.Tags, ", "), skill.UsageCount)
		if skill.WhenToUse != "" {
			fmt.Fprintf(&sb, "**When to use**: %s\n", skill.WhenToUse)
		}
		if len(skill.AllowedTools) > 0 {
			fmt.Fprintf(&sb, "**Allowed tools**: %s\n", strings.Join(skill.AllowedTools, ", "))
		}
		if skill.Agent != "" {
			fmt.Fprintf(&sb, "**Agent**: %s\n", skill.Agent)
		}
		if skill.Context != "" {
			fmt.Fprintf(&sb, "**Context**: %s\n", skill.Context)
		}
		if skill.Effort != "" {
			fmt.Fprintf(&sb, "**Effort**: %s\n", skill.Effort)
		}
		if skill.Model != "" {
			fmt.Fprintf(&sb, "**Model**: %s\n", skill.Model)
		}
		if skill.UserInvocable {
			sb.WriteString("**User invocable**: true\n")
		}
		if skill.DisableModelInvocation {
			sb.WriteString("**Disable model invocation**: true\n")
		}
		if skill.Exposure != "" {
			fmt.Fprintf(&sb, "**Exposure**: %s\n", skill.Exposure)
		}
		if len(skill.Paths) > 0 {
			fmt.Fprintf(&sb, "**Paths**: %s\n", strings.Join(skill.Paths, ", "))
		}
		if len(skill.Platforms) > 0 {
			fmt.Fprintf(&sb, "**Platforms**: %s\n", strings.Join(skill.Platforms, ", "))
		}
		if skill.Source != "" {
			fmt.Fprintf(&sb, "**Source**: %s\n", skill.Source)
		}
		sb.WriteString("\n")
		sb.WriteString(skill.Instruction)
		return NewTextResponse(sb.String()), nil
	}

	if params.Query == "" {
		return NewTextErrorResponse("search mode requires query"), nil
	}
	if params.Limit <= 0 {
		params.Limit = 5
	}
	results, err := t.skills.SearchMetadata(ctx, skillbank.QueryOptions{
		Query:    params.Query,
		Category: params.Category,
		Tags:     params.Tags,
		Limit:    params.Limit,
	})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Skill search failed: %v", err)), nil
	}
	if len(results) == 0 {
		allMeta, _ := t.skills.ListMetadata(ctx)
		categories := make(map[string]int)
		for _, m := range allMeta {
			if !allowSkillMeta(m, params) {
				continue
			}
			categories[m.Category]++
		}
		var sb strings.Builder
		sb.WriteString("No skills matched your query. Available categories:\n")
		for cat, count := range categories {
			fmt.Fprintf(&sb, "- %s (%d skills)\n", cat, count)
		}
		return NewTextResponse(sb.String()), nil
	}
	filtered := make([]skillbank.SkillSearchHit, 0, len(results))
	for _, hit := range results {
		if allowSkillMeta(hit.Meta, params) {
			filtered = append(filtered, hit)
		}
	}
	if len(filtered) == 0 {
		return NewTextResponse("No model-invocable skills matched your query."), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d skill(s):\n\n", len(filtered))
	for _, hit := range filtered {
		fmt.Fprintf(&sb, "## %s [%s]\n", hit.Meta.Name, hit.Meta.ID)
		fmt.Fprintf(&sb, "**Category**: %s | **Tags**: %s | **Usage**: %d\n",
			hit.Meta.Category, strings.Join(hit.Meta.Tags, ", "), hit.Meta.UsageCount)
		if hit.Meta.WhenToUse != "" {
			fmt.Fprintf(&sb, "**When to use**: %s\n", hit.Meta.WhenToUse)
		}
		if hit.Snippet != "" {
			fmt.Fprintf(&sb, "**Snippet**: %s\n", hit.Snippet)
		}
		sb.WriteString("Use `{\"mode\":\"view\",\"id\":\"")
		sb.WriteString(hit.Meta.ID)
		sb.WriteString("\"}` to load full instructions.\n\n---\n\n")
	}
	return NewTextResponse(sb.String()), nil
}

func allowSkillMeta(meta skillbank.SkillMeta, params skillQueryParams) bool {
	if !meta.IsModelInvocable() {
		return false
	}
	return matchesSkillMetaFilters(meta, params)
}

func matchesSkillMetaFilters(meta skillbank.SkillMeta, params skillQueryParams) bool {
	if params.Exposure != "" && strings.TrimSpace(params.Exposure) != string(meta.Exposure) {
		return false
	}
	if params.Path != "" && len(meta.Paths) > 0 {
		target := filepath.ToSlash(filepath.Clean(params.Path))
		matched := false
		for _, pattern := range meta.Paths {
			if ok, err := doublestar.Match(strings.TrimSpace(pattern), target); err == nil && ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func skillToMeta(s skillbank.Skill) skillbank.SkillMeta {
	return skillbank.SkillMeta{
		ID:                     s.ID,
		Name:                   s.Name,
		Description:            s.Description,
		Category:               s.Category,
		Tags:                   append([]string(nil), s.Tags...),
		WhenToUse:              s.WhenToUse,
		AllowedTools:           append([]string(nil), s.AllowedTools...),
		Agent:                  s.Agent,
		Context:                s.Context,
		Effort:                 s.Effort,
		Model:                  s.Model,
		Exposure:               s.Exposure,
		UserInvocable:          s.UserInvocable,
		DisableModelInvocation: s.DisableModelInvocation,
		Paths:                  append([]string(nil), s.Paths...),
		Platforms:              append([]string(nil), s.Platforms...),
		Source:                 s.Source,
		UsageCount:             s.UsageCount,
	}
}
