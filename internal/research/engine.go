package research

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research/orchestrator"
)

var (
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrNoPipeline     = errors.New("no pipeline found")
	ErrNoNextPhase    = errors.New("no next phase")
)

// Engine 管理研究流水线的生命周期
type Engine struct {
	store   *Store
	broker  *pubsub.Broker[ResearchEvent]
	journal *EventJournal   // 事件审计日志（Step 8）
	Handoff *HandoffManager // .handoff/ 任务和消息管理（Step 5/6，Create 后可用）
}

type AdvanceOptions struct {
	SkipCheckpoint bool
}

func NewEngine(store *Store, broker *pubsub.Broker[ResearchEvent]) *Engine {
	return &Engine{store: store, broker: broker}
}

// WithJournal 设置事件审计日志
func (e *Engine) WithJournal(j *EventJournal) {
	e.journal = j
}

// Create 创建 Pipeline 并初始化所有 Phase.
// workDir 为用户指定的工作目录；空字符串时自动基于主题生成。
func (e *Engine) Create(sessionID, topic, template string, mode AutomationMode, budgetLimit float64, workDir string) (*Pipeline, error) {
	return e.create(sessionID, topic, template, mode, budgetLimit, workDir, nil)
}

func (e *Engine) CreateWithPolicy(sessionID, topic, template string, mode AutomationMode, budgetLimit float64, workDir string, policy ResearchRunPolicy) (*Pipeline, error) {
	return e.create(sessionID, topic, template, mode, budgetLimit, workDir, &policy)
}

func (e *Engine) create(sessionID, topic, template string, mode AutomationMode, budgetLimit float64, workDir string, policy *ResearchRunPolicy) (*Pipeline, error) {
	phases, ok := Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s (available: empirical, aris_empirical, survey, theoretical, exploratory)", template)
	}
	var runPolicy ResearchRunPolicy
	if IsARISTemplate(template) {
		runPolicy = DefaultResearchRunPolicy()
		if policy != nil {
			runPolicy = NormalizeResearchRunPolicy(*policy)
		}
		if runPolicy.AutoProceed && !runPolicy.HumanCheckpoint && mode == ModeDefault {
			mode = ModeAuto
		}
		if runPolicy.HumanCheckpoint && mode == ModeAuto {
			mode = ModeDefault
		}
	}

	pid := uuid.NewString()
	if workDir == "" {
		slug := slugify(topic, "project")
		base := fmt.Sprintf("research-%s", slug)
		workDir = base
		// If directory already exists, append incrementing suffix
		for i := 2; ; i++ {
			if _, err := os.Stat(workDir); os.IsNotExist(err) {
				break
			}
			workDir = fmt.Sprintf("%s-%d", base, i)
		}
	}
	// Convert to absolute path
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("invalid work dir: %w", err)
	}
	workDir = absDir

	p := &Pipeline{
		ID:        pid,
		SessionID: sessionID,
		Topic:     topic,
		Template:  template,
		Mode:      mode,
		Status:    StatusPlanning,
		Budget:    Budget{Limit: budgetLimit},
		WorkDir:   workDir,
	}

	// 创建工作目录结构
	for _, dir := range []string{"paper", "paper/sections", "code", ".handoff", ".citations"} {
		if err := os.MkdirAll(filepath.Join(workDir, dir), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create work dir: %w", err)
		}
	}

	if err := e.store.CreatePipeline(p); err != nil {
		return nil, fmt.Errorf("failed to save pipeline: %w", err)
	}

	// 初始化 Handoff 管理器
	e.Handoff = NewHandoffManager(workDir)

	// 事件审计日志
	if e.journal != nil {
		_ = e.journal.Emit(p.ID, JournalPipelineCreated, map[string]any{
			"topic":    topic,
			"template": template,
			"mode":     mode,
		})
	}

	// Phase 9: 初始化实验目录
	if cfg := config.Get(); cfg != nil && cfg.Experiment.OrchestratorEnabled {
		_ = orchestrator.InitExperimentDirs(workDir)
	}
	if IsARISTemplate(template) {
		if err := InitARISWorkspace(workDir, runPolicy); err != nil {
			return nil, fmt.Errorf("init aris workspace: %w", err)
		}
	}

	// 创建所有 Phase
	for i, pt := range phases {
		ph := &Phase{
			ID:         uuid.NewString(),
			PipelineID: p.ID,
			Name:       pt.Name,
			Order:      i + 1,
			Status:     PhasePending,
			Checkpoint: pt.Checkpoint,
			MaxWorkers: pt.MaxWorkers,
		}
		if err := e.store.CreatePhase(ph); err != nil {
			return nil, fmt.Errorf("failed to save phase: %w", err)
		}
	}

	return p, nil
}

// Get 获取 Pipeline
func (e *Engine) Get(pipelineID string) (*Pipeline, error) {
	return e.store.GetPipeline(pipelineID)
}

// GetBySession 按会话查找最新 Pipeline
func (e *Engine) GetBySession(sessionID string) (*Pipeline, error) {
	return e.store.GetPipelineBySession(sessionID)
}

// GetPhases 获取 Pipeline 的所有 Phase
func (e *Engine) GetPhases(pipelineID string) ([]*Phase, error) {
	return e.store.ListPhases(pipelineID)
}

// StartPhase 标记 Phase 为 running
func (e *Engine) StartPhase(pipelineID, phaseID string) error {
	if err := e.store.UpdatePhaseStatus(phaseID, PhaseRunning); err != nil {
		return err
	}
	e.store.UpdatePipelineStatus(pipelineID, StatusRunning)
	if e.broker != nil {
		e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
			PipelineID: pipelineID, PhaseID: phaseID, Type: EventPhaseStarted,
		})
	}
	if e.journal != nil {
		_ = e.journal.Emit(pipelineID, JournalPhaseStarted, map[string]any{"phase_id": phaseID})
	}
	return nil
}

// Advance 标记当前阶段完成，判断检查点，自动推进或暂停
func (e *Engine) Advance(pipelineID string) error {
	return e.AdvanceWithOptions(pipelineID, AdvanceOptions{})
}

// AdvanceWithOptions completes the current phase and optionally skips the
// checkpoint gate when the caller has already collected explicit approval.
func (e *Engine) AdvanceWithOptions(pipelineID string, opts AdvanceOptions) error {
	p, err := e.store.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	phases, err := e.store.ListPhases(pipelineID)
	if err != nil {
		return err
	}

	current := currentRunningPhase(phases)
	if current == nil {
		return fmt.Errorf("no running phase found")
	}

	// 标记当前阶段完成
	if err := e.store.CompletePhase(current.ID); err != nil {
		return err
	}
	current.Status = PhaseCompleted

	if e.broker != nil {
		e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
			PipelineID: p.ID, PhaseID: current.ID, Type: EventPhaseCompleted,
		})
	}
	if e.journal != nil {
		_ = e.journal.Emit(p.ID, JournalPhaseCompleted, map[string]any{"phase_id": current.ID})
	}

	// 检查点判断
	if !opts.SkipCheckpoint && p.NeedsCheckpoint(current) {
		e.store.UpdatePipelineStatus(p.ID, StatusPaused)
		if e.broker != nil {
			e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
				PipelineID: p.ID, PhaseID: current.ID, Type: EventCheckpoint,
			})
		}
		return nil // 等待用户审批
	}

	// 自动进入下一阶段
	return e.startNextPhase(p, phases)
}

// FailPhase converges a running phase and its pipeline into a terminal failure
// state. If the phase has already advanced out of running, the call is a no-op.
func (e *Engine) FailPhase(pipelineID, phaseID string, failure PhaseFailureData) (bool, error) {
	updated, err := e.store.FailRunningPhase(phaseID)
	if err != nil {
		return false, err
	}
	if !updated {
		return false, nil
	}
	if err := e.store.UpdatePipelineStatus(pipelineID, StatusFailed); err != nil {
		return false, err
	}
	if e.broker != nil {
		e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
			PipelineID: pipelineID,
			PhaseID:    phaseID,
			Type:       EventPhaseFailed,
			Data:       failure,
		})
	}
	if e.journal != nil {
		_ = e.journal.Emit(pipelineID, JournalPhaseFailed, map[string]any{
			"phase_id":    phaseID,
			"reason":      failure.Reason,
			"error":       failure.Error,
			"recoverable": failure.Recoverable,
		})
	}
	return true, nil
}

// Approve 审批检查点，继续执行
func (e *Engine) Approve(pipelineID string) error {
	p, err := e.store.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	phases, err := e.store.ListPhases(pipelineID)
	if err != nil {
		return err
	}
	return e.startNextPhase(p, phases)
}

// Pause 暂停流水线
func (e *Engine) Pause(pipelineID string) error {
	return e.store.UpdatePipelineStatus(pipelineID, StatusPaused)
}

// Resume 恢复流水线
func (e *Engine) Resume(pipelineID string) error {
	return e.store.UpdatePipelineStatus(pipelineID, StatusRunning)
}

// SetMode 运行时切换自动化模式
func (e *Engine) SetMode(pipelineID string, mode AutomationMode) error {
	return e.store.UpdatePipelineMode(pipelineID, mode)
}

// UpdateBudget 更新预算花费，返回预警/超限事件
func (e *Engine) UpdateBudget(pipelineID string, spent float64) error {
	p, err := e.store.GetPipeline(pipelineID)
	if err != nil {
		return err
	}

	if err := e.store.UpdatePipelineBudget(pipelineID, spent); err != nil {
		return err
	}

	if p.Budget.Limit <= 0 {
		return nil
	}

	if spent >= p.Budget.Limit {
		e.store.UpdatePipelineStatus(pipelineID, StatusPaused)
		if e.broker != nil {
			e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
				PipelineID: pipelineID, Type: EventBudgetExceeded,
			})
		}
		return ErrBudgetExceeded
	}

	if spent >= p.Budget.Limit*0.8 {
		if e.broker != nil {
			e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
				PipelineID: pipelineID, Type: EventBudgetWarning,
			})
		}
		if e.journal != nil {
			_ = e.journal.Emit(pipelineID, JournalBudgetWarning, map[string]any{
				"spent": spent,
				"limit": p.Budget.Limit,
			})
		}
	}

	return nil
}

// AddPhase 动态添加阶段（仅 exploratory 模板可用）
func (e *Engine) AddPhase(pipelineID, name string, position, maxWorkers int, deliverables []string, checkpoint bool) (*Phase, error) {
	pipeline, err := e.Get(pipelineID)
	if err != nil {
		return nil, err
	}

	if pipeline.Template != "exploratory" {
		return nil, fmt.Errorf("add_phase only supported for exploratory template")
	}

	phases, err := e.GetPhases(pipelineID)
	if err != nil {
		return nil, err
	}

	if len(phases) >= 8 {
		return nil, fmt.Errorf("maximum 8 phases allowed")
	}

	// 调整后续 Phase 的 order
	for _, p := range phases {
		if p.Order >= position {
			p.Order++
			e.store.UpdatePhaseOrder(p.ID, p.Order)
		}
	}

	phase := &Phase{
		ID:         uuid.NewString(),
		PipelineID: pipelineID,
		Name:       name,
		Order:      position,
		Status:     PhasePending,
		Checkpoint: checkpoint,
		MaxWorkers: maxWorkers,
	}

	if err := e.store.CreatePhase(phase); err != nil {
		return nil, err
	}

	if e.broker != nil {
		e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
			PipelineID: pipelineID, Type: EventPhaseStarted,
		})
	}

	return phase, nil
}

// StartTreeSearch 在实验阶段启动树搜索
func (e *Engine) StartTreeSearch(pipelineID string, nodeStore NodeStore) (*TreeSearch, error) {
	pipeline, err := e.Get(pipelineID)
	if err != nil {
		return nil, err
	}

	config := DefaultTreeSearchConfig()
	ts := NewTreeSearch(pipelineID, config, nodeStore, pipeline.WorkDir)
	return ts, nil
}

// --- helpers ---

func (e *Engine) startNextPhase(p *Pipeline, phases []*Phase) error {
	next := nextPendingPhase(phases)
	if next == nil {
		// 所有阶段完成
		e.store.UpdatePipelineStatus(p.ID, StatusCompleted)
		return nil
	}
	next.Status = PhaseRunning
	next.CreatedAt = time.Now().Unix()
	e.store.UpdatePhaseStatus(next.ID, PhaseRunning)
	e.store.UpdatePipelineStatus(p.ID, StatusRunning)
	if e.broker != nil {
		e.broker.Publish(pubsub.CreatedEvent, ResearchEvent{
			PipelineID: p.ID, PhaseID: next.ID, Type: EventPhaseStarted,
		})
	}
	return nil
}

func currentRunningPhase(phases []*Phase) *Phase {
	for _, ph := range phases {
		if ph.Status == PhaseRunning {
			return ph
		}
	}
	return nil
}

func nextPendingPhase(phases []*Phase) *Phase {
	for _, ph := range phases {
		if ph.Status == PhasePending {
			return ph
		}
	}
	return nil
}

var nonSlugSafe = regexp.MustCompile(`[^\p{L}\p{N}-]+`)

// slugify turns a topic string into a filesystem-safe directory name.
// Keeps Unicode letters (including CJK) and digits; replaces the rest with hyphens.
// Falls back to the provided fallback if the result is empty.
func slugify(topic, fallback string) string {
	s := strings.TrimSpace(topic)
	s = strings.ToLower(s)
	s = nonSlugSafe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len([]rune(s)) > 30 {
		s = string([]rune(s)[:30])
	}
	s = strings.TrimRight(s, "-")
	if s == "" {
		return fallback
	}
	return s
}

// ValidateDeliverables checks that the expected deliverable files exist for a phase.
// Returns nil if all deliverables are present, or an error listing missing ones.
func ValidateDeliverables(workDir, templateName string, phaseOrder int) error {
	phases, ok := Templates[templateName]
	if !ok || phaseOrder < 1 || phaseOrder > len(phases) {
		return nil // unknown template/phase — skip validation
	}
	pt := phases[phaseOrder-1]
	if len(pt.Deliverables) == 0 {
		return nil
	}

	var missing []string
	for _, pattern := range pt.Deliverables {
		absPattern := filepath.Join(workDir, pattern)
		matches, err := filepath.Glob(absPattern)
		if err != nil || len(matches) == 0 {
			missing = append(missing, pattern)
			continue
		}
		// Check at least one match is non-empty
		hasContent := false
		for _, m := range matches {
			info, err := os.Stat(m)
			if err == nil && info.Size() > 0 {
				hasContent = true
				break
			}
		}
		if !hasContent {
			missing = append(missing, pattern+" (empty)")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("阶段交付物缺失，无法推进: %s", strings.Join(missing, ", "))
	}
	return nil
}
