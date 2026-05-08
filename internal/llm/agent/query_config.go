package agent

import (
	"math"

	"github.com/openscholar/openscholar/internal/config"
)

// QueryConfig 在 processGeneration() 入口一次性构建，整个循环内不可变。
// 参考 Claude Code 的 query/config.ts 设计。
type QueryConfig struct {
	SessionID string
	ModelID   string

	Loop    LoopLimits
	Budget  BudgetLimits
	Compact CompactLimits

	// 计算字段（构建时填充）
	EffectiveContextWindow   int
	AutoCompactThreshold     int
	BlockingCompactThreshold int
}

// LoopLimits 控制循环迭代次数和工具调用次数上限。
type LoopLimits struct {
	MaxIterations    int // 默认 200（由 StopHook 进一步约束，实际止血由 MaxIterationHook=12 控制）
	MaxToolCalls     int // 默认 100
	MaxStreamRetries int // 默认 5
}

// BudgetLimits 控制 token 和费用预算。
type BudgetLimits struct {
	TokenBudget              int64   // 0 = 不限
	CostBudgetUSD            float64 // 0 = 不限
	DiminishingMinDeltaToken int64   // 默认 500
	MaxContinuations         int     // 默认 3
	CompletionThreshold      float64 // 默认 0.90
}

// CompactLimits 控制上下文压缩行为。
type CompactLimits struct {
	Enabled                 bool
	ReserveOutputTokens     int // 默认 min(maxOutput, 20000)
	AutoCompactBufferTokens int // 默认 13000
	BlockingBufferTokens    int // 默认 3000
	WarningBufferTokens     int // 默认 20000
	ManualBufferTokens      int // 默认 3000
	MaxConsecutiveFailures  int // 默认 3
	MicroCompactKeepRecent  int // 默认 10
}

const (
	defaultMaxIterations    = 200
	defaultMaxToolCalls     = 100
	defaultMaxStreamRetries = 5

	defaultDiminishingMinDeltaToken = 500
	defaultMaxContinuations         = 3
	defaultCompletionThreshold      = 0.90

	defaultAutoCompactBufferTokens = 13000
	defaultBlockingBufferTokens    = 3000
	defaultWarningBufferTokens     = 20000
	defaultManualCompactBuffer     = 3000
	defaultMaxConsecutiveFailures  = 3
	defaultMicroCompactKeepRecent  = 10
	defaultReserveOutputTokens     = 20000
)

// BuildQueryConfig 从模型信息和配置构建不可变快照。
func BuildQueryConfig(sessionID string, modelID string, contextWindow int64, maxOutputTokens int64, cfg *config.Config) QueryConfig {
	// 计算 ReserveOutputTokens = min(maxOutputTokens, 20000)
	reserveOutput := int(maxOutputTokens)
	if reserveOutput > defaultReserveOutputTokens || reserveOutput <= 0 {
		reserveOutput = defaultReserveOutputTokens
	}
	ctxWin := int(contextWindow)

	// 构建 CompactLimits，从 cfg.Harness 读取配置
	compact := CompactLimits{
		Enabled:                 false,
		ReserveOutputTokens:     reserveOutput,
		AutoCompactBufferTokens: defaultAutoCompactBufferTokens,
		BlockingBufferTokens:    defaultBlockingBufferTokens,
		WarningBufferTokens:     defaultWarningBufferTokens,
		ManualBufferTokens:      defaultManualCompactBuffer,
		MaxConsecutiveFailures:  defaultMaxConsecutiveFailures,
		MicroCompactKeepRecent:  defaultMicroCompactKeepRecent,
	}

	if cfg != nil {
		h := cfg.Harness
		compact.Enabled = h.MicroCompactEnabled
		if h.MicroCompactKeepRecent > 0 {
			compact.MicroCompactKeepRecent = h.MicroCompactKeepRecent
		}
		// Backward-compatible override. Claude Code's canonical semantics are
		// effectiveWindow - 13k, but an explicit ratio can still lower the buffer.
		if h.AutoCompactBufferTokens > 0 {
			compact.AutoCompactBufferTokens = h.AutoCompactBufferTokens
		}
		if h.CompactBlockingBufferTokens > 0 {
			compact.BlockingBufferTokens = h.CompactBlockingBufferTokens
		}
		if h.AutoCompactThresholdRatio > 0 && h.AutoCompactBufferTokens <= 0 && ctxWin > 0 {
			effective := ctxWin - reserveOutput
			if effective > 0 {
				bufferByRatio := int(math.Round(float64(effective) * (1.0 - h.AutoCompactThresholdRatio)))
				if bufferByRatio > 0 {
					compact.AutoCompactBufferTokens = bufferByRatio
				}
			}
		}
	}

	// 计算 EffectiveContextWindow 和 AutoCompactThreshold
	effectiveContextWindow := max(ctxWin-reserveOutput, 0)
	autoCompactThreshold := max(effectiveContextWindow-compact.AutoCompactBufferTokens, 0)
	blockingCompactThreshold := max(effectiveContextWindow-compact.BlockingBufferTokens, 0)

	return QueryConfig{
		SessionID: sessionID,
		ModelID:   modelID,
		Loop: LoopLimits{
			MaxIterations:    defaultMaxIterations,
			MaxToolCalls:     defaultMaxToolCalls,
			MaxStreamRetries: defaultMaxStreamRetries,
		},
		Budget: BudgetLimits{
			TokenBudget:              0,
			CostBudgetUSD:            0,
			DiminishingMinDeltaToken: defaultDiminishingMinDeltaToken,
			MaxContinuations:         defaultMaxContinuations,
			CompletionThreshold:      defaultCompletionThreshold,
		},
		Compact:                  compact,
		EffectiveContextWindow:   effectiveContextWindow,
		AutoCompactThreshold:     autoCompactThreshold,
		BlockingCompactThreshold: blockingCompactThreshold,
	}
}
