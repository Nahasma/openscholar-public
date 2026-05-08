package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/openscholar/openscholar/internal/skillbank"
)

var bundledFSMu sync.Mutex

func TestSkillQueryTool_SearchAndViewModes(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	writeSkillFileForQueryTest(t, filepath.Join(userDir, "workflow", "search_target.md"), `---
name: "search target"
description: "metadata search target"
category: "workflow"
tags: ["query"]
when_to_use: "for search mode"
allowed-tools: ["SkillQuery"]
agent: "coder"
effort: "medium"
model: "gpt-5.4"
user-invocable: true
disable-model-invocation: true
paths: ["**/*.md"]
platforms: ["darwin"]
source: "bundle"
---
full instruction body for view mode
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillQueryTool(svc)

	searchInput := mustJSON(t, map[string]any{"query": "for search mode", "mode": "search", "limit": 1})
	searchCall := ToolCall{Name: "SkillQuery", Input: searchInput}
	searchResp, err := tool.Run(context.Background(), searchCall)
	if err != nil {
		t.Fatalf("search run error: %v", err)
	}
	if searchResp.IsError {
		t.Fatalf("unexpected search error response: %s", searchResp.Content)
	}
	if !strings.Contains(searchResp.Content, "No model-invocable skills matched") {
		t.Fatalf("search response should be filtered by model-invocable, got:\n%s", searchResp.Content)
	}
	if strings.Contains(searchResp.Content, "full instruction body for view mode") {
		t.Fatalf("search mode should not include full instruction body")
	}

	viewInput := mustJSON(t, map[string]any{"mode": "view", "id": "workflow/search_target"})
	viewCall := ToolCall{Name: "SkillQuery", Input: viewInput}
	viewResp, err := tool.Run(context.Background(), viewCall)
	if err != nil {
		t.Fatalf("view run error: %v", err)
	}
	if !viewResp.IsError {
		t.Fatalf("expected non-model-invocable skill view to be blocked")
	}

	viewInput = mustJSON(t, map[string]any{"mode": "search", "query": "for search mode", "include_explicit": true})
	viewCall = ToolCall{Name: "SkillQuery", Input: viewInput}
	viewResp, err = tool.Run(context.Background(), viewCall)
	if err != nil {
		t.Fatalf("explicit-included search run error: %v", err)
	}
	if viewResp.IsError {
		t.Fatalf("unexpected explicit-included search error response: %s", viewResp.Content)
	}
	if !strings.Contains(viewResp.Content, "No model-invocable skills matched") {
		t.Fatalf("search mode should keep explicit-only skills hidden from model-facing results, got:\n%s", viewResp.Content)
	}
	if strings.Contains(viewResp.Content, "workflow/search_target") || strings.Contains(viewResp.Content, "full instruction body for view mode") {
		t.Fatalf("search mode leaked non-model-invocable skill content, got:\n%s", viewResp.Content)
	}
}

func TestSkillQueryTool_BundledResearchSkills_DiscoveryQueries(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()

	withBundledSkillsFS(t, func() {
		svc := skillbank.NewService(conn, userDir, nil)
		tool := NewSkillQueryTool(svc)

		cases := []struct {
			query     string
			expectTop string
		}{
			{query: "structured_paper_reading", expectTop: "research/structured_paper_reading"},
			{query: "论文精读", expectTop: "research/structured_paper_reading"},
			{query: "结构化精读论文", expectTop: "research/structured_paper_reading"},
			{query: "literature_gap_mapping", expectTop: "research/literature_gap_mapping"},
			{query: "文献综述 研究空白", expectTop: "research/literature_gap_mapping"},
			{query: "research_weekly_report", expectTop: "workflow/research_weekly_report"},
			{query: "科研周报", expectTop: "workflow/research_weekly_report"},
			{query: "检查这张图是否误导", expectTop: "research/figure_quality_review"},
			{query: "规划论文图组", expectTop: "research/visual_storyboard"},
			{query: "图形摘要", expectTop: "research/graphical_abstract"},
			{query: "论文 story arc", expectTop: "writing/paper_argument_outline"},
			{query: "有证据支撑的段落", expectTop: "writing/claim_supported_paragraph"},
			{query: "复现实验 seed 参数", expectTop: "workflow/reproducibility_triage"},
			{query: "科研日志", expectTop: "workflow/research_activity_log"},
		}

		for _, tc := range cases {
			call := ToolCall{
				Name:  "SkillQuery",
				Input: mustJSON(t, map[string]any{"query": tc.query, "mode": "search", "limit": 3}),
			}
			resp, err := tool.Run(context.Background(), call)
			if err != nil {
				t.Fatalf("search run error for %q: %v", tc.query, err)
			}
			if resp.IsError {
				t.Fatalf("unexpected search error for %q: %s", tc.query, resp.Content)
			}
			if top := topSkillID(resp.Content); top != tc.expectTop {
				t.Fatalf("query %q expected top hit %q, got %q\nresponse:\n%s", tc.query, tc.expectTop, top, resp.Content)
			}
		}

		negative := ToolCall{
			Name:  "SkillQuery",
			Input: mustJSON(t, map[string]any{"query": "画一个系统架构图", "mode": "search", "limit": 3}),
		}
		negativeResp, err := tool.Run(context.Background(), negative)
		if err != nil {
			t.Fatalf("negative search run error: %v", err)
		}
		if negativeResp.IsError {
			t.Fatalf("unexpected negative search error response: %s", negativeResp.Content)
		}
		top := topSkillID(negativeResp.Content)
		if top != "research/d2_architecture" && top != "research/mermaid_diagrams" && top != "research/figure_routing" {
			t.Fatalf("negative query expected diagram skill top hit, got %q\nresponse:\n%s", top, negativeResp.Content)
		}
		for _, shouldNot := range []string{
			"research/structured_paper_reading",
			"research/literature_gap_mapping",
			"workflow/research_weekly_report",
		} {
			if strings.Contains(negativeResp.Content, "["+shouldNot+"]") {
				t.Fatalf("negative query should not include %s, got:\n%s", shouldNot, negativeResp.Content)
			}
		}
	})
}

func withBundledSkillsFS(t *testing.T, fn func()) {
	t.Helper()
	bundledFSMu.Lock()
	oldBundled := skillbank.BundledFS
	skillbank.BundledFS = os.DirFS(repoSkillsDirForQueryTest(t))
	if _, err := fs.ReadFile(skillbank.BundledFS, "research/structured_paper_reading.md"); err != nil {
		skillbank.BundledFS = oldBundled
		bundledFSMu.Unlock()
		t.Fatalf("bundled fs fixture missing: %v", err)
	}
	t.Cleanup(func() {
		skillbank.BundledFS = oldBundled
		bundledFSMu.Unlock()
	})
	fn()
}

func repoSkillsDirForQueryTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	return filepath.Join(repoRoot, "data", "skills")
}

func topSkillID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		start := strings.LastIndex(line, "[")
		end := strings.LastIndex(line, "]")
		if start >= 0 && end > start+1 {
			return line[start+1 : end]
		}
	}
	return ""
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(b)
}

func writeSkillFileForQueryTest(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func setupSkillQueryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "skill_query_test.db")
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
