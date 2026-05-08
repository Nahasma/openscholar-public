package evolution

// EvolutionCase 记录用户反馈案例
type EvolutionCase struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	SkillID      string `json:"skill_id,omitempty"`
	UserRequest  string `json:"user_request"`
	AgentOutput  string `json:"agent_output,omitempty"`
	Feedback     string `json:"feedback"`
	FeedbackType string `json:"feedback_type"` // "explicit"
	Status       string `json:"status"`        // pending | analyzed | resolved | dismissed
	Resolution   string `json:"resolution,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// SkillSnapshot 技能变更前的完整快照，支持回滚
type SkillSnapshot struct {
	ID        string `json:"id"`
	SkillID   string `json:"skill_id"`
	Version   int    `json:"version"`
	Content   string `json:"content"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// EvolutionResult 进化 Pipeline 的最终结果摘要
type EvolutionResult struct {
	CasesAnalyzed  int    `json:"cases_analyzed"`
	Action         string `json:"action"` // "update" | "create" | "no_change"
	SkillID        string `json:"skill_id,omitempty"`
	ChangesSummary string `json:"changes_summary"`
	Reasoning      string `json:"reasoning"`
}

// FeedbackRequest RecordFeedback 工具的输入
type FeedbackRequest struct {
	SessionID   string
	SkillID     string
	UserRequest string
	AgentOutput string
	Feedback    string
}

// SkillStatsResult 技能统计结果
type SkillStatsResult struct {
	SkillID       string `json:"skill_id"`
	UsageCount    int    `json:"usage_count"`
	FeedbackCount int    `json:"feedback_count"`
}
