package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// LLMCaller 调用 LLM 的函数签名（复用 skillbank.LLMCaller 类型）
type LLMCaller func(ctx context.Context, prompt string) (string, error)

// Pipeline 三阶段反思引擎
type Pipeline struct {
	mu  sync.RWMutex
	llm LLMCaller
}

func NewPipeline(llm LLMCaller) *Pipeline {
	return &Pipeline{llm: llm}
}

func (p *Pipeline) SetLLMCaller(llm LLMCaller) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.llm = llm
}

func (p *Pipeline) HasLLMCaller() bool {
	return p.llmCaller() != nil
}

func (p *Pipeline) llmCaller() LLMCaller {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.llm
}

// AnalysisResult Stage 1 输出
type AnalysisResult struct {
	FailurePatterns []FailurePattern `json:"failure_patterns"`
	Recommendations []Recommendation `json:"recommendations"`
	Summary         string           `json:"summary"`
}

type FailurePattern struct {
	PatternName   string   `json:"pattern_name"`
	AffectedCases []string `json:"affected_cases"`
	RootCause     string   `json:"root_cause"`
	Explanation   string   `json:"explanation"`
	PotentialFix  string   `json:"potential_fix"`
}

type Recommendation struct {
	Action      string `json:"action"`
	TargetSkill string `json:"target_skill"`
	Rationale   string `json:"rationale"`
	Priority    int    `json:"priority"`
}

// RefinementResult Stage 2 输出
type RefinementResult struct {
	Action  string        `json:"action"` // apply_changes | no_change_needed
	Summary string        `json:"summary"`
	Changes []SkillChange `json:"changes"`
}

type SkillChange struct {
	Action         string          `json:"action"` // add_new | refine_existing
	AddNew         *NewSkill       `json:"add_new,omitempty"`
	RefineExisting *ExistingChange `json:"refine_existing,omitempty"`
	Reasoning      string          `json:"reasoning"`
}

type NewSkill struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Instruction string   `json:"instruction"`
}

type ExistingChange struct {
	SkillID string            `json:"skill_id"`
	Changes map[string]string `json:"changes"` // field → new value
}

// Run 执行三阶段反思 Pipeline: Analysis → Reflection → Refinement
func (p *Pipeline) Run(ctx context.Context, cases []EvolutionCase, skillsContext, evolutionHistory string) (*RefinementResult, *AnalysisResult, error) {
	llm := p.llmCaller()
	if llm == nil {
		return nil, nil, ErrLLMCallerNotConfigured
	}

	casesText := formatCases(cases)

	// Stage 1: Analysis
	prompt1 := fmt.Sprintf(analysisPrompt, skillsContext, evolutionHistory, casesText)
	raw1, err := llm(ctx, prompt1)
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}
	var analysis AnalysisResult
	if err := json.Unmarshal(extractJSON(raw1), &analysis); err != nil {
		return nil, nil, fmt.Errorf("analysis parse failed: %w", err)
	}

	// Stage 1b: Reflection（最多 2 轮）
	prev := raw1
	for i := 0; i < 2; i++ {
		promptR := fmt.Sprintf(reflectionPrompt, prev, casesText, skillsContext)
		rawR, err := llm(ctx, promptR)
		if err != nil {
			break // reflection 失败不阻塞
		}
		if strings.TrimSpace(rawR) == strings.TrimSpace(prev) {
			break // 收敛
		}
		prev = rawR
		json.Unmarshal(extractJSON(rawR), &analysis)
	}

	// Stage 2: Refinement
	analysisJSON, _ := json.Marshal(analysis)
	prompt2 := fmt.Sprintf(refinementPrompt, string(analysisJSON), skillsContext, evolutionHistory)
	raw2, err := llm(ctx, prompt2)
	if err != nil {
		return nil, &analysis, fmt.Errorf("refinement failed: %w", err)
	}
	var result RefinementResult
	if err := json.Unmarshal(extractJSON(raw2), &result); err != nil {
		return nil, &analysis, fmt.Errorf("refinement parse failed: %w", err)
	}

	return &result, &analysis, nil
}

// extractJSON 从 LLM 输出中提取 JSON（处理 markdown 代码块包裹）
func extractJSON(raw string) []byte {
	s := strings.TrimSpace(raw)

	// 去掉 ```json ... ``` 包裹
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	// 找到第一个 { 和最后一个 }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return []byte(s[start : end+1])
	}

	return []byte(s)
}

// formatCases 将 EvolutionCase 列表格式化为 prompt 文本
func formatCases(cases []EvolutionCase) string {
	var sb strings.Builder
	for i, c := range cases {
		sb.WriteString(fmt.Sprintf("Case %d (id: %s):\n", i+1, c.ID[:8]))
		sb.WriteString(fmt.Sprintf("  用户请求: %s\n", c.UserRequest))
		if c.AgentOutput != "" {
			output := c.AgentOutput
			if len(output) > 500 {
				output = output[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("  Agent 输出: %s\n", output))
		}
		sb.WriteString(fmt.Sprintf("  用户反馈: %s\n", c.Feedback))
		if c.SkillID != "" {
			sb.WriteString(fmt.Sprintf("  关联技能: %s\n", c.SkillID))
		} else {
			sb.WriteString("  关联技能: 未识别\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
