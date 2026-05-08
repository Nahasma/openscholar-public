package research

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"context"

	"golang.org/x/sync/errgroup"
)

// SingleReview 单个 reviewer 的评审结果
type SingleReview struct {
	Perspective  string   `json:"perspective"`
	Soundness    float64  `json:"soundness"`
	Completeness float64  `json:"completeness"`
	Citations    float64  `json:"citations"`
	Writing      float64  `json:"writing"`
	Overall      float64  `json:"overall"`
	Strengths    []string `json:"strengths"`
	Weaknesses   []string `json:"weaknesses"`
	Decision     string   `json:"decision"`
}

// EnsembleReview 集成审稿汇总
type EnsembleReview struct {
	Reviews   []*SingleReview `json:"reviews"`
	Overall   float64         `json:"overall"`
	Decision  string          `json:"decision"`  // "accept" | "reject" | "escalate"
	Summary   string          `json:"summary"`
	Consensus float64         `json:"consensus"` // 0-1
}

// ReviewerPerspective 审查视角定义
type ReviewerPerspective struct {
	Name   string
	Prompt string
}

// DefaultReviewerPerspectives 返回默认 3 视角
func DefaultReviewerPerspectives() []ReviewerPerspective {
	return []ReviewerPerspective{
		{
			Name: "completeness",
			Prompt: `你是一位关注研究完整性的审稿人。请从以下维度评估：
1. 方法描述是否完整、逻辑链是否闭合
2. 实验设计是否充分验证了研究假设
3. 结果分析是否全面、是否遗漏关键发现
4. 讨论部分是否涵盖局限性和未来方向

请以 JSON 格式返回评审结果：
{"soundness": N, "completeness": N, "citations": N, "writing": N, "overall": N, "strengths": ["..."], "weaknesses": ["..."], "decision": "accept|reject"}`,
		},
		{
			Name: "citations",
			Prompt: `你是一位关注引用质量的审稿人。请从以下维度评估：
1. 引用是否覆盖该领域的关键文献
2. 每个核心 claim 是否有可靠来源支撑
3. Related Work 是否准确区分了本研究与已有工作
4. 引用格式是否规范、是否存在捏造引用

请以 JSON 格式返回评审结果：
{"soundness": N, "completeness": N, "citations": N, "writing": N, "overall": N, "strengths": ["..."], "weaknesses": ["..."], "decision": "accept|reject"}`,
		},
		{
			Name: "methodology",
			Prompt: `你是一位关注方法论严谨性的审稿人。请从以下维度评估：
1. 实验设计是否合理（baseline 选择、评估指标、数据集）
2. 统计分析是否严谨（显著性检验、误差线、多次运行）
3. 消融实验是否充分验证了各组件贡献
4. 可复现性（代码、数据、超参数是否完整描述）

请以 JSON 格式返回评审结果：
{"soundness": N, "completeness": N, "citations": N, "writing": N, "overall": N, "strengths": ["..."], "weaknesses": ["..."], "decision": "accept|reject"}`,
		},
	}
}

// ExtendedReviewerPerspectives 返回 5 视角（严格模式）
func ExtendedReviewerPerspectives() []ReviewerPerspective {
	perspectives := DefaultReviewerPerspectives()
	return append(perspectives,
		ReviewerPerspective{
			Name: "writing",
			Prompt: `你是一位关注学术写作质量的审稿人。请从以下维度评估：
1. 表述是否清晰、逻辑是否流畅
2. 术语使用是否一致
3. 图表是否清晰、caption 是否准确
4. 摘要和结论是否准确概括全文

请以 JSON 格式返回评审结果：
{"soundness": N, "completeness": N, "citations": N, "writing": N, "overall": N, "strengths": ["..."], "weaknesses": ["..."], "decision": "accept|reject"}`,
		},
		ReviewerPerspective{
			Name: "novelty",
			Prompt: `你是一位关注创新性的审稿人。请从以下维度评估：
1. 研究问题是否有足够的新颖性
2. 方法是否有实质性创新
3. 与已有工作的区分度是否明确
4. 贡献是否足以支撑发表

请以 JSON 格式返回评审结果：
{"soundness": N, "completeness": N, "citations": N, "writing": N, "overall": N, "strengths": ["..."], "weaknesses": ["..."], "decision": "accept|reject"}`,
		},
	)
}

// RunEnsembleReview 并行执行多个 reviewer 并汇总结果
func RunEnsembleReview(
	ctx context.Context,
	handoff string,
	perspectives []ReviewerPerspective,
	runAgent func(ctx context.Context, prompt string) (string, error),
) (*EnsembleReview, error) {
	if len(perspectives) == 0 {
		return nil, fmt.Errorf("no reviewer perspectives provided")
	}

	var mu sync.Mutex
	var reviews []*SingleReview

	g, gctx := errgroup.WithContext(ctx)
	for _, p := range perspectives {
		perspective := p
		g.Go(func() error {
			prompt := buildReviewPrompt(perspective, handoff)
			response, err := runAgent(gctx, prompt)
			if err != nil {
				return fmt.Errorf("reviewer %s failed: %w", perspective.Name, err)
			}
			review, err := ParseReviewJSON(response)
			if err != nil {
				return fmt.Errorf("reviewer %s parse failed: %w", perspective.Name, err)
			}
			review.Perspective = perspective.Name
			mu.Lock()
			reviews = append(reviews, review)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return AggregateReviews(reviews), nil
}

// GenerateMetaReview 用 LLM 生成 meta-review 汇总
func GenerateMetaReview(
	ctx context.Context,
	ensemble *EnsembleReview,
	runAgent func(ctx context.Context, prompt string) (string, error),
) (string, error) {
	prompt := fmt.Sprintf(`你是 meta-reviewer（审稿主席）。以下是 %d 位审稿人的独立评审结果：

%s

请综合所有审稿意见，生成汇总报告：
1. 主要共识（所有 reviewer 认同的优缺点）
2. 主要分歧（reviewer 间意见不一致的地方）
3. 最终建议（基于多数意见的改进建议）
4. 决策理由

简洁输出，不超过 500 字。`, len(ensemble.Reviews), FormatReviewsForPrompt(ensemble.Reviews))

	return runAgent(ctx, prompt)
}

// AggregateReviews 聚合多个 reviewer 的评审结果
func AggregateReviews(reviews []*SingleReview) *EnsembleReview {
	result := &EnsembleReview{Reviews: reviews}
	if len(reviews) == 0 {
		result.Decision = "reject"
		return result
	}

	var sum float64
	for _, r := range reviews {
		sum += r.Overall
	}
	result.Overall = sum / float64(len(reviews))
	result.Consensus = CalculateConsensus(reviews)

	rejectCount := 0
	for _, r := range reviews {
		if r.Decision == "reject" {
			rejectCount++
		}
	}

	switch {
	case result.Consensus < 0.5:
		result.Decision = "escalate"
	case result.Overall >= 7 && rejectCount == 0:
		result.Decision = "accept"
	default:
		result.Decision = "reject"
	}

	return result
}

// CalculateConsensus 计算评审一致性 (0-1)
func CalculateConsensus(reviews []*SingleReview) float64 {
	if len(reviews) <= 1 {
		return 1.0
	}
	var scores []float64
	for _, r := range reviews {
		scores = append(scores, r.Overall)
	}
	mean := 0.0
	for _, s := range scores {
		mean += s
	}
	mean /= float64(len(scores))

	variance := 0.0
	for _, s := range scores {
		variance += (s - mean) * (s - mean)
	}
	variance /= float64(len(scores))
	stdDev := math.Sqrt(variance)

	consensus := 1.0 - math.Min(stdDev/3.0, 1.0)
	return math.Round(consensus*100) / 100
}

// ParseReviewJSON 从 LLM 响应中解析 JSON 评审结果
func ParseReviewJSON(response string) (*SingleReview, error) {
	// 尝试直接解析
	var review SingleReview
	if err := json.Unmarshal([]byte(response), &review); err == nil {
		return &review, nil
	}

	// 尝试提取 JSON 块
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		jsonStr := response[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &review); err == nil {
			return &review, nil
		}
	}

	return nil, fmt.Errorf("failed to parse review JSON from response")
}

func buildReviewPrompt(perspective ReviewerPerspective, handoff string) string {
	return fmt.Sprintf(`%s

以下是需要评审的阶段产出：

---
%s
---

请根据上述指引进行评审。`, perspective.Prompt, handoff)
}

// FormatReviewsForPrompt 序列化 reviews 供 meta-review
func FormatReviewsForPrompt(reviews []*SingleReview) string {
	var sb strings.Builder
	for i, r := range reviews {
		sb.WriteString(fmt.Sprintf("### Reviewer %d (%s)\n", i+1, r.Perspective))
		sb.WriteString(fmt.Sprintf("- Overall: %.1f/10\n", r.Overall))
		sb.WriteString(fmt.Sprintf("- Soundness: %.1f | Completeness: %.1f | Citations: %.1f | Writing: %.1f\n",
			r.Soundness, r.Completeness, r.Citations, r.Writing))
		sb.WriteString(fmt.Sprintf("- Decision: %s\n", r.Decision))
		if len(r.Strengths) > 0 {
			sb.WriteString("- Strengths: " + strings.Join(r.Strengths, "; ") + "\n")
		}
		if len(r.Weaknesses) > 0 {
			sb.WriteString("- Weaknesses: " + strings.Join(r.Weaknesses, "; ") + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
