package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
)

type skillStatsCmd struct{}

func (c *skillStatsCmd) Name() string        { return "skill-stats" }
func (c *skillStatsCmd) Description() string { return "查看技能使用统计与反馈" }

func (c *skillStatsCmd) Execute(ctx command.Context) command.Result {
	if ctx.App.EvolutionService == nil {
		return command.Result{Output: "Evolution service not initialized."}
	}

	skillID := strings.TrimSpace(ctx.Args)
	if skillID != "" {
		return c.detail(ctx, skillID)
	}
	return c.overview(ctx)
}

func (c *skillStatsCmd) overview(ctx command.Context) command.Result {
	stats, err := ctx.App.EvolutionService.AllSkillStats(context.Background())
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Error: %v", err)}
	}
	if len(stats) == 0 {
		return command.Result{Output: "No skills found in the skill bank."}
	}

	var sb strings.Builder
	sb.WriteString("── Skill Statistics ──\n")
	fmt.Fprintf(&sb, "%-35s %5s %8s\n", "ID", "Uses", "Feedback")

	for _, s := range stats {
		flag := ""
		if s.FeedbackCount >= 3 {
			flag = " ⚠"
		}
		fmt.Fprintf(&sb, "%-35s %5d %7d%s\n", s.SkillID, s.UsageCount, s.FeedbackCount, flag)
	}

	fmt.Fprintf(&sb, "\nTotal: %d skills\n", len(stats))
	return command.Result{Output: sb.String()}
}

func (c *skillStatsCmd) detail(ctx command.Context, skillID string) command.Result {
	svc := ctx.App.EvolutionService

	stats, err := svc.SkillStats(context.Background(), skillID)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("未找到技能: %s", skillID)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "── %s ──\n", skillID)
	fmt.Fprintf(&sb, "Usage: %d | Feedback: %d\n\n", stats.UsageCount, stats.FeedbackCount)

	// 显示相关反馈
	if ctx.App.EvolutionStore != nil {
		cases, _ := ctx.App.EvolutionStore.ListCasesBySkill(skillID, 5)
		if len(cases) > 0 {
			sb.WriteString("Recent Feedback:\n")
			for _, c := range cases {
				fb := c.Feedback
				if len(fb) > 60 {
					fb = fb[:60] + "..."
				}
				fmt.Fprintf(&sb, "  - [%s] %s\n", c.Status, fb)
			}
		}
	}

	return command.Result{Output: sb.String()}
}
