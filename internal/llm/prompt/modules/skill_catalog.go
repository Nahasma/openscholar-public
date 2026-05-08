package modules

import (
	"fmt"
	"sort"
	"strings"
)

const skillQueryGuidance = "Use SkillQuery to search by need or skill name, then call it with `mode=view` and the returned id to load full instructions."

// SkillCatalogEntry describes a single skill for the system prompt catalog.
type SkillCatalogEntry struct {
	ID                     string
	Name                   string
	Description            string
	WhenToUse              string
	Category               string
	Exposure               string
	Paths                  []string
	Agent                  string
	Context                string
	Effort                 string
	Source                 string
	UsageCount             int
	AllowedTools           []string
	Model                  string
	UserInvocable          bool
	DisableModelInvocation bool
}

// NewSkillCatalogModule creates a prompt module that injects the skill catalog.
// Priority 7: appears after deferred_tools (6) and before feedback_collector (8).
// The catalog is rebuilt each time the provider is created (SetModel/ReloadProvider),
// picking up any skill changes since the last provider creation.
func NewSkillCatalogModule(entries []SkillCatalogEntry, contextWindow int) DynamicBaseModule {
	content := formatCatalogWithinBudget(entries, contextWindow)
	return NewDynamicBaseModule("skill_catalog", content, 7)
}

// formatCatalogWithinBudget formats the skill catalog within the token budget.
// Budget: 1% of contextWindow, approximated at ~4 chars/token.
func formatCatalogWithinBudget(entries []SkillCatalogEntry, contextWindow int) string {
	if len(entries) == 0 {
		return ""
	}

	// Budget in characters: 1% context window * 4 chars/token
	budget := contextWindow / 100 * 4

	// Group entries by category, sort each group by UsageCount DESC
	grouped := groupByCategory(entries)
	categories := sortedCategoryKeys(grouped)

	for _, cat := range categories {
		sort.Slice(grouped[cat], func(i, j int) bool {
			return grouped[cat][i].UsageCount > grouped[cat][j].UsageCount
		})
	}

	totalSkills := len(entries)

	// Phase 1: try richest format first — names with truncated descriptions
	withDesc := buildWithDescFormat(grouped, categories, totalSkills)
	if len(withDesc) <= budget {
		return withDesc
	}

	// Phase 2: compact format — just skill names per category (no descriptions)
	compact := buildCompactFormat(grouped, categories, totalSkills)
	if len(compact) <= budget {
		return compact
	}

	// Phase 3: top-N skills by usage_count with truncation notice
	return buildTopNFormat(entries, budget, totalSkills)
}

// buildCompactFormat produces the default compact listing.
func buildCompactFormat(grouped map[string][]SkillCatalogEntry, categories []string, total int) string {
	var sb strings.Builder
	sb.WriteString("# Available Skills\n")
	sb.WriteString(skillQueryGuidance)
	sb.WriteString("\n\n")

	for _, cat := range categories {
		skills := grouped[cat]
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = formatSkillLabel(s)
		}
		fmt.Fprintf(&sb, "**%s**: %s\n", cat, strings.Join(names, " · "))
	}

	catCount := len(categories)
	fmt.Fprintf(&sb, "\n%d skills across %d categories. Search with SkillQuery, then view by id for details.", total, catCount)
	return sb.String()
}

// buildWithDescFormat produces a listing with truncated descriptions per skill.
func buildWithDescFormat(grouped map[string][]SkillCatalogEntry, categories []string, total int) string {
	var sb strings.Builder
	sb.WriteString("# Available Skills\n")
	sb.WriteString(skillQueryGuidance)
	sb.WriteString("\n\n")

	for _, cat := range categories {
		skills := grouped[cat]
		parts := make([]string, len(skills))
		for i, s := range skills {
			desc := truncateDesc(s.WhenToUse, 40)
			if desc == "" {
				desc = truncateDesc(s.Description, 40)
			}
			label := formatSkillLabel(s)
			if desc != "" {
				parts[i] = fmt.Sprintf("%s (%s)", label, desc)
			} else {
				parts[i] = label
			}
		}
		fmt.Fprintf(&sb, "**%s**: %s\n", cat, strings.Join(parts, " · "))
	}

	catCount := len(categories)
	fmt.Fprintf(&sb, "\n%d skills across %d categories. Search with SkillQuery, then view by id for details.", total, catCount)
	return sb.String()
}

// buildTopNFormat picks top skills by usage_count to fit within budget.
func buildTopNFormat(entries []SkillCatalogEntry, budget int, total int) string {
	// Sort all entries by UsageCount DESC
	sorted := make([]SkillCatalogEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UsageCount > sorted[j].UsageCount
	})

	// Try decreasing N until it fits
	for n := len(sorted); n > 0; n-- {
		subset := sorted[:n]
		subGrouped := groupByCategory(subset)
		subCats := sortedCategoryKeys(subGrouped)

		var sb strings.Builder
		sb.WriteString("# Available Skills\n")
		sb.WriteString(skillQueryGuidance)
		sb.WriteString("\n\n")

		for _, cat := range subCats {
			skills := subGrouped[cat]
			names := make([]string, len(skills))
			for i, s := range skills {
				names[i] = formatSkillLabel(s)
			}
			fmt.Fprintf(&sb, "**%s**: %s\n", cat, strings.Join(names, " · "))
		}

		remaining := total - n
		if remaining > 0 {
			fmt.Fprintf(&sb, "\n... and %d more skills. Search with SkillQuery, then view by id for details.", remaining)
		} else {
			fmt.Fprintf(&sb, "\n%d skills. Search with SkillQuery, then view by id for details.", n)
		}

		if sb.Len() <= budget || n == 1 {
			return sb.String()
		}
	}

	// Absolute fallback: single line
	return fmt.Sprintf("# Available Skills\nUse SkillQuery to search, then view by id. %d skills available.", total)
}

// groupByCategory groups entries by their Category field.
func groupByCategory(entries []SkillCatalogEntry) map[string][]SkillCatalogEntry {
	grouped := make(map[string][]SkillCatalogEntry)
	for _, e := range entries {
		grouped[e.Category] = append(grouped[e.Category], e)
	}
	return grouped
}

// sortedCategoryKeys returns sorted category names from the map.
func sortedCategoryKeys(grouped map[string][]SkillCatalogEntry) []string {
	cats := make([]string, 0, len(grouped))
	for k := range grouped {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	return cats
}

// truncateDesc truncates a description to maxLen chars, appending "..." if cut.
func truncateDesc(s string, maxLen int) string {
	// Use first sentence if possible
	for i, c := range s {
		if c == '.' || c == '\n' {
			sentence := s[:i]
			if len(sentence) <= maxLen {
				return sentence
			}
			break
		}
	}
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func formatSkillLabel(entry SkillCatalogEntry) string {
	tags := runtimeTags(entry)
	if len(tags) == 0 {
		return entry.Name
	}
	return fmt.Sprintf("%s [%s]", entry.Name, strings.Join(tags, ", "))
}

func runtimeTags(entry SkillCatalogEntry) []string {
	var tags []string
	if len(entry.AllowedTools) > 0 {
		display := entry.AllowedTools
		if len(display) > 2 {
			display = display[:2]
		}
		tag := "tools:" + strings.Join(display, "/")
		if extra := len(entry.AllowedTools) - len(display); extra > 0 {
			tag += fmt.Sprintf("+%d", extra)
		}
		tags = append(tags, tag)
	}
	if entry.Model != "" {
		tags = append(tags, "model:"+truncateTagValue(entry.Model, 18))
	}
	if entry.UserInvocable {
		tags = append(tags, "slash")
	}
	if entry.Exposure != "" {
		tags = append(tags, "exp:"+entry.Exposure)
	}
	if len(entry.Paths) > 0 {
		tags = append(tags, "paths")
	}
	if entry.Agent != "" {
		tags = append(tags, "agent:"+truncateTagValue(entry.Agent, 12))
	}
	if entry.Context != "" {
		tags = append(tags, "ctx:"+truncateTagValue(entry.Context, 12))
	}
	if entry.Effort != "" {
		tags = append(tags, "effort:"+entry.Effort)
	}
	if entry.Source != "" {
		tags = append(tags, "src:"+truncateTagValue(entry.Source, 12))
	}
	return tags
}

func truncateTagValue(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
