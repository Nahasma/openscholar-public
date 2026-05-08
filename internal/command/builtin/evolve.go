package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
)

type evolveCmd struct{}

func (c *evolveCmd) Name() string        { return "evolve" }
func (c *evolveCmd) Description() string { return "基于用户反馈进化技能" }

func (c *evolveCmd) Execute(ctx command.Context) command.Result {
	if ctx.App.EvolutionService == nil {
		return command.Result{Output: "Evolution service not initialized."}
	}

	args := strings.TrimSpace(ctx.Args)
	parts := strings.Fields(args)

	if len(parts) > 0 {
		switch parts[0] {
		case "status":
			return c.status(ctx)
		case "rollback":
			if len(parts) < 2 {
				return command.Result{Output: "用法: /evolve rollback <skill_id>"}
			}
			return c.rollback(ctx, parts[1])
		case "dismiss":
			return c.dismiss(ctx)
		}
	}

	return c.runEvolution(ctx)
}

func (c *evolveCmd) runEvolution(ctx command.Context) command.Result {
	svc := ctx.App.EvolutionService
	result, analysis, cases, err := svc.RunEvolution(context.Background())
	if err != nil {
		return command.Result{Output: fmt.Sprintf("进化失败: %v", err)}
	}

	if result.Action == "no_change_needed" {
		msg := "当前技能库无需调整。"
		if result.Summary != "" {
			msg = result.Summary
		}
		if len(cases) > 0 {
			msg = fmt.Sprintf("分析 %d 条反馈后，%s", len(cases), msg)
		}
		return command.Result{Output: msg}
	}

	var sb strings.Builder
	if len(cases) > 0 {
		fmt.Fprintf(&sb, "分析 %d 条待处理反馈...\n\n", len(cases))
	}

	// 展示分析结果
	if analysis != nil && len(analysis.FailurePatterns) > 0 {
		sb.WriteString("── Analysis ──\n")
		for i, fp := range analysis.FailurePatterns {
			fmt.Fprintf(&sb, "%d. [%s] %s (%d cases)\n", i+1, fp.RootCause, fp.PatternName, len(fp.AffectedCases))
		}
		sb.WriteString("\n")
	}

	// 展示变更提案
	if len(result.Changes) > 0 {
		change := result.Changes[0]
		sb.WriteString("── Proposal ──\n")
		fmt.Fprintf(&sb, "Action: %s\n", change.Action)
		if change.AddNew != nil {
			fmt.Fprintf(&sb, "New Skill: %s/%s\n", change.AddNew.Category, change.AddNew.Name)
			fmt.Fprintf(&sb, "Description: %s\n", change.AddNew.Description)
		}
		if change.RefineExisting != nil {
			fmt.Fprintf(&sb, "Skill: %s\n", change.RefineExisting.SkillID)
			for field, val := range change.RefineExisting.Changes {
				preview := val
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}
				fmt.Fprintf(&sb, "  %s → %s\n", field, preview)
			}
		}
		fmt.Fprintf(&sb, "Reasoning: %s\n", change.Reasoning)
		sb.WriteString("\n")
	}

	sb.WriteString(result.Summary)

	// 用 Prompt 方式让 Agent 询问用户是否应用
	prompt := fmt.Sprintf(`用户刚执行了 /evolve 命令，进化分析结果如下：

%s

请向用户展示上述分析结果，然后询问用户是否应用这个变更（输入 y 确认，n 取消）。
如果用户确认，调用 SkillManage 工具执行对应的变更。`, sb.String())

	return command.Result{Prompt: prompt}
}

func (c *evolveCmd) status(ctx command.Context) command.Result {
	svc := ctx.App.EvolutionService

	pending, _ := svc.ListCases(context.Background(), "pending", 100)
	resolved, _ := svc.ListCases(context.Background(), "resolved", 100)
	dismissed, _ := svc.ListCases(context.Background(), "dismissed", 100)

	var sb strings.Builder
	sb.WriteString("── Evolution Status ──\n")
	fmt.Fprintf(&sb, "Pending:   %d 条待处理反馈\n", len(pending))
	fmt.Fprintf(&sb, "Resolved:  %d 条已处理\n", len(resolved))
	fmt.Fprintf(&sb, "Dismissed: %d 条已忽略\n\n", len(dismissed))

	if len(pending) > 0 {
		sb.WriteString("最近待处理反馈:\n")
		for i, c := range pending {
			if i >= 5 {
				fmt.Fprintf(&sb, "  ... 还有 %d 条\n", len(pending)-5)
				break
			}
			fb := c.Feedback
			if len(fb) > 60 {
				fb = fb[:60] + "..."
			}
			fmt.Fprintf(&sb, "  - [%s] %s\n", c.ID[:8], fb)
		}
	}

	return command.Result{Output: sb.String()}
}

func (c *evolveCmd) rollback(ctx command.Context, skillID string) command.Result {
	if err := ctx.App.EvolutionService.Rollback(context.Background(), skillID); err != nil {
		return command.Result{Output: fmt.Sprintf("回滚失败: %v", err)}
	}
	return command.Result{Output: fmt.Sprintf("已回滚 %s 到上一版本", skillID)}
}

func (c *evolveCmd) dismiss(ctx command.Context) command.Result {
	if err := ctx.App.EvolutionService.DismissAll(context.Background()); err != nil {
		return command.Result{Output: fmt.Sprintf("清除失败: %v", err)}
	}
	return command.Result{Output: "已清除所有待处理反馈"}
}
