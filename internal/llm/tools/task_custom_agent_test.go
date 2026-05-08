package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/db"
	agentcustom "github.com/Nahasma/openscholar-public/internal/llm/agent/custom"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
	"github.com/pressly/goose/v3"
)

func TestTaskCustomAgentExplicitEmptyToolsHasNoTools(t *testing.T) {
	dir := t.TempDir()
	writeCustomAgentForTaskTest(t, dir, "no-tools", `---
name: no-tools
description: no tool access
tools: []
---
Run without tools.
`)

	_, q := setupTaskCustomAgentTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")

	var captured []string
	taskTool := &taskTool{
		permissions: perms,
		sessions:    sessions,
		messages:    messages,
		runAgent: func(_ context.Context, _ config.AgentName, _ string, _ string, agentTools []BaseTool) (string, error) {
			for _, tool := range agentTools {
				captured = append(captured, tool.Info().Name)
			}
			return "ok", nil
		},
		agents: agentcustom.NewRegistry(dir),
	}
	toolCtx := context.WithValue(context.Background(), SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-custom-empty")
	input, _ := json.Marshal(map[string]string{
		"description": "custom",
		"prompt":      "run",
		"agent_type":  "no-tools",
	})
	resp, err := taskTool.Run(toolCtx, ToolCall{ID: "tc-custom-empty", Name: "Task", Input: string(input)})
	if err != nil {
		t.Fatalf("task run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("task returned error: %s", resp.Content)
	}
	if len(captured) != 0 {
		t.Fatalf("expected explicit tools: [] to produce no agent tools, got %v", captured)
	}
}

func TestTaskCustomAgentOmittedToolsUsesGeneralToolset(t *testing.T) {
	dir := t.TempDir()
	writeCustomAgentForTaskTest(t, dir, "inherit-tools", `---
name: inherit-tools
description: inherit tool access
---
Run with inherited tools.
`)

	_, q := setupTaskCustomAgentTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")

	var captured []string
	taskTool := &taskTool{
		permissions: perms,
		sessions:    sessions,
		messages:    messages,
		runAgent: func(_ context.Context, _ config.AgentName, _ string, _ string, agentTools []BaseTool) (string, error) {
			for _, tool := range agentTools {
				captured = append(captured, tool.Info().Name)
			}
			return "ok", nil
		},
		agents: agentcustom.NewRegistry(dir),
	}
	toolCtx := context.WithValue(context.Background(), SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-custom-inherit")
	input, _ := json.Marshal(map[string]string{
		"description": "custom",
		"prompt":      "run",
		"agent_type":  "inherit-tools",
	})
	resp, err := taskTool.Run(toolCtx, ToolCall{ID: "tc-custom-inherit", Name: "Task", Input: string(input)})
	if err != nil {
		t.Fatalf("task run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("task returned error: %s", resp.Content)
	}
	if len(captured) == 0 {
		t.Fatalf("expected omitted tools to inherit general toolset")
	}
}

func TestTaskCustomAgentOmittedToolsUsesReadOnlyDefaultsWhenOrchestrationEnabled(t *testing.T) {
	dir := t.TempDir()
	writeCustomAgentForTaskTest(t, dir, "readonly-default", `---
name: readonly-default
description: should be read only by default in orchestration mode
---
Run with inherited tools.
`)

	_, q := setupTaskCustomAgentTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")

	var captured []string
	var childSessionID string
	taskTool := &taskTool{
		permissions: perms,
		sessions:    sessions,
		messages:    messages,
		runAgent: func(_ context.Context, _ config.AgentName, sid string, _ string, agentTools []BaseTool) (string, error) {
			childSessionID = sid
			for _, tool := range agentTools {
				captured = append(captured, tool.Info().Name)
			}
			return "ok", nil
		},
		agents:   agentcustom.NewRegistry(dir),
		profiles: NewSubagentProfileResolver(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}}),
	}
	toolCtx := context.WithValue(context.Background(), SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-custom-readonly")
	input, _ := json.Marshal(map[string]string{
		"description": "custom",
		"prompt":      "run",
		"agent_type":  "readonly-default",
	})
	resp, err := taskTool.Run(toolCtx, ToolCall{ID: "tc-custom-readonly", Name: "Task", Input: string(input)})
	if err != nil {
		t.Fatalf("task run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("task returned error: %s", resp.Content)
	}
	expected := []string{"View", "Glob", "Grep"}
	if len(captured) != len(expected) {
		t.Fatalf("expected %d tools, got %v", len(expected), captured)
	}
	for i := range expected {
		if captured[i] != expected[i] {
			t.Fatalf("expected tools %v, got %v", expected, captured)
		}
	}
	if perms.SessionMode(childSessionID) != permission.ModePlan {
		t.Fatalf("expected child session mode plan, got %q", perms.SessionMode(childSessionID))
	}
}

func TestTaskV2ProfileVerifyPolicyAppliesToCreate(t *testing.T) {
	_, q := setupTaskCustomAgentTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(context.Background(), "parent")
	var mu sync.Mutex
	verifyCalled := false
	taskTool := &taskTool{
		permissions: perms,
		sessions:    sessions,
		messages:    messages,
		registry:    reg,
		runAgent: func(_ context.Context, agentName config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			if agentName == config.AgentVerify {
				mu.Lock()
				verifyCalled = true
				mu.Unlock()
				return "all checks passed\nVERDICT: PASS", nil
			}
			return "worker result", nil
		},
		profiles: NewSubagentProfileResolver(&config.Config{
			SubagentOrchestration: config.SubagentOrchestrationConfig{
				Enabled: true,
				Profiles: map[string]config.SubagentProfileConfig{
					"general": {VerifyPolicy: "required"},
				},
			},
		}),
	}
	toolCtx := context.WithValue(context.Background(), SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-profile-verify")
	input, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "task-profile-verify",
		"description": "profile verify",
		"prompt":      "run",
		"agent_type":  "general",
		"write_set":   []string{"tmp/profile-verify.txt"},
	})
	resp, err := taskTool.Run(toolCtx, ToolCall{ID: "tc-profile-verify", Name: "Task", Input: string(input)})
	if err != nil {
		t.Fatalf("task run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("task returned error: %s", resp.Content)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := reg.Get("task-profile-verify")
		if ok && st.TaskMeta().Status == task.StatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st, ok := reg.Get("task-profile-verify")
	if !ok {
		t.Fatal("task missing")
	}
	sub, ok := st.(*task.SubtaskState)
	if !ok {
		t.Fatalf("expected SubtaskState, got %T", st)
	}
	if sub.TaskMeta().Status != task.StatusCompleted {
		t.Fatalf("task status=%s, want completed; last error=%s", sub.TaskMeta().Status, sub.LastError)
	}
	if sub.VerifyPolicy != "required" || sub.VerifyStatus != task.VerifyStatusPassed || sub.VerifyVerdict != "PASS" || sub.VerifyResult != "all checks passed" {
		t.Fatalf("profile verify policy not applied: policy=%q status=%q verdict=%q result=%q", sub.VerifyPolicy, sub.VerifyStatus, sub.VerifyVerdict, sub.VerifyResult)
	}
	mu.Lock()
	called := verifyCalled
	mu.Unlock()
	if !called {
		t.Fatal("expected verify agent to run")
	}
}

func writeCustomAgentForTaskTest(t *testing.T, projectDir string, name string, body string) {
	t.Helper()
	path := filepath.Join(projectDir, ".openscholar", "agents", name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir custom agent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}
}

var taskCustomAgentGooseMu sync.Mutex

func setupTaskCustomAgentTestDB(t *testing.T) (*sql.DB, db.Querier) {
	t.Helper()
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			t.Fatalf("set pragma %s: %v", pragma, err)
		}
	}
	taskCustomAgentGooseMu.Lock()
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		taskCustomAgentGooseMu.Unlock()
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		taskCustomAgentGooseMu.Unlock()
		t.Fatalf("run migrations: %v", err)
	}
	taskCustomAgentGooseMu.Unlock()
	return conn, db.New(conn)
}
