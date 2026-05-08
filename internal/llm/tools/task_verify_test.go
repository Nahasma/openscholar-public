package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
)

type failingVerifySessionService struct {
	session.Service
	err error
}

func (f failingVerifySessionService) CreateTaskSession(_ context.Context, _, _, _ string) (session.Session, error) {
	return session.Session{}, f.err
}

func TestParseTaskVerifyVerdict(t *testing.T) {
	t.Run("PASS", func(t *testing.T) {
		verdict, report, err := parseTaskVerifyVerdict("looks good\nVERDICT: PASS")
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if verdict != "PASS" || report != "looks good" {
			t.Fatalf("unexpected parse result: verdict=%q report=%q", verdict, report)
		}
	})

	t.Run("FAIL", func(t *testing.T) {
		verdict, _, err := parseTaskVerifyVerdict("missing test coverage\nVERDICT: FAIL")
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if verdict != "FAIL" {
			t.Fatalf("unexpected verdict: %q", verdict)
		}
	})

	t.Run("PARTIAL", func(t *testing.T) {
		verdict, _, err := parseTaskVerifyVerdict("read-only limits\nVERDICT: PARTIAL")
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if verdict != "PARTIAL" {
			t.Fatalf("unexpected verdict: %q", verdict)
		}
	})

	t.Run("missing verdict", func(t *testing.T) {
		_, _, err := parseTaskVerifyVerdict("looks good")
		if err == nil {
			t.Fatal("expected parse error for missing verdict")
		}
	})

	t.Run("verdict must be final line", func(t *testing.T) {
		_, _, err := parseTaskVerifyVerdict("VERDICT: PASS\nextra text")
		if err == nil {
			t.Fatal("expected parse error when verdict is not the final line")
		}
	})

	t.Run("unsupported verdict", func(t *testing.T) {
		_, _, err := parseTaskVerifyVerdict("looks good\nVERDICT: MAYBE")
		if err == nil {
			t.Fatal("expected parse error for unsupported verdict")
		}
	})
}

func TestRunTaskVerificationFailClosed(t *testing.T) {
	_, q := setupTaskCustomAgentTestDB(t)
	sessions := session.NewService(q)
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")
	sub := task.NewSubtaskState(task.Meta{ID: "verify-task", Label: "verify-task"}, parent.ID, "child", "leader", "verify-task", "", "required")

	t.Run("PASS accepted", func(t *testing.T) {
		status, verdict, report, verifyErr := runTaskVerification(context.Background(), perms, sessions, func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			return "all checks passed\nVERDICT: PASS", nil
		}, sub, "prompt", "result")
		if status != task.VerifyStatusPassed || verdict != "PASS" || verifyErr != "" || report != "all checks passed" {
			t.Fatalf("unexpected verify result: status=%q verdict=%q report=%q err=%q", status, verdict, report, verifyErr)
		}
	})

	t.Run("PARTIAL rejected", func(t *testing.T) {
		status, verdict, _, verifyErr := runTaskVerification(context.Background(), perms, sessions, func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			return "cannot execute tests\nVERDICT: PARTIAL", nil
		}, sub, "prompt", "result")
		if status != task.VerifyStatusFailed || verdict != "PARTIAL" || !strings.Contains(verifyErr, "did not pass required gate") {
			t.Fatalf("unexpected verify result: status=%q verdict=%q err=%q", status, verdict, verifyErr)
		}
	})

	t.Run("missing verdict rejected", func(t *testing.T) {
		status, verdict, _, verifyErr := runTaskVerification(context.Background(), perms, sessions, func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			return "looks good", nil
		}, sub, "prompt", "result")
		if status != task.VerifyStatusFailed || verdict != "" || !strings.Contains(verifyErr, "missing required final verdict") {
			t.Fatalf("unexpected verify result: status=%q verdict=%q err=%q", status, verdict, verifyErr)
		}
	})

	t.Run("cancellation rejected", func(t *testing.T) {
		status, _, _, verifyErr := runTaskVerification(context.Background(), perms, sessions, func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			return "", context.Canceled
		}, sub, "prompt", "result")
		if status != task.VerifyStatusFailed || verifyErr != context.Canceled.Error() {
			t.Fatalf("unexpected verify result: status=%q err=%q", status, verifyErr)
		}
	})

	t.Run("canceled context with PASS rejected", func(t *testing.T) {
		verifyCtx, cancel := context.WithCancel(context.Background())
		status, verdict, _, verifyErr := runTaskVerification(verifyCtx, perms, sessions, func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			cancel()
			return "all checks passed\nVERDICT: PASS", nil
		}, sub, "prompt", "result")
		if status != task.VerifyStatusFailed || verdict != "" || verifyErr != context.Canceled.Error() {
			t.Fatalf("unexpected verify result: status=%q verdict=%q err=%q", status, verdict, verifyErr)
		}
	})

	t.Run("verify session creation failure rejected", func(t *testing.T) {
		status, _, _, verifyErr := runTaskVerification(
			context.Background(),
			perms,
			failingVerifySessionService{Service: sessions, err: errors.New("create failed")},
			func(_ context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
				return "VERDICT: PASS", nil
			},
			sub,
			"prompt",
			"result",
		)
		if status != task.VerifyStatusFailed || !strings.Contains(verifyErr, "failed to create verify session") {
			t.Fatalf("unexpected verify result: status=%q err=%q", status, verifyErr)
		}
	})
}

func TestStartSubtaskRunSchedulerAcquireFailureMarksVerificationFailed(t *testing.T) {
	q := setupTaskP2TestDB(t)
	ctx := context.Background()
	sessions := session.NewService(q)
	perms := permission.NewPermissionService()
	reg := task.NewRegistry()
	defer reg.Shutdown()

	parent, _ := sessions.Create(ctx, "parent")
	child1, _ := sessions.CreateTaskSession(ctx, "sched-child-1", parent.ID, "child 1")
	child2, _ := sessions.CreateTaskSession(ctx, "sched-child-2", parent.ID, "child 2")

	started := make(chan struct{})
	taskTool := &taskTool{
		permissions: perms,
		registry:    reg,
		scheduler: newSubagentScheduler(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{
			Enabled:             true,
			GlobalMaxConcurrent: 1,
		}}),
		runAgent: func(ctx context.Context, _ config.AgentName, _ string, _ string, _ []BaseTool) (string, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	profile := SubagentProfile{ID: "general", AgentType: "general", MaxConcurrent: 1}

	blocking := task.NewSubtaskState(task.Meta{
		ID:             "sched-block",
		Label:          "blocking",
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: true,
	}, parent.ID, child1.ID, "general", "blocking", "", "none")
	reg.Register(blocking)
	if !taskTool.startSubtaskRun(ctx, parent.ID, blocking, "block", "general", config.AgentGeneral, nil, nil, 0, profile, nil, nil) {
		t.Fatal("expected blocking task to start")
	}
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocking task did not acquire scheduler slot")
	}

	verifyRequired := task.NewSubtaskState(task.Meta{
		ID:             "sched-canceled",
		Label:          "verify required",
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: true,
	}, parent.ID, child2.ID, "general", "verify required", "", "required")
	reg.Register(verifyRequired)
	if !taskTool.startSubtaskRun(ctx, parent.ID, verifyRequired, "run", "general", config.AgentGeneral, nil, nil, time.Millisecond, profile, nil, nil) {
		t.Fatal("expected second task runtime to start")
	}

	sub := waitTaskDone(t, reg, "sched-canceled")
	if sub.TaskMeta().Status != task.StatusCanceled || sub.VerifyStatus != task.VerifyStatusFailed || !strings.Contains(sub.VerifyError, "before verification") {
		t.Fatalf("expected canceled task to fail verification, status=%s verify=%s verify_error=%q last_error=%q", sub.TaskMeta().Status, sub.VerifyStatus, sub.VerifyError, sub.LastError)
	}

	taskTool.cancelTask("sched-block")
	waitTaskDone(t, reg, "sched-block")
}
