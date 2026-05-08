package skillbank

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func TestScanUserDir_SupportsLegacyAndBundleLayouts(t *testing.T) {
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "memory", "legacy.md"), "legacy", "memory", "legacy content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "bundle", "SKILL.md"), "bundle", "memory", "bundle content")
	writeSkillFile(t, filepath.Join(userDir, "tool-usage", "shell.md"), "shell", "tool_usage", "shell content")

	writeSkillFile(t, filepath.Join(userDir, "_inbox", "ignored.md"), "ignored", "memory", "ignored content")
	writeSkillFile(t, filepath.Join(userDir, "_managed", "memory", "managed.md"), "managed", "memory", "managed content")
	writeSkillFile(t, filepath.Join(userDir, ".hidden", "ignored.md"), "ignored2", "memory", "ignored content")
	writeSkillFile(t, filepath.Join(userDir, "memory", ".draft", "SKILL.md"), "draft", "memory", "draft content")

	skills, err := scanUserDir(userDir)
	if err != nil {
		t.Fatalf("scanUserDir error: %v", err)
	}

	gotIDs := skillIDs(skills)
	wantIDs := []string{
		"memory/bundle",
		"memory/legacy",
		"tool_usage/shell",
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids mismatch\nwant=%v\ngot=%v", wantIDs, gotIDs)
	}
}

func TestScanUserDir_PrefersBundleWhenBothLayoutsExist(t *testing.T) {
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "memory", "collision.md"), "legacy collision", "memory", "legacy content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "collision", "SKILL.md"), "bundle collision", "memory", "bundle content")

	skills, err := scanUserDir(userDir)
	if err != nil {
		t.Fatalf("scanUserDir error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].ID != "memory/collision" {
		t.Fatalf("stable id mismatch: got %s", skills[0].ID)
	}
	if skills[0].Instruction != "bundle content" {
		t.Fatalf("expected bundle content, got %q", skills[0].Instruction)
	}
}

func TestScanBundledFS_SupportsLegacyAndBundleLayouts(t *testing.T) {
	fsys := fstest.MapFS{
		"memory/legacy.md":            {Data: []byte(skillMarkdown("legacy", "memory", "legacy content", "system"))},
		"memory/bundle/SKILL.md":      {Data: []byte(skillMarkdown("bundle", "memory", "bundle content", "system"))},
		"tool-usage/shell.md":         {Data: []byte(skillMarkdown("shell", "tool_usage", "shell content", "system"))},
		"_inbox/ignored.md":           {Data: []byte(skillMarkdown("ignored", "memory", "ignored content", "system"))},
		".hidden/ignored.md":          {Data: []byte(skillMarkdown("ignored2", "memory", "ignored2 content", "system"))},
		"memory/.draft/SKILL.md":      {Data: []byte(skillMarkdown("draft", "memory", "draft content", "system"))},
		"memory/nested/extra/file.md": {Data: []byte(skillMarkdown("extra", "memory", "extra content", "system"))},
	}

	skills, err := scanBundledFS(fsys)
	if err != nil {
		t.Fatalf("scanBundledFS error: %v", err)
	}

	gotIDs := skillIDs(skills)
	wantIDs := []string{
		"memory/bundle",
		"memory/legacy",
		"tool_usage/shell",
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids mismatch\nwant=%v\ngot=%v", wantIDs, gotIDs)
	}
}

func TestScanBundledFS_PrefersBundleWhenBothLayoutsExist(t *testing.T) {
	fsys := fstest.MapFS{
		"memory/collision.md":       {Data: []byte(skillMarkdown("legacy collision", "memory", "legacy content", "system"))},
		"memory/collision/SKILL.md": {Data: []byte(skillMarkdown("bundle collision", "memory", "bundle content", "system"))},
	}

	skills, err := scanBundledFS(fsys)
	if err != nil {
		t.Fatalf("scanBundledFS error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].ID != "memory/collision" {
		t.Fatalf("stable id mismatch: got %s", skills[0].ID)
	}
	if skills[0].Instruction != "bundle content" {
		t.Fatalf("expected bundle content, got %q", skills[0].Instruction)
	}
}

func TestLoadSkillByID_ReadsBothLayoutsAndKeepsStableID(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "memory", "user_legacy.md"), "user legacy", "memory", "user legacy content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "user_bundle", "SKILL.md"), "user bundle", "memory", "user bundle content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "override", "SKILL.md"), "override user", "memory", "override from user")

	bundled := fstest.MapFS{
		"memory/bundled_legacy.md":       {Data: []byte(skillMarkdown("bundled legacy", "memory", "bundled legacy content", "system"))},
		"memory/bundled_bundle/SKILL.md": {Data: []byte(skillMarkdown("bundled bundle", "memory", "bundled bundle content", "system"))},
		"memory/override.md":             {Data: []byte(skillMarkdown("override bundled", "memory", "override from bundled", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
	}

	assertLoadedInstruction(t, s, "memory/user_legacy", "user legacy content")
	assertLoadedInstruction(t, s, "memory/user_bundle", "user bundle content")
	assertLoadedInstruction(t, s, "memory/override", "override from user")
	assertLoadedInstruction(t, s, "memory/bundled_legacy", "bundled legacy content")
	assertLoadedInstruction(t, s, "memory/bundled_bundle", "bundled bundle content")
}

func TestLoadSkillByID_PrefersBundleWhenBothLayoutsExist(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "memory", "collision.md"), "legacy collision", "memory", "legacy content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "collision", "SKILL.md"), "bundle collision", "memory", "bundle content")

	bundled := fstest.MapFS{
		"memory/system_collision.md":       {Data: []byte(skillMarkdown("system legacy", "memory", "system legacy content", "system"))},
		"memory/system_collision/SKILL.md": {Data: []byte(skillMarkdown("system bundle", "memory", "system bundle content", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
	}

	assertLoadedInstruction(t, s, "memory/collision", "bundle content")
	assertLoadedInstruction(t, s, "memory/system_collision", "system bundle content")
}

func TestLoadSkillByID_DBFilePathFallbackStillWorks(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	dbPath := filepath.Join(userDir, "memory", "db_actual", "SKILL.md")
	writeSkillFile(t, dbPath, "db actual", "memory", "db fallback content")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
	}

	now := time.Now().Unix()
	err := s.store.Upsert(context.Background(), skillRow{
		ID:          "memory/db_alias",
		Name:        "db alias",
		Description: "db alias",
		Category:    "memory",
		Version:     1,
		Author:      "user",
		FilePath:    dbPath,
		CreatedAt:   now,
		UpdatedAt:   now,
		Content:     "db fallback content",
	})
	if err != nil {
		t.Fatalf("upsert fallback row: %v", err)
	}

	skill, err := s.loadSkillByID("memory/db_alias")
	if err != nil {
		t.Fatalf("loadSkillByID fallback error: %v", err)
	}
	if skill.ID != "memory/db_alias" {
		t.Fatalf("stable id mismatch: got %s", skill.ID)
	}
	if skill.Instruction != "db fallback content" {
		t.Fatalf("fallback content mismatch: got %q", skill.Instruction)
	}
}

func TestUpdate_BundledSkillInBundleLayoutCreatesUserOverride(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	bundled := fstest.MapFS{
		"memory/update_case/SKILL.md": {Data: []byte(skillMarkdown("update case", "memory", "bundled original", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
	}

	existing, err := s.Get(context.Background(), "memory/update_case")
	if err != nil {
		t.Fatalf("load existing skill: %v", err)
	}
	existing.Instruction = "updated by user"

	if err := s.Update(context.Background(), "memory/update_case", *existing); err != nil {
		t.Fatalf("update bundled bundle-layout skill: %v", err)
	}

	userOverridePath := filepath.Join(userDir, "memory", "update_case.md")
	if _, err := os.Stat(userOverridePath); err != nil {
		t.Fatalf("expected user override at %s: %v", userOverridePath, err)
	}
	assertLoadedInstruction(t, s, "memory/update_case", "updated by user")
}

func TestDelete_BundledSkillInBundleLayoutRemovesOverrideAndReverts(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	bundled := fstest.MapFS{
		"memory/delete_case/SKILL.md": {Data: []byte(skillMarkdown("delete case", "memory", "bundled content", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
	}

	userBundleOverride := filepath.Join(userDir, "memory", "delete_case", "SKILL.md")
	writeSkillFile(t, userBundleOverride, "delete case", "memory", "user override content")
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	if err := s.Delete(context.Background(), "memory/delete_case"); err != nil {
		t.Fatalf("delete bundled bundle-layout skill: %v", err)
	}

	if _, err := os.Stat(userBundleOverride); !os.IsNotExist(err) {
		t.Fatalf("expected user override removed, stat err=%v", err)
	}
	assertLoadedInstruction(t, s, "memory/delete_case", "bundled content")
}

func TestDelete_BundledSkillInBundleLayoutWithoutOverrideStillProtected(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	bundled := fstest.MapFS{
		"memory/protected_case/SKILL.md": {Data: []byte(skillMarkdown("protected case", "memory", "bundled content", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
	}

	err := s.Delete(context.Background(), "memory/protected_case")
	if err == nil {
		t.Fatalf("expected delete to fail for bundled skill without user override")
	}
}

func TestBundledInitSkill_ParseAndIndexMetadata(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	skillsRoot := filepath.Join(repoRoot, "data", "skills")

	oldBundled := BundledFS
	BundledFS = os.DirFS(skillsRoot)
	t.Cleanup(func() { BundledFS = oldBundled })

	svc := NewService(conn, userDir, nil)
	metas, err := svc.ListMetadata(context.Background())
	if err != nil {
		t.Fatalf("ListMetadata: %v", err)
	}

	var found *SkillMeta
	for i := range metas {
		if metas[i].ID == "workflow/init_skill" {
			found = &metas[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("workflow/init_skill not indexed; got ids=%v", skillMetaIDs(metas))
	}
	if found.Source != "builtin" {
		t.Fatalf("expected source builtin, got %q", found.Source)
	}
	if found.UserInvocable || !found.IsModelInvocable() || found.Exposure != SkillExposureImplicit {
		t.Fatalf("expected init skill to be implicit/model-invocable, got exposure=%q user=%t model=%t", found.Exposure, found.UserInvocable, found.IsModelInvocable())
	}
	if found.WhenToUse == "" {
		t.Fatalf("expected non-empty when_to_use")
	}
	if len(found.AllowedTools) == 0 {
		t.Fatalf("expected non-empty allowed_tools")
	}
}

func TestReindex_ListMetadata_SearchMetadata_View_StableAcrossLayoutCollision(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	// Same logical id memory/collision, bundle layout should win over legacy.
	writeSkillFile(t, filepath.Join(userDir, "memory", "collision.md"), "legacy collision", "memory", "legacy user content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "collision", "SKILL.md"), "bundle collision", "memory", "bundle user content")

	// Bundled also has same id, user override should still win.
	bundled := fstest.MapFS{
		"memory/collision/SKILL.md": {Data: []byte(skillMarkdown("bundled collision", "memory", "bundled content", "system"))},
	}

	s := &service{
		userDir:   userDir,
		bundledFS: bundled,
		store:     newStore(conn),
		fts:       newFTSSearcher(conn),
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	metas, err := s.ListMetadata(context.Background())
	if err != nil {
		t.Fatalf("ListMetadata: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != "memory/collision" {
		t.Fatalf("unexpected metadata ids: %#v", metas)
	}

	hits, err := s.SearchMetadata(context.Background(), QueryOptions{Query: "bundle", Limit: 5})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if len(hits) == 0 || hits[0].Meta.ID != "memory/collision" {
		t.Fatalf("unexpected search hit: %#v", hits)
	}

	view, err := s.View(context.Background(), "memory/collision")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Instruction != "bundle user content" {
		t.Fatalf("expected user bundle content, got %q", view.Instruction)
	}
}

func TestListMetadataAndSearchMetadata_ExposeRicherFrontmatter(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	raw := `---
name: "meta skill"
description: "desc"
category: "workflow"
when_to_use: "for metadata listing"
allowed-tools: ["SkillQuery"]
user-invocable: true
---
instruction body
`
	path := filepath.Join(userDir, "workflow", "meta_skill.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	metas, err := s.ListMetadata(context.Background())
	if err != nil {
		t.Fatalf("ListMetadata: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	if metas[0].WhenToUse != "for metadata listing" || len(metas[0].AllowedTools) != 1 || !metas[0].UserInvocable {
		t.Fatalf("unexpected richer metadata: %#v", metas[0])
	}

	hits, err := s.SearchMetadata(context.Background(), QueryOptions{Query: "metadata listing", Limit: 1})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Meta.ID != "workflow/meta_skill" {
		t.Fatalf("unexpected hit id: %s", hits[0].Meta.ID)
	}
}

func TestUpdate_RichMetadataFieldsPersist(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "workflow", "rich.md"), "rich", "workflow", "original body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	existing, err := s.Get(context.Background(), "workflow/rich")
	if err != nil {
		t.Fatalf("load existing skill: %v", err)
	}
	existing.WhenToUse = "updated when"
	existing.AllowedTools = []string{"SkillQuery", "View"}
	existing.Agent = "coder"
	existing.Effort = "high"
	existing.Model = "gpt-5.4"
	existing.UserInvocable = true
	existing.DisableModelInvocation = true
	existing.Paths = []string{"**/*.md"}
	existing.Platforms = []string{"darwin"}
	existing.Source = "bundle"

	if err := s.Update(context.Background(), "workflow/rich", *existing); err != nil {
		t.Fatalf("update rich metadata: %v", err)
	}

	view, err := s.View(context.Background(), "workflow/rich")
	if err != nil {
		t.Fatalf("view updated skill: %v", err)
	}
	if view.WhenToUse != "updated when" {
		t.Fatalf("when_to_use mismatch: %q", view.WhenToUse)
	}
	if got := strings.Join(view.AllowedTools, ","); got != "SkillQuery,View" {
		t.Fatalf("allowed tools mismatch: %q", got)
	}
	if view.Agent != "coder" || view.Effort != "high" || view.Model != "gpt-5.4" {
		t.Fatalf("agent metadata mismatch: %#v", view)
	}
	if !view.UserInvocable || !view.DisableModelInvocation {
		t.Fatalf("bool metadata mismatch: %#v", view)
	}
	if got := strings.Join(view.Paths, ","); got != "**/*.md" {
		t.Fatalf("paths mismatch: %q", got)
	}
	if got := strings.Join(view.Platforms, ","); got != "darwin" {
		t.Fatalf("platforms mismatch: %q", got)
	}
	if view.Source != "bundle" {
		t.Fatalf("source mismatch: %q", view.Source)
	}
}

func TestUpdate_RichMetadataFieldsCanClearToEmptyAndFalse(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	raw := `---
name: "rich clear"
description: "desc"
category: "workflow"
when_to_use: "before clear"
allowed-tools: ["SkillQuery", "View"]
agent: "coder"
effort: "high"
model: "gpt-5.4"
user-invocable: true
disable-model-invocation: true
paths: ["**/*.md"]
platforms: ["darwin"]
source: "bundle"
---
instruction body
`
	path := filepath.Join(userDir, "workflow", "rich_clear.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	if err := s.Update(context.Background(), "workflow/rich_clear", Skill{
		WhenToUse:              "",
		AllowedTools:           []string{},
		Agent:                  "",
		Effort:                 "",
		Model:                  "",
		Exposure:               SkillExposureImplicit,
		UserInvocable:          false,
		DisableModelInvocation: false,
		Paths:                  []string{},
		Platforms:              []string{},
		Source:                 "",
	}); err != nil {
		t.Fatalf("clear rich metadata: %v", err)
	}

	view, err := s.View(context.Background(), "workflow/rich_clear")
	if err != nil {
		t.Fatalf("view updated skill: %v", err)
	}
	if view.Name != "rich clear" || view.Description != "desc" || view.Instruction != "instruction body" {
		t.Fatalf("core fields changed unexpectedly: %#v", view)
	}
	if view.WhenToUse != "" || view.Agent != "" || view.Effort != "" || view.Model != "" || view.Source != "" {
		t.Fatalf("string metadata not cleared: %#v", view)
	}
	if len(view.AllowedTools) != 0 || len(view.Paths) != 0 || len(view.Platforms) != 0 {
		t.Fatalf("slice metadata not cleared: %#v", view)
	}
	if view.UserInvocable || view.DisableModelInvocation {
		t.Fatalf("bool metadata not cleared: %#v", view)
	}
}

func TestUpdate_RichMetadataFieldsCanBeCleared(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "workflow", "rich_clear.md"), "rich clear", "workflow", "original body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	existing, err := s.Get(context.Background(), "workflow/rich_clear")
	if err != nil {
		t.Fatalf("load existing skill: %v", err)
	}
	existing.WhenToUse = "set"
	existing.AllowedTools = []string{"View"}
	existing.Agent = "coder"
	existing.Effort = "high"
	existing.Model = "gpt-5.4"
	existing.UserInvocable = true
	existing.DisableModelInvocation = true
	existing.Paths = []string{"**/*.md"}
	existing.Platforms = []string{"darwin"}
	existing.Source = "bundle"
	if err := s.Update(context.Background(), "workflow/rich_clear", *existing); err != nil {
		t.Fatalf("seed rich metadata: %v", err)
	}

	updated, err := s.Get(context.Background(), "workflow/rich_clear")
	if err != nil {
		t.Fatalf("reload seeded skill: %v", err)
	}
	updated.WhenToUse = ""
	updated.AllowedTools = []string{}
	updated.Agent = ""
	updated.Effort = ""
	updated.Model = ""
	updated.Exposure = SkillExposureImplicit
	updated.UserInvocable = false
	updated.DisableModelInvocation = false
	updated.Paths = []string{}
	updated.Platforms = []string{}
	updated.Source = ""
	if err := s.Update(context.Background(), "workflow/rich_clear", *updated); err != nil {
		t.Fatalf("clear rich metadata: %v", err)
	}

	view, err := s.View(context.Background(), "workflow/rich_clear")
	if err != nil {
		t.Fatalf("view updated skill: %v", err)
	}
	if view.WhenToUse != "" || view.Agent != "" || view.Effort != "" || view.Model != "" || view.Source != "" {
		t.Fatalf("expected cleared strings, got %#v", view)
	}
	if len(view.AllowedTools) != 0 || len(view.Paths) != 0 || len(view.Platforms) != 0 {
		t.Fatalf("expected cleared slices, got %#v", view)
	}
	if view.UserInvocable || view.DisableModelInvocation {
		t.Fatalf("expected cleared bools, got %#v", view)
	}
}

func TestBundledSkillsInventory_IncludesResearchBatchAndRichFrontmatter(t *testing.T) {
	bundled := os.DirFS(repoSkillsDir(t))
	skills, err := scanBundledFS(bundled)
	if err != nil {
		t.Fatalf("scanBundledFS error: %v", err)
	}

	byID := make(map[string]Skill, len(skills))
	for _, s := range skills {
		byID[s.ID] = s
	}

	targets := []struct {
		id          string
		category    string
		mustContain []string
	}{
		{
			id:          "research/structured_paper_reading",
			category:    "research",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Claim-Evidence Ledger"},
		},
		{
			id:          "research/literature_gap_mapping",
			category:    "research",
			mustContain: []string{"## 输入与范围澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Search Log"},
		},
		{
			id:          "workflow/research_weekly_report",
			category:    "workflow",
			mustContain: []string{"## 输入与时间窗", "## 方法论流程", "## 输出模板", "## 质量门槛", "owner"},
		},
		{
			id:          "research/figure_quality_review",
			category:    "research",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "caption"},
		},
		{
			id:          "research/visual_storyboard",
			category:    "research",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Figure Set Map"},
		},
		{
			id:          "research/graphical_abstract",
			category:    "research",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Key Message"},
		},
		{
			id:          "writing/paper_argument_outline",
			category:    "writing",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Story Arc"},
		},
		{
			id:          "writing/claim_supported_paragraph",
			category:    "writing",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Evidence Map"},
		},
		{
			id:          "workflow/reproducibility_triage",
			category:    "workflow",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Provenance"},
		},
		{
			id:          "workflow/research_activity_log",
			category:    "workflow",
			mustContain: []string{"## 输入与澄清", "## 方法论流程", "## 输出模板", "## 质量门槛", "Activity Entry"},
		},
	}

	for _, tc := range targets {
		id := tc.id
		s, ok := byID[id]
		if !ok {
			all := make([]string, 0, len(byID))
			for k := range byID {
				all = append(all, k)
			}
			sort.Strings(all)
			t.Fatalf("missing bundled skill %s (available=%v)", id, all)
		}
		assertRichFrontmatter(t, bundled, s, tc.category, tc.mustContain)
	}

	if _, err := fs.ReadFile(bundled, "research/structured_paper_reading.md"); err != nil {
		t.Fatalf("expected bundled file exists: %v", err)
	}
}

func assertRichFrontmatter(t *testing.T, bundled fs.FS, s Skill, wantCategory string, mustContain []string) {
	t.Helper()
	if s.Version != 1 {
		t.Fatalf("%s: version mismatch: %d", s.ID, s.Version)
	}
	if s.Author != "system" {
		t.Fatalf("%s: author mismatch: %q", s.ID, s.Author)
	}
	if s.Name != strings.TrimPrefix(filepath.Base(s.ID), "/") {
		t.Fatalf("%s: name should match filename stem, got %q", s.ID, s.Name)
	}
	if s.Category != wantCategory {
		t.Fatalf("%s: category mismatch: want %q got %q", s.ID, wantCategory, s.Category)
	}
	if s.Description == "" || s.WhenToUse == "" {
		t.Fatalf("%s: expected non-empty description/when_to_use", s.ID)
	}
	if len(s.AllowedTools) == 0 {
		t.Fatalf("%s: expected non-empty allowed-tools", s.ID)
	}
	assertAllowedToolsResearchCompatible(t, s.ID, s.AllowedTools)
	if s.Agent != "research" {
		t.Fatalf("%s: agent mismatch: %q", s.ID, s.Agent)
	}
	if s.Effort == "" {
		t.Fatalf("%s: expected effort", s.ID)
	}
	if s.UserInvocable || !s.IsModelInvocable() || s.Exposure != SkillExposureImplicit {
		t.Fatalf("%s: expected implicit/model-invocable skill, got exposure=%q user=%t model=%t", s.ID, s.Exposure, s.UserInvocable, s.IsModelInvocable())
	}
	if s.Source != "builtin" {
		t.Fatalf("%s: source mismatch: %q", s.ID, s.Source)
	}
	rawPath := s.ID + ".md"
	raw, err := fs.ReadFile(bundled, rawPath)
	if err != nil {
		t.Fatalf("%s: read raw markdown: %v", s.ID, err)
	}
	rawText := string(raw)
	if !strings.Contains(rawText, "version: 1") {
		t.Fatalf("%s: expected explicit frontmatter version: 1", s.ID)
	}
	for _, req := range mustContain {
		if !strings.Contains(rawText, req) {
			t.Fatalf("%s: expected content marker %q", s.ID, req)
		}
	}
}

func repoSkillsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return filepath.Join(repoRoot, "data", "skills")
}

func assertAllowedToolsResearchCompatible(t *testing.T, skillID string, tools []string) {
	t.Helper()
	// Mirrors the leader-side research profile used when user-invocable skill
	// commands apply runtime allowed-tool overrides in research mode.
	valid := map[string]struct{}{
		"ScholarSearch":    {},
		"KBTree":           {},
		"KBQuery":          {},
		"KBSearch":         {},
		"KBList":           {},
		"View":             {},
		"Glob":             {},
		"Grep":             {},
		"AskUser":          {},
		"SkillQuery":       {},
		"ToolSearch":       {},
		"Task":             {},
		"ResearchControl":  {},
		"ResearchPipeline": {},
		"ResearchTask":     {},
		"ResearchMessage":  {},
		"WebSearch":        {},
		"WebFetch":         {},
		"DocxValidate":     {},
	}
	for _, name := range tools {
		if _, ok := valid[name]; !ok {
			t.Fatalf("%s: allowed-tool %q is not research-mode compatible", skillID, name)
		}
	}
}

func writeSkillFile(t *testing.T, path string, name string, category string, instruction string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(skillMarkdown(name, category, instruction, "user")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func skillMarkdown(name string, category string, instruction string, author string) string {
	return "---\n" +
		"name: \"" + name + "\"\n" +
		"description: \"" + name + "\"\n" +
		"category: \"" + category + "\"\n" +
		"version: 1\n" +
		"author: \"" + author + "\"\n" +
		"---\n" +
		instruction + "\n"
}

func skillIDs(skills []Skill) []string {
	ids := make([]string, 0, len(skills))
	for _, s := range skills {
		ids = append(ids, s.ID)
	}
	return ids
}

func assertLoadedInstruction(t *testing.T, s *service, id string, wantInstruction string) {
	t.Helper()
	skill, err := s.loadSkillByID(id)
	if err != nil {
		t.Fatalf("load %s error: %v", id, err)
	}
	if skill.ID != id {
		t.Fatalf("stable id mismatch for %s: got %s", id, skill.ID)
	}
	if skill.Instruction != wantInstruction {
		t.Fatalf("instruction mismatch for %s: want %q got %q", id, wantInstruction, skill.Instruction)
	}
}

func skillMetaIDs(metas []SkillMeta) []string {
	ids := make([]string, 0, len(metas))
	for _, m := range metas {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

func setupSkillbankTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "skillbank_test.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS skills_index (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			category TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]',
			meta_json TEXT NOT NULL DEFAULT '{}',
			resolved_source_id TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			author TEXT NOT NULL DEFAULT 'system',
			usage_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			file_path TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills_index(category)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_usage ON skills_index(usage_count DESC)`,
		`CREATE TABLE IF NOT EXISTS skill_sources (
			id TEXT PRIMARY KEY,
			skill_id TEXT NOT NULL,
			source_tier TEXT NOT NULL,
			source_kind TEXT NOT NULL,
			source_key TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			meta_json TEXT NOT NULL DEFAULT '{}',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_sources_skill_id ON skill_sources(skill_id)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS skills_fts USING fts5(
			id UNINDEXED,
			name,
			description,
			tags,
			content
		)`,
		`CREATE TRIGGER IF NOT EXISTS skills_fts_insert AFTER INSERT ON skills_index BEGIN
			INSERT INTO skills_fts(id, name, description, tags, content)
			VALUES (new.id, new.name, new.description, new.tags, '');
		END`,
		`CREATE TRIGGER IF NOT EXISTS skills_fts_update AFTER UPDATE ON skills_index BEGIN
			DELETE FROM skills_fts WHERE id = old.id;
			INSERT INTO skills_fts(id, name, description, tags, content)
			VALUES (new.id, new.name, new.description, new.tags, '');
		END`,
		`CREATE TRIGGER IF NOT EXISTS skills_fts_delete AFTER DELETE ON skills_index BEGIN
			DELETE FROM skills_fts WHERE id = old.id;
		END`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("exec schema statement failed: %v\nsql: %s", err, stmt)
		}
	}
	return conn
}
