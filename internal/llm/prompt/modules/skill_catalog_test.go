package modules

import (
	"strings"
	"testing"
)

// makeEntries creates a slice of SkillCatalogEntry for testing.
func makeEntries(specs []struct {
	name, desc, cat string
	usage           int
}) []SkillCatalogEntry {
	out := make([]SkillCatalogEntry, len(specs))
	for i, s := range specs {
		out[i] = SkillCatalogEntry{
			Name:        s.name,
			Description: s.desc,
			Category:    s.cat,
			UsageCount:  s.usage,
		}
	}
	return out
}

// TestSkillCatalogModule_Empty verifies that an empty entry list produces no content.
func TestSkillCatalogModule_Empty(t *testing.T) {
	m := NewSkillCatalogModule(nil, 200_000)
	if m.Name() != "skill_catalog" {
		t.Errorf("expected name 'skill_catalog', got %q", m.Name())
	}
	if m.Priority() != 7 {
		t.Errorf("expected priority 7, got %d", m.Priority())
	}
	if m.Content() != "" {
		t.Errorf("expected empty content for nil entries, got %q", m.Content())
	}

	// Also test with empty slice
	m2 := NewSkillCatalogModule([]SkillCatalogEntry{}, 200_000)
	if m2.Content() != "" {
		t.Errorf("expected empty content for empty entries, got %q", m2.Content())
	}
}

// TestSkillCatalogModule_Basic verifies that 10 entries produce a correctly categorised list.
func TestSkillCatalogModule_Basic(t *testing.T) {
	entries := makeEntries([]struct {
		name, desc, cat string
		usage           int
	}{
		{"figure_routing", "route figure requests to the right tool", "research", 20},
		{"matplotlib_scientific", "generate scientific figures", "research", 15},
		{"d2_architecture", "draw architecture diagrams", "research", 10},
		{"insert", "insert new memory", "memory", 30},
		{"update", "update existing memory", "memory", 25},
		{"delete", "delete a memory entry", "memory", 5},
		{"noop", "no operation placeholder", "memory", 3},
		{"capture_kb_finding", "capture knowledge base finding", "memory", 12},
		{"latex_table", "generate LaTeX tables", "writing", 18},
		{"abstract_writer", "write paper abstracts", "writing", 22},
	})

	m := NewSkillCatalogModule(entries, 200_000)
	content := m.Content()

	if content == "" {
		t.Fatal("expected non-empty content for 10 entries")
	}

	// Must contain header
	if !strings.Contains(content, "# Available Skills") {
		t.Error("missing '# Available Skills' header")
	}

	// Must mention SkillQuery
	if !strings.Contains(content, "SkillQuery") {
		t.Error("missing SkillQuery reference")
	}
	if !strings.Contains(content, "mode=view") {
		t.Error("missing view-mode guidance")
	}

	// Must contain all three categories
	for _, cat := range []string{"research", "memory", "writing"} {
		if !strings.Contains(content, "**"+cat+"**") {
			t.Errorf("missing category %q", cat)
		}
	}

	// Must contain all skill names
	for _, e := range entries {
		if !strings.Contains(content, e.Name) {
			t.Errorf("missing skill name %q", e.Name)
		}
	}

	// Must contain total count summary
	if !strings.Contains(content, "10 skills") {
		t.Error("missing total skill count")
	}
}

// TestSkillCatalogModule_Budget verifies that 100 entries stay within the character budget.
func TestSkillCatalogModule_Budget(t *testing.T) {
	// Build 100 entries across 4 categories with varied usage counts
	categories := []string{"research", "memory", "writing", "tools"}
	specs := make([]struct {
		name, desc, cat string
		usage           int
	}, 100)
	for i := 0; i < 100; i++ {
		cat := categories[i%len(categories)]
		specs[i] = struct {
			name, desc, cat string
			usage           int
		}{
			name:  strings.ToLower(cat[:3]) + "_skill_" + string(rune('a'+i%26)),
			desc:  "description for skill number " + cat,
			cat:   cat,
			usage: 100 - i,
		}
	}
	entries := makeEntries(specs)

	contextWindow := 200_000
	budget := contextWindow / 100 * 4 // 8000 chars

	m := NewSkillCatalogModule(entries, contextWindow)
	content := m.Content()

	if content == "" {
		t.Fatal("expected non-empty content")
	}

	if len(content) > budget {
		t.Errorf("content length %d exceeds budget %d", len(content), budget)
	}

	// Must still contain header and SkillQuery
	if !strings.Contains(content, "# Available Skills") {
		t.Error("missing header")
	}
	if !strings.Contains(content, "SkillQuery") {
		t.Error("missing SkillQuery reference")
	}
	if !strings.Contains(content, "mode=view") {
		t.Error("missing view-mode guidance")
	}
}

// TestSkillCatalogModule_Sorting verifies categories are sorted alphabetically
// and within each category skills are ordered by UsageCount DESC.
func TestSkillCatalogModule_Sorting(t *testing.T) {
	entries := makeEntries([]struct {
		name, desc, cat string
		usage           int
	}{
		{"z_low", "low usage", "beta", 1},
		{"a_high", "high usage", "beta", 100},
		{"m_mid", "mid usage", "beta", 50},
		{"alpha_skill", "alpha category skill", "alpha", 10},
		{"gamma_skill", "gamma category skill", "gamma", 5},
	})

	m := NewSkillCatalogModule(entries, 200_000)
	content := m.Content()

	// Categories must appear in alphabetical order: alpha < beta < gamma
	alphaPos := strings.Index(content, "**alpha**")
	betaPos := strings.Index(content, "**beta**")
	gammaPos := strings.Index(content, "**gamma**")

	if alphaPos < 0 || betaPos < 0 || gammaPos < 0 {
		t.Fatalf("missing category in content:\n%s", content)
	}
	if !(alphaPos < betaPos && betaPos < gammaPos) {
		t.Errorf("categories not in alphabetical order: alpha=%d beta=%d gamma=%d", alphaPos, betaPos, gammaPos)
	}

	// Within beta, a_high (usage=100) must appear before m_mid (50) which must appear before z_low (1)
	aHighPos := strings.Index(content, "a_high")
	mMidPos := strings.Index(content, "m_mid")
	zLowPos := strings.Index(content, "z_low")

	if aHighPos < 0 || mMidPos < 0 || zLowPos < 0 {
		t.Fatalf("missing skill in content:\n%s", content)
	}
	if !(aHighPos < mMidPos && mMidPos < zLowPos) {
		t.Errorf("beta skills not sorted by usage DESC: a_high=%d m_mid=%d z_low=%d", aHighPos, mMidPos, zLowPos)
	}
}

func TestSkillCatalogModule_IncludesRuntimeMetadataHints(t *testing.T) {
	entries := []SkillCatalogEntry{{
		Name:                   "latex_helper",
		Description:            "latex helper",
		Category:               "writing",
		UsageCount:             5,
		AllowedTools:           []string{"View", "Edit", "Write"},
		Model:                  "gpt-5.4",
		UserInvocable:          true,
		DisableModelInvocation: true,
	}}

	content := NewSkillCatalogModule(entries, 200_000).Content()
	for _, want := range []string{"tools:View/Edit+1", "model:gpt-5.4", "slash"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected runtime hint %q in content:\n%s", want, content)
		}
	}
}

func TestSkillCatalogModule_WorkflowAndTriggerMetadataWithinBudget(t *testing.T) {
	entries := []SkillCatalogEntry{
		{
			Name:          "structured_paper_reading",
			Description:   "structured paper reading",
			WhenToUse:     "用于结构化阅读论文与证据抽取",
			Category:      "research",
			UsageCount:    9,
			AllowedTools:  []string{"ScholarSearch", "KBQuery", "KBTree"},
			UserInvocable: true,
		},
		{
			Name:                   "deterministic_export_workflow",
			Description:            "deterministic export workflow",
			WhenToUse:              "固定模板导出与流程检查",
			Category:               "workflow",
			UsageCount:             11,
			AllowedTools:           []string{"KBList", "Grep", "DocExport"},
			DisableModelInvocation: true,
			UserInvocable:          true,
		},
		{
			Name:        "capture_kb_finding",
			Description: "memory capture",
			Category:    "memory",
			UsageCount:  3,
		},
	}

	content := NewSkillCatalogModule(entries, 200_000).Content()
	if !strings.Contains(content, "**workflow**") {
		t.Fatalf("expected workflow category in catalog:\n%s", content)
	}
	if !strings.Contains(content, "deterministic_export_workflow") {
		t.Fatalf("expected workflow skill entry in catalog:\n%s", content)
	}
	for _, want := range []string{"tools:KBList/Grep+1", "slash"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected runtime trigger hint %q in catalog:\n%s", want, content)
		}
	}

	budget := 200_000 / 100 * 4
	if len(content) > budget {
		t.Fatalf("catalog content exceeds budget: len=%d budget=%d", len(content), budget)
	}
}
