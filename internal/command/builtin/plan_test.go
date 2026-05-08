package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/plan"
)

func TestPlanInspectWrapsProposedEnvelope(t *testing.T) {
	perms := permission.NewPermissionService()
	plans := plan.NewService(t.TempDir())
	perms.EnterPlanMode("sess-plan")
	if _, err := plans.Write(context.Background(), "sess-plan", "<approved_plan>\nstep\n</approved_plan>"); err != nil {
		t.Fatalf("write: %v", err)
	}

	cmd := &planCmd{}
	result := cmd.Execute(command.Context{
		App: &app.App{
			Permissions: perms,
			Plans:       plans,
		},
		SessionID:   "sess-plan",
		ExecContext: context.Background(),
	})

	if !strings.Contains(result.Output, "<proposed_plan>") {
		t.Fatalf("missing proposed envelope: %q", result.Output)
	}
	if strings.Contains(result.Output, "<approved_plan>") {
		t.Fatalf("should not keep approved envelope in inspect output: %q", result.Output)
	}
}

func TestPlanInspectSubcommandDoesNotEnterPlanMode(t *testing.T) {
	perms := permission.NewPermissionService()
	plans := plan.NewService(t.TempDir())
	if _, err := plans.Write(context.Background(), "sess-plan", "step"); err != nil {
		t.Fatalf("write: %v", err)
	}

	cmd := &planCmd{}
	result := cmd.Execute(command.Context{
		App: &app.App{
			Permissions: perms,
			Plans:       plans,
		},
		SessionID:   "sess-plan",
		Args:        "inspect",
		ExecContext: context.Background(),
	})

	if result.Action != "" {
		t.Fatalf("inspect should not change mode, action = %q", result.Action)
	}
	if !strings.Contains(result.Output, "<proposed_plan>") {
		t.Fatalf("missing proposed envelope: %q", result.Output)
	}
	if perms.SessionMode("sess-plan") == permission.ModePlan {
		t.Fatal("inspect should not enter plan mode")
	}
}
