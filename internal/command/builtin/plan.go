package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/plan"
)

type planCmd struct{}

func (c *planCmd) Name() string        { return "plan" }
func (c *planCmd) Description() string { return "Enter or inspect plan mode" }

func (c *planCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)
	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}

	if ctx.App == nil || ctx.App.Permissions == nil || ctx.App.Plans == nil {
		return command.Result{Output: "Plan mode is not available."}
	}

	if strings.EqualFold(args, "open") {
		if ctx.SessionID == "" {
			return command.Result{Action: "plan-enter", Output: "Plan mode enabled. The plan file will be created when the session starts."}
		}
		pf, err := ctx.App.Plans.Ensure(execCtx, ctx.SessionID)
		if err != nil {
			return command.Result{Output: fmt.Sprintf("Failed to prepare plan file: %v", err)}
		}
		return command.Result{Output: fmt.Sprintf("Plan file: %s", pf.Path)}
	}

	if strings.EqualFold(args, "inspect") {
		return c.inspectPlan(execCtx, ctx)
	}

	if ctx.SessionID != "" && ctx.App.Permissions.SessionMode(ctx.SessionID) == permission.ModePlan {
		return c.inspectPlan(execCtx, ctx)
	}

	result := command.Result{
		Action: "plan-enter",
		Output: "Plan mode enabled. Explore, write the plan file, then call ExitPlanMode for approval.",
	}
	if args != "" {
		result.Prompt = args
	}
	return result
}

func (c *planCmd) inspectPlan(execCtx context.Context, ctx command.Context) command.Result {
	if ctx.SessionID == "" {
		return command.Result{Output: "Plan file is not available without an active session."}
	}
	pf, content, err := ctx.App.Plans.Read(execCtx, ctx.SessionID)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Failed to read plan file: %v", err)}
	}
	modePrefix := "Plan file"
	if ctx.App.Permissions.SessionMode(ctx.SessionID) == permission.ModePlan {
		modePrefix = "Plan mode is active.\nPlan file"
	}
	if strings.TrimSpace(plan.NormalizeBody(content)) == "" {
		return command.Result{Output: fmt.Sprintf("%s: %s\n\nNo plan has been written yet.", modePrefix, pf.Path)}
	}
	return command.Result{Output: fmt.Sprintf("%s: %s\n\n%s", modePrefix, pf.Path, plan.WrapProposed(plan.NormalizeBody(content)))}
}
