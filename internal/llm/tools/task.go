package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/hooks"
	agentcustom "github.com/Nahasma/openscholar-public/internal/llm/agent/custom"
	"github.com/Nahasma/openscholar-public/internal/llm/tools/codeagent"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
)

// AgentRunner is the function signature for running a sub-agent.
// This breaks the circular dependency between tools and agent packages.
type AgentRunner func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []BaseTool) (string, error)

type taskTool struct {
	permissions permission.Service
	sessions    session.Service
	messages    message.Service
	runAgent    AgentRunner
	kbs         *KBServices

	registry    *task.Registry
	hookService hooks.Service
	agents      *agentcustom.Registry
	profiles    *SubagentProfileResolver
	scheduler   *SubagentScheduler
}

type taskRuntime struct {
	cancel context.CancelFunc
}

const (
	readingWorkerAgentType     = "reader"
	readingWorkerMaxConcurrent = 5
	readingWorkerTimeout       = 12 * time.Minute
)

var baseReadingWorkerTools = []string{"View", "KBList", "KBTree", "KBQuery", "KBSearch"}

// NewTaskTool creates a Task tool that can spawn sub-agents.
// The agentRunner callback is provided by the caller to avoid circular imports.
func NewTaskTool(perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner) BaseTool {
	return NewTaskToolWithKB(perms, sessions, messages, runAgent, nil)
}

// NewTaskToolWithKB creates a Task tool with optional KB dependencies for spawned general/default sub-agents.
func NewTaskToolWithKB(perms permission.Service, sessions session.Service, messages message.Service, runAgent AgentRunner, kbs *KBServices) BaseTool {
	reg, hookSvc := getTaskRuntime()
	return &taskTool{
		permissions: perms,
		sessions:    sessions,
		messages:    messages,
		runAgent:    runAgent,
		kbs:         kbs,
		registry:    reg,
		hookService: hookSvc,
		agents:      agentcustom.NewRegistry(config.WorkingDirectory()),
		profiles:    NewSubagentProfileResolver(config.Get()),
		scheduler:   SharedSubagentScheduler(),
	}
}

func (t *taskTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "Task",
		Description: "Launch and manage sub-agents. Use agent_type=research for broad literature/web search so raw results stay out of the parent context. Legacy mode (without action) runs synchronously. TaskV2 supports create/send/stop/read/list.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "send", "stop", "read", "list"},
					"description": "TaskV2 action. Omit this field to use legacy synchronous mode.",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Task ID for send/stop/read. For create, optional override (defaults to tool call ID).",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "A short (3-10 word) description of the task",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Detailed task prompt for the sub-agent",
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "Compatibility alias for prompt. Prefer using prompt.",
				},
				"agent_type": map[string]any{
					"type":        "string",
					"description": "Type of sub-agent. Built-ins: general, research, explore, experiment, reader, leader, plan, verify, coordinator. Use research for broad ScholarSearch/WebSearch fanout and compressed evidence returns. Custom agent names from .openscholar/agents/*.md or .openscholar/modes/*.md are also accepted.",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Compatibility alias for agent_type. mode=coder/default maps to general.",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Optional same-provider model override for the sub-agent. Omit or use inherit to keep the parent model.",
				},
				"verify_policy": map[string]any{
					"type":        "string",
					"enum":        []string{"none", "required"},
					"description": "Verification policy for TaskV2 runs. Defaults to required for leader/plan/coordinator tasks.",
				},
				"debug_transcript": map[string]any{
					"type":        "boolean",
					"description": "For action=read only. Defaults false; set true to include a capped transcript for debugging.",
				},
				"parent_task_id": map[string]any{
					"type":        "string",
					"description": "Optional parent task id for task graph metadata.",
				},
				"fanout_group": map[string]any{
					"type":        "string",
					"description": "Optional fanout group id for related subtasks.",
				},
				"write_set": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Scoped write targets; required for writable workers when orchestration is enabled.",
				},
				"result_max_chars": map[string]any{
					"type":        "integer",
					"description": "Max chars for stored/returned result. Verification always uses full result.",
				},
				"max_turns": map[string]any{
					"type":        "integer",
					"description": "Optional per-task max turns metadata.",
				},
			},
		},
	}
}

type taskParams struct {
	Action          string   `json:"action,omitempty"`
	TaskID          string   `json:"task_id,omitempty"`
	Description     string   `json:"description,omitempty"`
	Prompt          string   `json:"prompt,omitempty"`
	Instruction     string   `json:"instruction,omitempty"`
	AgentType       string   `json:"agent_type,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	Model           string   `json:"model,omitempty"`
	VerifyPolicy    string   `json:"verify_policy,omitempty"`
	DebugTranscript bool     `json:"debug_transcript,omitempty"`
	ParentTaskID    string   `json:"parent_task_id,omitempty"`
	FanoutGroup     string   `json:"fanout_group,omitempty"`
	WriteSet        []string `json:"write_set,omitempty"`
	ResultMaxChars  int      `json:"result_max_chars,omitempty"`
	MaxTurns        int      `json:"max_turns,omitempty"`
}

type taskAgentResolution struct {
	AgentType     string
	AgentName     config.AgentName
	Config        *agentcustom.AgentConfig
	Model         string
	AllowedTools  []string
	DeniedTools   []string
	ToolsExplicit bool
	SessionMode   permission.Mode
	PromptPrefix  string
	Timeout       time.Duration
	VerifyPolicy  string
	Profile       SubagentProfile
}

func normalizeTaskPrompt(params taskParams) string {
	if prompt := strings.TrimSpace(params.Prompt); prompt != "" {
		return prompt
	}
	if instruction := strings.TrimSpace(params.Instruction); instruction != "" {
		return instruction
	}
	return strings.TrimSpace(params.Description)
}

func normalizeTaskAgentType(params taskParams) string {
	agentType := strings.ToLower(strings.TrimSpace(params.AgentType))
	if agentType != "" {
		return agentType
	}
	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	switch mode {
	case "", "default", "coder":
		return "general"
	default:
		return mode
	}
}

func normalizeTaskAgentTypeWithDefault(params taskParams, defaultAgentType string) string {
	if strings.TrimSpace(params.AgentType) == "" && strings.TrimSpace(params.Mode) == "" {
		return strings.TrimSpace(defaultAgentType)
	}
	return normalizeTaskAgentType(params)
}

func resolveTaskMaxTurns(paramMaxTurns int, resolved taskAgentResolution) int {
	if paramMaxTurns > 0 {
		return paramMaxTurns
	}
	if resolved.Profile.MaxTurns > 0 {
		return resolved.Profile.MaxTurns
	}
	return 0
}

func (t *taskTool) resolveTaskAgent(agentType string) (taskAgentResolution, error) {
	agentType = strings.ToLower(strings.TrimSpace(agentType))
	if agentType == "" {
		agentType = "general"
	}
	var cfg *agentcustom.AgentConfig
	var ok bool
	if t.agents != nil && !isKnownBuiltInAgentType(agentType) && !isReadingWorkerAgentType(agentType) {
		cfg, ok = t.agents.Load(agentType)
		if !ok {
			return taskAgentResolution{}, fmt.Errorf("unsupported agent_type: %s", agentType)
		}
	}
	resolver := t.profiles
	if resolver == nil {
		resolver = NewSubagentProfileResolver(config.Get())
	}
	profile, err := resolver.Resolve(agentType, cfg)
	if err != nil {
		return taskAgentResolution{}, err
	}
	allowedTools := append([]string(nil), profile.AllowedTools...)
	promptPrefix := profile.PromptPrefix
	if isReadingWorkerAgentType(profile.AgentType) {
		allowedTools = t.readingWorkerAllowedTools()
		promptPrefix = readingWorkerPromptPrefix(allowedTools)
	}
	return taskAgentResolution{
		AgentType:     profile.AgentType,
		AgentName:     profile.AgentName,
		Config:        profile.Config,
		Model:         profile.Model,
		AllowedTools:  allowedTools,
		DeniedTools:   append([]string(nil), profile.DeniedTools...),
		ToolsExplicit: profile.ToolsExplicit,
		SessionMode:   profile.SessionMode,
		PromptPrefix:  promptPrefix,
		Timeout:       profile.Timeout,
		VerifyPolicy:  profile.VerifyPolicy,
		Profile:       profile,
	}, nil
}

func isReadingWorkerAgentType(agentType string) bool {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case readingWorkerAgentType, "reading", "paper-reader", "paper_reader":
		return true
	default:
		return false
	}
}

func (t *taskTool) readingWorkerAllowedTools() []string {
	if t.kbs == nil || t.kbs.KB == nil {
		return []string{"View"}
	}
	if t.kbs.CallLLM == nil {
		return []string{"View", "KBList", "KBTree"}
	}
	return append([]string(nil), baseReadingWorkerTools...)
}

func readingWorkerPromptPrefix(allowedTools []string) string {
	return fmt.Sprintf(`# Reading Worker

You are a read-only paper reading worker. Stay within the assigned paper, section range, and question.

Allowed tools: %s.
Forbidden tools and actions: Write, Edit, Bash, ScholarSearch, KBAdd, WebSearch, WebFetch, downloads, file creation, file mutation, and external expansion.

Return a structured result with these fields:
- paper_id
- title
- coverage
- evidence: page/node references for every substantive claim
- claims
- method
- results
- limitations
- uncertainties
- not_covered

If the KB result says the index is basic, degraded, text-only, or summary-only, state that limitation in coverage and not_covered. Do not claim full-paper reading unless the available evidence supports it.

# Task`, strings.Join(allowedTools, ", "))
}

func (r taskAgentResolution) modelOverride(input string) string {
	if model := normalizeModelOverride(input); model != "" {
		return model
	}
	return normalizeModelOverride(r.Model)
}

func (r taskAgentResolution) runtimeAllowedTools() []string {
	if !r.ToolsExplicit || len(r.AllowedTools) == 0 || allowsAllTools(r.AllowedTools) {
		return nil
	}
	return filterDeniedToolNames(r.AllowedTools, r.DeniedTools)
}

func (r taskAgentResolution) prompt(input string) string {
	if r.Config == nil || strings.TrimSpace(r.Config.SystemPrompt) == "" {
		if strings.TrimSpace(r.PromptPrefix) != "" {
			return strings.TrimSpace(r.PromptPrefix) + "\n" + strings.TrimSpace(input)
		}
		return input
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Custom Agent: %s\n\n", r.Config.Name)
	if r.Config.Description != "" {
		fmt.Fprintf(&sb, "%s\n\n", strings.TrimSpace(r.Config.Description))
	}
	sb.WriteString(strings.TrimSpace(r.Config.SystemPrompt))
	sb.WriteString("\n\n# Task\n")
	sb.WriteString(strings.TrimSpace(input))
	return sb.String()
}

type taskView struct {
	TaskID          string            `json:"task_id"`
	Status          task.Status       `json:"status"`
	ParentSessionID string            `json:"parent_session_id"`
	ChildSessionID  string            `json:"child_session_id"`
	AgentType       string            `json:"agent_type"`
	Summary         string            `json:"summary,omitempty"`
	Model           string            `json:"model,omitempty"`
	VerifyPolicy    string            `json:"verify_policy,omitempty"`
	VerifyStatus    task.VerifyStatus `json:"verify_status,omitempty"`
	VerifyVerdict   string            `json:"verify_verdict,omitempty"`
	VerifyResult    string            `json:"verify_result,omitempty"`
	VerifyError     string            `json:"verify_error,omitempty"`
	Result          string            `json:"result,omitempty"`
	LastError       string            `json:"last_error,omitempty"`
	ResultTruncated bool              `json:"result_truncated,omitempty"`
	StartedAt       time.Time         `json:"started_at"`
	EndedAt         *time.Time        `json:"ended_at,omitempty"`
}

type taskMessageView struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type taskReadView struct {
	taskView
	Transcript []taskMessageView `json:"transcript,omitempty"`
	Truncated  bool              `json:"truncated,omitempty"`
}

func (t *taskTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params taskParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}
	if strings.TrimSpace(params.Action) == "" {
		return t.runLegacySync(ctx, call, params)
	}
	return t.runV2(ctx, call, params)
}

func (t *taskTool) runLegacySync(ctx context.Context, call ToolCall, params taskParams) (ToolResponse, error) {
	params.Prompt = normalizeTaskPrompt(params)
	if params.Prompt == "" {
		return NewTextErrorResponse("prompt is required — provide a detailed task description in the 'prompt' field"), nil
	}
	params.AgentType = normalizeTaskAgentType(params)

	resolved, err := t.resolveTaskAgent(params.AgentType)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	parentSessionID, _ := GetContextValues(ctx)
	if parentSessionID == "" {
		return NewTextErrorResponse("no active session"), nil
	}
	if err := t.validateNestedTaskCreate(ctx, parentSessionID); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	writeSet := normalizeWriteSet(params.WriteSet)
	if err := t.validateWriteSetRequirement(resolved, writeSet); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	childSession, err := t.sessions.CreateTaskSession(ctx, call.ID, parentSessionID, params.Description)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create task session: %s", err)), nil
	}
	legacyState := t.registerLegacySyncState(call.ID, parentSessionID, childSession.ID, params.Description, resolved, params.Model, writeSet)

	t.permissions.SetSessionMode(childSession.ID, subagentSessionMode(t.permissions, parentSessionID, resolved.SessionMode))
	t.permissions.RequirePromptSession(childSession.ID)
	defer t.permissions.RemoveRequirePromptSession(childSession.ID)

	agentTools := t.selectAgentToolsForResolved(ctx, resolved)
	runCtx := ctx
	if resolved.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, resolved.Timeout)
		defer cancel()
	}
	if IsResearchMode(runCtx) {
		runCtx = context.WithValue(runCtx, ResearchToolProfileContextKey, researchToolProfileForAgentType(resolved.AgentType))
	}
	modelOverride := resolved.modelOverride(params.Model)
	runtimeAllowedTools := resolved.runtimeAllowedTools()
	if strings.TrimSpace(modelOverride) != "" || len(runtimeAllowedTools) > 0 {
		runCtx = WithTaskRuntimeOverride(runCtx, TaskRuntimeOverride{
			Model:        modelOverride,
			AllowedTools: runtimeAllowedTools,
		})
	}
	lease, err := acquireSubagentLease(runCtx, t.scheduler, parentSessionID, profileForScheduler(resolved), writeSet)
	if err != nil {
		t.finishLegacySyncState(legacyState, task.StatusCanceled, "", err.Error())
		return NewTextErrorResponse(err.Error()), nil
	}
	defer lease.Release()

	result, err := t.runAgent(runCtx, resolved.AgentName, childSession.ID, resolved.prompt(params.Prompt), agentTools)
	if err != nil {
		t.finishLegacySyncState(legacyState, task.StatusFailed, "", err.Error())
		return NewTextErrorResponse(fmt.Sprintf("sub-agent error: %s", err)), nil
	}

	t.finishLegacySyncState(legacyState, task.StatusCompleted, strings.TrimSpace(result), "")
	if strings.TrimSpace(result) == "" {
		return NewTextResponse("Task completed (no output)"), nil
	}

	return NewTextResponse(result), nil
}

func (t *taskTool) registerLegacySyncState(taskID, parentSessionID, childSessionID, description string, resolved taskAgentResolution, model string, writeSet []string) *task.SubtaskState {
	if !t.orchestrationEnabled() || t.registry == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	resolvedModel := resolved.modelOverride(model)
	state := task.NewSubtaskState(task.Meta{
		ID:             taskID,
		Label:          strings.TrimSpace(description),
		SessionID:      parentSessionID,
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      time.Now(),
		IsBackgrounded: false,
		Notified:       true,
	}, parentSessionID, childSessionID, resolved.AgentType, description, resolvedModel, "none")
	if state.TaskMeta().Label == "" {
		state.TaskMeta().Label = "Task " + taskID
	}
	state.FanoutGroup = "legacy-sync"
	state.WriteSet = append([]string(nil), writeSet...)
	state.MaxTurns = resolved.Profile.MaxTurns
	t.registry.Register(state)
	return state
}

func (t *taskTool) finishLegacySyncState(state *task.SubtaskState, status task.Status, result string, lastErr string) {
	if state == nil || t == nil || t.registry == nil {
		return
	}
	now := time.Now()
	t.registry.Mutate(state.TaskMeta().ID, func(s task.State) {
		sub, ok := s.(*task.SubtaskState)
		if !ok {
			return
		}
		sub.Result = strings.TrimSpace(result)
		sub.LastError = strings.TrimSpace(lastErr)
		sub.TaskMeta().Status = status
		sub.TaskMeta().EndedAt = &now
		sub.TaskMeta().Notified = true
		sub.TaskMeta().NotifyClaimed = false
	})
}

func (t *taskTool) runV2(ctx context.Context, call ToolCall, params taskParams) (ToolResponse, error) {
	if t.registry == nil {
		return NewTextErrorResponse("TaskV2 is unavailable: task registry is not configured"), nil
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	switch action {
	case "create":
		return t.createTaskV2(ctx, call, params)
	case "send":
		return t.sendTaskV2(ctx, params)
	case "stop":
		return t.stopTaskV2(ctx, params)
	case "read":
		return t.readTaskV2(ctx, params)
	case "list":
		return t.listTaskV2(ctx)
	default:
		return NewTextErrorResponse("invalid action: use create|send|stop|read|list"), nil
	}
}

func (t *taskTool) createTaskV2(ctx context.Context, call ToolCall, params taskParams) (ToolResponse, error) {
	parentSessionID, _ := GetContextValues(ctx)
	if parentSessionID == "" {
		return NewTextErrorResponse("no active session"), nil
	}
	if err := t.validateNestedTaskCreate(ctx, parentSessionID); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	params.Prompt = normalizeTaskPrompt(params)
	if strings.TrimSpace(params.Prompt) == "" {
		return NewTextErrorResponse("prompt is required for action=create"), nil
	}
	params.AgentType = normalizeTaskAgentType(params)
	resolved, err := t.resolveTaskAgent(params.AgentType)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	if isReadingWorkerAgentType(resolved.AgentType) && t.runningReadingWorkerCount(parentSessionID) >= readingWorkerMaxConcurrent {
		return NewTextErrorResponse(fmt.Sprintf("reading worker limit reached: max %d concurrent reader tasks per parent session", readingWorkerMaxConcurrent)), nil
	}
	verifyPolicy, err := resolved.normalizeVerifyPolicy(params.VerifyPolicy)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	writeSet := normalizeWriteSet(params.WriteSet)
	if err := t.validateWriteSetRequirement(resolved, writeSet); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	resultCap := t.resolveResultMaxChars(params.ResultMaxChars, resolved)
	maxTurns := resolveTaskMaxTurns(params.MaxTurns, resolved)

	taskID := strings.TrimSpace(params.TaskID)
	if taskID == "" {
		taskID = call.ID
	}
	if taskID == "" {
		return NewTextErrorResponse("task_id is required for action=create when tool call id is empty"), nil
	}

	if _, exists := t.registry.Get(taskID); exists {
		return NewTextErrorResponse("task_id already exists"), nil
	}

	title := strings.TrimSpace(params.Description)
	if title == "" {
		title = "Task " + taskID
	}

	launcher := NewBackgroundSubtaskLauncher(t.permissions, t.sessions, t.runAgent)
	state, err := launcher.Launch(ctx, BackgroundSubtaskSpec{
		TaskID:          taskID,
		ParentSessionID: parentSessionID,
		Description:     title,
		Prompt:          resolved.prompt(params.Prompt),
		AgentType:       resolved.AgentType,
		AgentName:       resolved.AgentName,
		AgentTools:      t.selectAgentToolsForResolved(ctx, resolved),
		SessionMode:     subagentSessionMode(t.permissions, parentSessionID, resolved.SessionMode),
		Model:           resolved.modelOverride(params.Model),
		AllowedTools:    resolved.runtimeAllowedTools(),
		VerifyPolicy:    verifyPolicy,
		Timeout:         resolved.Timeout,
		ParentTaskID:    strings.TrimSpace(params.ParentTaskID),
		FanoutGroup:     strings.TrimSpace(params.FanoutGroup),
		WriteSet:        writeSet,
		ResultMaxChars:  resultCap,
		MaxTurns:        maxTurns,
		Scheduler:       t.scheduler,
		Profile:         profileForScheduler(resolved),
	})
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	return newJSONResponse(taskViewFromState(state)), nil
}

func (t *taskTool) sendTaskV2(ctx context.Context, params taskParams) (ToolResponse, error) {
	if strings.TrimSpace(params.TaskID) == "" {
		return NewTextErrorResponse("task_id is required for action=send"), nil
	}
	params.Prompt = normalizeTaskPrompt(params)
	if strings.TrimSpace(params.Prompt) == "" {
		return NewTextErrorResponse("prompt is required for action=send"), nil
	}

	state, errResp := t.getSubtaskForParent(ctx, params.TaskID)
	if errResp != "" {
		return NewTextErrorResponse(errResp), nil
	}
	if t.isTaskRunning(state.TaskMeta().ID) {
		return NewTextErrorResponse("task is already running"), nil
	}

	agentType := normalizeTaskAgentTypeWithDefault(params, state.AgentType)
	resolved, err := t.resolveTaskAgent(agentType)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	if isReadingWorkerAgentType(resolved.AgentType) && t.runningReadingWorkerCount(state.ParentSessionID) >= readingWorkerMaxConcurrent {
		return NewTextErrorResponse(fmt.Sprintf("reading worker limit reached: max %d concurrent reader tasks per parent session", readingWorkerMaxConcurrent)), nil
	}
	modelOverride := resolved.modelOverride(params.Model)
	if modelOverride == "" {
		modelOverride = strings.TrimSpace(state.Model)
	}
	verifyPolicyInput := params.VerifyPolicy
	if strings.TrimSpace(verifyPolicyInput) == "" {
		verifyPolicyInput = state.VerifyPolicy
	}
	verifyPolicy, err := resolved.normalizeVerifyPolicy(verifyPolicyInput)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	writeSet := state.WriteSet
	if len(params.WriteSet) > 0 {
		writeSet = normalizeWriteSet(params.WriteSet)
	}
	if err := t.validateWriteSetRequirement(resolved, writeSet); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	resultCap := state.ResultMaxChars
	if params.ResultMaxChars > 0 {
		resultCap = t.resolveResultMaxChars(params.ResultMaxChars, resolved)
	} else if resultCap <= 0 {
		resultCap = t.resolveResultMaxChars(0, resolved)
	}
	maxTurns := state.MaxTurns
	if params.MaxTurns > 0 {
		maxTurns = params.MaxTurns
	} else if maxTurns <= 0 {
		maxTurns = resolveTaskMaxTurns(0, resolved)
	}
	state.AgentType = resolved.AgentType
	state.Model = modelOverride
	state.VerifyPolicy = verifyPolicy
	state.VerifyStatus = taskVerifyStatusForPolicy(verifyPolicy)
	state.VerifyVerdict = ""
	state.Result = ""
	state.ResultTruncated = false
	state.VerifyResult = ""
	state.VerifyError = ""
	state.WriteSet = append([]string(nil), writeSet...)
	state.ResultMaxChars = resultCap
	state.MaxTurns = maxTurns

	view := taskView{
		TaskID:          state.TaskMeta().ID,
		Status:          task.StatusRunning,
		ParentSessionID: state.ParentSessionID,
		ChildSessionID:  state.ChildSessionID,
		AgentType:       resolved.AgentType,
		Summary:         state.Summary,
		Model:           modelOverride,
		VerifyPolicy:    verifyPolicy,
		VerifyStatus:    taskVerifyStatusForPolicy(verifyPolicy),
		VerifyVerdict:   "",
		ResultTruncated: false,
		StartedAt:       time.Now(),
	}
	if !t.startSubtaskRun(ctx, state.ParentSessionID, state, resolved.prompt(params.Prompt), resolved.AgentType, resolved.AgentName, t.selectAgentToolsForResolved(ctx, resolved), resolved.runtimeAllowedTools(), resolved.Timeout, profileForScheduler(resolved), writeSet, func() {
		t.registry.Mutate(state.TaskMeta().ID, func(s task.State) {
			sub, ok := s.(*task.SubtaskState)
			if !ok {
				return
			}
			sub.AgentType = resolved.AgentType
			sub.Model = modelOverride
			sub.VerifyPolicy = verifyPolicy
			sub.VerifyStatus = taskVerifyStatusForPolicy(verifyPolicy)
			sub.VerifyVerdict = ""
			sub.VerifyResult = ""
			sub.VerifyError = ""
			sub.Result = ""
			sub.ResultTruncated = false
			sub.LastError = ""
			sub.WriteSet = append([]string(nil), writeSet...)
			sub.ResultMaxChars = resultCap
			sub.MaxTurns = maxTurns
			sub.TaskMeta().Status = task.StatusRunning
			sub.TaskMeta().StartedAt = view.StartedAt
			sub.TaskMeta().EndedAt = nil
			sub.TaskMeta().Notified = false
			sub.TaskMeta().NotifyClaimed = false
		})
		t.permissions.SetSessionMode(state.ChildSessionID, subagentSessionMode(t.permissions, state.ParentSessionID, resolved.SessionMode))
	}) {
		return NewTextErrorResponse("task is already running"), nil
	}

	return newJSONResponse(view), nil
}

func (t *taskTool) stopTaskV2(ctx context.Context, params taskParams) (ToolResponse, error) {
	if strings.TrimSpace(params.TaskID) == "" {
		return NewTextErrorResponse("task_id is required for action=stop"), nil
	}
	state, errResp := t.getSubtaskForParent(ctx, params.TaskID)
	if errResp != "" {
		return NewTextErrorResponse(errResp), nil
	}
	if !t.cancelTask(state.TaskMeta().ID) {
		return NewTextErrorResponse("task is not running"), nil
	}
	return NewTextResponse("stop signal sent"), nil
}

func (t *taskTool) readTaskV2(ctx context.Context, params taskParams) (ToolResponse, error) {
	if strings.TrimSpace(params.TaskID) == "" {
		return NewTextErrorResponse("task_id is required for action=read"), nil
	}
	state, errResp := t.getSubtaskForParent(ctx, params.TaskID)
	if errResp != "" {
		return NewTextErrorResponse(errResp), nil
	}
	view := t.buildTaskReadView(ctx, state, params.DebugTranscript)
	if isTerminalTaskStatus(state.TaskMeta().Status) {
		t.markTaskNotified(state.TaskMeta().ID)
	}
	return newJSONResponse(view), nil
}

func (t *taskTool) listTaskV2(ctx context.Context) (ToolResponse, error) {
	parentSessionID, _ := GetContextValues(ctx)
	if parentSessionID == "" {
		return NewTextErrorResponse("no active session"), nil
	}
	all := t.registry.All()
	out := make([]taskView, 0)
	for _, st := range all {
		sub, ok := st.(*task.SubtaskState)
		if !ok {
			continue
		}
		if sub.ParentSessionID != parentSessionID {
			continue
		}
		out = append(out, taskViewFromState(sub))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return newJSONResponse(out), nil
}

func (t *taskTool) runningReadingWorkerCount(parentSessionID string) int {
	if t.registry == nil {
		return 0
	}
	count := 0
	for _, st := range t.registry.All() {
		sub, ok := st.(*task.SubtaskState)
		if !ok {
			continue
		}
		if sub.ParentSessionID == parentSessionID &&
			isReadingWorkerAgentType(sub.AgentType) &&
			sub.TaskMeta().Status == task.StatusRunning {
			count++
		}
	}
	return count
}

func (t *taskTool) startSubtaskRun(parentCtx context.Context, parentSessionID string, state *task.SubtaskState, prompt string, agentType string, agentName config.AgentName, agentTools []BaseTool, allowedTools []string, timeout time.Duration, profile SubagentProfile, writeSet []string, prepare func()) bool {
	baseCtx := detachedTaskContext(parentCtx)
	runCtx, cancel := context.WithCancel(baseCtx)
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(baseCtx, timeout)
	}
	if IsResearchMode(runCtx) {
		runCtx = context.WithValue(runCtx, ResearchToolProfileContextKey, researchToolProfileForAgentType(agentType))
	}
	if !t.setTaskRuntime(state.TaskMeta().ID, &taskRuntime{cancel: cancel}) {
		cancel()
		return false
	}
	if prepare != nil {
		prepare()
	}
	if strings.TrimSpace(state.Model) != "" || len(allowedTools) > 0 {
		runCtx = WithTaskRuntimeOverride(runCtx, TaskRuntimeOverride{
			Model:        state.Model,
			AllowedTools: allowedTools,
		})
	}
	hookCtx := context.WithoutCancel(runCtx)
	t.permissions.RequirePromptSession(state.ChildSessionID)
	_ = t.emitHook(hookCtx, hooks.SubagentStart, parentSessionID)

	go func(taskID string, childSessionID string) {
		lease, acquireErr := acquireSubagentLease(runCtx, t.scheduler, parentSessionID, profile, writeSet)
		if acquireErr != nil {
			now := time.Now()
			t.permissions.RemoveRequirePromptSession(childSessionID)
			t.registry.Mutate(taskID, func(s task.State) {
				sub, ok := s.(*task.SubtaskState)
				if !ok {
					return
				}
				sub.LastError = acquireErr.Error()
				sub.TaskMeta().Status = task.StatusCanceled
				sub.TaskMeta().EndedAt = &now
				sub.TaskMeta().Notified = false
				sub.TaskMeta().NotifyClaimed = false
				if taskNeedsVerification(sub.VerifyPolicy) {
					sub.VerifyStatus = task.VerifyStatusFailed
					sub.VerifyVerdict = ""
					sub.VerifyError = "task execution failed before verification: " + acquireErr.Error()
				}
			})
			t.clearTaskRuntime(taskID)
			_ = t.emitHook(hookCtx, hooks.SubagentStop, parentSessionID)
			_ = t.emitHook(hookCtx, hooks.TaskCompleted, parentSessionID)
			return
		}
		defer lease.Release()

		result, err := t.runAgentWithNotificationResumes(runCtx, agentName, childSessionID, prompt, agentTools, profile, state.MaxTurns)
		t.permissions.RemoveRequirePromptSession(childSessionID)

		fullResult := strings.TrimSpace(result)
		cappedResult, resultTruncated := applyResultCap(fullResult, state.ResultMaxChars)
		verifyStatus := state.VerifyStatus
		verifyVerdict := ""
		verifyResult := ""
		verifyError := ""
		finalStatus := task.StatusCompleted

		now := time.Now()
		if err != nil {
			finalStatus = task.StatusFailed
			if runCtx.Err() == context.Canceled {
				finalStatus = task.StatusCanceled
			}
			if taskNeedsVerification(state.VerifyPolicy) {
				verifyStatus = task.VerifyStatusFailed
				verifyError = "task execution failed before verification"
			}
		} else if taskNeedsVerification(state.VerifyPolicy) {
			t.registry.Mutate(taskID, func(s task.State) {
				sub, ok := s.(*task.SubtaskState)
				if !ok {
					return
				}
				sub.Result = cappedResult
				sub.ResultTruncated = resultTruncated
				sub.VerifyStatus = task.VerifyStatusRunning
				sub.VerifyVerdict = ""
				sub.VerifyError = ""
				sub.VerifyResult = ""
				sub.TaskMeta().Notified = false
				sub.TaskMeta().NotifyClaimed = false
			})
			verifyStatus, verifyVerdict, verifyResult, verifyError = runTaskVerification(runCtx, t.permissions, t.sessions, t.runAgent, state, prompt, fullResult)
			if verifyStatus != task.VerifyStatusPassed {
				finalStatus = task.StatusFailed
				if runCtx.Err() == context.Canceled {
					finalStatus = task.StatusCanceled
				}
			}
		}

		t.registry.Mutate(taskID, func(s task.State) {
			sub, ok := s.(*task.SubtaskState)
			if !ok {
				return
			}
			sub.Result = cappedResult
			sub.ResultTruncated = resultTruncated
			sub.VerifyStatus = verifyStatus
			sub.VerifyVerdict = verifyVerdict
			sub.VerifyResult = verifyResult
			sub.VerifyError = verifyError
			sub.TaskMeta().EndedAt = &now
			sub.TaskMeta().Notified = false
			sub.TaskMeta().NotifyClaimed = false
			if err != nil {
				sub.LastError = err.Error()
				sub.TaskMeta().Status = finalStatus
				return
			}
			if verifyError != "" {
				sub.LastError = verifyError
				sub.TaskMeta().Status = finalStatus
				return
			}
			sub.LastError = ""
			sub.TaskMeta().Status = finalStatus
		})

		t.clearTaskRuntime(taskID)
		_ = t.emitHook(hookCtx, hooks.SubagentStop, parentSessionID)
		_ = t.emitHook(hookCtx, hooks.TaskCompleted, parentSessionID)
	}(state.TaskMeta().ID, state.ChildSessionID)

	return true
}

func (t *taskTool) runAgentWithNotificationResumes(ctx context.Context, agentName config.AgentName, childSessionID string, prompt string, agentTools []BaseTool, profile SubagentProfile, maxTurns int) (string, error) {
	result, err := t.runAgent(ctx, agentName, childSessionID, prompt, agentTools)
	if err != nil || !profile.CanSpawnTask || !t.orchestrationEnabled() {
		return result, err
	}
	limit := subagentNotificationResumeLimit(maxTurns)
	for i := 0; i < limit; i++ {
		ready, waitErr := t.waitForChildTaskNotifications(ctx, childSessionID)
		if waitErr != nil {
			return result, waitErr
		}
		if !ready {
			return result, nil
		}
		next, runErr := t.runAgent(ctx, agentName, childSessionID, "Continue after completed child task notifications. Use the injected <task-notification> messages to synthesize child results, continue delegated work if needed, or finish the original task.", agentTools)
		if strings.TrimSpace(next) != "" {
			result = next
		}
		if runErr != nil {
			return result, runErr
		}
	}
	if ready, _ := t.childTaskNotificationState(childSessionID); ready {
		return result, fmt.Errorf("subagent notification resume limit reached for session %s", childSessionID)
	}
	return result, nil
}

func (t *taskTool) waitForChildTaskNotifications(ctx context.Context, parentSessionID string) (bool, error) {
	return waitForChildTaskNotificationsByRegistry(ctx, t.registry, parentSessionID, t.childTaskNotificationState)
}

func (t *taskTool) childTaskNotificationState(parentSessionID string) (ready bool, running bool) {
	if t == nil || t.registry == nil {
		return false, false
	}
	for _, st := range t.registry.All() {
		sub, ok := st.(*task.SubtaskState)
		if !ok || sub.ParentSessionID != parentSessionID {
			continue
		}
		switch sub.TaskMeta().Status {
		case task.StatusPending, task.StatusRunning:
			running = true
		case task.StatusCompleted, task.StatusFailed, task.StatusCanceled:
			if !sub.TaskMeta().Notified && !sub.TaskMeta().NotifyClaimed {
				ready = true
			}
		}
	}
	return ready, running
}

func (t *taskTool) selectAgentTools(ctx context.Context, agentType string) []BaseTool {
	agentType = strings.ToLower(strings.TrimSpace(agentType))
	if agentType == "" {
		agentType = "general"
	}
	var agentTools []BaseTool
	switch agentType {
	case "explore":
		agentTools = NewExploreRegistry(t.permissions)
	case "research":
		agentTools = NewResearchSearchRegistry(t.permissions, t.kbs)
	case "experiment":
		codeAgentReg := codeagent.NewRegistry()
		agentTools = NewExperimentWorkerRegistry(t.permissions, codeAgentReg)
	case "plan":
		agentTools = NewPlanRegistry(t.permissions)
	case "verify":
		agentTools = NewVerifyRegistry(t.permissions)
	case "leader":
		if IsResearchMode(ctx) {
			agentTools = ResearchLeaderTools(t.permissions, t.sessions, t.messages, t.runAgent)
		} else {
			agentTools = LeaderTools(t.permissions, t.sessions, t.messages, t.runAgent)
		}
	case "coordinator":
		agentTools = NewCoordinatorRegistry(t.permissions, t.sessions, t.messages, t.runAgent)
	default:
		agentTools = NewRegistry(t.permissions, t.kbs)
	}

	if IsResearchMode(ctx) {
		profile := ResearchToolProfileLeader
		switch agentType {
		case "general", "research", "experiment":
			profile = ResearchToolProfileWorker
		}
		agentTools = FilterResearchToolsForProfile(agentTools, profile)
	}
	return agentTools
}

func (t *taskTool) selectAgentToolsForResolved(ctx context.Context, resolved taskAgentResolution) []BaseTool {
	baseType := resolved.AgentType
	if resolved.Config != nil {
		baseType = "general"
	}
	agentTools := t.selectAgentTools(ctx, baseType)
	if !resolved.ToolsExplicit || allowsAllTools(resolved.AllowedTools) {
		return filterDeniedTools(agentTools, resolved.DeniedTools)
	}
	if len(resolved.AllowedTools) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(resolved.AllowedTools))
	for _, name := range resolved.AllowedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowed[strings.ToLower(name)] = struct{}{}
	}
	filtered := make([]BaseTool, 0, len(agentTools))
	for _, tool := range agentTools {
		if _, ok := allowed[strings.ToLower(tool.Info().Name)]; ok {
			filtered = append(filtered, tool)
		}
	}
	return filterDeniedTools(filtered, resolved.DeniedTools)
}

func allowsAllTools(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == "*" {
			return true
		}
	}
	return false
}

func filterDeniedTools(agentTools []BaseTool, deniedTools []string) []BaseTool {
	if len(deniedTools) == 0 || len(agentTools) == 0 {
		return agentTools
	}
	denied := make(map[string]struct{}, len(deniedTools))
	for _, name := range deniedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		denied[strings.ToLower(name)] = struct{}{}
	}
	if len(denied) == 0 {
		return agentTools
	}
	filtered := make([]BaseTool, 0, len(agentTools))
	for _, tool := range agentTools {
		if _, ok := denied[strings.ToLower(tool.Info().Name)]; ok {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

func filterDeniedToolNames(allowedTools []string, deniedTools []string) []string {
	if len(allowedTools) == 0 || len(deniedTools) == 0 {
		return append([]string(nil), allowedTools...)
	}
	denied := make(map[string]struct{}, len(deniedTools))
	for _, name := range deniedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		denied[strings.ToLower(name)] = struct{}{}
	}
	filtered := make([]string, 0, len(allowedTools))
	for _, name := range allowedTools {
		if _, ok := denied[strings.ToLower(strings.TrimSpace(name))]; ok {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func normalizeModelOverride(model string) string {
	model = strings.TrimSpace(model)
	if strings.EqualFold(model, "inherit") {
		return ""
	}
	return model
}

func (t *taskTool) orchestrationEnabled() bool {
	cfg := t.runtimeConfig()
	return cfg != nil && cfg.SubagentOrchestration.Enabled
}

func (t *taskTool) runtimeConfig() *config.Config {
	if t != nil && t.profiles != nil && t.profiles.cfg != nil {
		return t.profiles.cfg
	}
	return config.Get()
}

func (t *taskTool) validateWriteSetRequirement(resolved taskAgentResolution, writeSet []string) error {
	if !t.orchestrationEnabled() {
		return nil
	}
	if !(resolved.Profile.CanWriteFiles || resolved.Profile.RequireWriteSet) {
		return nil
	}
	if len(writeSet) == 0 {
		return fmt.Errorf("write_set is required for writable worker profile %q when subagent orchestration is enabled", resolved.AgentType)
	}
	return nil
}

func (t *taskTool) validateNestedTaskCreate(ctx context.Context, parentSessionID string) error {
	if !t.orchestrationEnabled() {
		return nil
	}
	if parentSub := t.parentSubtaskForSession(parentSessionID); parentSub != nil {
		resolvedParent, err := t.resolveTaskAgent(parentSub.AgentType)
		if err != nil {
			return fmt.Errorf("failed to resolve parent subagent profile %q: %w", parentSub.AgentType, err)
		}
		if !resolvedParent.Profile.CanSpawnTask {
			return fmt.Errorf("agent profile %q is not allowed to spawn Task subtasks", parentSub.AgentType)
		}
	}

	cfg := t.runtimeConfig()
	if cfg == nil || cfg.SubagentOrchestration.MaxNestedDepth <= 0 || t.sessions == nil {
		return nil
	}
	depth, ok := t.sessionDepth(ctx, parentSessionID)
	if !ok {
		return nil
	}
	if depth+1 > cfg.SubagentOrchestration.MaxNestedDepth {
		return fmt.Errorf("max nested subagent depth exceeded: creating depth %d would exceed configured max %d", depth+1, cfg.SubagentOrchestration.MaxNestedDepth)
	}
	return nil
}

func (t *taskTool) parentSubtaskForSession(sessionID string) *task.SubtaskState {
	if t == nil || t.registry == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	for _, st := range t.registry.All() {
		sub, ok := st.(*task.SubtaskState)
		if !ok {
			continue
		}
		if sub.ChildSessionID == sessionID {
			return sub
		}
	}
	return nil
}

func (t *taskTool) sessionDepth(ctx context.Context, sessionID string) (int, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || t == nil || t.sessions == nil {
		return 0, false
	}
	seen := map[string]struct{}{}
	depth := 0
	for {
		if _, ok := seen[sessionID]; ok {
			return depth, true
		}
		seen[sessionID] = struct{}{}
		sess, err := t.sessions.Get(ctx, sessionID)
		if err != nil {
			return 0, false
		}
		parentID := strings.TrimSpace(sess.ParentSessionID)
		if parentID == "" {
			return depth, true
		}
		depth++
		sessionID = parentID
	}
}

func (t *taskTool) resolveResultMaxChars(explicit int, resolved taskAgentResolution) int {
	if !t.orchestrationEnabled() {
		return 0
	}
	if explicit > 0 {
		return explicit
	}
	if resolved.Profile.ResultMaxChars > 0 {
		return resolved.Profile.ResultMaxChars
	}
	cfg := t.runtimeConfig()
	if cfg != nil && cfg.SubagentOrchestration.DefaultResultMaxChars > 0 {
		return cfg.SubagentOrchestration.DefaultResultMaxChars
	}
	return 0
}

func applyResultCap(result string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return result, false
	}
	runes := []rune(result)
	if len(runes) <= maxChars {
		return result, false
	}
	return string(runes[:maxChars]), true
}

func profileForScheduler(resolved taskAgentResolution) SubagentProfile {
	return resolved.Profile
}

func subagentSessionMode(perms permission.Service, parentSessionID string, requested permission.Mode) permission.Mode {
	parentMode := permission.ModeDefault
	if perms != nil && strings.TrimSpace(parentSessionID) != "" {
		parentMode = perms.SessionMode(parentSessionID)
	}
	if parentMode == permission.ModePlan || requested == permission.ModePlan {
		return permission.ModePlan
	}
	return permission.ModeDefault
}

func (t *taskTool) getSubtaskForParent(ctx context.Context, taskID string) (*task.SubtaskState, string) {
	parentSessionID, _ := GetContextValues(ctx)
	if parentSessionID == "" {
		return nil, "no active session"
	}
	st, ok := t.registry.Get(taskID)
	if !ok {
		return nil, "task not found"
	}
	sub, ok := st.(*task.SubtaskState)
	if !ok {
		return nil, "task type mismatch"
	}
	if sub.ParentSessionID != parentSessionID {
		return nil, "task does not belong to current session"
	}
	return sub, ""
}

func (t *taskTool) markTaskNotified(taskID string) {
	if t.registry == nil {
		return
	}
	t.registry.MarkNotificationDelivered(taskID)
}

func (t *taskTool) setTaskRuntime(taskID string, rt *taskRuntime) bool {
	return beginTaskRuntime(taskID, rt)
}

func (t *taskTool) clearTaskRuntime(taskID string) {
	finishTaskRuntime(taskID)
}

func (t *taskTool) isTaskRunning(taskID string) bool {
	return isTaskRuntimeRunning(taskID)
}

func (t *taskTool) cancelTask(taskID string) bool {
	return cancelTaskRuntime(taskID)
}

func (t *taskTool) emitHook(ctx context.Context, event hooks.Event, sessionID string) error {
	if t.hookService == nil {
		return nil
	}
	return t.hookService.Run(ctx, event, hooks.Input{
		SessionID: sessionID,
		Timestamp: time.Now(),
	})
}

func normalizeTaskVerifyPolicy(policy, agentType string) (string, error) {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return defaultTaskVerifyPolicyForType(agentType), nil
	}
	switch policy {
	case "none", "required":
		return policy, nil
	default:
		return "", fmt.Errorf("unsupported verify_policy: %s", policy)
	}
}

func (r taskAgentResolution) normalizeVerifyPolicy(policy string) (string, error) {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		policy = strings.ToLower(strings.TrimSpace(r.VerifyPolicy))
	}
	if policy == "" {
		return defaultTaskVerifyPolicyForType(r.AgentType), nil
	}
	switch policy {
	case "none", "required":
		return policy, nil
	default:
		return "", fmt.Errorf("unsupported verify_policy: %s", policy)
	}
}

func isKnownBuiltInAgentType(agentType string) bool {
	_, err := mapBuiltInTaskAgentType(agentType)
	return err == nil
}

func taskVerifyStatusForPolicy(policy string) task.VerifyStatus {
	if strings.EqualFold(strings.TrimSpace(policy), "required") {
		return task.VerifyStatusPending
	}
	return task.VerifyStatusSkipped
}

func taskNeedsVerification(policy string) bool {
	return strings.EqualFold(strings.TrimSpace(policy), "required")
}

func buildTaskVerificationPrompt(sub *task.SubtaskState, prompt, result string) string {
	description := sub.Summary
	if strings.TrimSpace(description) == "" {
		description = sub.TaskMeta().Label
	}
	return fmt.Sprintf(`你是一个子任务验收员。请评审以下代理子任务的最终结果。

任务类型：%s
任务描述：%s
原始提示：
---
%s
---

最终结果：
---
%s
---

请独立验收并给出简短证据。最后一行必须是：
VERDICT: PASS|FAIL|PARTIAL

判定规则：
- PASS：满足要求，可通过验收。
- FAIL：不满足要求，或有关键问题。
- PARTIAL：部分满足，但存在阻断性缺口（例如只能读不能跑验证）。

输出格式：
1) 先写简短验收意见（1-6 行）
2) 最后一行严格输出：VERDICT: PASS|FAIL|PARTIAL`, sub.AgentType, description, prompt, result)
}

func runTaskVerification(ctx context.Context, permissions permission.Service, sessions session.Service, runAgent AgentRunner, sub *task.SubtaskState, prompt string, result string) (task.VerifyStatus, string, string, string) {
	if runAgent == nil || permissions == nil || sessions == nil {
		return task.VerifyStatusFailed, "", "", "verify worker dependencies are unavailable"
	}
	verifyPrompt := buildTaskVerificationPrompt(sub, prompt, result)
	verifySessionID := fmt.Sprintf("%s-verify-%d", sub.TaskMeta().ID, time.Now().UnixNano())
	verifySession, err := sessions.CreateTaskSession(ctx, verifySessionID, sub.ParentSessionID, "Verify "+sub.TaskMeta().Label)
	if err != nil {
		return task.VerifyStatusFailed, "", "", fmt.Sprintf("failed to create verify session: %v", err)
	}

	permissions.SetSessionMode(verifySession.ID, permission.ModePlan)
	permissions.AutoApproveSession(verifySession.ID)
	defer permissions.RemoveAutoApproveSession(verifySession.ID)

	verifyOutput, err := runAgent(ctx, config.AgentVerify, verifySession.ID, verifyPrompt, NewVerifyRegistry(permissions))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return task.VerifyStatusFailed, "", "", err.Error()
		}
		return task.VerifyStatusFailed, "", "", fmt.Sprintf("verify worker failed: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return task.VerifyStatusFailed, "", "", err.Error()
	}

	verdict, report, err := parseTaskVerifyVerdict(verifyOutput)
	if err != nil {
		return task.VerifyStatusFailed, "", "", err.Error()
	}
	if verdict != "PASS" {
		return task.VerifyStatusFailed, verdict, report, fmt.Sprintf("verify verdict %s did not pass required gate", strings.ToLower(verdict))
	}
	return task.VerifyStatusPassed, verdict, report, ""
}

func parseTaskVerifyVerdict(result string) (string, string, error) {
	lines := strings.Split(strings.TrimSpace(result), "\n")
	lastLine := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			lastLine = trimmed
			break
		}
	}
	const prefix = "VERDICT:"
	if !strings.HasPrefix(strings.ToUpper(lastLine), prefix) {
		return "", "", fmt.Errorf("verify output missing required final verdict line (VERDICT: PASS|FAIL|PARTIAL)")
	}
	verdict := strings.ToUpper(strings.TrimSpace(lastLine[len(prefix):]))
	switch verdict {
	case "PASS", "FAIL", "PARTIAL":
	default:
		return "", "", fmt.Errorf("verify output has unsupported verdict %q", verdict)
	}
	reportText := strings.TrimSpace(result)
	lastIndex := strings.LastIndex(reportText, lastLine)
	if lastIndex >= 0 {
		reportText = strings.TrimSpace(reportText[:lastIndex])
	}
	report := strings.TrimSpace(reportText)
	if report == "" {
		report = "VERDICT: " + verdict
	}
	return verdict, report, nil
}

func taskViewFromState(sub *task.SubtaskState) taskView {
	meta := sub.TaskMeta()
	return taskView{
		TaskID:          meta.ID,
		Status:          meta.Status,
		ParentSessionID: sub.ParentSessionID,
		ChildSessionID:  sub.ChildSessionID,
		AgentType:       sub.AgentType,
		Summary:         sub.Summary,
		Model:           sub.Model,
		VerifyPolicy:    sub.VerifyPolicy,
		VerifyStatus:    sub.VerifyStatus,
		VerifyVerdict:   sub.VerifyVerdict,
		VerifyResult:    sub.VerifyResult,
		VerifyError:     sub.VerifyError,
		Result:          sub.Result,
		LastError:       sub.LastError,
		ResultTruncated: sub.ResultTruncated,
		StartedAt:       meta.StartedAt,
		EndedAt:         meta.EndedAt,
	}
}

func isTerminalTaskStatus(status task.Status) bool {
	return status == task.StatusCompleted || status == task.StatusFailed || status == task.StatusCanceled
}

func newJSONResponse(v any) ToolResponse {
	data, _ := json.Marshal(v)
	return NewTextResponse(string(data))
}

func (t *taskTool) buildTaskReadView(ctx context.Context, sub *task.SubtaskState, includeTranscript bool) taskReadView {
	const maxTranscriptMessages = 12
	const maxTranscriptMessageChars = 2000

	view := taskReadView{taskView: taskViewFromState(sub)}
	if !includeTranscript {
		return view
	}
	msgs, err := t.messages.List(ctx, sub.ChildSessionID)
	if err != nil {
		if view.LastError == "" {
			view.LastError = err.Error()
		} else {
			view.LastError = view.LastError + " | " + err.Error()
		}
		return view
	}
	if len(msgs) > maxTranscriptMessages {
		msgs = msgs[len(msgs)-maxTranscriptMessages:]
		view.Truncated = true
	}
	for _, msg := range msgs {
		content := strings.TrimSpace(msg.Content().String())
		if content == "" {
			continue
		}
		if len([]rune(content)) > maxTranscriptMessageChars {
			content = string([]rune(content)[:maxTranscriptMessageChars]) + "\n[transcript message truncated]"
			view.Truncated = true
		}
		view.Transcript = append(view.Transcript, taskMessageView{
			Role:    string(msg.Role),
			Content: content,
		})
	}
	return view
}

func detachedTaskContext(parent context.Context) context.Context {
	ctx := context.Background()
	for _, key := range []any{
		WorkspaceDirContextKey,
		ResearchModeContextKey,
		ResearchToolProfileContextKey,
		ResearchRootSessionContextKey,
		ResearchWorkDirContextKey,
		ResearchContextContextKey,
		FileReadNotifierContextKey,
		FileToolUsageNotifierContextKey,
	} {
		if parent != nil {
			if value := parent.Value(key); value != nil {
				ctx = context.WithValue(ctx, key, value)
			}
		}
	}
	if ctx.Value(WorkspaceDirContextKey) == nil {
		ctx = context.WithValue(ctx, WorkspaceDirContextKey, config.WorkingDirectory())
	}
	return ctx
}
