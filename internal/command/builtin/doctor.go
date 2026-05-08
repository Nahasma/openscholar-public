package builtin

import (
	"context"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/doctor"
)

type doctorBuiltinCmd struct{}

func (c *doctorBuiltinCmd) Name() string        { return "doctor" }
func (c *doctorBuiltinCmd) Description() string { return "Check environment dependencies" }

func (c *doctorBuiltinCmd) Execute(ctx command.Context) command.Result {
	if strings.TrimSpace(ctx.Args) == "providers" {
		return command.Result{Output: builtinProviderConnectivityReport(ctx)}
	}
	checks := doctor.RunChecks()
	return command.Result{Output: doctor.FormatReport(checks)}
}

func builtinProviderConnectivityReport(ctx command.Context) string {
	cfg := config.Get()
	if cfg == nil {
		return "provider connectivity skipped: config is not loaded"
	}
	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}
	report := doctor.CollectProviderConnectivity(execCtx, cfg)
	return doctor.FormatProviderConnectivityReport(report)
}
