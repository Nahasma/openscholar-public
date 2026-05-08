package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/llm/tools"
)

// defaultMaxIterationHook is the default max iteration limit for the stop controller.
// Set to 200 as a safety net; finer-grained control comes from NoProgress/RepeatTool hooks (Phase B).
const defaultMaxIterationHook = 200

const (
	defaultSoftSearchToolCalls = 24
	defaultMaxSearchToolCalls  = 96
)

// MaxIterationHook 在迭代次数达到上限时终止 Agent 循环。
type MaxIterationHook struct {
	MaxIterations int
}

// ProviderCooldownLoopHook terminates early when the same provider/source cooldown
// is hit across rounds (query may differ).
type ProviderCooldownLoopHook struct {
	maxTurns int
}

func NewProviderCooldownLoopHook(maxTurns int) *ProviderCooldownLoopHook {
	if maxTurns <= 0 {
		maxTurns = 2
	}
	return &ProviderCooldownLoopHook{maxTurns: maxTurns}
}

func (h *ProviderCooldownLoopHook) Name() string { return "provider_cooldown" }

func (h *ProviderCooldownLoopHook) Check(_ context.Context, state *LoopState) StopDecision {
	scope := ""
	for _, out := range state.LastRoundToolOutcomes {
		if !out.IsError {
			continue
		}
		if out.ErrorKind != "rate_limited" && out.ErrorKind != "provider_cooldown" && out.ErrorKind != "auth" && out.ErrorKind != "quota_exhausted" {
			continue
		}
		provider := strings.TrimSpace(out.Provider)
		source := strings.TrimSpace(out.Source)
		if provider == "" {
			provider = source
		}
		if source == "" {
			source = provider
		}
		if provider == "" {
			continue
		}
		action := strings.TrimSpace(out.Action)
		if action == "" {
			action = "search"
		}
		scope = strings.ToLower(provider + ":" + source + ":" + action)
		break
	}
	if scope == "" {
		state.ProviderCooldownConsecutive = 0
		state.ProviderCooldownLastScope = ""
		return StopDecision{Action: ActionContinue, Code: StopCodeNone}
	}
	if scope == state.ProviderCooldownLastScope {
		state.ProviderCooldownConsecutive++
	} else {
		state.ProviderCooldownConsecutive = 1
		state.ProviderCooldownLastScope = scope
	}
	if state.ProviderCooldownConsecutive >= h.maxTurns {
		return StopDecision{
			Action: ActionTerminate,
			Reason: fmt.Sprintf("provider cooldown repeated for %s (%d rounds)", scope, state.ProviderCooldownConsecutive),
			Code:   StopCodeProviderCooldown,
			Diagnostics: map[string]any{
				"scope":       scope,
				"consecutive": state.ProviderCooldownConsecutive,
			},
		}
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

type ToolCallLimitHook struct {
	maxCalls int
}

func NewToolCallLimitHook(maxCalls int) *ToolCallLimitHook {
	return &ToolCallLimitHook{maxCalls: maxCalls}
}

func (h *ToolCallLimitHook) Name() string { return "tool_call_limit" }

func (h *ToolCallLimitHook) Check(_ context.Context, state *LoopState) StopDecision {
	if h.maxCalls <= 0 {
		return StopDecision{Action: ActionContinue, Code: StopCodeNone}
	}
	if state.TotalToolCalls >= h.maxCalls {
		return StopDecision{
			Action: ActionTerminate,
			Reason: fmt.Sprintf("reached max tool calls: %d/%d", state.TotalToolCalls, h.maxCalls),
			Code:   StopCodeMaxToolCalls,
			Diagnostics: map[string]any{
				"tool_calls": state.TotalToolCalls,
				"max":        h.maxCalls,
			},
		}
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

type SearchConvergenceBudgetHook struct {
	// MaxSearchToolCalls is the soft search budget. Past this point search can
	// continue only while recent rounds keep producing new evidence or durable
	// progress. This preserves room for long research tasks without letting
	// empty or repetitive search loops run indefinitely.
	MaxSearchToolCalls      int
	HardMaxSearchToolCalls  int
	MaxSameTargetFetch      int
	MaxSameTargetOutcome    int
	MaxExactFailure         int
	MaxProviderCooldown     int
	MaxSearchNoProgressTurn int
	searchNoProgressTurns   int
}

func NewSearchConvergenceBudgetHook() *SearchConvergenceBudgetHook {
	return &SearchConvergenceBudgetHook{
		MaxSearchToolCalls:      defaultSoftSearchToolCalls,
		HardMaxSearchToolCalls:  defaultMaxSearchToolCalls,
		MaxSameTargetFetch:      3,
		MaxSameTargetOutcome:    3,
		MaxExactFailure:         3,
		MaxProviderCooldown:     2,
		MaxSearchNoProgressTurn: 4,
	}
}

func (h *SearchConvergenceBudgetHook) Name() string { return "search_convergence_budget" }

func (h *SearchConvergenceBudgetHook) Check(_ context.Context, state *LoopState) StopDecision {
	if h.HardMaxSearchToolCalls > 0 && state.SearchToolCalls >= h.HardMaxSearchToolCalls {
		return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "search hard budget exhausted", Diagnostics: map[string]any{"search_tool_calls": state.SearchToolCalls, "hard_max": h.HardMaxSearchToolCalls}}
	}
	if h.MaxSearchToolCalls > 0 && state.SearchToolCalls >= h.MaxSearchToolCalls && h.shouldStopAtSoftSearchBudget(state) {
		return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "search soft budget reached without new evidence", Diagnostics: map[string]any{"search_tool_calls": state.SearchToolCalls, "soft_max": h.MaxSearchToolCalls, "new_evidence": state.LastRoundProgress.NewEvidenceCount, "durable_progress": state.LastRoundProgress.DurableProgress}}
	}
	if h.MaxSearchNoProgressTurn > 0 {
		rp := state.LastRoundProgress
		if roundHasSearchTool(state) && rp.ToolCalls > 0 && !rp.DurableProgress && rp.LowValueInspection {
			h.searchNoProgressTurns++
		} else {
			h.searchNoProgressTurns = 0
		}
		if h.searchNoProgressTurns >= h.MaxSearchNoProgressTurn {
			return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "search rounds have no durable progress"}
		}
	}
	for key, count := range state.TargetCounts {
		if strings.HasPrefix(strings.ToLower(key), "webfetch:") && count >= h.MaxSameTargetFetch && lastRoundHasRepeatedLowValueTarget(state, key) {
			return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "same low-value webfetch target repeated", Diagnostics: map[string]any{"target_key": key, "count": count}}
		}
	}
	for _, out := range state.LastRoundToolOutcomes {
		if out.TargetKey != "" && out.OutcomeHash != "" && strings.HasPrefix(strings.ToLower(out.ToolName), "webfetch") {
			k := out.TargetKey + "|" + out.OutcomeHash
			if state.OutcomeCounts[k] >= h.MaxSameTargetOutcome {
				return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "same target and outcome repeated", Diagnostics: map[string]any{"target_outcome": k, "count": state.OutcomeCounts[k]}}
			}
		}
		if out.IsError && out.Signature != "" && state.FailureCounts[out.Signature] >= h.MaxExactFailure {
			return StopDecision{Action: ActionTerminate, Code: StopCodeHook, Reason: "exact failure repeated", Diagnostics: map[string]any{"signature": out.Signature, "count": state.FailureCounts[out.Signature]}}
		}
	}
	if state.ProviderCooldownConsecutive >= h.MaxProviderCooldown {
		return StopDecision{Action: ActionTerminate, Code: StopCodeProviderCooldown, Reason: "provider/source cooldown repeated"}
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

func (h *SearchConvergenceBudgetHook) shouldStopAtSoftSearchBudget(state *LoopState) bool {
	if state == nil || !roundHasSearchTool(state) {
		return false
	}
	rp := state.LastRoundProgress
	return rp.NewEvidenceCount == 0 && !rp.DurableProgress
}

func roundHasSearchTool(state *LoopState) bool {
	if state == nil {
		return false
	}
	for _, out := range state.LastRoundToolOutcomes {
		if isSearchFamilyTool(out.ToolName) {
			return true
		}
	}
	return false
}

func lastRoundHasRepeatedLowValueTarget(state *LoopState, targetKey string) bool {
	for _, out := range state.LastRoundToolOutcomes {
		if out.TargetKey != targetKey {
			continue
		}
		if out.LowValueReason != "" || isLowValueInspectionOutcome(out) {
			return true
		}
	}
	return false
}

type RepeatedInspectionTargetHook struct{}

func NewRepeatedInspectionTargetHook() *RepeatedInspectionTargetHook {
	return &RepeatedInspectionTargetHook{}
}
func (h *RepeatedInspectionTargetHook) Name() string { return "repeated_inspection_target" }
func (h *RepeatedInspectionTargetHook) Check(_ context.Context, state *LoopState) StopDecision {
	for _, out := range state.LastRoundToolOutcomes {
		class := strings.ToLower(strings.TrimSpace(out.ContentClass))
		if class != "list_page" && class != "search_page" && class != "index_page" {
			continue
		}
		if out.TargetKey == "" {
			continue
		}
		if state.TargetCounts[out.TargetKey] >= 3 {
			return StopDecision{
				Action: ActionTerminate,
				Code:   StopCodeHook,
				Reason: "repeated inspection target detected",
				Diagnostics: map[string]any{
					"target_key":       out.TargetKey,
					"content_class":    class,
					"count":            state.TargetCounts[out.TargetKey],
					"low_value_reason": out.LowValueReason,
				},
			}
		}
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

// NewMaxIterationHook 创建一个最大迭代次数限制策略。
func NewMaxIterationHook(maxIter int) *MaxIterationHook {
	return &MaxIterationHook{MaxIterations: maxIter}
}

// Name 返回策略标识符。
func (h *MaxIterationHook) Name() string {
	return "max_iteration"
}

// Check 判断当前迭代次数是否已达上限。
// 若 state.Iteration >= h.MaxIterations，返回 ActionTerminate + StopCodeMaxIterations；
// 否则返回 ActionContinue。
func (h *MaxIterationHook) Check(_ context.Context, state *LoopState) StopDecision {
	diag := map[string]any{
		"iteration": state.Iteration,
		"max":       h.MaxIterations,
	}
	if state.Iteration >= h.MaxIterations {
		return StopDecision{
			Action:      ActionTerminate,
			Reason:      fmt.Sprintf("reached max iterations: %d/%d", state.Iteration, h.MaxIterations),
			Code:        StopCodeMaxIterations,
			Diagnostics: diag,
		}
	}
	return StopDecision{
		Action:      ActionContinue,
		Code:        StopCodeNone,
		Diagnostics: diag,
	}
}

// --- RepeatedToolPatternHook ---

// RepeatedToolPatternHook 检测工具调用重复模式。
// 使用升级版 DoomLoopDetector（窗口 5, 阈值 3）。
type RepeatedToolPatternHook struct {
	detector *tools.DoomLoopDetector
}

// NewRepeatedToolPatternHook 创建一个重复工具模式检测策略。
func NewRepeatedToolPatternHook() *RepeatedToolPatternHook {
	return &RepeatedToolPatternHook{
		detector: tools.NewDoomLoopDetectorWithConfig(5, 3),
	}
}

// Name 返回策略标识符。
func (h *RepeatedToolPatternHook) Name() string {
	return "repeated_tool_pattern"
}

// Check 使用 Observe 检查 LastRoundToolSignatures 是否存在重复模式。
func (h *RepeatedToolPatternHook) Check(_ context.Context, state *LoopState) StopDecision {
	if len(state.LastRoundToolSignatures) == 0 {
		return StopDecision{Action: ActionContinue, Code: StopCodeNone}
	}

	if h.detector.Observe(state.LastRoundToolSignatures) {
		// 确认检测到重复后，将签名录入内部状态
		for _, sig := range state.LastRoundToolSignatures {
			h.detector.RecordCall(sig)
		}
		return StopDecision{
			Action: ActionTerminate,
			Reason: "repeated tool call pattern detected",
			Code:   StopCodeRepeatTool,
			Diagnostics: map[string]any{
				"signatures": state.LastRoundToolSignatures,
			},
		}
	}

	// 未检测到重复，录入签名以积累历史
	for _, sig := range state.LastRoundToolSignatures {
		h.detector.RecordCall(sig)
	}
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

// --- NoProgressHook ---

// NoProgressHook 在连续多轮无业务进展时终止循环。
type NoProgressHook struct {
	maxNoProgressTurns int
	noProgressCount    int
}

// NewNoProgressHook 创建一个无进展检测策略。
// maxTurns: 连续无进展轮数阈值（默认 6）。
func NewNoProgressHook(maxTurns int) *NoProgressHook {
	return &NoProgressHook{
		maxNoProgressTurns: maxTurns,
	}
}

// Name 返回策略标识符。
func (h *NoProgressHook) Name() string {
	return "no_progress"
}

// Check 判断是否连续多轮无进展。
func (h *NoProgressHook) Check(_ context.Context, state *LoopState) StopDecision {
	rp := state.LastRoundProgress
	noProgress := !rp.DurableProgress && (rp.ToolCalls == 0 || rp.FailedTools == rp.ToolCalls || rp.LowValueInspection || rp.ResultBytes == 0)
	if noProgress {
		h.noProgressCount++
	} else {
		h.noProgressCount = 0
	}

	diag := map[string]any{
		"no_progress_count":      h.noProgressCount,
		"max":                    h.maxNoProgressTurns,
		"token_delta_diagnostic": state.LastDeltaTokens,
		"tool_calls":             rp.ToolCalls,
		"durable_progress":       rp.DurableProgress,
		"low_value_inspection":   rp.LowValueInspection,
		"result_bytes":           rp.ResultBytes,
	}

	if h.noProgressCount >= h.maxNoProgressTurns {
		return StopDecision{
			Action:      ActionTerminate,
			Reason:      fmt.Sprintf("no progress for %d consecutive turns", h.noProgressCount),
			Code:        StopCodeNoProgress,
			Diagnostics: diag,
		}
	}

	return StopDecision{
		Action:      ActionContinue,
		Code:        StopCodeNone,
		Diagnostics: diag,
	}
}

// --- BudgetExceededHook ---

// BudgetExceededHook 在每轮检查 token/cost 预算是否超限。
// 与 BudgetController 不同，这是硬停检查（tool-use 前）。
type BudgetExceededHook struct {
	tokenBudget   int64
	costBudgetUSD float64
}

// NewBudgetExceededHook 创建预算超限检测策略。
func NewBudgetExceededHook(bl BudgetLimits) *BudgetExceededHook {
	return &BudgetExceededHook{
		tokenBudget:   bl.TokenBudget,
		costBudgetUSD: bl.CostBudgetUSD,
	}
}

// Name 返回策略标识符。
func (h *BudgetExceededHook) Name() string {
	return "budget_exceeded"
}

// Check 判断是否超出预算。
func (h *BudgetExceededHook) Check(_ context.Context, state *LoopState) StopDecision {
	// Cost limit
	if h.costBudgetUSD > 0 && state.CostUnknown {
		return StopDecision{
			Action: ActionTerminate,
			Reason: "cost budget cannot be enforced for unknown-cost model",
			Code:   StopCodeCostLimit,
			Diagnostics: map[string]any{
				"cost_known":  false,
				"cost_budget": h.costBudgetUSD,
			},
		}
	}
	if h.costBudgetUSD > 0 && state.CostAccumulated >= h.costBudgetUSD {
		return StopDecision{
			Action: ActionTerminate,
			Reason: fmt.Sprintf("cost budget exceeded: $%.4f >= $%.4f", state.CostAccumulated, h.costBudgetUSD),
			Code:   StopCodeCostLimit,
			Diagnostics: map[string]any{
				"cost_accumulated": state.CostAccumulated,
				"cost_budget":      h.costBudgetUSD,
			},
		}
	}

	// Token limit
	if h.tokenBudget > 0 && state.TotalPromptTokens >= h.tokenBudget {
		return StopDecision{
			Action: ActionTerminate,
			Reason: fmt.Sprintf("token budget exceeded: %d >= %d", state.TotalPromptTokens, h.tokenBudget),
			Code:   StopCodeTokenBudget,
			Diagnostics: map[string]any{
				"total_prompt_tokens": state.TotalPromptTokens,
				"token_budget":        h.tokenBudget,
			},
		}
	}

	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}
