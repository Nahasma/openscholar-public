package research

import (
	"strings"
	"time"
)

// NodeType 实验节点类型
type NodeType string

const (
	NodePreliminary    NodeType = "preliminary"
	NodeHyperparameter NodeType = "hyperparameter"
	NodeResearchAgenda NodeType = "research"
	NodeAblation       NodeType = "ablation"
	NodeReplication    NodeType = "replication"
	NodeAggregation    NodeType = "aggregation"
	NodeDebug          NodeType = "debug"

	// Generic (domain-neutral, used by non-ML domains)
	NodeBaseline            NodeType = "baseline"
	NodeVariant             NodeType = "variant"
	NodeSensitivityAnalysis NodeType = "sensitivity_analysis"
	NodeValidation          NodeType = "validation"
	NodePreprocessing       NodeType = "preprocessing"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeRunning   NodeStatus = "running"
	NodeCompleted NodeStatus = "completed"
	NodeBuggy     NodeStatus = "buggy"
	NodePruned    NodeStatus = "pruned"
)

// ExperimentNode 树搜索中的实验节点
type ExperimentNode struct {
	ID          string             `json:"id"`
	ParentID    string             `json:"parent_id"`
	PipelineID  string             `json:"pipeline_id"`
	Type        NodeType           `json:"type"`
	Status      NodeStatus         `json:"status"`
	Description string             `json:"description"`
	CodePath    string             `json:"code_path"`
	Metrics     map[string]float64 `json:"metrics"`
	ErrorLog    string             `json:"error_log"`
	Score       float64            `json:"score"`
	Depth       int                `json:"depth"`
	CreatedAt   int64              `json:"created_at"`

	// Phase 9: 外包编排扩展
	ContractPath   string   `json:"contract_path,omitempty"`
	ManifestPath   string   `json:"manifest_path,omitempty"`
	AssessmentPath string   `json:"assessment_path,omitempty"`
	ExecutionID    string   `json:"execution_id,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Artifacts      []string `json:"artifacts,omitempty"`
}

// NewExperimentNode 创建新的实验节点
func NewExperimentNode(pipelineID string, nodeType NodeType, description string) *ExperimentNode {
	return &ExperimentNode{
		PipelineID:  pipelineID,
		Type:        nodeType,
		Status:      NodePending,
		Description: description,
		Metrics:     make(map[string]float64),
		CreatedAt:   time.Now().Unix(),
	}
}

// IsLeaf 判断是否为叶节点
func (n *ExperimentNode) IsLeaf(allNodes []*ExperimentNode) bool {
	for _, other := range allNodes {
		if other.ParentID == n.ID {
			return false
		}
	}
	return true
}

// IsBuggy 判断节点是否有错误
func (n *ExperimentNode) IsBuggy() bool {
	return n.Status == NodeBuggy
}

// IsTerminal 判断节点是否处于终态
func (n *ExperimentNode) IsTerminal() bool {
	return n.Status == NodeCompleted || n.Status == NodeBuggy || n.Status == NodePruned
}

// NormalizeNodeType validates and normalizes a node type string.
// Known types are returned as-is. Unknown types are accepted but trimmed.
func NormalizeNodeType(s string) NodeType {
	s = strings.TrimSpace(s)
	if s == "" {
		return NodePreliminary
	}
	return NodeType(s)
}
