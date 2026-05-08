package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	agentcustom "github.com/openscholar/openscholar/internal/llm/agent/custom"
	"github.com/openscholar/openscholar/internal/permission"
)

const (
	subagentProfileVerifyNone     = "none"
	subagentProfileVerifyRequired = "required"
)

var subagentSafeReadOnlyTools = []string{"View", "Glob", "Grep"}

const (
	researchSearchAgentType         = "research"
	researchSearchWorkerTimeout     = 20 * time.Minute
	researchSearchWorkerResultChars = 12_000
	researchSearchWorkerMaxTurns    = 8
)

type SubagentProfile struct {
	ID                string
	AgentType         string
	AgentName         config.AgentName
	Config            *agentcustom.AgentConfig
	ModelTier         string
	Model             string
	AllowedTools      []string
	DeniedTools       []string
	ToolsExplicit     bool
	SessionMode       permission.Mode
	PromptPrefix      string
	Timeout           time.Duration
	MaxConcurrent     int
	DefaultBackground bool
	CanSpawnTask      bool
	CanWriteFiles     bool
	RequireWriteSet   bool
	VerifyPolicy      string
	ResultMaxChars    int
	MaxTurns          int
}

type SubagentProfileResolver struct {
	cfg *config.Config
}

func NewSubagentProfileResolver(cfg *config.Config) *SubagentProfileResolver {
	return &SubagentProfileResolver{cfg: cfg}
}

func (r *SubagentProfileResolver) orchestrationEnabled() bool {
	return r != nil && r.cfg != nil && r.cfg.SubagentOrchestration.Enabled
}

func (r *SubagentProfileResolver) Resolve(agentType string, customCfg *agentcustom.AgentConfig) (SubagentProfile, error) {
	agentType = strings.ToLower(strings.TrimSpace(agentType))
	if agentType == "" {
		agentType = "general"
	}

	if isReadingWorkerAgentType(agentType) {
		prof := SubagentProfile{
			ID:            readingWorkerAgentType,
			AgentType:     readingWorkerAgentType,
			AgentName:     config.AgentGeneral,
			AllowedTools:  append([]string(nil), baseReadingWorkerTools...),
			ToolsExplicit: true,
			SessionMode:   permission.ModeDefault,
			PromptPrefix:  readingWorkerPromptPrefix(baseReadingWorkerTools),
			Timeout:       readingWorkerTimeout,
			VerifyPolicy:  subagentProfileVerifyNone,
		}
		return r.applyConfig(prof, agentType)
	}

	if agentType == researchSearchAgentType {
		prof := SubagentProfile{
			ID:                researchSearchAgentType,
			AgentType:         researchSearchAgentType,
			AgentName:         config.AgentGeneral,
			SessionMode:       permission.ModeDefault,
			PromptPrefix:      researchSearchWorkerPromptPrefix(),
			Timeout:           researchSearchWorkerTimeout,
			VerifyPolicy:      subagentProfileVerifyNone,
			DefaultBackground: true,
			CanWriteFiles:     false,
			ResultMaxChars:    researchSearchWorkerResultChars,
			MaxTurns:          researchSearchWorkerMaxTurns,
		}
		return r.applyConfig(prof, agentType)
	}

	if mapped, err := mapBuiltInTaskAgentType(agentType); err == nil {
		prof := SubagentProfile{
			ID:            agentType,
			AgentType:     agentType,
			AgentName:     mapped,
			SessionMode:   sessionModeForBuiltInTaskAgentType(agentType),
			VerifyPolicy:  defaultTaskVerifyPolicyForType(agentType),
			CanSpawnTask:  canSpawnTaskBuiltInAgentType(agentType),
			CanWriteFiles: !isReadOnlyBuiltInAgentType(agentType),
		}
		return r.applyConfig(prof, agentType)
	}

	if customCfg == nil {
		return SubagentProfile{}, fmt.Errorf("unsupported agent_type: %s", agentType)
	}
	prof := SubagentProfile{
		ID:            strings.TrimSpace(customCfg.Name),
		AgentType:     strings.TrimSpace(customCfg.Name),
		AgentName:     config.AgentGeneral,
		Config:        customCfg,
		Model:         strings.TrimSpace(customCfg.Model),
		AllowedTools:  append([]string(nil), customCfg.Tools...),
		ToolsExplicit: customCfg.ToolsExplicit,
		SessionMode:   sessionModeForCustomTaskAgent(customCfg),
		VerifyPolicy:  subagentProfileVerifyNone,
		CanWriteFiles: true,
	}
	if r.orchestrationEnabled() && !customCfg.ToolsExplicit {
		prof.AllowedTools = append([]string(nil), subagentSafeReadOnlyTools...)
		prof.ToolsExplicit = true
		prof.SessionMode = permission.ModePlan
		prof.CanWriteFiles = false
	}
	return r.applyConfig(prof, agentType)
}

func (r *SubagentProfileResolver) applyConfig(prof SubagentProfile, requestedAgentType string) (SubagentProfile, error) {
	if !r.orchestrationEnabled() || r.cfg == nil || len(r.cfg.SubagentOrchestration.Profiles) == 0 {
		return prof, nil
	}
	override, ok := r.profileConfigFor(prof, requestedAgentType)
	if !ok {
		return prof, nil
	}
	if override.AgentType != "" {
		prof.AgentType = strings.ToLower(strings.TrimSpace(override.AgentType))
		if prof.AgentType == "" {
			prof.AgentType = strings.ToLower(strings.TrimSpace(prof.ID))
		}
		if override.AgentName == "" {
			if mapped, err := mapBuiltInTaskAgentType(prof.AgentType); err == nil {
				prof.AgentName = mapped
			}
		}
	}
	if override.AgentName != "" {
		agentName, ok := config.ParseAgentName(override.AgentName)
		if !ok {
			return SubagentProfile{}, fmt.Errorf("unsupported profile agent_name for %s: %s", prof.ID, override.AgentName)
		}
		prof.AgentName = agentName
	}
	if override.ModelTier != "" {
		prof.ModelTier = strings.TrimSpace(override.ModelTier)
	}
	if override.Model != "" {
		prof.Model = strings.TrimSpace(override.Model)
	}
	if len(override.AllowedTools) > 0 {
		prof.AllowedTools = cleanToolNames(override.AllowedTools)
		prof.ToolsExplicit = true
	}
	if len(override.DeniedTools) > 0 {
		prof.DeniedTools = cleanToolNames(override.DeniedTools)
	}
	if override.PermissionMode != "" {
		mode, err := parseSubagentPermissionMode(override.PermissionMode)
		if err != nil {
			return SubagentProfile{}, err
		}
		prof.SessionMode = mode
	}
	if override.MaxConcurrent > 0 {
		prof.MaxConcurrent = override.MaxConcurrent
	}
	if override.DefaultBackground {
		prof.DefaultBackground = true
	}
	if override.CanSpawnTask {
		prof.CanSpawnTask = true
	}
	if override.CanWriteFiles {
		prof.CanWriteFiles = true
	}
	if override.RequireWriteSet {
		prof.RequireWriteSet = true
	}
	if override.VerifyPolicy != "" {
		policy := strings.ToLower(strings.TrimSpace(override.VerifyPolicy))
		switch policy {
		case subagentProfileVerifyNone, subagentProfileVerifyRequired:
			prof.VerifyPolicy = policy
		default:
			return SubagentProfile{}, fmt.Errorf("unsupported profile verify_policy for %s: %s", prof.ID, override.VerifyPolicy)
		}
	}
	if override.ResultMaxChars > 0 {
		prof.ResultMaxChars = override.ResultMaxChars
	}
	if override.TimeoutSeconds > 0 {
		prof.Timeout = time.Duration(override.TimeoutSeconds) * time.Second
	}
	if override.MaxTurns > 0 {
		prof.MaxTurns = override.MaxTurns
	}
	prof.CanWriteFiles = prof.CanWriteFiles && toolListCanWrite(prof.AllowedTools, prof.DeniedTools, prof.ToolsExplicit)
	return prof, nil
}

func (r *SubagentProfileResolver) profileConfigFor(prof SubagentProfile, requestedAgentType string) (config.SubagentProfileConfig, bool) {
	profiles := r.cfg.SubagentOrchestration.Profiles
	keys := []string{
		strings.ToLower(strings.TrimSpace(requestedAgentType)),
		strings.ToLower(strings.TrimSpace(prof.ID)),
		strings.ToLower(strings.TrimSpace(prof.AgentType)),
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if cfg, ok := profiles[key]; ok {
			return cfg, true
		}
		for name, cfg := range profiles {
			if strings.EqualFold(strings.TrimSpace(name), key) {
				return cfg, true
			}
		}
	}
	return config.SubagentProfileConfig{}, false
}

func parseSubagentPermissionMode(mode string) (permission.Mode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		return permission.ModeDefault, nil
	case "plan":
		return permission.ModePlan, nil
	case "auto":
		return permission.ModeAuto, nil
	default:
		return "", fmt.Errorf("unsupported profile permission_mode: %s", mode)
	}
}

func cleanToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func toolListCanWrite(allowedTools, deniedTools []string, toolsExplicit bool) bool {
	denied := make(map[string]struct{}, len(deniedTools))
	for _, name := range deniedTools {
		denied[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	canUse := func(name string) bool {
		_, blocked := denied[strings.ToLower(name)]
		return !blocked
	}
	if !toolsExplicit || allowsAllTools(allowedTools) {
		return canUse("Edit") || canUse("Write") || canUse("Bash")
	}
	for _, name := range allowedTools {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "edit", "write", "bash":
			if canUse(name) {
				return true
			}
		}
	}
	return false
}

func researchSearchWorkerPromptPrefix() string {
	return `# Research Search Worker

You are an isolated literature/web search worker. Your job is to absorb noisy ScholarSearch/WebSearch/WebFetch/KBSearch results in this child context and return only compressed, decision-ready evidence to the parent.

Use the available search tools to run broad or multi-query exploration. Use model prior knowledge to seed an initial taxonomy and query plan before searching, then search only the gaps, recent claims, and citation-sensitive facts. When several independent topics or queries are needed, issue them in the same turn so they can run concurrently. Do not write files, download PDFs, mutate the workspace, spawn nested tasks, or return raw search dumps.

Return exactly these sections:
- scope: the topic, inclusion/exclusion rules, and search strategy
- queries_run: compact list of each ScholarSearch/WebSearch/KBSearch query and source
- candidate_papers: ranked table with title, authors, year, venue/source, citations when available, DOI/arXiv/URL, why it matters, and confidence
- evidence_table: claim -> supporting paper/source -> evidence note -> limitations
- convergence: what signals indicate enough coverage or what remains missing
- recommended_next_steps: no more than 5 concrete follow-ups

Prefer quality over volume. Deduplicate aggressively by title/DOI/arXiv/URL, group near-duplicates, and omit weak or off-topic hits unless they explain a gap. Stop when the ranked candidates and evidence table are sufficient for the parent to answer; do not keep searching for completeness alone.`
}

func mapBuiltInTaskAgentType(agentType string) (config.AgentName, error) {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "", "general", "default", "coder", "research":
		return config.AgentGeneral, nil
	case "explore":
		return config.AgentExplore, nil
	case "experiment":
		return config.AgentGeneral, nil
	case "leader":
		return config.AgentLeader, nil
	case "plan":
		return config.AgentPlan, nil
	case "verify":
		return config.AgentVerify, nil
	case "coordinator":
		return config.AgentCoordinator, nil
	default:
		return "", fmt.Errorf("unsupported agent_type: %s", agentType)
	}
}

func sessionModeForBuiltInTaskAgentType(agentType string) permission.Mode {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "explore", "plan", "verify", "coordinator":
		return permission.ModePlan
	default:
		return permission.ModeDefault
	}
}

func sessionModeForCustomTaskAgent(cfg *agentcustom.AgentConfig) permission.Mode {
	if cfg == nil {
		return permission.ModeDefault
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Permission)) {
	case "plan":
		return permission.ModePlan
	case "auto":
		return permission.ModeAuto
	default:
		return permission.ModeDefault
	}
}

func defaultTaskVerifyPolicyForType(agentType string) string {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "leader", "plan", "coordinator":
		return subagentProfileVerifyRequired
	default:
		return subagentProfileVerifyNone
	}
}

func isReadOnlyBuiltInAgentType(agentType string) bool {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "research", "explore", "plan", "verify", "coordinator":
		return true
	default:
		return false
	}
}

func canSpawnTaskBuiltInAgentType(agentType string) bool {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "leader", "coordinator":
		return true
	default:
		return false
	}
}
