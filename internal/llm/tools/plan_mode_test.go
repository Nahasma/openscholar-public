package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
)

func TestExitPlanModePublishesCanonicalPlanAndApprovedEnvelope(t *testing.T) {
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-1")
	perms := permission.NewPermissionService()
	plans := plan.NewService(t.TempDir())
	broker := pubsub.NewBroker[PlanApprovalEvent]()
	tool := NewExitPlanModeTool(perms, plans, broker)
	perms.EnterPlanMode("sess-1")
	if _, err := plans.Write(ctx, "sess-1", "<proposed_plan>\n- a\n- b\n</proposed_plan>"); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	sub := broker.Subscribe(context.Background())
	done := make(chan ToolResponse, 1)
	go func() {
		resp, _ := tool.Run(ctx, ToolCall{})
		done <- resp
	}()

	var ev PlanApprovalEvent
	select {
	case msg := <-sub:
		ev = msg.Payload
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for plan approval event")
	}
	if !strings.Contains(ev.Plan, "<proposed_plan>") {
		t.Fatalf("Plan should be proposed envelope, got: %q", ev.Plan)
	}
	if strings.Contains(ev.PlanBody, "<proposed_plan>") || strings.TrimSpace(ev.PlanBody) != "- a\n- b" {
		t.Fatalf("PlanBody should be normalized body, got: %q", ev.PlanBody)
	}
	ev.ResponseCh <- PlanApprovalResponse{Approved: true}

	select {
	case resp := <-done:
		if !strings.Contains(resp.Content, "<approved_plan>") {
			t.Fatalf("expected approved envelope in response, got: %q", resp.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tool response")
	}

	_, saved, err := plans.Read(ctx, "sess-1")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if strings.TrimSpace(saved) != "- a\n- b" {
		t.Fatalf("wrapped plan file should be normalized after approval, got: %q", saved)
	}
}

func TestExitPlanModeEditedPlanNormalizedBeforeSave(t *testing.T) {
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-2")
	perms := permission.NewPermissionService()
	plans := plan.NewService(t.TempDir())
	broker := pubsub.NewBroker[PlanApprovalEvent]()
	tool := NewExitPlanModeTool(perms, plans, broker)
	perms.EnterPlanMode("sess-2")
	if _, err := plans.Write(ctx, "sess-2", "old"); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	sub := broker.Subscribe(context.Background())
	done := make(chan struct{}, 1)
	go func() {
		_, _ = tool.Run(ctx, ToolCall{})
		done <- struct{}{}
	}()
	msg := <-sub
	msg.Payload.ResponseCh <- PlanApprovalResponse{
		Approved:   true,
		EditedPlan: "<approved_plan>\nnew body\n</approved_plan>",
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tool completion")
	}

	_, saved, err := plans.Read(ctx, "sess-2")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if strings.Contains(saved, "<approved_plan>") || strings.TrimSpace(saved) != "new body" {
		t.Fatalf("saved plan should be normalized body only, got: %q", saved)
	}
}
