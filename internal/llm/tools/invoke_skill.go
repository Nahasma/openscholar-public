package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	agentcustom "github.com/openscholar/openscholar/internal/llm/agent/custom"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/skillbank"
)

type invokeSkillTool struct {
	skills   skillbank.Service
	perms    permission.Service
	sessions session.Service
	messages message.Service
	runAgent AgentRunner
	kbs      *KBServices
}

type invokeSkillParams struct {
	ID   string `json:"id"`
	Task string `json:"task,omitempty"`
	Args string `json:"args,omitempty"`
	Mode string `json:"mode,omitempty"`
}

func NewInvokeSkillTool(skills skillbank.Service, perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner, kbs *KBServices) BaseTool {
	return &invokeSkillTool{
		skills:   skills,
		perms:    perms,
		sessions: sessions,
		messages: messages,
		runAgent: runAgent,
		kbs:      kbs,
	}
}

func (t *invokeSkillTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "InvokeSkill",
		Description: "Invoke a model-invocable SkillBank skill by id. Inline skills return executable instructions; fork skills run in a sub-agent with skill metadata constraints.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Skill id, e.g. research/structured_paper_reading.",
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Current task description to pass into the skill.",
				},
				"args": map[string]any{
					"type":        "string",
					"description": "Optional argument text to substitute for $ARGUMENTS.",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "inline", "fork"},
					"description": "Execution mode. auto honors skill context/agent metadata.",
				},
			},
			"required": []string{"id"},
		},
		Required: []string{"id"},
	}
}

func (t *invokeSkillTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params invokeSkillParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	id := strings.TrimSpace(params.ID)
	if id == "" {
		return NewTextErrorResponse("id is required"), nil
	}
	skill, err := t.skills.View(ctx, id)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Skill view failed: %v", err)), nil
	}
	if !skill.IsModelInvocable() {
		return NewTextErrorResponse(fmt.Sprintf("Skill %s is not model-invocable (exposure=%s).", skill.ID, skill.Exposure)), nil
	}
	_ = t.skills.RecordUsage(ctx, skill.ID, true)

	prompt := buildInvokeSkillPrompt(skill, params)
	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" || mode == "auto" {
		mode = invokeModeForSkill(skill)
	}
	if mode == "fork" {
		return t.runForkedSkill(ctx, call, skill, prompt)
	}
	return NewTextResponse(prompt), nil
}

func invokeModeForSkill(skill *skillbank.Skill) string {
	if skill == nil {
		return "inline"
	}
	if strings.EqualFold(strings.TrimSpace(skill.Context), "fork") || strings.TrimSpace(skill.Agent) != "" {
		return "fork"
	}
	return "inline"
}

func buildInvokeSkillPrompt(skill *skillbank.Skill, params invokeSkillParams) string {
	body := strings.ReplaceAll(skill.Instruction, "$ARGUMENTS", strings.TrimSpace(params.Args))
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Skill: %s [%s]\n\n", skill.Name, skill.ID)
	if skill.WhenToUse != "" {
		fmt.Fprintf(&sb, "When to use: %s\n\n", skill.WhenToUse)
	}
	if strings.TrimSpace(params.Task) != "" {
		fmt.Fprintf(&sb, "Current task:\n%s\n\n", strings.TrimSpace(params.Task))
	}
	if strings.TrimSpace(params.Args) != "" {
		fmt.Fprintf(&sb, "Arguments:\n%s\n\n", strings.TrimSpace(params.Args))
	}
	sb.WriteString("Instructions:\n")
	sb.WriteString(body)
	return strings.TrimSpace(sb.String())
}

func (t *invokeSkillTool) runForkedSkill(ctx context.Context, call ToolCall, skill *skillbank.Skill, prompt string) (ToolResponse, error) {
	if t.perms == nil || t.sessions == nil || t.runAgent == nil {
		return NewTextErrorResponse("forked skill execution is unavailable: Task runtime dependencies are not configured"), nil
	}
	parentSessionID, _ := GetContextValues(ctx)
	if parentSessionID == "" {
		return NewTextErrorResponse("forked skill execution requires an active session"), nil
	}
	agentType := strings.TrimSpace(skill.Agent)
	if agentType == "" {
		agentType = "general"
	}
	taskTool := &taskTool{
		permissions: t.perms,
		sessions:    t.sessions,
		messages:    t.messages,
		runAgent:    t.runAgent,
		kbs:         t.kbs,
		agents:      agentcustom.NewRegistry(config.WorkingDirectory()),
	}
	resolved, err := taskTool.resolveTaskAgent(agentType)
	if err != nil {
		resolved, err = taskTool.resolveTaskAgent("general")
		if err != nil {
			return NewTextErrorResponse(err.Error()), nil
		}
	}
	if len(skill.AllowedTools) > 0 {
		resolved.AllowedTools = append([]string(nil), skill.AllowedTools...)
	}
	modelOverride := normalizeModelOverride(skill.Model)
	agentTools := taskTool.selectAgentToolsForResolved(ctx, resolved)
	if len(skill.AllowedTools) > 0 {
		agentTools = filterToolsByName(agentTools, skill.AllowedTools)
	}

	taskID := strings.TrimSpace(call.ID)
	if taskID == "" {
		taskID = fmt.Sprintf("invoke-skill-%d", time.Now().UnixNano())
	}
	childSession, err := t.sessions.CreateTaskSession(ctx, taskID, parentSessionID, "InvokeSkill "+skill.ID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create skill task session: %v", err)), nil
	}
	t.perms.SetSessionMode(childSession.ID, subagentSessionMode(t.perms, parentSessionID, resolved.SessionMode))
	t.perms.RequirePromptSession(childSession.ID)
	defer t.perms.RemoveRequirePromptSession(childSession.ID)

	runCtx := ctx
	if modelOverride != "" {
		runCtx = WithTaskRuntimeOverride(runCtx, TaskRuntimeOverride{Model: modelOverride})
	}
	result, err := t.runAgent(runCtx, resolved.AgentName, childSession.ID, prompt, agentTools)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("forked skill failed: %v", err)), nil
	}
	if strings.TrimSpace(result) == "" {
		return NewTextResponse("Forked skill completed (no output)."), nil
	}
	return NewTextResponse(result), nil
}

func filterToolsByName(tools []BaseTool, names []string) []BaseTool {
	if len(names) == 0 {
		return tools
	}
	if allowsAllTools(names) {
		return tools
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowed[strings.ToLower(name)] = struct{}{}
	}
	filtered := make([]BaseTool, 0, len(tools))
	for _, tool := range tools {
		if _, ok := allowed[strings.ToLower(tool.Info().Name)]; ok {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}
