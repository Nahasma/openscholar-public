package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nahasma/openscholar-public/internal/evolution"
)

type recordFeedbackTool struct {
	svc evolution.Service
}

func NewRecordFeedbackTool(svc evolution.Service) BaseTool {
	return &recordFeedbackTool{svc: svc}
}

type recordFeedbackParams struct {
	Feedback    string `json:"feedback"`
	UserRequest string `json:"user_request"`
	AgentOutput string `json:"agent_output,omitempty"`
	SkillID     string `json:"skill_id,omitempty"`
}

func (t *recordFeedbackTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "RecordFeedback",
		Description: "记录用户对 Agent 输出的反馈。当用户指出错误、表达不满或提出改进建议时调用此工具。不要对新任务请求、追问或简单确认调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"feedback": map[string]any{
					"type":        "string",
					"description": "用户反馈的核心内容（由你提炼总结）",
				},
				"user_request": map[string]any{
					"type":        "string",
					"description": "用户原始任务请求（从上下文回溯提取）",
				},
				"agent_output": map[string]any{
					"type":        "string",
					"description": "你的输出摘要（前500字），帮助后续分析定位问题",
				},
				"skill_id": map[string]any{
					"type":        "string",
					"description": "相关技能 ID（如 writing/citation_format），best-effort 匹配，可空",
				},
			},
			"required": []string{"feedback", "user_request"},
		},
		Required: []string{"feedback", "user_request"},
	}
}

func (t *recordFeedbackTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if IsResearchMode(ctx) {
		return NewTextErrorResponse("RecordFeedback is not available in research mode."), nil
	}

	var params recordFeedbackParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.Feedback == "" {
		return NewTextErrorResponse("feedback 参数不能为空"), nil
	}
	if params.UserRequest == "" {
		return NewTextErrorResponse("user_request 参数不能为空"), nil
	}

	sessionID, _ := ctx.Value(SessionIDContextKey).(string)

	c, err := t.svc.RecordFeedback(ctx, evolution.FeedbackRequest{
		SessionID:   sessionID,
		SkillID:     params.SkillID,
		UserRequest: params.UserRequest,
		AgentOutput: params.AgentOutput,
		Feedback:    params.Feedback,
	})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("记录失败: %v", err)), nil
	}

	return NewTextResponse(fmt.Sprintf(
		"已记录反馈 (case_id: %s)，可通过 /evolve 查看待处理建议。",
		c.ID[:8],
	)), nil
}
