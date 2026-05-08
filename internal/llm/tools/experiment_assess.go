package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/research/orchestrator"
)

type experimentAssessTool struct{}

// NewExperimentAssessTool creates an ExperimentAssess tool.
func NewExperimentAssessTool() BaseTool {
	return &experimentAssessTool{}
}

func (t *experimentAssessTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ExperimentAssess",
		Description: `Assess experiment results and write assessment.json to the node directory.
Loads the contract, manifest, and artifact report, then runs the structured assessment pipeline
to produce a scored evaluation (accepted / repair_needed / rejected) with per-check breakdown.
Call this after ExperimentCollect to get a decision on whether the node output is usable.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": map[string]any{
					"type":        "string",
					"description": "The experiment node ID to assess.",
				},
			},
			"required": []string{"node_id"},
		},
		Required: []string{"node_id"},
	}
}

type experimentAssessParams struct {
	NodeID string `json:"node_id"`
}

func (t *experimentAssessTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params experimentAssessParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if params.NodeID == "" {
		return NewTextErrorResponse("node_id is required"), nil
	}

	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return NewTextErrorResponse("ExperimentAssess requires a research workspace context"), nil
	}

	nodeDir := filepath.Join(workDir, "experiments", "nodes", params.NodeID)

	// Load contract
	contract, err := orchestrator.LoadContract(nodeDir)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to load contract for node %q: %s", params.NodeID, err)), nil
	}

	// Load manifest
	manifest, err := orchestrator.LoadManifest(workDir, params.NodeID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to load manifest for node %q: %s", params.NodeID, err)), nil
	}

	// Collect artifacts
	report, err := orchestrator.CollectArtifacts(workDir, params.NodeID, contract.Deliverables, contract.Protocol.MetricArtifactPath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to collect artifacts: %s", err)), nil
	}

	// Run assessment
	assessment := orchestrator.Assess(contract, manifest, report)

	// Save assessment.json
	if err := assessment.Save(workDir); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to save assessment: %s", err)), nil
	}

	// Build summary
	var sb strings.Builder
	fmt.Fprintf(&sb, "Assessment — Node %q\n\n", params.NodeID)
	fmt.Fprintf(&sb, "Status : %s\n", assessment.Status)
	fmt.Fprintf(&sb, "Score  : %.0f%%\n\n", assessment.Score*100)

	sb.WriteString("Checks:\n")
	for _, check := range assessment.Checks {
		mark := "✓"
		if !check.Passed {
			mark = "✗"
		}
		fmt.Fprintf(&sb, "  %s %s\n", mark, check.Name)
	}

	if len(assessment.Issues) > 0 {
		fmt.Fprintf(&sb, "\nIssues (%d):\n", len(assessment.Issues))
		for _, iss := range assessment.Issues {
			fmt.Fprintf(&sb, "  [%s] %s — %s\n", iss.Severity, iss.Check, iss.Message)
		}
	}

	if len(assessment.PromotableArtifacts) > 0 {
		fmt.Fprintf(&sb, "\nPromotable Artifacts (%d):\n", len(assessment.PromotableArtifacts))
		for _, a := range assessment.PromotableArtifacts {
			fmt.Fprintf(&sb, "  - %s\n", a)
		}
	}

	fmt.Fprintf(&sb, "\nAssessment saved to: %s\n", filepath.Join(nodeDir, "assessment.json"))

	return NewTextResponse(sb.String()), nil
}
