package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
)

// ResearchMode context key and helper
type researchModeContextKey string

const ResearchModeContextKey researchModeContextKey = "research_mode"

type researchToolProfileContextKey string

const ResearchToolProfileContextKey researchToolProfileContextKey = "research_tool_profile"

type ResearchToolProfile string

const (
	ResearchToolProfileLeader ResearchToolProfile = "leader"
	ResearchToolProfileWorker ResearchToolProfile = "worker"
)

func IsResearchMode(ctx context.Context) bool {
	v := ctx.Value(ResearchModeContextKey)
	if v == nil {
		return false
	}
	return v.(bool)
}

// Research context key — carries pipeline metadata for system prompt injection.
type researchContextKey string

const ResearchContextContextKey researchContextKey = "research_context"

// ResearchWorkDir context key — carries the workspace directory for sandbox validation.
type researchWorkDirContextKey string

const ResearchWorkDirContextKey researchWorkDirContextKey = "research_work_dir"

// ResearchWorkDir returns the research workspace directory from context.
func ResearchWorkDir(ctx context.Context) string {
	v := ctx.Value(ResearchWorkDirContextKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

// ValidateResearchPath checks that targetPath is within the research workspace.
// Relative paths are resolved against the workspace directory.
func ValidateResearchPath(ctx context.Context, targetPath string) error {
	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return fmt.Errorf("no research workspace configured")
	}
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(workDir, targetPath)
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	resolvedTarget := absTarget
	if r, err := filepath.EvalSymlinks(absTarget); err == nil {
		resolvedTarget = r
	} else {
		dir := filepath.Dir(absTarget)
		if rd, derr := filepath.EvalSymlinks(dir); derr == nil {
			resolvedTarget = filepath.Join(rd, filepath.Base(absTarget))
		}
	}
	absWork := filepath.Clean(workDir)
	if rw, err := filepath.EvalSymlinks(absWork); err == nil {
		absWork = rw
	}
	resolvedTarget = filepath.Clean(resolvedTarget)
	if resolvedTarget == absWork || strings.HasPrefix(resolvedTarget, absWork+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("路径 %s 超出研究工作空间 %s", resolvedTarget, absWork)
}

// Workspace context key — carries the working directory for universal sandbox validation.
type workspaceDirContextKey string

const WorkspaceDirContextKey workspaceDirContextKey = "workspace_dir"

// WorkspaceDir returns the workspace directory from context, falling back to config.WorkingDirectory().
func WorkspaceDir(ctx context.Context) string {
	v := ctx.Value(WorkspaceDirContextKey)
	if v == nil {
		return config.WorkingDirectory()
	}
	return v.(string)
}

// ValidateWorkspacePath checks that targetPath is within the workspace directory.
// Only validates if workspace was explicitly set via context key (not fallback).
// Core logic delegated to fileop.ValidateBoundary.
func ValidateWorkspacePath(ctx context.Context, targetPath string) error {
	// Only enforce if workspace was explicitly set in context
	v := ctx.Value(WorkspaceDirContextKey)
	if v == nil {
		return nil
	}
	wsDir := v.(string)
	if wsDir == "" {
		return nil
	}
	return fileop.ValidateBoundary(targetPath, wsDir)
}

// ResearchAllowedTools is the worker-side whitelist of tools available in
// research mode. Leader/default research sessions intentionally use a stricter
// subset via ResearchLeaderAllowedTools.
var ResearchAllowedTools = map[string]bool{
	"View": true, "Glob": true, "Grep": true,
	"Task": true, "KBQuery": true, "KBSearch": true,
	"KBList": true, "KBTree": true, "ScholarSearch": true,
	"SkillQuery": true, "ToolSearch": true, "AskUser": true,
	"Write": true, "Edit": true, // sandboxed to workspace
	"ResearchControl":  true, // legacy phase management wrapper
	"ResearchPipeline": true, // phase transitions (narrow)
	"ResearchTask":     true, // handoff task management (narrow)
	"ResearchMessage":  true, // inter-agent messaging (narrow)
	"CodeAgent":        true, // external coding agent for experiments
	"WebSearch":        true, // web search for current information
	"WebFetch":         true, // web page content retrieval
	"DocxPatch":        true, // DOCX template analysis and filling
	"DocxEdit":         true, // DOCX local editing (replace/insert/delete paragraphs)
	"DocxValidate":     true, // DOCX format and content validation
}

// ResearchLeaderAllowedTools is the fail-closed default tool set for research
// leaders and the main research-mode coder session. Workers still receive the
// broader ResearchAllowedTools profile through Task dispatch.
var ResearchLeaderAllowedTools = map[string]bool{
	"View": true, "Glob": true, "Grep": true,
	"Task": true, "KBQuery": true, "KBSearch": true,
	"KBList": true, "KBTree": true, "ScholarSearch": true,
	"SkillQuery": true, "ToolSearch": true, "AskUser": true,
	"ResearchControl":  true,
	"ResearchPipeline": true,
	"ResearchTask":     true,
	"ResearchMessage":  true,
	"WebSearch":        true,
	"WebFetch":         true,
	"DocxValidate":     true,
}

func researchToolProfileForAgentType(agentType string) ResearchToolProfile {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "general", "experiment":
		return ResearchToolProfileWorker
	default:
		return ResearchToolProfileLeader
	}
}

func researchToolProfile(ctx context.Context) ResearchToolProfile {
	if ctx != nil {
		if profile, ok := ctx.Value(ResearchToolProfileContextKey).(ResearchToolProfile); ok {
			if profile != "" {
				return profile
			}
		}
		if profile, ok := ctx.Value(ResearchToolProfileContextKey).(string); ok {
			switch strings.ToLower(strings.TrimSpace(profile)) {
			case string(ResearchToolProfileWorker):
				return ResearchToolProfileWorker
			case string(ResearchToolProfileLeader):
				return ResearchToolProfileLeader
			}
		}
	}
	return ResearchToolProfileLeader
}

func researchAllowedToolSetForProfile(profile ResearchToolProfile) map[string]bool {
	if profile == ResearchToolProfileWorker {
		return ResearchAllowedTools
	}
	return ResearchLeaderAllowedTools
}

// FilterResearchTools returns only the tools allowed in default/leader
// research-mode sessions.
func FilterResearchTools(allTools []BaseTool) []BaseTool {
	return FilterResearchToolsForProfile(allTools, ResearchToolProfileLeader)
}

// FilterResearchToolsForContext returns only the tools allowed for the current
// research tool profile stored on ctx.
func FilterResearchToolsForContext(ctx context.Context, allTools []BaseTool) []BaseTool {
	return FilterResearchToolsForProfile(allTools, researchToolProfile(ctx))
}

// FilterResearchToolsForProfile returns only the tools allowed for the given
// research tool profile.
func FilterResearchToolsForProfile(allTools []BaseTool, profile ResearchToolProfile) []BaseTool {
	allowed := researchAllowedToolSetForProfile(profile)
	var filtered []BaseTool
	for _, t := range allTools {
		if allowed[t.Info().Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// ResearchToolAllowed reports whether toolName is allowed for the research
// tool profile carried by ctx.
func ResearchToolAllowed(ctx context.Context, toolName string) bool {
	return researchAllowedToolSetForProfile(researchToolProfile(ctx))[toolName]
}

// ValidateResearchToolCall checks if a tool call is allowed in research mode.
// Returns nil if allowed or not in research mode; returns error if the tool is blocked.
func ValidateResearchToolCall(ctx context.Context, toolName string) error {
	if !IsResearchMode(ctx) {
		return nil
	}
	if ResearchToolAllowed(ctx, toolName) {
		return nil
	}
	profile := researchToolProfile(ctx)
	if profile == ResearchToolProfileWorker {
		return fmt.Errorf("tool '%s' is not available in research worker mode", toolName)
	}
	return fmt.Errorf("tool '%s' is not available in research leader mode; file writes must be delegated to workers via Task", toolName)
}

type ToolInfo struct {
	Name           string
	Description    string
	Parameters     map[string]any
	Required       []string
	MaxResultBytes int // 0=default 20KB, -1=no truncation
}

// PlanModeToolAllowed reports whether a tool should be visible while the
// main session is in plan mode. Permission checks remain the enforcement layer.
func PlanModeToolAllowed(toolName string) bool {
	switch toolName {
	case "View", "Glob", "Grep", "ToolSearch", "AskUser", "EnterPlanMode", "ExitPlanMode", "Write", "Edit":
		return true
	default:
		return false
	}
}

type toolResponseType string

type (
	sessionIDContextKey       string
	messageIDContextKey       string
	turnIDContextKey          string
	checkpointStoreContextKey string
)

const (
	ToolResponseTypeText toolResponseType = "text"

	SessionIDContextKey       sessionIDContextKey       = "session_id"
	MessageIDContextKey       messageIDContextKey       = "message_id"
	TurnIDContextKey          turnIDContextKey          = "turn_id"
	CheckpointStoreContextKey checkpointStoreContextKey = "checkpoint_store"
)

// FileReadNotifier is called by the View tool after successful file reads.
// Implemented by magicdoc.Service but defined here to avoid import cycles.
type FileReadNotifier interface {
	OnFileRead(sessionID, filePath, content string)
}

type fileReadNotifierContextKey string

// FileReadNotifierContextKey is the context key for FileReadNotifier.
const FileReadNotifierContextKey fileReadNotifierContextKey = "file_read_notifier"

// FileToolUsageEvent carries file-path touch signals from file tools.
type FileToolUsageEvent struct {
	ToolName  string
	Workspace string
	Paths     []string
}

// FileToolUsageNotifier is called by file tools (View/Edit/Glob/Grep)
// after successful operations.
type FileToolUsageNotifier interface {
	OnFileToolUsage(sessionID string, evt FileToolUsageEvent)
}

type fileToolUsageNotifierContextKey string

// FileToolUsageNotifierContextKey is the context key for FileToolUsageNotifier.
const FileToolUsageNotifierContextKey fileToolUsageNotifierContextKey = "file_tool_usage_notifier"

type readStateContextKey string
type readLimitsContextKey string

const (
	ReadStateContextKey  readStateContextKey  = "file_read_state"
	ReadLimitsContextKey readLimitsContextKey = "file_read_limits"
)

type ReadLimits struct {
	DefaultLines int
	MaxLineChars int
	MaxSizeBytes int64
}

// WriteCheckpointer is the interface that Edit/Write tools use to capture file snapshots.
// Implemented by agent.CheckpointStore but defined here to avoid import cycles.
type WriteCheckpointer interface {
	CaptureBeforeWrite(ctx context.Context, sessionID, turnID, filePath string) error
}

// captureCheckpointBeforeWrite extracts checkpoint store and IDs from context and captures.
// Fail-open: errors are logged but do not block the write operation.
func captureCheckpointBeforeWrite(ctx context.Context, filePath string) {
	cs, _ := ctx.Value(CheckpointStoreContextKey).(WriteCheckpointer)
	if cs == nil {
		return
	}
	sessionID, _ := GetContextValues(ctx)
	turnID, _ := ctx.Value(TurnIDContextKey).(string)
	if sessionID == "" || turnID == "" {
		return
	}
	if err := cs.CaptureBeforeWrite(ctx, sessionID, turnID, filePath); err != nil {
		slog.Warn("checkpoint capture failed (fail-open)", "file", filePath, "err", err)
	}
}

func notifyFileToolUsage(ctx context.Context, toolName string, paths []string) {
	notifier, ok := ctx.Value(FileToolUsageNotifierContextKey).(FileToolUsageNotifier)
	if !ok || notifier == nil {
		return
	}
	uniq := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}
	if len(uniq) == 0 {
		return
	}
	if toolName == "Glob" || toolName == "Grep" {
		const maxNotifyPaths = 32
		if len(uniq) > maxNotifyPaths {
			uniq = uniq[:maxNotifyPaths]
		}
	}
	sessionID, _ := GetContextValues(ctx)
	notifier.OnFileToolUsage(sessionID, FileToolUsageEvent{
		ToolName:  toolName,
		Workspace: WorkspaceDir(ctx),
		Paths:     uniq,
	})
}

type ToolResponse struct {
	Type     toolResponseType `json:"type"`
	Content  string           `json:"content"`
	Metadata string           `json:"metadata,omitempty"`
	IsError  bool             `json:"is_error"`
}

func NewTextResponse(content string) ToolResponse {
	return ToolResponse{Type: ToolResponseTypeText, Content: content}
}

func NewTextErrorResponse(content string) ToolResponse {
	return ToolResponse{Type: ToolResponseTypeText, Content: content, IsError: true}
}

func WithResponseMetadata(response ToolResponse, metadata any) ToolResponse {
	if metadata != nil {
		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			return response
		}
		response.Metadata = string(metadataBytes)
	}
	return response
}

type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

type BaseTool interface {
	Info() ToolInfo
	Run(ctx context.Context, params ToolCall) (ToolResponse, error)
}

// AvailabilityChecker is an optional interface that tools can implement
// to report their runtime availability status. Used by DeferredRegistry
// to inform the LLM before tool invocation (avoiding wasted iterations).
type AvailabilityChecker interface {
	// Available reports whether the tool can execute successfully.
	// Returns (available, reason). If not available, reason explains why.
	Available() (bool, string)
}

// extractJSONStringField attempts to extract a string field value from potentially
// malformed JSON input using regex. This is a fallback for weak models that produce
// garbled tool call parameters (BUG-7).
func extractJSONStringField(input, field string) string {
	// Try pattern: "field":"value" or "field": "value"
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*"((?:[^"\\]|\\.)*)`)
	matches := pattern.FindStringSubmatch(input)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func GetContextValues(ctx context.Context) (string, string) {
	sessionID := ctx.Value(SessionIDContextKey)
	messageID := ctx.Value(MessageIDContextKey)
	if sessionID == nil {
		return "", ""
	}
	if messageID == nil {
		return sessionID.(string), ""
	}
	return sessionID.(string), messageID.(string)
}

// FlexibleInt accepts both JSON number and numeric string ("10" or 10).
// This handles weak LLM models that pass integer parameters as quoted strings.
type FlexibleInt int

func (fi *FlexibleInt) UnmarshalJSON(data []byte) error {
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fi = FlexibleInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid numeric value: %q", s)
		}
		*fi = FlexibleInt(n)
		return nil
	}
	return fmt.Errorf("expected integer or numeric string, got %s", string(data))
}
