package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/research"
)

type researchCmd struct{}

func (c *researchCmd) Name() string        { return "research" }
func (c *researchCmd) Description() string { return "管理科研流水线" }

func (c *researchCmd) Execute(ctx command.Context) command.Result {
	if ctx.App.ResearchEngine == nil {
		return command.Result{Output: "Research engine not initialized."}
	}

	args := strings.TrimSpace(ctx.Args)
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return c.usage()
	}

	switch parts[0] {
	case "start":
		return c.start(ctx, args[len(parts[0]):])
	case "status":
		return c.status(ctx)
	case "pause":
		return c.pause(ctx)
	case "resume":
		return c.resume(ctx)
	case "cost":
		return c.cost(ctx)
	default:
		return c.usage()
	}
}

func (c *researchCmd) usage() command.Result {
	return command.Result{Output: `用法:
  /research start "主题"              启动科研流水线
  /research start "主题" --template survey --mode auto --budget 10
  /research start "主题" --template aris_empirical --mode auto --effort max --assurance submission
  /research start "主题" --dir ~/papers/my-research  指定工作目录
  /research status                    查看进度
  /research pause                     暂停
  /research resume                    恢复
  /research cost                      费用明细`}
}

func (c *researchCmd) start(ctx command.Context, argsStr string) command.Result {
	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}

	sessionID := strings.TrimSpace(ctx.SessionID)
	if sessionID != "" {
		if sess, err := ctx.App.Sessions.Get(execCtx, sessionID); err == nil && strings.TrimSpace(sess.ParentSessionID) != "" {
			return command.Result{Output: "启动 research pipeline 需要主会话，当前子任务会话不支持直接启动。"}
		}
	}
	if sessionID == "" {
		sess, err := ctx.App.Sessions.Create(execCtx, "")
		if err != nil {
			return command.Result{Output: fmt.Sprintf("创建会话失败: %v", err)}
		}
		sessionID = sess.ID
	}

	// 解析参数
	topic, flags := parseStartArgs(strings.TrimSpace(argsStr))
	if topic == "" {
		return command.Result{Output: "请提供研究主题，例如: /research start \"Transformer 蛋白质结构预测\""}
	}

	template := flags["template"]
	if template == "" {
		template = "empirical"
	}
	mode := research.AutomationMode(flags["mode"])
	if mode == "" {
		mode = research.ModeDefault
	}
	var budget float64
	if b, ok := flags["budget"]; ok {
		budget, _ = strconv.ParseFloat(b, 64)
	}
	if budget == 0 {
		budget = 8 // 默认 $8（混合策略）
	}

	dir := flags["dir"]
	// Expand ~ to home directory
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}

	engine := ctx.App.ResearchEngine
	var p *research.Pipeline
	var err error
	var policy research.ResearchRunPolicy
	if research.IsARISTemplate(template) {
		policy = researchRunPolicyFromFlags(flags)
		if policy.HumanCheckpoint && mode == research.ModeAuto {
			mode = research.ModeDefault
		}
		p, err = engine.CreateWithPolicy(sessionID, topic, template, mode, budget, dir, policy)
	} else {
		p, err = engine.Create(sessionID, topic, template, mode, budget, dir)
	}
	if err != nil {
		return command.Result{Output: fmt.Sprintf("创建失败: %v", err), SessionID: sessionID}
	}

	phases, _ := engine.GetPhases(p.ID)

	// 启动第一个阶段
	if len(phases) > 0 {
		if err := engine.StartPhase(p.ID, phases[0].ID); err != nil {
			return command.Result{
				Output:    fmt.Sprintf("已创建研究流水线 [%s]，但第 1 阶段启动失败: %v", p.ID[:8], err),
				SessionID: sessionID,
			}
		}
	}

	// Send a simple prompt to trigger research mode; pipeline context is injected via buildResearchContext
	firstPhase := ""
	if len(phases) > 0 {
		firstPhase = phases[0].Name
	}

	output := fmt.Sprintf("已创建研究流水线 [%s]\n主题: %s\n模板: %s | 模式: %s | 预算: $%.0f\n阶段: %d 个\n工作目录: %s/\n",
		p.ID[:8], topic, template, p.Mode, budget, len(phases), p.WorkDir)
	if research.IsARISTemplate(template) {
		policy = research.NormalizeResearchRunPolicy(policy)
		output += fmt.Sprintf("ARIS: effort=%s | assurance=%s | reviewer=%s | rounds=%d | batch=%s | trace=%s | auto_write=%t | human_checkpoint=%t\n",
			policy.Effort, policy.Assurance, policy.ReviewerDifficulty, policy.MaxReviewRounds, policy.BatchPolicy, policy.TraceMode, policy.AutoWrite, policy.HumanCheckpoint)
	}
	output += fmt.Sprintf("\n正在启动第 1 阶段「%s」...\n后台 leader 任务将自动执行该阶段。", firstPhase)

	return command.Result{
		Action:    "research-started",
		SessionID: sessionID,
		Output:    output,
	}
}

func (c *researchCmd) status(ctx command.Context) command.Result {
	engine := ctx.App.ResearchEngine
	p, err := engine.GetBySession(ctx.SessionID)
	if err != nil {
		return command.Result{Output: "当前会话没有活跃的研究流水线。"}
	}
	phases, _ := engine.GetPhases(p.ID)

	var sb strings.Builder
	fmt.Fprintf(&sb, "── Research Pipeline [%s] ──\n", p.ID[:8])
	fmt.Fprintf(&sb, "主题: %s\n", p.Topic)
	fmt.Fprintf(&sb, "状态: %s | 模式: %s | 预算: $%.2f/$%.0f\n\n", p.Status, p.Mode, p.Budget.Spent, p.Budget.Limit)

	for _, ph := range phases {
		icon := phaseIcon(ph.Status)
		fmt.Fprintf(&sb, "  %s %d/%d %s", icon, ph.Order, len(phases), ph.Name)
		if ph.Status == research.PhaseCompleted && ph.CompletedAt > 0 {
			sb.WriteString(" ✓")
		}
		sb.WriteString("\n")
	}

	return command.Result{Output: sb.String()}
}

func (c *researchCmd) pause(ctx command.Context) command.Result {
	engine := ctx.App.ResearchEngine
	p, err := engine.GetBySession(ctx.SessionID)
	if err != nil {
		return command.Result{Output: "当前会话没有活跃的研究流水线。"}
	}
	engine.Pause(p.ID)
	return command.Result{Output: "已暂停研究流水线。"}
}

func (c *researchCmd) resume(ctx command.Context) command.Result {
	engine := ctx.App.ResearchEngine
	p, err := engine.GetBySession(ctx.SessionID)
	if err != nil {
		return command.Result{Output: "当前会话没有活跃的研究流水线。"}
	}
	engine.Resume(p.ID)
	return command.Result{Output: "已恢复研究流水线。"}
}

func (c *researchCmd) cost(ctx command.Context) command.Result {
	engine := ctx.App.ResearchEngine
	p, err := engine.GetBySession(ctx.SessionID)
	if err != nil {
		return command.Result{Output: "当前会话没有活跃的研究流水线。"}
	}
	return command.Result{Output: fmt.Sprintf("费用: $%.2f / $%.0f (%.0f%%)",
		p.Budget.Spent, p.Budget.Limit,
		func() float64 {
			if p.Budget.Limit > 0 {
				return p.Budget.Spent / p.Budget.Limit * 100
			}
			return 0
		}())}
}

// --- helpers ---

func phaseIcon(status research.PhaseStatus) string {
	switch status {
	case research.PhasePending:
		return "⏳"
	case research.PhaseRunning:
		return "⠋"
	case research.PhaseCompleted:
		return "✓"
	case research.PhaseFailed:
		return "✗"
	case research.PhasePaused:
		return "⏸"
	default:
		return "?"
	}
}

// parseStartArgs 解析 /research start 的参数：topic + --flag value 对
func parseStartArgs(args string) (topic string, flags map[string]string) {
	flags = make(map[string]string)

	// 提取引号包裹的主题
	if idx := strings.Index(args, "\""); idx >= 0 {
		end := strings.Index(args[idx+1:], "\"")
		if end >= 0 {
			topic = args[idx+1 : idx+1+end]
			args = args[idx+1+end+1:]
		}
	} else {
		// 无引号，第一个 -- 之前的部分作为 topic
		if dashIdx := strings.Index(args, "--"); dashIdx > 0 {
			topic = strings.TrimSpace(args[:dashIdx])
			args = args[dashIdx:]
		} else {
			topic = strings.TrimSpace(args)
			return
		}
	}

	// 解析 --key value 对；布尔开关可省略 value。
	parts := strings.Fields(args)
	for i := 0; i < len(parts); i++ {
		if !strings.HasPrefix(parts[i], "--") {
			continue
		}
		key := strings.TrimPrefix(parts[i], "--")
		if i+1 >= len(parts) || strings.HasPrefix(parts[i+1], "--") {
			flags[key] = "true"
			continue
		}
		flags[key] = parts[i+1]
		i++
	}
	return
}

func researchRunPolicyFromFlags(flags map[string]string) research.ResearchRunPolicy {
	p := research.ResearchRunPolicy{}
	if v := strings.TrimSpace(flags["effort"]); v != "" {
		p.Effort = v
	}
	if v := strings.TrimSpace(flags["assurance"]); v != "" {
		p.Assurance = v
	}
	if v := strings.TrimSpace(flags["reviewer-difficulty"]); v != "" {
		p.ReviewerDifficulty = v
	}
	if v := strings.TrimSpace(flags["venue"]); v != "" {
		p.Venue = v
	}
	if v := strings.TrimSpace(flags["batch"]); v != "" {
		p.BatchPolicy = v
	}
	if v := strings.TrimSpace(flags["trace"]); v != "" {
		p.TraceMode = v
	}
	if rounds, ok := parsePositiveIntFlag(flags, "max-review-rounds"); ok {
		p.MaxReviewRounds = rounds
	}
	p.AutoWrite = parseBoolStartFlag(flags, "auto-write")
	p.HumanCheckpoint = parseBoolStartFlag(flags, "human-checkpoint")
	p = research.NormalizeResearchRunPolicy(p)
	if p.HumanCheckpoint {
		p.AutoProceed = false
	}
	return p
}

func parsePositiveIntFlag(flags map[string]string, key string) (int, bool) {
	raw, ok := flags[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parseBoolStartFlag(flags map[string]string, key string) bool {
	raw, ok := flags[key]
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}
