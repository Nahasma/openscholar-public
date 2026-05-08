package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
)

var gooseMu sync.Mutex

func setupTaskP2TestDB(t *testing.T) db.Querier {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	gooseMu.Lock()
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		gooseMu.Unlock()
		t.Fatalf("failed to set dialect: %v", err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		gooseMu.Unlock()
		t.Fatalf("failed to run migrations: %v", err)
	}
	gooseMu.Unlock()
	return db.New(conn)
}

func loadTaskP2Config(t *testing.T, subagentConfig string) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	wd := t.TempDir()
	cfg := `{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"model": "qwen2.5-coder:latest"}
  },
  "subagent_orchestration": ` + subagentConfig + `
}`
	if err := os.MkdirAll(filepath.Dir(config.ConfigFilePath(wd)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(config.ConfigFilePath(wd), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(wd); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func waitTaskDone(t *testing.T, reg *task.Registry, id string) *task.SubtaskState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := reg.Get(id)
		if ok && (st.TaskMeta().Status == task.StatusCompleted || st.TaskMeta().Status == task.StatusFailed || st.TaskMeta().Status == task.StatusCanceled) {
			sub, _ := st.(*task.SubtaskState)
			return sub
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish", id)
	return nil
}

func TestTaskP2WriteSetRequiredOnlyWhenEnabled(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-write-set")

	mkCall := func(taskID string) ToolCall {
		in, _ := json.Marshal(map[string]any{
			"action":      "create",
			"task_id":     taskID,
			"description": "write",
			"prompt":      "do write",
			"agent_type":  "general",
		})
		return ToolCall{ID: "tc-" + taskID, Name: "Task", Input: string(in)}
	}

	baseCfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}}
	toolEnabled := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) { return "ok", nil }).(*taskTool)
	toolEnabled.profiles = NewSubagentProfileResolver(baseCfg)
	toolEnabled.scheduler = newSubagentScheduler(baseCfg)

	resp, err := toolEnabled.Run(toolCtx, mkCall("p2-reject"))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "write_set is required") {
		t.Fatalf("expected write_set requirement error, got %#v", resp)
	}

	legacyCfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: false}}
	toolDisabled := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) { return "ok", nil }).(*taskTool)
	toolDisabled.profiles = NewSubagentProfileResolver(legacyCfg)
	toolDisabled.scheduler = newSubagentScheduler(legacyCfg)
	resp, err = toolDisabled.Run(toolCtx, mkCall("p2-allow"))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("legacy mode should allow missing write_set: %s", resp.Content)
	}
}

func TestTaskP2ReadOnlyProfileDoesNotRequireWriteSet(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-readonly-profile")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{
		Enabled: true,
		Profiles: map[string]config.SubagentProfileConfig{
			"general": {AllowedTools: []string{"View"}},
		},
	}}
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return "ok", nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	in, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "p2-readonly-profile",
		"description": "read only",
		"prompt":      "read",
		"agent_type":  "general",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-readonly-profile", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("read-only profile should not require write_set: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-readonly-profile")
	if sub.TaskMeta().Status != task.StatusCompleted {
		t.Fatalf("task status=%s, want completed; last error=%s", sub.TaskMeta().Status, sub.LastError)
	}
}

func TestTaskP2ResultCapAndTruncatedFlag(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-cap")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true, DefaultResultMaxChars: 5}}
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return "123456789", nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	in, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "p2-cap-1",
		"description": "write",
		"prompt":      "do write",
		"agent_type":  "general",
		"write_set":   []string{"a.txt"},
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-cap", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("create failed: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-cap-1")
	if sub.Result != "12345" || !sub.ResultTruncated {
		t.Fatalf("unexpected capped result: %q truncated=%v", sub.Result, sub.ResultTruncated)
	}

	readIn, _ := json.Marshal(map[string]any{"action": "read", "task_id": "p2-cap-1"})
	readResp, err := tool.Run(toolCtx, ToolCall{ID: "tc-cap-read", Name: "Task", Input: string(readIn)})
	if err != nil || readResp.IsError {
		t.Fatalf("read failed: err=%v resp=%s", err, readResp.Content)
	}
	if !strings.Contains(readResp.Content, `"result_truncated":true`) {
		t.Fatalf("expected result_truncated flag in read view, got %s", readResp.Content)
	}
}

func TestTaskP2ResultCapIgnoredWhenDisabled(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-cap-disabled")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: false}}
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return "123456789", nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	in, _ := json.Marshal(map[string]any{
		"action":           "create",
		"task_id":          "p2-cap-disabled",
		"description":      "legacy result",
		"prompt":           "run",
		"agent_type":       "general",
		"result_max_chars": 5,
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-cap-disabled", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("create failed: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-cap-disabled")
	if sub.Result != "123456789" || sub.ResultTruncated {
		t.Fatalf("disabled mode should ignore result cap: result=%q truncated=%v", sub.Result, sub.ResultTruncated)
	}
}

func TestTaskP2SendPreservesWriteSet(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-send")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}}
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return "ok", nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	createIn, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "p2-send-1",
		"description": "write",
		"prompt":      "run one",
		"agent_type":  "general",
		"write_set":   []string{"internal/a.go"},
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-send-create", Name: "Task", Input: string(createIn)})
	if err != nil || resp.IsError {
		t.Fatalf("create failed: err=%v resp=%s", err, resp.Content)
	}
	waitTaskDone(t, reg, "p2-send-1")

	sendIn, _ := json.Marshal(map[string]any{
		"action":  "send",
		"task_id": "p2-send-1",
		"prompt":  "run two",
	})
	resp, err = tool.Run(toolCtx, ToolCall{ID: "tc-send-send", Name: "Task", Input: string(sendIn)})
	if err != nil || resp.IsError {
		t.Fatalf("send failed: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-send-1")
	if len(sub.WriteSet) != 1 || sub.WriteSet[0] != "internal/a.go" {
		t.Fatalf("write_set should be preserved on send, got %v", sub.WriteSet)
	}
}

func TestTaskP2SendUsesUpdatedResultCap(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-send-cap")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}}
	result := "initial"
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return result, nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	createIn, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "p2-send-cap",
		"description": "write",
		"prompt":      "run one",
		"agent_type":  "general",
		"write_set":   []string{"internal/cap.go"},
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-send-cap-create", Name: "Task", Input: string(createIn)})
	if err != nil || resp.IsError {
		t.Fatalf("create failed: err=%v resp=%s", err, resp.Content)
	}
	waitTaskDone(t, reg, "p2-send-cap")

	result = "abcdef"
	sendIn, _ := json.Marshal(map[string]any{
		"action":           "send",
		"task_id":          "p2-send-cap",
		"prompt":           "run two",
		"result_max_chars": 3,
	})
	resp, err = tool.Run(toolCtx, ToolCall{ID: "tc-send-cap-send", Name: "Task", Input: string(sendIn)})
	if err != nil || resp.IsError {
		t.Fatalf("send failed: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-send-cap")
	if sub.Result != "abc" || !sub.ResultTruncated {
		t.Fatalf("send result cap not applied: result=%q truncated=%v", sub.Result, sub.ResultTruncated)
	}
}

func TestTaskP2SendResultCapIgnoredWhenDisabled(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	toolCtx = context.WithValue(toolCtx, MessageIDContextKey, "msg-p2-send-cap-disabled")

	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: false}}
	result := "initial"
	tool := NewTaskTool(perms, sessions, messages, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return result, nil
	}).(*taskTool)
	tool.profiles = NewSubagentProfileResolver(cfg)
	tool.scheduler = newSubagentScheduler(cfg)

	createIn, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "p2-send-cap-disabled",
		"description": "legacy send result",
		"prompt":      "run one",
		"agent_type":  "general",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "tc-send-cap-disabled-create", Name: "Task", Input: string(createIn)})
	if err != nil || resp.IsError {
		t.Fatalf("create failed: err=%v resp=%s", err, resp.Content)
	}
	waitTaskDone(t, reg, "p2-send-cap-disabled")

	result = "abcdef"
	sendIn, _ := json.Marshal(map[string]any{
		"action":           "send",
		"task_id":          "p2-send-cap-disabled",
		"prompt":           "run two",
		"result_max_chars": 3,
	})
	resp, err = tool.Run(toolCtx, ToolCall{ID: "tc-send-cap-disabled-send", Name: "Task", Input: string(sendIn)})
	if err != nil || resp.IsError {
		t.Fatalf("send failed: err=%v resp=%s", err, resp.Content)
	}
	sub := waitTaskDone(t, reg, "p2-send-cap-disabled")
	if sub.Result != "abcdef" || sub.ResultTruncated {
		t.Fatalf("disabled send should ignore result cap: result=%q truncated=%v", sub.Result, sub.ResultTruncated)
	}
}

func TestBackgroundSubtaskLauncherRequiresWriteSetForWritableScheduledProfile(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	cfg := &config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}}
	launcher := NewBackgroundSubtaskLauncher(perms, sessions, func(context.Context, config.AgentName, string, string, []BaseTool) (string, error) {
		return "should not run", nil
	})
	_, err := launcher.Launch(ctx, BackgroundSubtaskSpec{
		TaskID:          "p2-launcher-missing-write-set",
		ParentSessionID: parent.ID,
		Description:     "missing write set",
		Prompt:          "run",
		AgentType:       "general",
		AgentName:       config.AgentGeneral,
		SessionMode:     permission.ModeDefault,
		VerifyPolicy:    "none",
		Scheduler:       newSubagentScheduler(cfg),
		Profile:         SubagentProfile{ID: "general", AgentType: "general", CanWriteFiles: true},
	})
	if err == nil || !strings.Contains(err.Error(), "write_set is required") {
		t.Fatalf("expected launcher write_set requirement, got %v", err)
	}
	if _, ok := reg.Get("p2-launcher-missing-write-set"); ok {
		t.Fatal("launcher should reject before registering task")
	}
}

func TestTaskP2TaskToolInstancesShareScheduler(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "global_max_concurrent": 1}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	started := make(chan string, 2)
	release := make(chan struct{})
	runner := func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		started <- sessionID
		<-release
		return "done", nil
	}

	toolA := NewTaskTool(perms, sessions, messages, runner)
	toolB := NewTaskTool(perms, sessions, messages, runner)
	createCall := func(taskID string) ToolCall {
		in, _ := json.Marshal(map[string]any{
			"action":      "create",
			"task_id":     taskID,
			"description": "explore",
			"prompt":      "inspect",
			"agent_type":  "explore",
		})
		return ToolCall{ID: "tc-" + taskID, Name: "Task", Input: string(in)}
	}

	if resp, err := toolA.Run(toolCtx, createCall("shared-scheduler-a")); err != nil || resp.IsError {
		t.Fatalf("create first task resp=%+v err=%v", resp, err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}

	if resp, err := toolB.Run(toolCtx, createCall("shared-scheduler-b")); err != nil || resp.IsError {
		t.Fatalf("create second task resp=%+v err=%v", resp, err)
	}
	select {
	case sessionID := <-started:
		t.Fatalf("second task started before shared scheduler released global slot: %s", sessionID)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	waitTaskDone(t, reg, "shared-scheduler-a")
	waitTaskDone(t, reg, "shared-scheduler-b")
}

func TestTaskP2NestedCreateRequiresSpawningProfile(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "max_nested_depth": 2}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	root, _ := sessions.Create(ctx, "root")
	child, err := sessions.CreateTaskSession(ctx, "general-child", root.ID, "general child")
	if err != nil {
		t.Fatalf("create child session: %v", err)
	}
	parentState := task.NewSubtaskState(task.Meta{
		ID:             "general-parent-task",
		Label:          "general parent",
		SessionID:      root.ID,
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: true,
	}, root.ID, child.ID, "general", "general parent", "", "none")
	reg.Register(parentState)

	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		return "should not run", nil
	})
	toolCtx := context.WithValue(ctx, SessionIDContextKey, child.ID)
	in, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "nested-not-allowed",
		"description": "nested",
		"prompt":      "try nested",
		"agent_type":  "explore",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "nested-not-allowed", Name: "Task", Input: string(in)})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "not allowed to spawn Task subtasks") {
		t.Fatalf("expected spawn policy rejection, got %+v", resp)
	}
}

func TestTaskP2NestedCreateHonorsMaxDepth(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "max_nested_depth": 2}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	root, _ := sessions.Create(ctx, "root")
	child, err := sessions.CreateTaskSession(ctx, "leader-child", root.ID, "leader child")
	if err != nil {
		t.Fatalf("create child session: %v", err)
	}
	grandchild, err := sessions.CreateTaskSession(ctx, "coordinator-grandchild", child.ID, "coordinator grandchild")
	if err != nil {
		t.Fatalf("create grandchild session: %v", err)
	}
	parentState := task.NewSubtaskState(task.Meta{
		ID:             "coordinator-parent-task",
		Label:          "coordinator parent",
		SessionID:      child.ID,
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: true,
	}, child.ID, grandchild.ID, "coordinator", "coordinator parent", "", "required")
	reg.Register(parentState)

	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		return "should not run", nil
	})
	toolCtx := context.WithValue(ctx, SessionIDContextKey, grandchild.ID)
	in, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "nested-too-deep",
		"description": "nested",
		"prompt":      "try nested",
		"agent_type":  "explore",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "nested-too-deep", Name: "Task", Input: string(in)})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "max nested subagent depth exceeded") {
		t.Fatalf("expected max depth rejection, got %+v", resp)
	}
}

func TestTaskP2LegacySyncHonorsNestedSpawnPolicyWhenEnabled(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "max_nested_depth": 2}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	root, _ := sessions.Create(ctx, "root")
	child, err := sessions.CreateTaskSession(ctx, "legacy-general-child", root.ID, "general child")
	if err != nil {
		t.Fatalf("create child session: %v", err)
	}
	parentState := task.NewSubtaskState(task.Meta{
		ID:             "legacy-general-parent-task",
		Label:          "legacy general parent",
		SessionID:      root.ID,
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: true,
	}, root.ID, child.ID, "general", "legacy general parent", "", "none")
	reg.Register(parentState)

	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		return "should not run", nil
	})
	toolCtx := context.WithValue(ctx, SessionIDContextKey, child.ID)
	in, _ := json.Marshal(map[string]any{
		"description": "legacy nested",
		"prompt":      "try legacy nested",
		"agent_type":  "explore",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "legacy-nested-not-allowed", Name: "Task", Input: string(in)})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "not allowed to spawn Task subtasks") {
		t.Fatalf("expected legacy spawn policy rejection, got %+v", resp)
	}
}

func TestTaskP2LegacySyncRequiresWriteSetForWritableProfileWhenEnabled(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		return "should not run", nil
	})
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	in, _ := json.Marshal(map[string]any{
		"description": "legacy write",
		"prompt":      "do write",
		"agent_type":  "general",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "legacy-write-missing-set", Name: "Task", Input: string(in)})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "write_set is required") {
		t.Fatalf("expected legacy write_set rejection, got %+v", resp)
	}
}

func TestTaskP2LegacySyncRegistersParentStateForNestedPolicy(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "max_nested_depth": 3}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	var legacyChildSession string
	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		legacyChildSession = sessionID
		return "legacy done", nil
	})
	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	in, _ := json.Marshal(map[string]any{
		"description": "legacy read-only",
		"prompt":      "inspect",
		"agent_type":  "explore",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "legacy-parent-state", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("legacy task failed: err=%v resp=%+v", err, resp)
	}
	if legacyChildSession == "" {
		t.Fatal("legacy child session was not captured")
	}
	state, ok := reg.Get("legacy-parent-state")
	if !ok {
		t.Fatal("legacy sync should register a task state when orchestration is enabled")
	}
	sub, ok := state.(*task.SubtaskState)
	if !ok {
		t.Fatalf("legacy state type = %T, want SubtaskState", state)
	}
	if sub.ChildSessionID != legacyChildSession || sub.AgentType != "explore" || sub.TaskMeta().Status != task.StatusCompleted {
		t.Fatalf("unexpected legacy state: child=%q agent=%q status=%s", sub.ChildSessionID, sub.AgentType, sub.TaskMeta().Status)
	}

	nestedTool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		return "should not run", nil
	})
	nestedCtx := context.WithValue(ctx, SessionIDContextKey, legacyChildSession)
	nestedIn, _ := json.Marshal(map[string]any{
		"action":      "create",
		"task_id":     "legacy-child-nested-not-allowed",
		"description": "nested",
		"prompt":      "try nested",
		"agent_type":  "explore",
	})
	nestedResp, err := nestedTool.Run(nestedCtx, ToolCall{ID: "legacy-child-nested-not-allowed", Name: "Task", Input: string(nestedIn)})
	if err != nil {
		t.Fatalf("run nested task: %v", err)
	}
	if !nestedResp.IsError || !strings.Contains(nestedResp.Content, "not allowed to spawn Task subtasks") {
		t.Fatalf("expected nested rejection for actual legacy child, got %+v", nestedResp)
	}
}

func TestTaskP2SendResumesCoordinatorAfterChildNotification(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": true, "max_nested_depth": 3}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	child, err := sessions.CreateTaskSession(ctx, "send-coordinator-child", parent.ID, "coordinator child")
	if err != nil {
		t.Fatalf("create coordinator session: %v", err)
	}
	now := time.Now()
	endedAt := now
	state := task.NewSubtaskState(task.Meta{
		ID:             "send-coordinator",
		Label:          "send coordinator",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      now.Add(-time.Second),
		EndedAt:        &endedAt,
		IsBackgrounded: true,
		Notified:       true,
	}, parent.ID, child.ID, "coordinator", "send coordinator", "", "none")
	reg.Register(state)

	workerTaskID := "send-coordinator-worker"
	coordinatorCalls := 0
	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		switch agentName {
		case config.AgentCoordinator:
			coordinatorCalls++
			if coordinatorCalls == 1 {
				for _, tool := range agentTools {
					if tool.Info().Name != "Task" {
						continue
					}
					toolCtx := context.WithValue(ctx, SessionIDContextKey, sessionID)
					in, _ := json.Marshal(map[string]any{
						"action":      "create",
						"task_id":     workerTaskID,
						"description": "worker",
						"prompt":      "inspect",
						"agent_type":  "explore",
					})
					resp, err := tool.Run(toolCtx, ToolCall{ID: workerTaskID, Name: "Task", Input: string(in)})
					if err != nil {
						return "", err
					}
					if resp.IsError {
						return "", fmt.Errorf("%s", resp.Content)
					}
					return "waiting for worker", nil
				}
				return "", fmt.Errorf("Task tool not found")
			}
			reg.MarkNotificationDelivered(workerTaskID)
			return "resumed after worker", nil
		case config.AgentExplore:
			return "worker done", nil
		default:
			return "", fmt.Errorf("unexpected agent %s", agentName)
		}
	})

	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	in, _ := json.Marshal(map[string]any{
		"action":        "send",
		"task_id":       "send-coordinator",
		"prompt":        "continue",
		"agent_type":    "coordinator",
		"verify_policy": "none",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "send-coordinator-run", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("send failed: err=%v resp=%+v", err, resp)
	}
	finalState := waitTaskDone(t, reg, "send-coordinator")
	if coordinatorCalls < 2 {
		t.Fatalf("coordinator should resume after worker notification, calls=%d", coordinatorCalls)
	}
	if finalState.Result != "resumed after worker" {
		t.Fatalf("final coordinator result = %q, want resumed result", finalState.Result)
	}
	waitTaskDone(t, reg, workerTaskID)
}

func TestTaskP2SendDoesNotWaitForNotificationWhenOrchestrationDisabled(t *testing.T) {
	loadTaskP2Config(t, `{"enabled": false}`)

	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	messages := message.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()
	SetTaskRuntime(reg, nil)
	defer SetTaskRuntime(nil, nil)

	parent, _ := sessions.Create(ctx, "parent")
	child, err := sessions.CreateTaskSession(ctx, "send-disabled-coordinator-child", parent.ID, "coordinator child")
	if err != nil {
		t.Fatalf("create coordinator session: %v", err)
	}
	now := time.Now()
	endedAt := now
	state := task.NewSubtaskState(task.Meta{
		ID:             "send-disabled-coordinator",
		Label:          "send disabled coordinator",
		SessionID:      parent.ID,
		Status:         task.StatusCompleted,
		StartedAt:      now.Add(-time.Second),
		EndedAt:        &endedAt,
		IsBackgrounded: true,
		Notified:       true,
	}, parent.ID, child.ID, "coordinator", "send disabled coordinator", "", "none")
	reg.Register(state)

	workerTaskID := "send-disabled-worker"
	coordinatorCalls := 0
	tool := NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error) {
		switch agentName {
		case config.AgentCoordinator:
			coordinatorCalls++
			for _, tool := range agentTools {
				if tool.Info().Name != "Task" {
					continue
				}
				toolCtx := context.WithValue(ctx, SessionIDContextKey, sessionID)
				in, _ := json.Marshal(map[string]any{
					"action":      "create",
					"task_id":     workerTaskID,
					"description": "worker",
					"prompt":      "inspect",
					"agent_type":  "explore",
				})
				resp, err := tool.Run(toolCtx, ToolCall{ID: workerTaskID, Name: "Task", Input: string(in)})
				if err != nil {
					return "", err
				}
				if resp.IsError {
					return "", fmt.Errorf("%s", resp.Content)
				}
				return "waiting in disabled mode", nil
			}
			return "", fmt.Errorf("Task tool not found")
		case config.AgentExplore:
			return "worker done", nil
		default:
			return "", fmt.Errorf("unexpected agent %s", agentName)
		}
	})

	toolCtx := context.WithValue(ctx, SessionIDContextKey, parent.ID)
	in, _ := json.Marshal(map[string]any{
		"action":        "send",
		"task_id":       "send-disabled-coordinator",
		"prompt":        "continue",
		"agent_type":    "coordinator",
		"verify_policy": "none",
	})
	resp, err := tool.Run(toolCtx, ToolCall{ID: "send-disabled-coordinator-run", Name: "Task", Input: string(in)})
	if err != nil || resp.IsError {
		t.Fatalf("send failed: err=%v resp=%+v", err, resp)
	}
	finalState := waitTaskDone(t, reg, "send-disabled-coordinator")
	if coordinatorCalls != 1 {
		t.Fatalf("disabled orchestration should not notification-resume coordinator, calls=%d", coordinatorCalls)
	}
	if finalState.Result != "waiting in disabled mode" {
		t.Fatalf("final coordinator result = %q, want disabled-mode first result", finalState.Result)
	}
	waitTaskDone(t, reg, workerTaskID)
}
