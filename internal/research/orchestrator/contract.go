package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
)

const contractFileName = "contract.json"

// ContractVersion is the current version of the TaskContract schema.
const ContractVersion = "1.0"

// TaskContract is the authoritative specification written by the Orchestrator
// and read by the Worker Agent for a single experiment node.
type TaskContract struct {
	ContractVersion string          `json:"contract_version"`
	PipelineID      string          `json:"pipeline_id"`
	NodeID          string          `json:"node_id"`
	NodeType        string          `json:"node_type"`
	Goal            string          `json:"goal"`
	Background      Background      `json:"background"`
	Scope           Scope           `json:"scope"`
	Deliverables    []string        `json:"deliverables"`
	AcceptCriteria  []string        `json:"accept_criteria"`
	Verification    []string             `json:"verification,omitempty"`
	RetryPolicy     RetryPolicy          `json:"retry_policy"`
	Protocol        config.DomainProfile `json:"protocol"`
	CreatedAt       time.Time            `json:"created_at"`
}

// Background holds context inherited from the parent node.
type Background struct {
	ParentNodeID   string            `json:"parent_node_id,omitempty"`
	Hypothesis     string            `json:"hypothesis,omitempty"`
	ParentMetrics  map[string]float64 `json:"parent_metrics,omitempty"`
	ParentSummary  string            `json:"parent_summary,omitempty"`
}

// Scope defines the filesystem boundaries the Worker Agent may access.
type Scope struct {
	AllowedWriteRoots []string `json:"allowed_write_roots"`
	ReadRoots         []string `json:"read_roots"`
	ForbiddenPaths    []string `json:"forbidden_paths,omitempty"`
}

// RetryPolicy controls how many retries are allowed for a node execution.
type RetryPolicy struct {
	MaxAgentReplans      int `json:"max_agent_replans"`
	MaxExecutionRetries  int `json:"max_execution_retries"`
	TimeoutSec           int `json:"timeout_sec"`
}

// NewContract creates a TaskContract from the provided parameters.
// workDir is the research pipeline root directory used to derive Scope defaults.
func NewContract(
	pipelineID, nodeID, nodeType, goal, hypothesis string,
	parentNodeID string, parentMetrics map[string]float64,
	workDir string, cfg config.ExperimentConfig,
) *TaskContract {
	profile := ResolveDomain(cfg.Domain, cfg.DomainOverride)
	nodeDir := filepath.Join("experiments", "nodes", nodeID)
	return &TaskContract{
		ContractVersion: ContractVersion,
		PipelineID:      pipelineID,
		NodeID:          nodeID,
		NodeType:        nodeType,
		Goal:            goal,
		Background: Background{
			ParentNodeID:  parentNodeID,
			Hypothesis:    hypothesis,
			ParentMetrics: parentMetrics,
		},
		Scope: Scope{
			AllowedWriteRoots: []string{
				filepath.Join(nodeDir, "work"),
				filepath.Join(nodeDir, "outputs"),
			},
			ReadRoots: []string{
				"code/shared", "code/configs", ".handoff",
			},
			ForbiddenPaths: []string{
				"paper", "results/final", ".git",
			},
		},
		Deliverables:   profile.Deliverables,
		AcceptCriteria: profile.AcceptCriteria,
		Protocol:       profile,
		RetryPolicy: RetryPolicy{
			MaxAgentReplans:     cfg.MaxAgentReplans,
			MaxExecutionRetries: cfg.MaxDebugRetries,
			TimeoutSec:          cfg.DefaultTimeoutSec,
		},
		CreatedAt: time.Now().UTC(),
	}
}

// Save writes the contract as contract.json into the given node directory.
// The directory is created if it does not already exist.
func (c *TaskContract) Save(nodeDir string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("orchestrator: marshal contract: %w", err)
	}
	data = append(data, '\n')
	dst := filepath.Join(nodeDir, contractFileName)
	return fileop.SafeWrite(dst, data, fileop.WithMkdir())
}

// LoadContract reads contract.json from the given node directory and returns
// the parsed TaskContract.
func LoadContract(nodeDir string) (*TaskContract, error) {
	src := filepath.Join(nodeDir, contractFileName)
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: read contract %s: %w", src, err)
	}

	var c TaskContract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("orchestrator: parse contract %s: %w", src, err)
	}
	return &c, nil
}
