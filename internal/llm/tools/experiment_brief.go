package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/research/orchestrator"
)

type experimentBriefTool struct{}

// NewExperimentBriefTool creates an ExperimentBrief tool.
func NewExperimentBriefTool() BaseTool {
	return &experimentBriefTool{}
}

func (t *experimentBriefTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ExperimentBrief",
		Description: `Generate a TaskContract and initialise the workspace for an experiment node.
This tool creates contract.json, sets up the node directory structure, copies shared code, and writes AGENTS.md.
Call this before dispatching an experiment node to an external coding agent.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pipeline_id": map[string]any{
					"type":        "string",
					"description": "The parent pipeline/tree ID this node belongs to.",
				},
				"node_id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for this experiment node (e.g. 'node-001' or a UUID).",
				},
				"node_type": map[string]any{
					"type":        "string",
					"description": "Category of experiment: 'preliminary', 'baseline', 'ablation', 'replication', 'sensitivity_analysis', 'validation', 'variant', or any custom type.",
				},
				"goal": map[string]any{
					"type":        "string",
					"description": "Plain-language description of what the experiment should accomplish.",
				},
				"hypothesis": map[string]any{
					"type":        "string",
					"description": "The scientific hypothesis being tested by this node.",
				},
				"domain": map[string]any{
					"type":        "string",
					"description": "Scientific domain preset. Options: 'general' (default), 'ml', 'nlp', 'simulation', 'molecular', 'bioinformatics', 'statistics', 'econometrics', 'optimization', 'systems', 'engineering', 'climate'. Controls deliverables, language, metric format, and experiment stages.",
				},
			},
			"required": []string{"pipeline_id", "node_id", "node_type", "goal", "hypothesis"},
		},
		Required: []string{"pipeline_id", "node_id", "node_type", "goal", "hypothesis"},
	}
}

type experimentBriefParams struct {
	PipelineID string `json:"pipeline_id"`
	NodeID     string `json:"node_id"`
	NodeType   string `json:"node_type"`
	Goal       string `json:"goal"`
	Hypothesis string `json:"hypothesis"`
	Domain     string `json:"domain,omitempty"`
}

func (t *experimentBriefTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params experimentBriefParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if params.PipelineID == "" || params.NodeID == "" || params.NodeType == "" || params.Goal == "" {
		return NewTextErrorResponse("pipeline_id, node_id, node_type, and goal are required"), nil
	}

	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return NewTextErrorResponse("ExperimentBrief requires a research workspace context (ResearchWorkDirContextKey not set)"), nil
	}

	cfg := config.Get()

	// Phase 9.1: per-node domain override
	expCfg := cfg.Experiment
	if params.Domain != "" {
		expCfg.Domain = params.Domain
	}

	contract := orchestrator.NewContract(
		params.PipelineID,
		params.NodeID,
		params.NodeType,
		params.Goal,
		params.Hypothesis,
		"", // parentNodeID
		nil, // parentMetrics
		workDir,
		expCfg,
	)

	// Initialise node directory structure
	if err := orchestrator.InitNodeWorkspace(workDir, params.NodeID); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to init node workspace: %s", err)), nil
	}

	// Save contract
	nodeDir := filepath.Join(workDir, "experiments", "nodes", params.NodeID)
	if err := contract.Save(nodeDir); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to save contract: %s", err)), nil
	}

	// Copy shared code into node work directory
	sharedFiles, err := orchestrator.CopySharedCode(workDir, params.NodeID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to copy shared code: %s", err)), nil
	}

	// Generate AGENTS.md
	if err := orchestrator.GenerateAgentsMD(workDir, params.NodeID, contract, sharedFiles); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to generate AGENTS.md: %s", err)), nil
	}

	summary := fmt.Sprintf(
		"ExperimentBrief created for node %q (pipeline %q).\n\nNode type: %s\nGoal: %s\nHypothesis: %s\n\nWorkspace: %s\nShared files copied: %d\nContract: %s\nAGENTS.md: %s",
		params.NodeID,
		params.PipelineID,
		params.NodeType,
		params.Goal,
		params.Hypothesis,
		nodeDir,
		len(sharedFiles),
		filepath.Join(nodeDir, "contract.json"),
		filepath.Join(nodeDir, "AGENTS.md"),
	)

	return NewTextResponse(summary), nil
}
