package research

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/research/orchestrator"
)

// TreeSearchConfig 树搜索配置
type TreeSearchConfig struct {
	MaxDepth     int           // 最大深度，默认 4
	MaxNodes     int           // 最大节点数，默认 30
	NodeTimeout  time.Duration // 单节点超时，默认 7200s
	DebugRetries int           // debug 最大重试次数，默认 4
}

// DefaultTreeSearchConfig 返回默认配置
func DefaultTreeSearchConfig() TreeSearchConfig {
	return TreeSearchConfig{
		MaxDepth:     4,
		MaxNodes:     30,
		NodeTimeout:  7200 * time.Second,
		DebugRetries: 4,
	}
}

// NodeStore 节点存储接口
type NodeStore interface {
	CreateNode(node *ExperimentNode) error
	UpdateNode(node *ExperimentNode) error
	GetNodesByPipeline(pipelineID string) ([]*ExperimentNode, error)
	GetNodeByID(id string) (*ExperimentNode, error)
}

// TreeSearch 管理实验探索的树结构
type TreeSearch struct {
	pipelineID      string
	config          TreeSearchConfig
	store           NodeStore
	workDir         string
	worktreeManager *WorktreeManager
	nodeProgression []string // Phase 9.1: 深度→节点类型映射，从 DomainProfile 读取
	rootNodeType    string   // Phase 9.1: 根节点类型，从 DomainProfile 读取
}

// NewTreeSearch 创建树搜索引擎
func NewTreeSearch(pipelineID string, config TreeSearchConfig, store NodeStore, workDir string) *TreeSearch {
	return &TreeSearch{
		pipelineID: pipelineID,
		config:     config,
		store:      store,
		workDir:    workDir,
	}
}

// WithWorktreeManager 设置 WorktreeManager，启用 worktree 实验隔离
func (ts *TreeSearch) WithWorktreeManager(wm *WorktreeManager) {
	ts.worktreeManager = wm
}

// WithDomainProfile configures tree search with domain-specific node progression.
func (ts *TreeSearch) WithDomainProfile(progression []string, rootType string) {
	ts.nodeProgression = progression
	ts.rootNodeType = rootType
}

// CreateRoot 创建根节点
func (ts *TreeSearch) CreateRoot(description string) (*ExperimentNode, error) {
	rootType := NodePreliminary
	if ts.rootNodeType != "" {
		rootType = NodeType(ts.rootNodeType)
	}
	node := &ExperimentNode{
		ID:          generateNodeID(),
		PipelineID:  ts.pipelineID,
		Type:        rootType,
		Status:      NodePending,
		Description: description,
		Metrics:     make(map[string]float64),
		Depth:       0,
		CreatedAt:   time.Now().Unix(),
	}

	node.CodePath = ts.allocateNodePath(node.ID, "HEAD")

	if err := ts.store.CreateNode(node); err != nil {
		return nil, fmt.Errorf("save root node: %w", err)
	}
	return node, nil
}

// allocateNodePath 为节点分配代码路径；若 worktree 隔离启用则创建 worktree，否则 fallback 到普通目录
func (ts *TreeSearch) allocateNodePath(nodeID, baseRef string) string {
	cfg := config.Get()
	if ts.worktreeManager != nil && cfg != nil && cfg.Harness.WorktreeEnabled {
		wt, err := ts.worktreeManager.Create(nodeID, baseRef)
		if err == nil {
			return wt.Path
		}
		// fallback on error
	}
	// Default: create directory under workDir/code/tree/<nodeID>
	nodeDir := filepath.Join(ts.workDir, "code", "tree", nodeID)
	os.MkdirAll(nodeDir, 0755)
	return nodeDir
}

// Expand 选择最优叶节点并生成子节点
func (ts *TreeSearch) Expand() ([]*ExperimentNode, error) {
	allNodes, err := ts.store.GetNodesByPipeline(ts.pipelineID)
	if err != nil {
		return nil, err
	}

	if len(allNodes) >= ts.config.MaxNodes {
		return nil, fmt.Errorf("reached max nodes limit (%d)", ts.config.MaxNodes)
	}

	leaves := filterLeaves(allNodes)
	if len(leaves) == 0 {
		return nil, fmt.Errorf("no leaf nodes available")
	}

	buggy, healthy := partitionByStatus(leaves)
	var candidates []*ExperimentNode

	// 优先处理 buggy 节点（生成 debug 分支）
	for _, n := range buggy {
		if n.Depth >= ts.config.MaxDepth {
			continue
		}
		child := &ExperimentNode{
			ID:          generateNodeID(),
			ParentID:    n.ID,
			PipelineID:  ts.pipelineID,
			Type:        NodeDebug,
			Status:      NodePending,
			Description: fmt.Sprintf("修复: %s", truncateStr(n.ErrorLog, 200)),
			Metrics:     make(map[string]float64),
			Depth:       n.Depth + 1,
			CreatedAt:   time.Now().Unix(),
		}
		child.CodePath = ts.allocateNodePath(child.ID, "HEAD")
		if err := ts.store.CreateNode(child); err != nil {
			return nil, err
		}
		candidates = append(candidates, child)
	}

	// 在 healthy completed 节点中选最优展开
	completedHealthy := filterByStatus(healthy, NodeCompleted)
	if len(completedHealthy) > 0 {
		sort.Slice(completedHealthy, func(i, j int) bool {
			return completedHealthy[i].Score > completedHealthy[j].Score
		})
		best := completedHealthy[0]
		if best.Depth < ts.config.MaxDepth {
			nextType := nextNodeType(best.Depth, ts.nodeProgression)
			child := &ExperimentNode{
				ID:          generateNodeID(),
				ParentID:    best.ID,
				PipelineID:  ts.pipelineID,
				Type:        nextType,
				Status:      NodePending,
				Description: fmt.Sprintf("基于 %s 的改进实验", best.ID),
				Metrics:     make(map[string]float64),
				Depth:       best.Depth + 1,
				CreatedAt:   time.Now().Unix(),
			}
			child.CodePath = ts.allocateNodePath(child.ID, "HEAD")
			if err := ts.store.CreateNode(child); err != nil {
				return nil, err
			}
			candidates = append(candidates, child)
		}
	}

	return candidates, nil
}

// SelectBest 选择最终最优节点
// Phase 9: 优先使用 assessment.json 中的 score（如果存在）
func (ts *TreeSearch) SelectBest() (*ExperimentNode, error) {
	allNodes, err := ts.store.GetNodesByPipeline(ts.pipelineID)
	if err != nil {
		return nil, err
	}
	completed := filterByStatus(allNodes, NodeCompleted)
	if len(completed) == 0 {
		return nil, fmt.Errorf("no completed nodes")
	}

	// 构建 score map，优先 assessment score
	scores := make(map[string]float64, len(completed))
	for _, node := range completed {
		score := node.Score
		if node.AssessmentPath != "" {
			if a, aErr := orchestrator.LoadAssessment(ts.workDir, node.ID); aErr == nil {
				score = a.Score
			}
		}
		scores[node.ID] = score
	}

	sort.Slice(completed, func(i, j int) bool {
		return scores[completed[i].ID] > scores[completed[j].ID]
	})
	return completed[0], nil
}

// Prune 剪除低分分支
func (ts *TreeSearch) Prune(threshold float64) (int, error) {
	allNodes, err := ts.store.GetNodesByPipeline(ts.pipelineID)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, n := range allNodes {
		if n.Score < threshold && n.Status == NodeCompleted {
			n.Status = NodePruned
			if err := ts.store.UpdateNode(n); err != nil {
				return pruned, err
			}
			pruned++
		}
	}
	return pruned, nil
}

// GetTree 获取完整树结构
func (ts *TreeSearch) GetTree() ([]*ExperimentNode, error) {
	return ts.store.GetNodesByPipeline(ts.pipelineID)
}

// NodeCount 返回当前节点数
func (ts *TreeSearch) NodeCount() (int, error) {
	nodes, err := ts.store.GetNodesByPipeline(ts.pipelineID)
	if err != nil {
		return 0, err
	}
	return len(nodes), nil
}

// --- helpers ---

func nextNodeType(currentDepth int, progression []string) NodeType {
	if len(progression) == 0 {
		// ML default for backward compatibility
		progression = []string{"hyperparameter", "research", "ablation", "replication"}
	}
	if currentDepth < len(progression) {
		return NodeType(progression[currentDepth])
	}
	return NodeType(progression[len(progression)-1])
}

func filterLeaves(nodes []*ExperimentNode) []*ExperimentNode {
	childOf := make(map[string]bool)
	for _, n := range nodes {
		if n.ParentID != "" {
			childOf[n.ParentID] = true
		}
	}
	var leaves []*ExperimentNode
	for _, n := range nodes {
		if childOf[n.ID] {
			continue // has children → not a leaf
		}
		if n.Status == NodePruned {
			continue // pruned → skip
		}
		// Include: completed (expandable), buggy (debug-able), pending, running
		leaves = append(leaves, n)
	}
	return leaves
}

func filterByStatus(nodes []*ExperimentNode, status NodeStatus) []*ExperimentNode {
	var result []*ExperimentNode
	for _, n := range nodes {
		if n.Status == status {
			result = append(result, n)
		}
	}
	return result
}

func partitionByStatus(nodes []*ExperimentNode) (buggy, healthy []*ExperimentNode) {
	for _, n := range nodes {
		if n.IsBuggy() {
			buggy = append(buggy, n)
		} else {
			healthy = append(healthy, n)
		}
	}
	return
}

func generateNodeID() string {
	return fmt.Sprintf("node-%s", uuid.NewString()[:8])
}

// CleanupWorktrees 删除所有非最优节点的 worktrees，保留 bestNodeID 对应的 worktree
func (ts *TreeSearch) CleanupWorktrees(bestNodeID string) error {
	if ts.worktreeManager == nil {
		return nil
	}
	allNodes, err := ts.store.GetNodesByPipeline(ts.pipelineID)
	if err != nil {
		return err
	}
	for _, n := range allNodes {
		if n.ID == bestNodeID {
			continue
		}
		// Extract worktree name: worktree name == node.ID
		_ = ts.worktreeManager.Remove(n.ID, true)
	}
	return nil
}
