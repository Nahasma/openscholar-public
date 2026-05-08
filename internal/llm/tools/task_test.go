package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type noopIndexer struct{}

func (noopIndexer) BuildTree(_ context.Context, _ string, _ kb.IndexOptions) (*kb.IndexResult, error) {
	return &kb.IndexResult{}, nil
}

func mockAgentRunner(result string, delay time.Duration) tools.AgentRunner {
	return func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return result, nil
	}
}

type mockHookService struct {
	mu     sync.Mutex
	events []hooks.Event
}

func (m *mockHookService) Run(_ context.Context, event hooks.Event, _ hooks.Input) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockHookService) RunBlocking(_ context.Context, event hooks.Event, _ hooks.Input) error {
	return m.Run(context.Background(), event, hooks.Input{})
}

func (m *mockHookService) RunAsync(_ context.Context, event hooks.Event, _ hooks.Input) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockHookService) Reload() error { return nil }

func (m *mockHookService) count(event hooks.Event) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.events {
		if e == event {
			n++
		}
	}
	return n
}

type cancelAwareHookService struct {
	mu     sync.Mutex
	events []hooks.Event
}

func (m *cancelAwareHookService) Run(ctx context.Context, event hooks.Event, _ hooks.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *cancelAwareHookService) RunBlocking(ctx context.Context, event hooks.Event, input hooks.Input) error {
	return m.Run(ctx, event, input)
}

func (m *cancelAwareHookService) RunAsync(ctx context.Context, event hooks.Event, _ hooks.Input) {
	if ctx.Err() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *cancelAwareHookService) Reload() error { return nil }

func (m *cancelAwareHookService) count(event hooks.Event) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.events {
		if e == event {
			n++
		}
	}
	return n
}

func waitForTaskStatus(t *testing.T, reg *task.Registry, taskID string, want task.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, ok := reg.Get(taskID)
		if ok && s.TaskMeta().Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s, ok := reg.Get(taskID)
	if !ok {
		t.Fatalf("task %s not found", taskID)
	}
	t.Fatalf("task %s status=%s, want %s", taskID, s.TaskMeta().Status, want)
}

func waitForHookCount(t *testing.T, counter interface{ count(hooks.Event) int }, event hooks.Event, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if counter.count(event) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hook %s count=%d, want >= %d", event, counter.count(event), want)
}

func TestTaskSessionIsolation(t *testing.T) {
	t.Run("child session has independent message history", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)

		parent, err := sessions.Create(ctx, "parent")
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}

		// Add message to parent
		_, err = messages.Create(ctx, parent.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "parent msg"}},
		})
		if err != nil {
			t.Fatalf("create parent message: %v", err)
		}

		// Create child session
		child, err := sessions.CreateTaskSession(ctx, "child-1", parent.ID, "task")
		if err != nil {
			t.Fatalf("create task session: %v", err)
		}

		// Child has no messages
		childMsgs, err := messages.List(ctx, child.ID)
		if err != nil {
			t.Fatalf("list child messages: %v", err)
		}
		if len(childMsgs) != 0 {
			t.Errorf("child should have 0 messages, got %d", len(childMsgs))
		}

		// Add message to child
		_, err = messages.Create(ctx, child.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "child msg"}},
		})
		if err != nil {
			t.Fatalf("create child message: %v", err)
		}

		// Parent still has only 1 message
		parentMsgs, err := messages.List(ctx, parent.ID)
		if err != nil {
			t.Fatalf("list parent messages: %v", err)
		}
		if len(parentMsgs) != 1 {
			t.Errorf("parent should have 1 message, got %d", len(parentMsgs))
		}
	})

	t.Run("parent only receives final result string", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()

		parent, _ := sessions.Create(ctx, "parent")
		expected := "analysis complete: 42 methods found"

		taskTool := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner(expected, 0))

		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-1")

		input, _ := json.Marshal(map[string]string{
			"description": "analyze",
			"prompt":      "find methods",
			"agent_type":  "general",
		})

		resp, err := taskTool.Run(toolCtx, tools.ToolCall{
			ID: "tc-1", Name: "Task", Input: string(input),
		})
		if err != nil {
			t.Fatalf("task tool failed: %v", err)
		}
		if resp.Content != expected {
			t.Errorf("expected %q, got %q", expected, resp.Content)
		}
	})

	t.Run("child session uses correct tool set", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()

		parent, _ := sessions.Create(ctx, "parent")

		var capturedToolCount int
		var capturedToolNames []string
		runner := func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			capturedToolCount = len(agentTools)
			capturedToolNames = capturedToolNames[:0]
			for _, tool := range agentTools {
				capturedToolNames = append(capturedToolNames, tool.Info().Name)
			}
			return "done", nil
		}

		taskTool := tools.NewTaskTool(perms, sessions, messages, runner)
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-2")
		runTask := func(id string, params map[string]string) {
			t.Helper()
			capturedToolCount = -1
			capturedToolNames = nil
			input, _ := json.Marshal(params)
			resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: id, Name: "Task", Input: string(input)})
			if err != nil {
				t.Fatalf("%s failed: %v", id, err)
			}
			if resp.IsError {
				t.Fatalf("%s returned tool error: %s", id, resp.Content)
			}
		}

		// General agent → full registry tools
		runTask("tc-g", map[string]string{
			"description": "general", "prompt": "do", "agent_type": "general",
		})
		if capturedToolCount != 10 {
			t.Errorf("general agent: expected 10 tools, got %d", capturedToolCount)
		}

		// Explore agent → 3 tools
		runTask("tc-e", map[string]string{
			"description": "explore", "prompt": "read", "agent_type": "explore",
		})
		if capturedToolCount != 3 {
			t.Errorf("explore agent: expected 3 tools, got %d", capturedToolCount)
		}

		// Plan agent → read-only 3 tools
		runTask("tc-p", map[string]string{
			"description": "plan", "prompt": "plan", "agent_type": "plan",
		})
		expectedPlanTools := []string{"View", "Glob", "Grep"}
		if strings.Join(capturedToolNames, ",") != strings.Join(expectedPlanTools, ",") {
			t.Errorf("plan agent tools: expected %v, got %v", expectedPlanTools, capturedToolNames)
		}

		// Verify agent → read-only 3 tools
		runTask("tc-v", map[string]string{
			"description": "verify", "prompt": "verify", "agent_type": "verify",
		})
		expectedVerifyTools := []string{"View", "Glob", "Grep"}
		if strings.Join(capturedToolNames, ",") != strings.Join(expectedVerifyTools, ",") {
			t.Errorf("verify agent tools: expected %v, got %v", expectedVerifyTools, capturedToolNames)
		}

		// Coordinator agent → read-only + Task delegation
		runTask("tc-c", map[string]string{
			"description": "coordinator", "prompt": "coordinate", "agent_type": "coordinator",
		})
		expectedCoordinatorTools := []string{"View", "Glob", "Grep", "Task"}
		if strings.Join(capturedToolNames, ",") != strings.Join(expectedCoordinatorTools, ",") {
			t.Errorf("coordinator agent tools: expected %v, got %v", expectedCoordinatorTools, capturedToolNames)
		}

		// Leader agent → read + write/edit + delegation
		runTask("tc-l", map[string]string{
			"description": "leader", "prompt": "coordinate", "agent_type": "leader",
		})
		if capturedToolCount != 6 {
			t.Errorf("leader agent: expected 6 tools, got %d", capturedToolCount)
		}
		expectedLeaderTools := []string{"View", "Edit", "Write", "Glob", "Grep", "Task"}
		if strings.Join(capturedToolNames, ",") != strings.Join(expectedLeaderTools, ",") {
			t.Errorf("leader agent tools: expected %v, got %v", expectedLeaderTools, capturedToolNames)
		}
	})

	t.Run("parent-child session hierarchy", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)

		parent, _ := sessions.Create(ctx, "parent")
		child, err := sessions.CreateTaskSession(ctx, "child-h", parent.ID, "hierarchy")
		if err != nil {
			t.Fatalf("create task session: %v", err)
		}

		if child.ParentSessionID != parent.ID {
			t.Errorf("ParentSessionID: expected %q, got %q", parent.ID, child.ParentSessionID)
		}

		fetched, err := sessions.Get(ctx, child.ID)
		if err != nil {
			t.Fatalf("get child: %v", err)
		}
		if fetched.ParentSessionID != parent.ID {
			t.Errorf("fetched ParentSessionID: expected %q, got %q", parent.ID, fetched.ParentSessionID)
		}
	})

	t.Run("concurrent child sessions are independent", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()

		parent, _ := sessions.Create(ctx, "parent")

		const n = 5
		var wg sync.WaitGroup
		results := make([]string, n)
		errs := make([]error, n)

		for i := range n {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				expected := fmt.Sprintf("result-%d", idx)
				taskTool := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner(expected, 10*time.Millisecond))

				toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
				toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, fmt.Sprintf("msg-c-%d", idx))

				input, _ := json.Marshal(map[string]string{
					"description": fmt.Sprintf("task %d", idx),
					"prompt":      fmt.Sprintf("do %d", idx),
					"agent_type":  "general",
				})
				resp, err := taskTool.Run(toolCtx, tools.ToolCall{
					ID: fmt.Sprintf("tc-c-%d", idx), Name: "Task", Input: string(input),
				})
				results[idx] = resp.Content
				errs[idx] = err
			}(i)
		}
		wg.Wait()

		for i := range n {
			if errs[i] != nil {
				t.Errorf("worker %d error: %v", i, errs[i])
			}
			expected := fmt.Sprintf("result-%d", i)
			if results[i] != expected {
				t.Errorf("worker %d: expected %q, got %q", i, expected, results[i])
			}
		}
	})

	t.Run("legacy task invokes runner and cleans prompt guard", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()

		parent, _ := sessions.Create(ctx, "parent")

		called := false
		runner := func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			called = true
			return "done", nil
		}

		taskTool := tools.NewTaskTool(perms, sessions, messages, runner)
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-aa")

		input, _ := json.Marshal(map[string]string{
			"description": "prompt guard", "prompt": "test", "agent_type": "general",
		})
		_, err := taskTool.Run(toolCtx, tools.ToolCall{
			ID: "tc-aa", Name: "Task", Input: string(input),
		})
		if err != nil {
			t.Fatalf("task failed: %v", err)
		}
		if !called {
			t.Error("runner was not called")
		}
		// Successful completion implies the per-child prompt guard cleanup ran.
	})

	t.Run("TaskV2 create/read/list/send", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		hooksMock := &mockHookService{}
		tools.SetTaskRuntime(reg, hooksMock)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2")

		runner1 := func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			_, _ = messages.Create(context.Background(), sessionID, message.CreateMessageParams{
				Role:  message.Assistant,
				Parts: []message.ContentPart{message.TextContent{Text: "v2 transcript " + prompt + "\n" + strings.Repeat("x", 2100)}},
			})
			return "v2-result-1", nil
		}
		taskTool := tools.NewTaskTool(perms, sessions, messages, runner1)
		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-1",
			"description": "analyze",
			"prompt":      "run one",
			"agent_type":  "general",
		})
		_, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-1", Name: "Task", Input: string(createInput)})
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		waitForTaskStatus(t, reg, "task-v2-1", task.StatusCompleted)

		readInput, _ := json.Marshal(map[string]string{
			"action":  "read",
			"task_id": "task-v2-1",
		})
		readResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-r", Name: "Task", Input: string(readInput)})
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if readResp.IsError {
			t.Fatalf("read returned error: %s", readResp.Content)
		}
		var readObj map[string]any
		if err := json.Unmarshal([]byte(readResp.Content), &readObj); err != nil {
			t.Fatalf("invalid read JSON: %v", err)
		}
		if readObj["status"] != string(task.StatusCompleted) {
			t.Fatalf("expected completed, got %v", readObj["status"])
		}
		if strings.Contains(readResp.Content, "transcript") {
			t.Fatalf("expected no transcript by default, got %s", readResp.Content)
		}

		readDebugInput, _ := json.Marshal(map[string]any{
			"action":           "read",
			"task_id":          "task-v2-1",
			"debug_transcript": true,
		})
		readDebugResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-r-debug", Name: "Task", Input: string(readDebugInput)})
		if err != nil {
			t.Fatalf("debug read failed: %v", err)
		}
		if !strings.Contains(readDebugResp.Content, "transcript") || !strings.Contains(readDebugResp.Content, "v2 transcript run one") {
			t.Fatalf("expected transcript in debug read response, got %s", readDebugResp.Content)
		}
		if !strings.Contains(readDebugResp.Content, "[transcript message truncated]") {
			t.Fatalf("expected debug transcript message to be capped, got %s", readDebugResp.Content)
		}

		sendInput, _ := json.Marshal(map[string]string{
			"action":  "send",
			"task_id": "task-v2-1",
			"prompt":  "run two",
		})
		taskTool2 := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner("v2-result-2", 20*time.Millisecond))
		_, err = taskTool2.Run(toolCtx, tools.ToolCall{ID: "tc-v2-s", Name: "Task", Input: string(sendInput)})
		if err != nil {
			t.Fatalf("send failed: %v", err)
		}
		waitForTaskStatus(t, reg, "task-v2-1", task.StatusCompleted)

		state, ok := reg.Get("task-v2-1")
		if !ok {
			t.Fatal("task missing")
		}
		sub, ok := state.(*task.SubtaskState)
		if !ok {
			t.Fatalf("unexpected state type: %T", state)
		}
		if sub.Result != "v2-result-2" {
			t.Fatalf("expected updated result, got %q", sub.Result)
		}

		listInput, _ := json.Marshal(map[string]string{"action": "list"})
		listResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-l", Name: "Task", Input: string(listInput)})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		var listObj []map[string]any
		if err := json.Unmarshal([]byte(listResp.Content), &listObj); err != nil {
			t.Fatalf("invalid list JSON: %v", err)
		}
		if len(listObj) != 1 {
			t.Fatalf("expected 1 task in list, got %d", len(listObj))
		}
		if hooksMock.count(hooks.TaskCreated) == 0 || hooksMock.count(hooks.TaskCompleted) == 0 {
			t.Fatal("expected task lifecycle hooks to fire")
		}
		if hooksMock.count(hooks.SubagentStart) == 0 || hooksMock.count(hooks.SubagentStop) == 0 {
			t.Fatal("expected subagent lifecycle hooks to fire")
		}
	})

	t.Run("TaskV2 stop cancels running task", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-stop")

		runner := func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		taskTool := tools.NewTaskTool(perms, sessions, messages, runner)
		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-stop",
			"description": "long run",
			"prompt":      "run",
			"agent_type":  "general",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-stop-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		stopInput, _ := json.Marshal(map[string]string{
			"action":  "stop",
			"task_id": "task-v2-stop",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-stop-s", Name: "Task", Input: string(stopInput)}); err != nil {
			t.Fatalf("stop failed: %v", err)
		}
		waitForTaskStatus(t, reg, "task-v2-stop", task.StatusCanceled)
	})

	t.Run("TaskV2 stop keeps task reserved until canceled run exits across tool instances", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-race")

		started := make(chan struct{}, 1)
		release := make(chan struct{})
		blockingRunner := func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-ctx.Done()
			<-release
			return "", ctx.Err()
		}

		createTool := tools.NewTaskTool(perms, sessions, messages, blockingRunner)
		stopTool := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner("ignored", 0))
		sendTool := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner("rerun-ok", 0))

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-race",
			"description": "blocking run",
			"prompt":      "run",
			"agent_type":  "general",
		})
		if _, err := createTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-race-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for task runner to start")
		}

		stopInput, _ := json.Marshal(map[string]string{
			"action":  "stop",
			"task_id": "task-v2-race",
		})
		if _, err := stopTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-race-stop", Name: "Task", Input: string(stopInput)}); err != nil {
			t.Fatalf("stop failed: %v", err)
		}

		sendInput, _ := json.Marshal(map[string]string{
			"action":  "send",
			"task_id": "task-v2-race",
			"prompt":  "retry before exit",
		})
		resp, err := sendTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-race-send-1", Name: "Task", Input: string(sendInput)})
		if err != nil {
			t.Fatalf("send while canceling failed: %v", err)
		}
		if !resp.IsError || !strings.Contains(resp.Content, "already running") {
			t.Fatalf("expected already running error while prior run is exiting, got %#v", resp)
		}

		close(release)
		waitForTaskStatus(t, reg, "task-v2-race", task.StatusCanceled)

		resp, err = sendTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-race-send-2", Name: "Task", Input: string(sendInput)})
		if err != nil {
			t.Fatalf("send after cancellation failed: %v", err)
		}
		if resp.IsError {
			t.Fatalf("expected restart after canceled run exits, got error: %s", resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-race", task.StatusCompleted)

		state, ok := reg.Get("task-v2-race")
		if !ok {
			t.Fatal("task missing after rerun")
		}
		sub, ok := state.(*task.SubtaskState)
		if !ok {
			t.Fatalf("unexpected state type: %T", state)
		}
		if sub.Result != "rerun-ok" {
			t.Fatalf("expected rerun result, got %q", sub.Result)
		}
	})

	t.Run("TaskV2 list does not mark task notified for GC", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-gc")

		taskTool := tools.NewTaskTool(perms, sessions, messages, mockAgentRunner("gc-result", 0))
		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-gc",
			"description": "gc run",
			"prompt":      "run",
			"agent_type":  "general",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-gc-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}
		waitForTaskStatus(t, reg, "task-v2-gc", task.StatusCompleted)

		listInput, _ := json.Marshal(map[string]string{"action": "list"})
		listResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-gc-l", Name: "Task", Input: string(listInput)})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if listResp.IsError {
			t.Fatalf("list returned error: %s", listResp.Content)
		}

		future := time.Now().Add(10 * time.Minute)
		reg.GC(future)
		state, ok := reg.Get("task-v2-gc")
		if !ok {
			t.Fatal("list should not make task eligible for GC")
		}
		if state.TaskMeta().Notified {
			t.Fatal("list should not mark task notified")
		}

		readInput, _ := json.Marshal(map[string]string{
			"action":  "read",
			"task_id": "task-v2-gc",
		})
		readResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-gc-r", Name: "Task", Input: string(readInput)})
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if readResp.IsError {
			t.Fatalf("read returned error: %s", readResp.Content)
		}

		state, ok = reg.Get("task-v2-gc")
		if !ok {
			t.Fatal("task missing before GC after read")
		}
		if !state.TaskMeta().Notified {
			t.Fatal("read should mark task notified")
		}

		reg.GC(future)
		if _, ok := reg.Get("task-v2-gc"); ok {
			t.Fatal("read should make completed task eligible for GC")
		}
	})

	t.Run("TaskV2 read while running does not make final result GC-eligible", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-read-running")

		started := make(chan struct{}, 1)
		release := make(chan struct{})
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return "late-result", nil
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-read-running",
			"description": "long run",
			"prompt":      "run",
			"agent_type":  "general",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-read-running-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for task to start")
		}

		readInput, _ := json.Marshal(map[string]string{
			"action":  "read",
			"task_id": "task-v2-read-running",
		})
		readResp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-read-running-r1", Name: "Task", Input: string(readInput)})
		if err != nil {
			t.Fatalf("read while running failed: %v", err)
		}
		if readResp.IsError {
			t.Fatalf("read while running returned error: %s", readResp.Content)
		}

		state, ok := reg.Get("task-v2-read-running")
		if !ok {
			t.Fatal("task missing after running read")
		}
		if state.TaskMeta().Notified {
			t.Fatal("running read should not mark task notified")
		}

		close(release)
		waitForTaskStatus(t, reg, "task-v2-read-running", task.StatusCompleted)

		future := time.Now().Add(10 * time.Minute)
		reg.GC(future)
		if _, ok := reg.Get("task-v2-read-running"); !ok {
			t.Fatal("final unread result should survive GC after a running read")
		}

		readResp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-read-running-r2", Name: "Task", Input: string(readInput)})
		if err != nil {
			t.Fatalf("read after completion failed: %v", err)
		}
		if readResp.IsError {
			t.Fatalf("read after completion returned error: %s", readResp.Content)
		}

		reg.GC(future)
		if _, ok := reg.Get("task-v2-read-running"); ok {
			t.Fatal("final read should make completed task eligible for GC")
		}
	})

	t.Run("TaskV2 canceled task still emits final hooks", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		hooksMock := &cancelAwareHookService{}
		tools.SetTaskRuntime(reg, hooksMock)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-cancel-hooks")

		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-cancel-hooks",
			"description": "cancel me",
			"prompt":      "run",
			"agent_type":  "general",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-cancel-hooks-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		stopInput, _ := json.Marshal(map[string]string{
			"action":  "stop",
			"task_id": "task-v2-cancel-hooks",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-cancel-hooks-s", Name: "Task", Input: string(stopInput)}); err != nil {
			t.Fatalf("stop failed: %v", err)
		}
		waitForTaskStatus(t, reg, "task-v2-cancel-hooks", task.StatusCanceled)
		waitForHookCount(t, hooksMock, hooks.SubagentStop, 1)
		waitForHookCount(t, hooksMock, hooks.TaskCompleted, 1)
	})

	t.Run("TaskV2 create propagates model override into runner context", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-model")

		var gotModel string
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			if runtime, ok := tools.TaskRuntimeOverrideFromContext(ctx); ok {
				gotModel = runtime.Model
			}
			return "ok", nil
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-model",
			"description": "model override",
			"prompt":      "run",
			"agent_type":  "general",
			"model":       "gpt-5.4",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-model-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		waitForTaskStatus(t, reg, "task-v2-model", task.StatusCompleted)
		if gotModel != "gpt-5.4" {
			t.Fatalf("expected model override gpt-5.4, got %q", gotModel)
		}
	})

	t.Run("TaskV2 create preserves research worker tool profile", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-research-worker")
		toolCtx = context.WithValue(toolCtx, tools.ResearchModeContextKey, true)
		toolCtx = context.WithValue(toolCtx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileLeader)

		var mu sync.Mutex
		var gotProfile tools.ResearchToolProfile
		seenTools := map[string]bool{}
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, agentTools []tools.BaseTool) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if profile, ok := ctx.Value(tools.ResearchToolProfileContextKey).(tools.ResearchToolProfile); ok {
				gotProfile = profile
			}
			for _, tool := range agentTools {
				seenTools[tool.Info().Name] = true
			}
			return "worker done", nil
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-research-worker",
			"description": "worker task",
			"prompt":      "write the deliverable",
			"agent_type":  "general",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-research-worker-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		waitForTaskStatus(t, reg, "task-v2-research-worker", task.StatusCompleted)
		mu.Lock()
		defer mu.Unlock()
		if gotProfile != tools.ResearchToolProfileWorker {
			t.Fatalf("expected research worker profile, got %q", gotProfile)
		}
		if !seenTools["Write"] || !seenTools["Edit"] {
			t.Fatalf("expected research worker toolset to keep Write/Edit, got %#v", seenTools)
		}
	})

	t.Run("legacy Task defaults omitted research agent_type to worker profile", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-legacy-research-worker")
		toolCtx = context.WithValue(toolCtx, tools.ResearchModeContextKey, true)
		toolCtx = context.WithValue(toolCtx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileLeader)

		var mu sync.Mutex
		var gotProfile tools.ResearchToolProfile
		seenTools := map[string]bool{}
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, agentTools []tools.BaseTool) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if profile, ok := ctx.Value(tools.ResearchToolProfileContextKey).(tools.ResearchToolProfile); ok {
				gotProfile = profile
			}
			for _, tool := range agentTools {
				seenTools[tool.Info().Name] = true
			}
			return "worker done", nil
		})

		input, _ := json.Marshal(map[string]string{
			"description": "worker task",
			"prompt":      "write the deliverable",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-legacy-research-worker", Name: "Task", Input: string(input)})
		if err != nil {
			t.Fatalf("legacy task failed: %v", err)
		}
		if resp.IsError {
			t.Fatalf("legacy task returned error: %s", resp.Content)
		}

		mu.Lock()
		defer mu.Unlock()
		if gotProfile != tools.ResearchToolProfileWorker {
			t.Fatalf("expected research worker profile, got %q", gotProfile)
		}
		if !seenTools["Write"] || !seenTools["Edit"] {
			t.Fatalf("expected research worker toolset to keep Write/Edit, got %#v", seenTools)
		}
	})

	t.Run("TaskV2 leader tasks require verify gate by default", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-verify")

		taskTool := tools.NewTaskTool(perms, sessions, messages, func(ctx context.Context, agentName config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			switch agentName {
			case config.AgentLeader:
				return "leader draft", nil
			case config.AgentVerify:
				return "evidence missing\nVERDICT: FAIL", nil
			default:
				return "", fmt.Errorf("unexpected agent %s", agentName)
			}
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-verify",
			"description": "verify me",
			"prompt":      "run",
			"agent_type":  "leader",
		})
		if _, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-verify-c", Name: "Task", Input: string(createInput)}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		waitForTaskStatus(t, reg, "task-v2-verify", task.StatusFailed)
		st, ok := reg.Get("task-v2-verify")
		if !ok {
			t.Fatal("expected task state")
		}
		sub, ok := st.(*task.SubtaskState)
		if !ok {
			t.Fatalf("expected SubtaskState, got %T", st)
		}
		if sub.VerifyPolicy != "required" {
			t.Fatalf("expected verify policy required, got %q", sub.VerifyPolicy)
		}
		if sub.VerifyStatus != task.VerifyStatusFailed {
			t.Fatalf("expected verify status failed, got %s", sub.VerifyStatus)
		}
		if sub.VerifyVerdict != "FAIL" {
			t.Fatalf("expected verify verdict FAIL, got %q", sub.VerifyVerdict)
		}
		if sub.VerifyResult != "evidence missing" {
			t.Fatalf("expected verify result report, got %q", sub.VerifyResult)
		}
		if !strings.Contains(sub.LastError, "did not pass required gate") {
			t.Fatalf("expected verification verdict error, got %q", sub.LastError)
		}
	})
}

func TestTaskNormalizationAndKBInjection(t *testing.T) {
	t.Run("legacy prompt precedence prompt > instruction > description", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-precedence")

		var gotPrompt string
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(_ context.Context, _ config.AgentName, _ string, prompt string, _ []tools.BaseTool) (string, error) {
			gotPrompt = prompt
			return "ok", nil
		})

		input, _ := json.Marshal(map[string]string{
			"description": "from-description",
			"instruction": "from-instruction",
			"prompt":      "from-prompt",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-precedence-1", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("legacy task failed: err=%v content=%s", err, resp.Content)
		}
		if gotPrompt != "from-prompt" {
			t.Fatalf("expected prompt precedence, got %q", gotPrompt)
		}

		input, _ = json.Marshal(map[string]string{
			"description": "from-description",
			"instruction": "from-instruction",
		})
		resp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-precedence-2", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("legacy task failed: err=%v content=%s", err, resp.Content)
		}
		if gotPrompt != "from-instruction" {
			t.Fatalf("expected instruction fallback, got %q", gotPrompt)
		}
	})

	t.Run("agent_type precedence and mode coder/default compatibility", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-mode")

		var gotAgent config.AgentName
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(_ context.Context, agentName config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			gotAgent = agentName
			return "ok", nil
		})

		input, _ := json.Marshal(map[string]string{
			"description": "agent",
			"prompt":      "run",
			"agent_type":  "explore",
			"mode":        "coder",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-agent-precedence", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("legacy task failed: err=%v content=%s", err, resp.Content)
		}
		if gotAgent != config.AgentExplore {
			t.Fatalf("expected agent_type to win, got %s", gotAgent)
		}

		input, _ = json.Marshal(map[string]string{
			"description": "agent",
			"prompt":      "run",
			"mode":        "coder",
		})
		resp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-mode-coder", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("legacy task failed: err=%v content=%s", err, resp.Content)
		}
		if gotAgent != config.AgentGeneral {
			t.Fatalf("expected mode=coder -> general, got %s", gotAgent)
		}

		input, _ = json.Marshal(map[string]string{
			"description": "agent",
			"prompt":      "run",
			"mode":        "unsupported-mode",
		})
		resp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-mode-unsupported", Name: "Task", Input: string(input)})
		if err != nil {
			t.Fatalf("legacy task returned error: %v", err)
		}
		if !resp.IsError || !strings.Contains(resp.Content, "unsupported agent_type: unsupported-mode") {
			t.Fatalf("expected clear unsupported mode error, got %#v", resp)
		}
	})

	t.Run("TaskV2 create/send use instruction/mode normalization", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-normalize")

		var prompts []string
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(_ context.Context, agentName config.AgentName, _ string, prompt string, _ []tools.BaseTool) (string, error) {
			if agentName != config.AgentGeneral {
				t.Fatalf("expected general agent from mode normalization, got %s", agentName)
			}
			prompts = append(prompts, prompt)
			return "ok", nil
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-norm",
			"description": "desc",
			"instruction": "create-instruction",
			"mode":        "default",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-norm-create", Name: "Task", Input: string(createInput)})
		if err != nil || resp.IsError {
			t.Fatalf("create failed: err=%v content=%s", err, resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-norm", task.StatusCompleted)

		sendInput, _ := json.Marshal(map[string]string{
			"action":      "send",
			"task_id":     "task-v2-norm",
			"description": "ignored",
			"instruction": "send-instruction",
			"mode":        "coder",
		})
		resp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-norm-send", Name: "Task", Input: string(sendInput)})
		if err != nil || resp.IsError {
			t.Fatalf("send failed: err=%v content=%s", err, resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-norm", task.StatusCompleted)

		if len(prompts) < 2 {
			t.Fatalf("expected two task runs, got %d", len(prompts))
		}
		if prompts[0] != "create-instruction" {
			t.Fatalf("expected create prompt from instruction, got %q", prompts[0])
		}
		if prompts[1] != "send-instruction" {
			t.Fatalf("expected send prompt from instruction, got %q", prompts[1])
		}
	})

	t.Run("TaskV2 send preserves existing agent type without override", func(t *testing.T) {
		_, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-preserve-agent")

		agents := make(chan config.AgentName, 2)
		taskTool := tools.NewTaskTool(perms, sessions, messages, func(_ context.Context, agentName config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			agents <- agentName
			return "ok", nil
		})

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-preserve-agent",
			"description": "desc",
			"instruction": "create-instruction",
			"agent_type":  "explore",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-preserve-create", Name: "Task", Input: string(createInput)})
		if err != nil || resp.IsError {
			t.Fatalf("create failed: err=%v content=%s", err, resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-preserve-agent", task.StatusCompleted)

		sendInput, _ := json.Marshal(map[string]string{
			"action":      "send",
			"task_id":     "task-v2-preserve-agent",
			"instruction": "send-instruction",
		})
		resp, err = taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-preserve-send", Name: "Task", Input: string(sendInput)})
		if err != nil || resp.IsError {
			t.Fatalf("send failed: err=%v content=%s", err, resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-preserve-agent", task.StatusCompleted)

		for i := 0; i < 2; i++ {
			select {
			case got := <-agents:
				if got != config.AgentExplore {
					t.Fatalf("run %d used agent %s, want %s", i+1, got, config.AgentExplore)
				}
			default:
				t.Fatalf("missing captured agent for run %d", i+1)
			}
		}
	})

	t.Run("KB tools injected for general only when KBServices exists", func(t *testing.T) {
		conn, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-kb-tools")

		containsKBTools := func(toolNames []string) bool {
			have := map[string]bool{}
			for _, n := range toolNames {
				have[n] = true
			}
			return have["KBAdd"] && have["KBList"] && have["KBTree"]
		}

		var namesWithout []string
		taskToolWithout := tools.NewTaskTool(perms, sessions, messages, func(_ context.Context, _ config.AgentName, _ string, _ string, agentTools []tools.BaseTool) (string, error) {
			namesWithout = namesWithout[:0]
			for _, tool := range agentTools {
				namesWithout = append(namesWithout, tool.Info().Name)
			}
			return "ok", nil
		})

		input, _ := json.Marshal(map[string]string{"description": "t", "prompt": "p", "agent_type": "general"})
		resp, err := taskToolWithout.Run(toolCtx, tools.ToolCall{ID: "tc-kb-without", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("task without kbs failed: err=%v content=%s", err, resp.Content)
		}
		if containsKBTools(namesWithout) {
			t.Fatalf("unexpected KB tools without KBServices: %v", namesWithout)
		}

		kbSvc := kb.NewService(q, conn)
		var namesWith []string
		taskToolWith := tools.NewTaskToolWithKB(perms, sessions, messages, func(_ context.Context, _ config.AgentName, _ string, _ string, agentTools []tools.BaseTool) (string, error) {
			namesWith = namesWith[:0]
			for _, tool := range agentTools {
				namesWith = append(namesWith, tool.Info().Name)
			}
			return "ok", nil
		}, &tools.KBServices{KB: kbSvc, Indexer: noopIndexer{}})

		resp, err = taskToolWith.Run(toolCtx, tools.ToolCall{ID: "tc-kb-with", Name: "Task", Input: string(input)})
		if err != nil || resp.IsError {
			t.Fatalf("task with kbs failed: err=%v content=%s", err, resp.Content)
		}
		if !containsKBTools(namesWith) {
			t.Fatalf("expected KB tools with KBServices, got %v", namesWith)
		}
	})

	t.Run("TaskV2 reader uses read-only KB runtime", func(t *testing.T) {
		conn, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-reader")

		kbSvc := kb.NewService(q, conn)
		kbs := &tools.KBServices{
			KB:      kbSvc,
			Indexer: noopIndexer{},
			CallLLM: func(context.Context, string) (string, error) {
				return "ok", nil
			},
		}
		var gotPrompt string
		var gotAllowed []string
		var gotTools []string
		var hasDeadline bool
		var deadline time.Time
		taskTool := tools.NewTaskToolWithKB(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, prompt string, agentTools []tools.BaseTool) (string, error) {
			gotPrompt = prompt
			if runtime, ok := tools.TaskRuntimeOverrideFromContext(ctx); ok {
				gotAllowed = append([]string(nil), runtime.AllowedTools...)
			}
			deadline, hasDeadline = ctx.Deadline()
			for _, tool := range agentTools {
				gotTools = append(gotTools, tool.Info().Name)
			}
			return "reader done", nil
		}, kbs)

		createInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-reader",
			"description": "read paper",
			"prompt":      "read paper p1",
			"agent_type":  "reader",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-reader-c", Name: "Task", Input: string(createInput)})
		if err != nil || resp.IsError {
			t.Fatalf("create reader failed: err=%v content=%s", err, resp.Content)
		}
		waitForTaskStatus(t, reg, "task-v2-reader", task.StatusCompleted)

		if !strings.Contains(gotPrompt, "# Reading Worker") || !strings.Contains(gotPrompt, "not_covered") {
			t.Fatalf("reader prompt missing read-only/schema prefix:\n%s", gotPrompt)
		}
		if !hasDeadline || time.Until(deadline) <= 10*time.Minute || time.Until(deadline) > 13*time.Minute {
			t.Fatalf("expected reader deadline near 12 minutes, has=%v deadline=%v", hasDeadline, deadline)
		}
		assertSameToolSet(t, gotAllowed, []string{"View", "KBList", "KBTree", "KBQuery", "KBSearch"})
		assertSameToolSet(t, gotTools, []string{"View", "KBList", "KBTree", "KBQuery", "KBSearch"})
		for _, forbidden := range []string{"Write", "Edit", "Bash", "ScholarSearch", "KBAdd"} {
			if containsString(gotAllowed, forbidden) || containsString(gotTools, forbidden) {
				t.Fatalf("reader should not receive forbidden tool %s; allowed=%v tools=%v", forbidden, gotAllowed, gotTools)
			}
		}
	})

	t.Run("TaskV2 reader enforces max concurrent workers", func(t *testing.T) {
		conn, q := testutil.SetupTestDB(t)
		ctx := context.Background()
		sessions := session.NewService(q)
		messages := message.NewService(q)
		perms := permission.NewPermissionService()
		reg := task.NewRegistry()
		defer reg.Shutdown()
		tools.SetTaskRuntime(reg, nil)
		defer tools.SetTaskRuntime(nil, nil)

		parent, _ := sessions.Create(ctx, "parent")
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parent.ID)
		toolCtx = context.WithValue(toolCtx, tools.MessageIDContextKey, "msg-v2-reader-limit")

		release := make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		kbSvc := kb.NewService(q, conn)
		taskTool := tools.NewTaskToolWithKB(perms, sessions, messages, func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []tools.BaseTool) (string, error) {
			select {
			case <-release:
				return "reader done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}, &tools.KBServices{
			KB:      kbSvc,
			Indexer: noopIndexer{},
			CallLLM: func(context.Context, string) (string, error) {
				return "ok", nil
			},
		})

		for i := 0; i < 5; i++ {
			createInput, _ := json.Marshal(map[string]string{
				"action":      "create",
				"task_id":     fmt.Sprintf("task-v2-reader-limit-%d", i),
				"description": "read paper",
				"prompt":      "read paper",
				"agent_type":  "reader",
			})
			resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: fmt.Sprintf("tc-v2-reader-limit-%d", i), Name: "Task", Input: string(createInput)})
			if err != nil || resp.IsError {
				t.Fatalf("create reader %d failed: err=%v content=%s", i, err, resp.Content)
			}
		}

		overflowInput, _ := json.Marshal(map[string]string{
			"action":      "create",
			"task_id":     "task-v2-reader-limit-overflow",
			"description": "read paper",
			"prompt":      "read paper",
			"agent_type":  "reader",
		})
		resp, err := taskTool.Run(toolCtx, tools.ToolCall{ID: "tc-v2-reader-limit-overflow", Name: "Task", Input: string(overflowInput)})
		if err != nil {
			t.Fatalf("overflow create returned error: %v", err)
		}
		if !resp.IsError || !strings.Contains(resp.Content, "reading worker limit reached") {
			t.Fatalf("expected reader limit error, got %#v", resp)
		}

		close(release)
		released = true
		for i := 0; i < 5; i++ {
			waitForTaskStatus(t, reg, fmt.Sprintf("task-v2-reader-limit-%d", i), task.StatusCompleted)
		}
	})
}

func assertSameToolSet(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tool set length mismatch: got=%v want=%v", got, want)
	}
	for _, name := range want {
		if !containsString(got, name) {
			t.Fatalf("missing tool %s in %v", name, got)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
