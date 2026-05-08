package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func TestDrainTaskNotificationsConcurrentClaimsOnce(t *testing.T) {
	loadDrainNotificationConfig(t, true)
	sessions, messages := setupDrainTest(t)
	ctx := context.Background()
	parent := testutil.CreateTestSession(t, sessions)

	reg := task.NewRegistry()
	defer reg.Shutdown()
	reg.Register(doneSubtask("notify-1", parent.ID, task.StatusCompleted))

	a := &agent{
		agentName:    config.AgentCoder,
		messages:     messages,
		taskRegistry: reg,
	}

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := a.drainTaskNotifications(ctx, parent.ID)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("drain error: %v", err)
		}
	}

	parentMessages, err := messages.List(ctx, parent.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	notifications := 0
	for _, msg := range parentMessages {
		if v, ok := msg.Meta["subagent_notification"].(bool); ok && v {
			notifications++
		}
	}
	if notifications != 1 {
		t.Fatalf("expected exactly one persisted notification, got %d", notifications)
	}

	state, ok := reg.Get("notify-1")
	if !ok {
		t.Fatal("expected task to remain registered")
	}
	meta := state.TaskMeta()
	if !meta.Notified {
		t.Fatal("expected delivered task to be notified")
	}
	if meta.NotifyClaimed {
		t.Fatal("delivered task should not remain claimed")
	}
}

func TestDrainTaskNotificationsIncludesFailedAndCanceled(t *testing.T) {
	loadDrainNotificationConfig(t, true)
	sessions, messages := setupDrainTest(t)
	ctx := context.Background()
	parent := testutil.CreateTestSession(t, sessions)

	reg := task.NewRegistry()
	defer reg.Shutdown()
	reg.Register(doneSubtask("failed-1", parent.ID, task.StatusFailed))
	reg.Register(doneSubtask("canceled-1", parent.ID, task.StatusCanceled))

	a := &agent{
		agentName:    config.AgentCoder,
		messages:     messages,
		taskRegistry: reg,
	}
	if _, err := a.drainTaskNotifications(ctx, parent.ID); err != nil {
		t.Fatalf("drain: %v", err)
	}

	parentMessages, err := messages.List(ctx, parent.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	joined := strings.Builder{}
	for _, msg := range parentMessages {
		if v, ok := msg.Meta["subagent_notification"].(bool); ok && v {
			joined.WriteString(msg.Content().String())
			joined.WriteByte('\n')
		}
	}
	text := joined.String()
	if strings.Count(text, "<task-notification>") != 2 {
		t.Fatalf("expected two task notifications, got:\n%s", text)
	}
	if !strings.Contains(text, "<status>failed</status>") {
		t.Fatalf("missing failed notification:\n%s", text)
	}
	if !strings.Contains(text, "<status>canceled</status>") {
		t.Fatalf("missing canceled notification:\n%s", text)
	}
}

func TestDrainTaskNotificationsAllowsLeaderAndCoordinator(t *testing.T) {
	loadDrainNotificationConfig(t, true)

	for _, agentName := range []config.AgentName{config.AgentLeader, config.AgentCoordinator} {
		t.Run(string(agentName), func(t *testing.T) {
			sessions, messages := setupDrainTest(t)
			ctx := context.Background()
			parent := testutil.CreateTestSession(t, sessions)

			reg := task.NewRegistry()
			defer reg.Shutdown()
			reg.Register(doneSubtask("notify-"+string(agentName), parent.ID, task.StatusCompleted))

			a := &agent{
				agentName:    agentName,
				messages:     messages,
				taskRegistry: reg,
			}
			if _, err := a.drainTaskNotifications(ctx, parent.ID); err != nil {
				t.Fatalf("drain: %v", err)
			}

			parentMessages, err := messages.List(ctx, parent.ID)
			if err != nil {
				t.Fatalf("list messages: %v", err)
			}
			notifications := 0
			for _, msg := range parentMessages {
				if v, ok := msg.Meta["subagent_notification"].(bool); ok && v {
					notifications++
				}
			}
			if notifications != 1 {
				t.Fatalf("expected exactly one persisted notification, got %d", notifications)
			}
		})
	}
}

func setupDrainTest(t *testing.T) (session.Service, message.Service) {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	return session.NewService(q), message.NewService(q)
}

func loadDrainNotificationConfig(t *testing.T, enabled bool) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	wd := t.TempDir()
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	cfg := `{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"model": "qwen2.5-coder:latest"}
  },
  "subagent_orchestration": {
    "enabled": ` + enabledValue + `
  }
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

func doneSubtask(id, parentID string, status task.Status) *task.SubtaskState {
	endedAt := time.Now().UTC()
	state := task.NewSubtaskState(task.Meta{
		ID:             id,
		Label:          id,
		SessionID:      parentID,
		Kind:           task.KindAgent,
		Status:         status,
		StartedAt:      endedAt.Add(-time.Second),
		EndedAt:        &endedAt,
		IsBackgrounded: true,
	}, parentID, id+"-child", "general", "summary", "test-model", "none")
	state.Result = "result"
	return state
}
