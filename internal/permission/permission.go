package permission

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/pubsub"
)

var ErrorPermissionDenied = errors.New("permission denied")

type CreatePermissionRequest struct {
	SessionID   string `json:"session_id"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Params      any    `json:"params"`
	Path        string `json:"path"`
	Mode        Mode   `json:"mode,omitempty"` // current permission mode (injected by agent)
}

type PermissionRequest struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	ToolName     string `json:"tool_name"`
	Description  string `json:"description"`
	Action       string `json:"action"`
	Params       any    `json:"params"`
	Path         string `json:"path"`
	OriginalPath string `json:"original_path,omitempty"` // original file path before dir normalization
}

type Service interface {
	pubsub.Subscriber[PermissionRequest]
	Grant(permission PermissionRequest)
	GrantPersistent(permission PermissionRequest)
	GrantAlways(permission PermissionRequest)
	Deny(permission PermissionRequest)
	Request(opts CreatePermissionRequest) bool
	RequestDecision(opts CreatePermissionRequest) DecisionResult
	RequestWithMode(opts CreatePermissionRequest, mode Mode) bool
	AutoApproveSession(sessionID string)
	RemoveAutoApproveSession(sessionID string)
	RequirePromptSession(sessionID string)
	RemoveRequirePromptSession(sessionID string)
	SetSessionMode(sessionID string, mode Mode)
	SessionMode(sessionID string) Mode
	EnterPlanMode(sessionID string)
	ExitPlanMode(sessionID string, target Mode)
	TransitionSessionMode(sessionID string, to Mode)
	SessionModeState(sessionID string) ModeState
	SetPlanFileResolver(resolver PlanFileResolver)
	DenialTracker() *DenialTracker
}

// fileTools are the tools that operate on file paths (for workspace boundary checks).
var fileTools = map[string]bool{
	"Edit": true, "Write": true, "View": true, "Glob": true, "Grep": true,
}

type permissionService struct {
	*pubsub.Broker[PermissionRequest]
	mu sync.RWMutex

	queries             db.Querier // nil if DB not available
	sessionPermissions  []PermissionRequest
	pendingRequests     sync.Map
	autoApproveSessions []string
	promptSessions      []string
	workspaceDir        string // workspace directory for boundary checks and rule matching
	rules               []Rule // combined deny + allow rules
	denialTracker       *DenialTracker
	sessionModes        map[string]Mode // sessionID → current mode
	prePlanModes        map[string]Mode // sessionID → pre-plan mode
	planFileResolver    PlanFileResolver
}

// NewPermissionService creates a permission service without DB persistence (legacy).
func NewPermissionService() Service {
	return &permissionService{
		Broker:             pubsub.NewBroker[PermissionRequest](),
		sessionPermissions: make([]PermissionRequest, 0),
		denialTracker:      NewDenialTracker(),
		sessionModes:       make(map[string]Mode),
		prePlanModes:       make(map[string]Mode),
	}
}

// NewPermissionServiceWithDB creates a permission service with DB persistence and rule engine.
func NewPermissionServiceWithDB(queries db.Querier, workspaceDir string) Service {
	rules := append(DefaultDenyRules(), DefaultAskRules()...)
	rules = append(rules, DefaultAllowRules()...)
	return &permissionService{
		Broker:             pubsub.NewBroker[PermissionRequest](),
		queries:            queries,
		sessionPermissions: make([]PermissionRequest, 0),
		workspaceDir:       workspaceDir,
		rules:              rules,
		denialTracker:      NewDenialTracker(),
		sessionModes:       make(map[string]Mode),
		prePlanModes:       make(map[string]Mode),
	}
}

func (s *permissionService) Grant(permission PermissionRequest) {
	if respCh, ok := s.pendingRequests.Load(permission.ID); ok {
		respCh.(chan bool) <- true
	}
	s.denialTracker.RecordGrant(permission.SessionID)
}

func (s *permissionService) GrantPersistent(permission PermissionRequest) {
	if respCh, ok := s.pendingRequests.Load(permission.ID); ok {
		respCh.(chan bool) <- true
	}
	s.mu.Lock()
	s.sessionPermissions = append(s.sessionPermissions, permission)
	s.mu.Unlock()
	s.denialTracker.RecordGrant(permission.SessionID)
}

// GrantAlways grants the permission and persists it to the DB for future sessions.
func (s *permissionService) GrantAlways(permission PermissionRequest) {
	// Release the waiting channel
	if respCh, ok := s.pendingRequests.Load(permission.ID); ok {
		respCh.(chan bool) <- true
	}

	// Also add to session cache
	s.mu.Lock()
	s.sessionPermissions = append(s.sessionPermissions, permission)
	s.mu.Unlock()

	// Persist to DB
	if s.queries != nil {
		s.queries.CreatePermissionRule(context.Background(), db.CreatePermissionRuleParams{
			ID:       uuid.New().String(),
			ToolName: permission.ToolName,
			Action:   permission.Action,
			Path:     permission.Path,
			Decision: "allow",
			Scope:    "always",
		})
	}
	s.denialTracker.RecordGrant(permission.SessionID)
}

func (s *permissionService) Deny(permission PermissionRequest) {
	if respCh, ok := s.pendingRequests.Load(permission.ID); ok {
		respCh.(chan bool) <- false
	}
	// Track denial for downgrade detection using original file path (not dir)
	target := permission.OriginalPath
	if target == "" {
		target = permission.Path
	}
	s.denialTracker.RecordDenial(permission.ToolName, permission.Description, target, permission.SessionID, SourceUser, ReasonUserDenied)
}

// DenialTracker returns the denial tracker for external access (e.g., consecutive denial count).
func (s *permissionService) DenialTracker() *DenialTracker {
	return s.denialTracker
}

// RequestWithMode evaluates a permission request under a specific mode.
// Delegates to Request with mode set on the opts.
func (s *permissionService) RequestWithMode(opts CreatePermissionRequest, mode Mode) bool {
	opts.Mode = mode
	return s.Request(opts)
}

// SetSessionMode sets the permission mode for a session.
func (s *permissionService) SetSessionMode(sessionID string, mode Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionModes[sessionID] = mode
}

// SessionMode returns the current mode for a session.
func (s *permissionService) SessionMode(sessionID string) Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionModes[sessionID]
}

func (s *permissionService) SetPlanFileResolver(resolver PlanFileResolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planFileResolver = resolver
}

func (s *permissionService) SessionModeState(sessionID string) ModeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ModeState{
		Mode:        s.sessionModes[sessionID],
		PrePlanMode: s.prePlanModes[sessionID],
	}
}

func (s *permissionService) EnterPlanMode(sessionID string) {
	s.mu.Lock()
	current := s.sessionModes[sessionID]
	if current != ModePlan {
		s.prePlanModes[sessionID] = current
	}
	s.sessionModes[sessionID] = ModePlan
	s.mu.Unlock()
	s.RemoveAutoApproveSession(sessionID)
}

func (s *permissionService) ExitPlanMode(sessionID string, target Mode) {
	s.mu.Lock()
	if target == ModeRestore {
		target = s.prePlanModes[sessionID]
	}
	if target == ModePlan || target == "" {
		target = ModeDefault
	}
	s.sessionModes[sessionID] = target
	delete(s.prePlanModes, sessionID)
	s.mu.Unlock()

	if target == ModeAuto || target == ModeResearch {
		s.AutoApproveSession(sessionID)
		return
	}
	s.RemoveAutoApproveSession(sessionID)
}

func (s *permissionService) TransitionSessionMode(sessionID string, to Mode) {
	if to == ModePlan {
		s.EnterPlanMode(sessionID)
		return
	}
	if s.SessionMode(sessionID) == ModePlan {
		s.ExitPlanMode(sessionID, to)
		return
	}
	s.SetSessionMode(sessionID, to)
	if to == ModeAuto || to == ModeResearch {
		s.AutoApproveSession(sessionID)
		return
	}
	s.RemoveAutoApproveSession(sessionID)
}

// Request is a backward-compatible shim; all logic lives in RequestDecision.
func (s *permissionService) Request(opts CreatePermissionRequest) bool {
	return s.RequestDecision(opts).Allowed
}

// RequestDecision evaluates the permission request and returns a structured DecisionResult.
func (s *permissionService) RequestDecision(opts CreatePermissionRequest) DecisionResult {
	// Auto-inject mode from session if not explicitly set
	if opts.Mode == ModeDefault && opts.SessionID != "" {
		s.mu.RLock()
		m, ok := s.sessionModes[opts.SessionID]
		s.mu.RUnlock()
		if ok {
			opts.Mode = m
		}
	}

	// Step 0a: Plan mode enforcement — fail closed before auto approvals.
	// Bash stores the command in Description; structured tools use Action.
	if IsReadOnlyMode(opts.Mode) {
		writeLikeAction := opts.Action
		if opts.ToolName == "Bash" {
			writeLikeAction = opts.Description
		}
		if opts.ToolName == "Write" || opts.ToolName == "Edit" {
			if !s.isAllowedPlanFileWrite(opts) {
				return denyPlanMode(opts.ToolName, opts.Description)
			}
			return allowAuto(SourceMode)
		} else if IsWriteLikeTool(opts.ToolName, writeLikeAction) || isSideEffectAction(opts.Action) {
			return denyPlanMode(opts.ToolName, opts.Description)
		}
	}

	// Step 0b: Downgrade detection — block bypass of previously denied requests
	// For Bash, extract write target from command string; for file tools, use Path
	denialTarget := opts.Path
	if opts.ToolName == "Bash" {
		denialTarget = opts.Description // command string
	}
	if isDown, reason := s.denialTracker.IsDowngrade(opts.ToolName, opts.Description, denialTarget, opts.SessionID); isDown {
		return denyDowngrade(reason)
	}

	// Step 0c: Path security pre-validation (file tools only)
	// resolvedPath stores the symlink-resolved path for consistent workspace checks later.
	resolvedPath := opts.Path
	if fileTools[opts.ToolName] && opts.Path != "" {
		writeOp := isWriteToolOp(opts.ToolName)
		if err := validatePathSecurity(opts.Path, writeOp); err != nil {
			return denyPathSecurity()
		}
		// 所有文件操作：解析符号链接并验证目标仍在工作区（防止 symlink 逃逸读/写）
		if s.workspaceDir != "" {
			rp, err := resolveAndValidatePath(opts.Path, s.workspaceDir)
			if err != nil {
				return denyPathSecurity()
			}
			resolvedPath = rp
		}
	}

	// Step 0d: Dangerous removal target detection (Bash only)
	if opts.ToolName == "Bash" && isDangerousRemovalTarget(opts.Description) {
		return denyDangerousTarget()
	}

	// Determine the target for rule evaluation:
	// - File tools: workspace-relative path
	// - Bash: the command string (stored in Description)
	target := s.evaluationTarget(opts)

	// Step 1: Rule evaluation — deny/ask checked BEFORE autoApprove.
	var ruleDecision Decision
	if len(s.rules) > 0 {
		ruleDecision = Evaluate(s.rules, opts.ToolName, target)
		if ruleDecision == Deny {
			result := denyRule()
			result.RuleDecision = Deny
			return result
		}
	}

	// Step 1.5: Auto-mode fallback — revert to prompting after too many denials
	// Step 2: Auto-approve sessions (SubAgent mode, research auto mode)
	// Ask-ruled commands (dangerous patterns) skip auto-approve → go to TUI confirmation.
	s.mu.RLock()
	autoApproved := slices.Contains(s.autoApproveSessions, opts.SessionID)
	promptRequired := slices.Contains(s.promptSessions, opts.SessionID) && isPromptRequiredOperation(opts)
	s.mu.RUnlock()
	if ruleDecision != Ask &&
		!s.denialTracker.ShouldFallbackToPrompting(opts.SessionID) &&
		autoApproved &&
		!promptRequired {
		return allowAuto(SourceAuto)
	}

	dir := filepath.Dir(opts.Path)
	if dir == "." {
		dir = config.WorkingDirectory()
	}

	perm := PermissionRequest{
		ID:           uuid.New().String(),
		Path:         dir,
		OriginalPath: opts.Path, // preserve original path for denial tracking
		SessionID:    opts.SessionID,
		ToolName:     opts.ToolName,
		Description:  opts.Description,
		Action:       opts.Action,
		Params:       opts.Params,
	}

	// Step 3: Check in-memory session permissions (directory-level matching)
	s.mu.RLock()
	for _, p := range s.sessionPermissions {
		if p.ToolName == perm.ToolName && p.Action == perm.Action &&
			p.SessionID == perm.SessionID && isSubPath(perm.Path, p.Path) {
			s.mu.RUnlock()
			return allowAuto(SourceAuto)
		}
	}
	s.mu.RUnlock()

	// Step 4: Check DB for persistent "always" rules (directory-level matching)
	if s.queries != nil {
		dbRules, err := s.queries.ListPermissionRules(context.Background())
		if err == nil {
			for _, rule := range dbRules {
				if rule.ToolName == perm.ToolName && rule.Action == perm.Action &&
					rule.Decision == "allow" && isSubPath(perm.Path, rule.Path) {
					return allowAuto(SourceAuto)
				}
			}
		}
	}

	// Step 5: For file tools, check workspace boundary using resolved path.
	// Paths outside workspace skip built-in allow rules → go straight to ask.
	isFileOp := fileTools[opts.ToolName]
	outsideWorkspace := false
	if isFileOp && s.workspaceDir != "" && resolvedPath != "" {
		absPath, err := filepath.Abs(resolvedPath)
		if err == nil {
			absWork := filepath.Clean(s.workspaceDir)
			// 也解析工作区路径的符号链接，保持一致性
			if resolvedWork, e := filepath.EvalSymlinks(absWork); e == nil {
				absWork = resolvedWork
			}
			if absPath != absWork && !strings.HasPrefix(absPath, absWork+string(filepath.Separator)) {
				outsideWorkspace = true
			}
		}
	}

	// Step 6: Built-in allow rules (only for paths inside workspace)
	if !outsideWorkspace && len(s.rules) > 0 {
		decision := Evaluate(s.rules, opts.ToolName, target)
		if decision == Allow && !promptRequired {
			return allowRule()
		}
	}

	// Step 7: No match → ask user via TUI dialog
	respCh := make(chan bool, 1)
	s.pendingRequests.Store(perm.ID, respCh)
	defer s.pendingRequests.Delete(perm.ID)

	s.Publish(pubsub.CreatedEvent, perm)

	granted := <-respCh
	if granted {
		return allowUser(true)
	}
	return denyUser()
}

// evaluationTarget returns the appropriate target string for rule evaluation.
func (s *permissionService) evaluationTarget(opts CreatePermissionRequest) string {
	if opts.ToolName == "Bash" {
		// For Bash, the command is stored in Description
		return opts.Description
	}
	// For file tools, convert to workspace-relative path
	if opts.Path != "" && s.workspaceDir != "" {
		return MakeRelativePath(opts.Path, s.workspaceDir)
	}
	return opts.Path
}

func (s *permissionService) AutoApproveSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.autoApproveSessions, sessionID) {
		return
	}
	s.autoApproveSessions = append(s.autoApproveSessions, sessionID)
}

func (s *permissionService) RemoveAutoApproveSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.autoApproveSessions[:0]
	for _, id := range s.autoApproveSessions {
		if id != sessionID {
			next = append(next, id)
		}
	}
	s.autoApproveSessions = next
}

// RequirePromptSession forces side-effecting operations in a session to ask,
// even when broad built-in allow rules or auto-approve mode would otherwise
// approve them. Read-only requests can still use normal allow rules.
func (s *permissionService) RequirePromptSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.promptSessions, sessionID) {
		return
	}
	s.promptSessions = append(s.promptSessions, sessionID)
}

func (s *permissionService) RemoveRequirePromptSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.promptSessions[:0]
	for _, id := range s.promptSessions {
		if id != sessionID {
			next = append(next, id)
		}
	}
	s.promptSessions = next
}

func isPromptRequiredOperation(opts CreatePermissionRequest) bool {
	action := opts.Action
	if opts.ToolName == "Bash" {
		action = opts.Description
	}
	return IsWriteLikeTool(opts.ToolName, action) || isSideEffectAction(opts.Action)
}

// isSubPath checks if child path is under (or equal to) the parent directory.
func isSubPath(child, parent string) bool {
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

func (s *permissionService) isAllowedPlanFileWrite(opts CreatePermissionRequest) bool {
	if opts.SessionID == "" || strings.TrimSpace(opts.Path) == "" {
		return false
	}
	s.mu.RLock()
	resolver := s.planFileResolver
	s.mu.RUnlock()
	if resolver == nil {
		return false
	}
	return resolver.IsPlanFile(context.Background(), opts.SessionID, opts.Path)
}
