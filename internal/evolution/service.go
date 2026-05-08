package evolution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/skillbank"
)

// Service 技能进化服务接口
type Service interface {
	RecordFeedback(ctx context.Context, req FeedbackRequest) (*EvolutionCase, error)
	RunEvolution(ctx context.Context) (*RefinementResult, *AnalysisResult, []EvolutionCase, error)
	ApplyChanges(ctx context.Context, change SkillChange, cases []EvolutionCase) error
	AutoEvolve(ctx context.Context) error // 自动运行 Pipeline（不静默应用变更）
	SetLLMCaller(caller LLMCaller)
	Rollback(ctx context.Context, skillID string) error
	ListCases(ctx context.Context, status string, limit int) ([]EvolutionCase, error)
	DismissAll(ctx context.Context) error
	SkillStats(ctx context.Context, skillID string) (*SkillStatsResult, error)
	AllSkillStats(ctx context.Context) ([]SkillStatsResult, error)
}

var ErrLLMCallerNotConfigured = errors.New("evolution LLM caller is not configured")
var ErrAutoEvolveDisabled = errors.New("automatic skill evolution is disabled; use /evolve to review proposals")

type service struct {
	store    *Store
	skills   skillbank.Service
	pipeline *Pipeline
}

func NewService(store *Store, skills skillbank.Service, llm LLMCaller) Service {
	return &service{
		store:    store,
		skills:   skills,
		pipeline: NewPipeline(llm),
	}
}

func (s *service) SetLLMCaller(caller LLMCaller) {
	s.pipeline.SetLLMCaller(caller)
}

func (s *service) RecordFeedback(ctx context.Context, req FeedbackRequest) (*EvolutionCase, error) {
	c := &EvolutionCase{
		SessionID:   req.SessionID,
		SkillID:     req.SkillID,
		UserRequest: req.UserRequest,
		AgentOutput: req.AgentOutput,
		Feedback:    req.Feedback,
	}
	if err := s.store.CreateCase(c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *service) RunEvolution(ctx context.Context) (*RefinementResult, *AnalysisResult, []EvolutionCase, error) {
	if !s.pipeline.HasLLMCaller() {
		return nil, nil, nil, ErrLLMCallerNotConfigured
	}

	cases, err := s.store.ListCasesByStatus("pending", 20)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(cases) == 0 {
		return &RefinementResult{Action: "no_change_needed", Summary: "无待处理反馈"}, nil, nil, nil
	}

	// 构建技能库上下文
	allSkills, _ := s.skills.List(ctx, "")
	skillsContext := formatSkillsForPrompt(allSkills)

	// 构建进化历史
	evolutionHistory := s.buildEvolutionHistory()

	// 运行三阶段 Pipeline
	result, analysis, err := s.pipeline.Run(ctx, cases, skillsContext, evolutionHistory)
	if err != nil {
		return nil, analysis, cases, err
	}

	return result, analysis, cases, nil
}

// AutoEvolve is disabled because skill changes require explicit user review.
func (s *service) AutoEvolve(ctx context.Context) error {
	return ErrAutoEvolveDisabled
}

func (s *service) ApplyChanges(ctx context.Context, change SkillChange, cases []EvolutionCase) error {
	switch change.Action {
	case "add_new":
		if change.AddNew == nil {
			return fmt.Errorf("add_new change missing new skill data")
		}
		// 检查容量上限（30 个技能）
		allSkills, _ := s.skills.List(ctx, "")
		if len(allSkills) >= 30 {
			return fmt.Errorf("技能库已满（上限 30），请先删除不需要的技能")
		}
		new := change.AddNew
		skill := skillbank.Skill{
			Name:        new.Name,
			Category:    new.Category,
			Description: new.Description,
			Tags:        new.Tags,
			Instruction: new.Instruction,
			Author:      "agent",
		}
		if err := s.skills.Create(ctx, skill); err != nil {
			return fmt.Errorf("创建技能失败: %w", err)
		}

	case "refine_existing":
		if change.RefineExisting == nil {
			return fmt.Errorf("refine_existing change missing skill data")
		}
		ref := change.RefineExisting

		// 检查进化轮次上限（6 轮）
		snapCount, _ := s.store.SnapshotCount(ref.SkillID)
		if snapCount >= 6 {
			return fmt.Errorf("技能 %s 已达进化上限（6 轮），请手动优化或回滚", ref.SkillID)
		}

		existing, err := s.skills.Get(ctx, ref.SkillID)
		if err != nil {
			return fmt.Errorf("技能不存在: %s", ref.SkillID)
		}

		// Snapshot before mutate
		snap := &SkillSnapshot{
			SkillID: ref.SkillID,
			Version: existing.Version,
			Content: existing.Instruction,
			Reason:  "evolution",
		}
		if err := s.store.CreateSnapshot(snap); err != nil {
			return fmt.Errorf("快照创建失败: %w", err)
		}

		// Apply changes
		updated := *existing
		if desc, ok := ref.Changes["description"]; ok && desc != "" {
			updated.Description = desc
		}
		if inst, ok := ref.Changes["instruction"]; ok && inst != "" {
			updated.Instruction = inst
		}
		if err := s.skills.Update(ctx, ref.SkillID, updated); err != nil {
			return fmt.Errorf("更新技能失败: %w", err)
		}

	default:
		return fmt.Errorf("unknown action: %s", change.Action)
	}

	// 标记 cases 为 resolved
	for _, c := range cases {
		s.store.UpdateCaseStatus(c.ID, "resolved", change.Reasoning)
	}
	return nil
}

func (s *service) Rollback(ctx context.Context, skillID string) error {
	snap, err := s.store.LatestSnapshot(skillID)
	if err != nil {
		return fmt.Errorf("未找到 %s 的快照", skillID)
	}
	existing, err := s.skills.Get(ctx, skillID)
	if err != nil {
		return fmt.Errorf("技能不存在: %s", skillID)
	}
	existing.Instruction = snap.Content
	return s.skills.Update(ctx, skillID, *existing)
}

func (s *service) ListCases(ctx context.Context, status string, limit int) ([]EvolutionCase, error) {
	return s.store.ListCasesByStatus(status, limit)
}

func (s *service) DismissAll(ctx context.Context) error {
	_, err := s.store.DismissAll()
	return err
}

func (s *service) SkillStats(ctx context.Context, skillID string) (*SkillStatsResult, error) {
	skill, err := s.skills.Get(ctx, skillID)
	if err != nil {
		return nil, err
	}
	fbCount, _ := s.store.FeedbackCountBySkill(skillID)
	// success_count is internal to skillRow; use usage_count from public Skill
	return &SkillStatsResult{
		SkillID:       skillID,
		UsageCount:    skill.UsageCount,
		FeedbackCount: fbCount,
	}, nil
}

func (s *service) AllSkillStats(ctx context.Context) ([]SkillStatsResult, error) {
	allSkills, err := s.skills.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var results []SkillStatsResult
	for _, skill := range allSkills {
		fbCount, _ := s.store.FeedbackCountBySkill(skill.ID)
		results = append(results, SkillStatsResult{
			SkillID:       skill.ID,
			UsageCount:    skill.UsageCount,
			FeedbackCount: fbCount,
		})
	}
	return results, nil
}

// --- helpers ---

func formatSkillsForPrompt(skills []skillbank.Skill) string {
	var sb strings.Builder
	for i, s := range skills {
		sb.WriteString(fmt.Sprintf("[Skill %d] id: %s\n", i+1, s.ID))
		sb.WriteString(fmt.Sprintf("  描述: %s\n", s.Description))
		inst := s.Instruction
		if len(inst) > 200 {
			inst = inst[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("  指令: %s\n\n", inst))
	}
	return sb.String()
}

func (s *service) buildEvolutionHistory() string {
	resolved, _ := s.store.ListCasesByStatus("resolved", 10)
	if len(resolved) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 进化历史\n")
	for _, c := range resolved {
		if c.Resolution != "" {
			sb.WriteString(fmt.Sprintf("- 技能 %s: %s\n", c.SkillID, c.Resolution))
		}
	}
	sb.WriteString("\n避免重复类似的修改。\n")
	return sb.String()
}
