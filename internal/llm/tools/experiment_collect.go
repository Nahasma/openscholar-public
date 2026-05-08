package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/research/orchestrator"
)

type experimentCollectTool struct{}

// NewExperimentCollectTool creates an ExperimentCollect tool.
func NewExperimentCollectTool() BaseTool {
	return &experimentCollectTool{}
}

func (t *experimentCollectTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ExperimentCollect",
		Description: `Collect and report artifacts from a completed experiment node.
Scans the node outputs directory, checks expected deliverables against what was produced,
parses metrics.json, and reports any validation issues.
Call this after an experiment node has finished executing.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": map[string]any{
					"type":        "string",
					"description": "The experiment node ID to collect artifacts from.",
				},
			},
			"required": []string{"node_id"},
		},
		Required: []string{"node_id"},
	}
}

type experimentCollectParams struct {
	NodeID string `json:"node_id"`
}

func (t *experimentCollectTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params experimentCollectParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if params.NodeID == "" {
		return NewTextErrorResponse("node_id is required"), nil
	}

	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return NewTextErrorResponse("ExperimentCollect requires a research workspace context"), nil
	}

	nodeDir := filepath.Join(workDir, "experiments", "nodes", params.NodeID)

	// Load contract to get expected deliverables
	contract, err := orchestrator.LoadContract(nodeDir)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to load contract for node %q: %s", params.NodeID, err)), nil
	}

	// Collect artifacts
	report, err := orchestrator.CollectArtifacts(workDir, params.NodeID, contract.Deliverables, contract.Protocol.MetricArtifactPath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to collect artifacts: %s", err)), nil
	}

	// Validate
	issues := orchestrator.ValidateArtifacts(report)

	// Build formatted report
	var sb strings.Builder
	fmt.Fprintf(&sb, "Artifact Report — Node %q\n\n", params.NodeID)

	// Found artifacts
	fmt.Fprintf(&sb, "Found (%d):\n", len(report.Found))
	for _, entry := range report.Found {
		fmt.Fprintf(&sb, "  %-40s  %d bytes  [%s]\n", entry.Path, entry.Size, entry.Checksum)
	}

	// Missing
	if len(report.Missing) > 0 {
		fmt.Fprintf(&sb, "\nMissing (%d):\n", len(report.Missing))
		for _, m := range report.Missing {
			fmt.Fprintf(&sb, "  - %s\n", m)
		}
	}

	// Extra
	if len(report.Extra) > 0 {
		fmt.Fprintf(&sb, "\nExtra (unexpected) (%d):\n", len(report.Extra))
		for _, e := range report.Extra {
			fmt.Fprintf(&sb, "  + %s  (%d bytes)\n", e.Path, e.Size)
		}
	}

	// Metrics
	if len(report.Metrics) > 0 {
		fmt.Fprintf(&sb, "\nMetrics (%d):\n", len(report.Metrics))
		for k, v := range report.Metrics {
			fmt.Fprintf(&sb, "  %s = %g\n", k, v)
		}
	} else {
		sb.WriteString("\nMetrics: (none found)\n")
	}

	// Warnings
	if len(report.Warnings) > 0 {
		fmt.Fprintf(&sb, "\nWarnings (%d):\n", len(report.Warnings))
		for _, w := range report.Warnings {
			fmt.Fprintf(&sb, "  ! %s\n", w)
		}
	}

	// Validation issues
	if len(issues) > 0 {
		fmt.Fprintf(&sb, "\nValidation Issues (%d):\n", len(issues))
		for _, iss := range issues {
			fmt.Fprintf(&sb, "  [%s] %s — %s\n", iss.Severity, iss.Check, iss.Message)
		}
	} else {
		sb.WriteString("\nValidation: PASSED (no issues)\n")
	}

	return NewTextResponse(sb.String()), nil
}
