package agent

import "context"

// StopCode 枚举停止原因
type StopCode int

const (
	StopCodeNone             StopCode = iota
	StopCodeMaxIterations             // 达到迭代上限
	StopCodeTokenBudget               // token 预算耗尽
	StopCodeCostLimit                 // 成本上限
	StopCodeNoProgress                // 无进展（预留）
	StopCodeRepeatTool                // 重复工具调用（预留）
	StopCodeMaxToolCalls              // 工具调用次数达到上限
	StopCodeProviderCooldown          // 外部 provider 冷却反复命中
	StopCodeCompactFuse               // compact 熔断（预留）
	StopCodeHook                      // 外部 hook 阻止
)

// StopAction 表示停止决策
type StopAction int

const (
	ActionContinue StopAction = iota
	ActionTerminate
)

// StopDecision 停止判定结果
type StopDecision struct {
	Action      StopAction
	PolicyName  string // name of the policy that fired (empty on ActionContinue)
	Reason      string
	Code        StopCode
	Diagnostics map[string]any
}

// LoopState is defined in loop_state.go

// LoopStopPolicy 是纯停止判定接口（无副作用）
type LoopStopPolicy interface {
	Name() string
	Check(ctx context.Context, state *LoopState) StopDecision
}

// StopController 管理一组停止策略，链式执行。
// 非线程安全，仅在单 goroutine 内使用。
type StopController struct {
	policies []LoopStopPolicy
}

// NewStopController 创建一个携带初始策略集合的 StopController。
func NewStopController(policies ...LoopStopPolicy) *StopController {
	sc := &StopController{
		policies: make([]LoopStopPolicy, 0, len(policies)),
	}
	sc.policies = append(sc.policies, policies...)
	return sc
}

// AddPolicy 追加一条停止策略。
func (sc *StopController) AddPolicy(p LoopStopPolicy) {
	sc.policies = append(sc.policies, p)
}

// Run 依次调用每个策略的 Check。
// 一旦某条策略返回 ActionTerminate，立即返回该决策；全部通过则返回 ActionContinue。
func (sc *StopController) Run(ctx context.Context, state *LoopState) StopDecision {
	for _, p := range sc.policies {
		if d := p.Check(ctx, state); d.Action == ActionTerminate {
			d.PolicyName = p.Name()
			return d
		}
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

// stopCodeToTerminalReason maps a StopCode to the corresponding TerminalReason.
func stopCodeToTerminalReason(code StopCode) TerminalReason {
	switch code {
	case StopCodeMaxIterations:
		return ReasonMaxTurnsReached
	case StopCodeNoProgress:
		return ReasonNoProgress
	case StopCodeRepeatTool:
		return ReasonRepeatedToolPattern
	case StopCodeMaxToolCalls:
		return ReasonMaxTurnsReached
	case StopCodeTokenBudget:
		return ReasonTokenBudgetExceeded
	case StopCodeCostLimit:
		return ReasonCostBudgetExceeded
	case StopCodeCompactFuse:
		return ReasonCompactExhausted
	case StopCodeProviderCooldown:
		return ReasonProviderCooldown
	case StopCodeHook:
		return ReasonLoopHookStopped
	default:
		return ReasonLoopHookStopped
	}
}

func warningForStopDecision(d StopDecision) string {
	if d.Reason != "" {
		return "Agent stopped: " + d.Reason
	}
	return TerminalReasonMessage(stopCodeToTerminalReason(d.Code))
}
