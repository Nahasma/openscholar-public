package tools

// research_pipeline.go — "ResearchPipeline" tool
// Handles phase-level actions: advance, status, pause, set_mode.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
)

type researchPipelineTool struct {
	ctrl             ResearchController
	checkpointBroker *pubsub.Broker[CheckpointEvent]
	permissions      permission.Service
	runAgent         AgentRunner
	sessions         session.Service
	messages         message.Service
}

type researchPipelineParams struct {
	Action  string `json:"action"`
	Summary string `json:"summary,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// NewResearchPipelineTool creates the ResearchPipeline tool for phase control.
func NewResearchPipelineTool(
	ctrl ResearchController,
	broker *pubsub.Broker[CheckpointEvent],
	perms permission.Service,
	runAgent AgentRunner,
	sessions session.Service,
	messages message.Service,
) BaseTool {
	return &researchPipelineTool{
		ctrl:             ctrl,
		checkpointBroker: broker,
		permissions:      perms,
		runAgent:         runAgent,
		sessions:         sessions,
		messages:         messages,
	}
}

func (t *researchPipelineTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ResearchPipeline",
		Description: "Control research pipeline phase transitions. " +
			"Use 'advance' to complete the current phase and move to the next (requires a summary). " +
			"Use 'status' to view the pipeline state and all phases. " +
			"Use 'pause' to suspend the pipeline. " +
			"Use 'set_mode' to change automation mode (default/auto/strict).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"advance", "status", "pause", "set_mode"},
					"description": "Phase control action to perform.",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "Checkpoint summary describing completed work (required for 'advance').",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"default", "auto", "strict"},
					"description": "Target automation mode (required for 'set_mode').",
				},
			},
			"required": []string{"action"},
		},
		Required: []string{"action"},
	}
}

func (t *researchPipelineTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if !IsResearchMode(ctx) {
		return NewTextErrorResponse("ResearchPipeline is only available in research mode."), nil
	}

	var params researchPipelineParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	sessionID := ResearchSessionID(ctx)
	if sessionID == "" {
		return NewTextErrorResponse("No session context available."), nil
	}

	pipeline, err := t.ctrl.GetBySession(sessionID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("No research pipeline found for this session: %v", err)), nil
	}

	switch params.Action {
	case "advance":
		return t.handleAdvance(ctx, pipeline, params.Summary)
	case "status":
		return t.handleStatus(pipeline)
	case "pause":
		return t.handlePause(pipeline)
	case "set_mode":
		return t.handleSetMode(pipeline, params.Mode)
	default:
		return NewTextErrorResponse(fmt.Sprintf("Unknown action: %s. Valid actions: advance, status, pause, set_mode.", params.Action)), nil
	}
}

func (t *researchPipelineTool) handleAdvance(ctx context.Context, pipeline ResearchPipelineView, summary string) (ToolResponse, error) {
	phases, err := t.ctrl.GetPhasesView(pipeline.ID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to get phases: %v", err)), nil
	}
	var currentPhase *ResearchPhaseView
	for i := range phases {
		if phases[i].Status == "running" {
			currentPhase = &phases[i]
			break
		}
	}
	if currentPhase == nil {
		return NewTextErrorResponse("No running phase found. Cannot advance."), nil
	}

	// Validate deliverables before allowing phase advancement
	if err := research.ValidateDeliverables(pipeline.WorkDir, pipeline.Template, currentPhase.Order); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Cannot advance: %v. Please produce the required deliverables before calling advance.", err)), nil
	}

	score, report, err := t.reviewPhase(ctx, pipeline, currentPhase, summary)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("自动评审失败：%v。未推进阶段，请修复后重试。", err)), nil
	}
	if score < 7 {
		return NewTextResponse(fmt.Sprintf(
			"## 评审未通过 (得分: %.0f/10)\n\n%s\n\n请根据评审意见迭代改进后重新调用 ResearchPipeline(action=\"advance\")。",
			score, report,
		)), nil
	}
	if pipeline.Mode == "strict" || (pipeline.Mode == "default" && currentPhase.Checkpoint) {
		return t.handleConfirmAdvance(ctx, pipeline, currentPhase, summary, score, report)
	}
	return t.advanceReviewedPhase(pipeline, currentPhase, score, report)
}

func (t *researchPipelineTool) reviewPhase(ctx context.Context, pipeline ResearchPipelineView, phase *ResearchPhaseView, summary string) (float64, string, error) {
	workDir := ResearchWorkDir(ctx)
	if research.IsARISTemplate(pipeline.Template) {
		workDir = strings.TrimSpace(pipeline.WorkDir)
		if workDir == "" {
			workDir = ResearchWorkDir(ctx)
		}
		reviewPrompt := buildARISReviewPrompt(workDir, pipeline.Template, phase)
		return t.runReviewer(ctx, reviewPrompt)
	}
	handoffContent := t.readHandoffFile(workDir, phase.Order)

	reviewPrompt := fmt.Sprintf(`你是一个研究阶段产出审查员。请评审以下研究阶段产出。

评分标准（1-10）：
- 完整性：预期交付物是否齐全
- 质量：内容是否结构化、有实质
- 引用覆盖：主张是否有据可查

阶段：%s
摘要：%s
产出内容：
---
%s
---

	请用 JSON 格式回复：{"score": 数字, "report": "评审意见"}
	仅输出 JSON，不要输出其他内容。`, phase.Name, summary, handoffContent)

	return t.runReviewer(ctx, reviewPrompt)
}

func buildARISReviewPrompt(workDir, template string, phase *ResearchPhaseView) string {
	phaseOrder := 0
	phaseName := ""
	if phase != nil {
		phaseOrder = phase.Order
		phaseName = phase.Name
	}
	filePaths := collectPhaseDeliverables(workDir, template, phaseOrder)
	if len(filePaths) == 0 {
		filePaths = []string{"(no matched deliverable files; validate expected outputs by path patterns)"}
	}
	return fmt.Sprintf(`你是独立审稿员（ARIS reviewer）。只基于文件路径进行审阅，不接受执行者总结。

角色:
- objective reviewer

阶段:
- %d - %s

文件路径:
- %s

评审目标与rubric:
- 完整性：是否覆盖该阶段应交付内容
- 可验证性：关键信息是否可从文件定位与复核
- 一致性：文件间结论、数据、图表、代码是否一致
- 风险项：明确列出阻断性问题与修复建议

输出 JSON schema:
{
  "score": "number (0-10)",
  "report": "string"
}

仅输出 JSON。`, phaseOrder, phaseName, strings.Join(filePaths, "\n- "))
}

func collectPhaseDeliverables(workDir, template string, phaseOrder int) []string {
	phases, ok := research.Templates[template]
	if !ok || phaseOrder < 1 || phaseOrder > len(phases) || workDir == "" {
		return nil
	}
	var out []string
	for _, pattern := range phases[phaseOrder-1].Deliverables {
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err != nil || len(matches) == 0 {
			out = append(out, pattern)
			continue
		}
		for _, m := range matches {
			if rel, err := filepath.Rel(workDir, m); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	return out
}

func (t *researchPipelineTool) advanceReviewedPhase(
	pipeline ResearchPipelineView,
	phase *ResearchPhaseView,
	score float64,
	report string,
) (ToolResponse, error) {
	if err := t.ctrl.AdvancePipeline(pipeline.ID); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to advance: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf(
		"## 评审通过 (得分: %.0f/10)\n\n%s\n\n阶段 %d「%s」已推进。\n\n%s",
		score, report, phase.Order, phase.Name, t.formatStatus(pipeline),
	)), nil
}

// handleConfirmAdvance publishes a checkpoint event and blocks until user responds.
func (t *researchPipelineTool) handleConfirmAdvance(ctx context.Context, pipeline ResearchPipelineView, phase *ResearchPhaseView, summary string, reviewScore float64, reviewReport string) (ToolResponse, error) {
	if t.checkpointBroker == nil {
		return NewTextErrorResponse("Checkpoint UI is unavailable. Phase was verified but cannot advance without explicit approval."), nil
	}

	responseCh := make(chan CheckpointResponse, 1)
	eventIDPrefix := pipeline.ID
	if len(eventIDPrefix) > 8 {
		eventIDPrefix = eventIDPrefix[:8]
	}
	event := CheckpointEvent{
		ID:           fmt.Sprintf("cp_%s_%d", eventIDPrefix, phase.Order),
		PipelineID:   pipeline.ID,
		PhaseName:    phase.Name,
		PhaseOrder:   phase.Order,
		Summary:      summary,
		Mode:         pipeline.Mode,
		ReviewScore:  reviewScore,
		ReviewReport: reviewReport,
		ResponseCh:   responseCh,
	}

	t.checkpointBroker.Publish(pubsub.CreatedEvent, event)

	select {
	case resp := <-responseCh:
		if resp.Approved {
			if err := t.ctrl.AdvancePipelineWithOptions(pipeline.ID, ResearchAdvanceOptions{SkipCheckpoint: true}); err != nil {
				return NewTextErrorResponse(fmt.Sprintf("Failed to advance: %v", err)), nil
			}
			return NewTextResponse(fmt.Sprintf("阶段 %d「%s」已通过审核，推进到下一阶段。\n\n%s",
				phase.Order, phase.Name, t.formatStatus(pipeline))), nil
		}
		feedback := resp.Feedback
		if feedback == "" {
			feedback = "（用户未提供具体反馈）"
		}
		return NewTextResponse(fmt.Sprintf(
			"## 阶段推进被拒绝\n\n用户反馈：%s\n\n请根据反馈迭代改进后重新调用 ResearchPipeline(action=\"advance\")。",
			feedback,
		)), nil
	case <-ctx.Done():
		return NewTextErrorResponse("Phase advance cancelled."), nil
	}
}

type reviewerTaskRead struct {
	Status    string `json:"status"`
	Result    string `json:"result"`
	LastError string `json:"last_error"`
}

// runReviewer dispatches a verify worker via Task runtime and parses score output.
func (t *researchPipelineTool) runReviewer(ctx context.Context, prompt string) (float64, string, error) {
	if t.permissions == nil || t.sessions == nil || t.messages == nil || t.runAgent == nil {
		return 0, "", fmt.Errorf("verify worker dependencies are unavailable")
	}
	parentSessionID := ResearchSessionID(ctx)
	if strings.TrimSpace(parentSessionID) == "" {
		return 0, "", fmt.Errorf("no parent session context for verify worker")
	}
	controlCtx := context.WithValue(ctx, SessionIDContextKey, parentSessionID)

	reg, _ := getTaskRuntime()
	if reg == nil {
		return 0, "", fmt.Errorf("task runtime is unavailable")
	}

	taskTool := NewTaskTool(t.permissions, t.sessions, t.messages, t.runAgent)
	taskID := "research-verify-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	createInput := fmt.Sprintf(`{"action":"create","task_id":"%s","description":"Research phase verification","prompt":%s,"agent_type":"verify"}`,
		taskID, strconv.Quote(prompt))
	createResp, err := taskTool.Run(controlCtx, ToolCall{
		ID:    taskID,
		Name:  "Task",
		Input: createInput,
	})
	if err != nil {
		return 0, "", err
	}
	if createResp.IsError {
		return 0, "", fmt.Errorf("%s", createResp.Content)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		readInput := fmt.Sprintf(`{"action":"read","task_id":"%s"}`, taskID)
		readResp, readErr := taskTool.Run(controlCtx, ToolCall{
			ID:    taskID + "-read",
			Name:  "Task",
			Input: readInput,
		})
		if readErr != nil {
			return 0, "", readErr
		}
		if readResp.IsError {
			return 0, "", fmt.Errorf("%s", readResp.Content)
		}

		var view reviewerTaskRead
		if err := json.Unmarshal([]byte(readResp.Content), &view); err != nil {
			return 0, "", fmt.Errorf("invalid verify task output: %w", err)
		}
		switch task.Status(view.Status) {
		case task.StatusCompleted:
			score, report, err := parseReviewerOutput(view.Result)
			if err != nil {
				return 0, "", err
			}
			return score, report, nil
		case task.StatusFailed, task.StatusCanceled:
			if strings.TrimSpace(view.LastError) != "" {
				return 0, "", fmt.Errorf("verify task %s: %s", view.Status, strings.TrimSpace(view.LastError))
			}
			return 0, "", fmt.Errorf("verify task %s", view.Status)
		case task.StatusPending, task.StatusRunning:
			select {
			case <-ctx.Done():
				t.cleanupReviewerTask(parentSessionID, taskTool, taskID)
				return 0, "", ctx.Err()
			case <-ticker.C:
			}
		default:
			return 0, "", fmt.Errorf("verify task returned unknown status: %s", view.Status)
		}
	}
}

func (t *researchPipelineTool) cleanupReviewerTask(parentSessionID string, taskTool BaseTool, taskID string) {
	if strings.TrimSpace(parentSessionID) == "" || taskTool == nil {
		return
	}

	controlCtx, cancel := context.WithTimeout(
		context.WithValue(context.Background(), SessionIDContextKey, parentSessionID),
		2*time.Second,
	)
	defer cancel()

	stopInput := fmt.Sprintf(`{"action":"stop","task_id":"%s"}`, taskID)
	_, _ = taskTool.Run(controlCtx, ToolCall{
		ID:    taskID + "-stop",
		Name:  "Task",
		Input: stopInput,
	})

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		readInput := fmt.Sprintf(`{"action":"read","task_id":"%s"}`, taskID)
		readResp, err := taskTool.Run(controlCtx, ToolCall{
			ID:    taskID + "-cleanup-read",
			Name:  "Task",
			Input: readInput,
		})
		if err == nil && !readResp.IsError {
			var view reviewerTaskRead
			if json.Unmarshal([]byte(readResp.Content), &view) == nil {
				switch task.Status(view.Status) {
				case task.StatusCompleted, task.StatusFailed, task.StatusCanceled:
					return
				}
			}
		}

		select {
		case <-controlCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

func parseReviewerOutput(result string) (float64, string, error) {
	var review struct {
		Score  float64 `json:"score"`
		Report string  `json:"report"`
	}
	jsonStart := strings.Index(result, "{")
	jsonEnd := strings.LastIndex(result, "}")
	if jsonStart < 0 || jsonEnd <= jsonStart {
		return 0, "", fmt.Errorf("verify output is not valid JSON")
	}
	if err := json.Unmarshal([]byte(result[jsonStart:jsonEnd+1]), &review); err != nil {
		return 0, "", fmt.Errorf("failed to parse verify output: %w", err)
	}
	if review.Score < 0 || review.Score > 10 {
		return 0, "", fmt.Errorf("verify score %.2f out of range", review.Score)
	}
	if strings.TrimSpace(review.Report) == "" {
		return 0, "", fmt.Errorf("verify report is empty")
	}
	return review.Score, review.Report, nil
}

// readHandoffFile reads the handoff file for the given phase order.
func (t *researchPipelineTool) readHandoffFile(workDir string, phaseOrder int) string {
	if workDir == "" {
		return "(no workspace directory)"
	}
	handoffDir := filepath.Join(workDir, ".handoff")
	prefix := fmt.Sprintf("%02d-", phaseOrder)

	entries, err := os.ReadDir(handoffDir)
	if err != nil {
		return "(handoff directory not found)"
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			content, err := os.ReadFile(filepath.Join(handoffDir, entry.Name()))
			if err != nil {
				continue
			}
			s := string(content)
			runes := []rune(s)
			if len(runes) > 4000 {
				s = string(runes[:4000]) + "\n...(truncated)"
			}
			return s
		}
	}
	return "(no handoff file found for this phase)"
}

func (t *researchPipelineTool) handlePause(pipeline ResearchPipelineView) (ToolResponse, error) {
	if err := t.ctrl.PausePipeline(pipeline.ID); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to pause: %v", err)), nil
	}
	return NewTextResponse("Pipeline paused successfully."), nil
}

func (t *researchPipelineTool) handleSetMode(pipeline ResearchPipelineView, mode string) (ToolResponse, error) {
	if mode != "default" && mode != "auto" && mode != "strict" {
		return NewTextErrorResponse(fmt.Sprintf("Invalid mode: %s. Use 'default', 'auto', or 'strict'.", mode)), nil
	}
	if err := t.ctrl.SetPipelineMode(pipeline.ID, mode); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to set mode: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Pipeline mode changed to: %s", mode)), nil
}

func (t *researchPipelineTool) handleStatus(pipeline ResearchPipelineView) (ToolResponse, error) {
	return NewTextResponse(t.formatStatus(pipeline)), nil
}

func (t *researchPipelineTool) formatStatus(pipeline ResearchPipelineView) string {
	phases, err := t.ctrl.GetPhasesView(pipeline.ID)
	if err != nil {
		return fmt.Sprintf("Failed to get phases: %v", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Research Pipeline Status\n")
	fmt.Fprintf(&sb, "- Topic: %s\n", pipeline.Topic)
	fmt.Fprintf(&sb, "- Status: %s\n", pipeline.Status)
	fmt.Fprintf(&sb, "- Mode: %s\n", pipeline.Mode)
	fmt.Fprintf(&sb, "- WorkDir: %s\n", pipeline.WorkDir)
	fmt.Fprintf(&sb, "- Budget: $%.2f / $%.0f\n\n", pipeline.BudgetSpent, pipeline.BudgetLimit)

	fmt.Fprintf(&sb, "## Phases\n")
	for _, ph := range phases {
		marker := "  "
		switch ph.Status {
		case "running":
			marker = "▶ "
		case "completed":
			marker = "✓ "
		case "failed":
			marker = "✗ "
		case "paused":
			marker = "⏸ "
		}
		fmt.Fprintf(&sb, "%s%d. %s [%s]\n", marker, ph.Order, ph.Name, ph.Status)
	}
	return sb.String()
}
