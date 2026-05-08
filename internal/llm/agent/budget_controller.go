package agent

// ContinuationAction 表示 continuation gate 的决策。
type ContinuationAction int

const (
	ContinuationDeny     ContinuationAction = iota // 正常结束（不 continuation）
	ContinuationAllow                              // 允许继续
	ContinuationHardStop                           // 硬停（成本超限）
)

// ContinuationDecision 是 BudgetController.ShouldContinue 的返回值。
type ContinuationDecision struct {
	Action    ContinuationAction
	Reason    string
	NudgeText string // 非空时注入 ephemeral user message
}

// BudgetController 在 end_turn 时决定是否允许 LLM 继续生成。
type BudgetController struct {
	tokenBudget         int64
	costBudgetUSD       float64
	completionThreshold float64
	diminishingMinDelta int64
	maxContinuations    int
}

// NewBudgetController 从 BudgetLimits 初始化 BudgetController。
func NewBudgetController(bl BudgetLimits) *BudgetController {
	return &BudgetController{
		tokenBudget:         bl.TokenBudget,
		costBudgetUSD:       bl.CostBudgetUSD,
		completionThreshold: bl.CompletionThreshold,
		diminishingMinDelta: bl.DiminishingMinDeltaToken,
		maxContinuations:    bl.MaxContinuations,
	}
}

// ShouldContinue 根据当前循环状态决定是否允许 LLM 继续生成。
// 判定顺序见文档注释。
func (bc *BudgetController) ShouldContinue(state *LoopState) ContinuationDecision {
	// 1. Cost limit 硬停
	if bc.costBudgetUSD > 0 && state.CostUnknown {
		return ContinuationDecision{
			Action: ContinuationHardStop,
			Reason: "cost budget cannot be enforced for unknown-cost model",
		}
	}
	if bc.costBudgetUSD > 0 && state.CostAccumulated >= bc.costBudgetUSD {
		return ContinuationDecision{
			Action: ContinuationHardStop,
			Reason: "cost budget exceeded",
		}
	}

	// 2. 无 budget 配置：保持现有行为，不做 continuation
	if bc.tokenBudget <= 0 {
		return ContinuationDecision{
			Action: ContinuationDeny,
			Reason: "no token budget configured",
		}
	}

	// 3. Continuation 上限
	if state.ContinuationCount >= bc.maxContinuations {
		return ContinuationDecision{
			Action: ContinuationDeny,
			Reason: "continuation limit reached",
		}
	}

	// 4. Diminishing returns
	if state.LastDeltaTokens > 0 && state.LastDeltaTokens < bc.diminishingMinDelta {
		return ContinuationDecision{
			Action: ContinuationDeny,
			Reason: "diminishing returns: token delta below minimum threshold",
		}
	}

	// 5. Token budget 耗尽
	ratio := float64(state.TotalPromptTokens) / float64(bc.tokenBudget)
	if ratio >= 1.0 {
		return ContinuationDecision{
			Action: ContinuationDeny,
			Reason: "token budget exhausted",
		}
	}

	// 6. 接近阈值
	if ratio >= bc.completionThreshold {
		return ContinuationDecision{
			Action:    ContinuationAllow,
			Reason:    "approaching token budget limit",
			NudgeText: "You are approaching your token budget limit. Please wrap up your current task and provide a final response.",
		}
	}

	// 7. 默认：允许继续
	return ContinuationDecision{
		Action: ContinuationAllow,
		Reason: "within budget",
	}
}
