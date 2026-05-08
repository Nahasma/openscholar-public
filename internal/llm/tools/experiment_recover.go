package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/research/orchestrator"
)

type experimentRecoverTool struct{}

// NewExperimentRecoverTool creates an ExperimentRecover tool.
func NewExperimentRecoverTool() BaseTool {
	return &experimentRecoverTool{}
}

func (t *experimentRecoverTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ExperimentRecover",
		Description: `Classify a node failure and recommend a recovery strategy.
Loads the contract, manifest, and assessment for a node, classifies the failure type,
and returns a structured RecoveryPlan with a recommended action (retry / switch_provider / repair / rollback / archive).
Use this when a dispatched experiment node fails or its assessment is rejected/repair_needed.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": map[string]any{
					"type":        "string",
					"description": "The experiment node ID that failed.",
				},
				"failure_type": map[string]any{
					"type": "string",
					"description": "Optional override for the failure classification. " +
						"Valid values: contract_failure, execution_failure, verification_failure, goal_deviation, research_failure. " +
						"If omitted, the failure type is inferred automatically from contract/manifest/assessment state.",
				},
			},
			"required": []string{"node_id"},
		},
		Required: []string{"node_id"},
	}
}

type experimentRecoverParams struct {
	NodeID      string `json:"node_id"`
	FailureType string `json:"failure_type,omitempty"`
}

func (t *experimentRecoverTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params experimentRecoverParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if params.NodeID == "" {
		return NewTextErrorResponse("node_id is required"), nil
	}

	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return NewTextErrorResponse("ExperimentRecover requires a research workspace context"), nil
	}

	nodeDir := filepath.Join(workDir, "experiments", "nodes", params.NodeID)

	// Load contract (may be nil if contract_failure)
	contract, _ := orchestrator.LoadContract(nodeDir)

	// Load manifest (best-effort)
	manifest, _ := orchestrator.LoadManifest(workDir, params.NodeID)

	// Load assessment (best-effort)
	assessment, _ := orchestrator.LoadAssessment(workDir, params.NodeID)

	// Classify failure
	failureType := params.FailureType
	if failureType == "" {
		failureType = orchestrator.ClassifyFailure(contract, manifest, assessment)
	}

	// Plan recovery
	cfg := config.Get()
	plan := orchestrator.PlanRecovery(failureType, manifest, cfg.Experiment)

	// Build response
	var sb strings.Builder
	fmt.Fprintf(&sb, "Recovery Plan — Node %q\n\n", params.NodeID)
	fmt.Fprintf(&sb, "Failure Type : %s\n", plan.FailureType)
	fmt.Fprintf(&sb, "Strategy     : %s\n", plan.Strategy)
	fmt.Fprintf(&sb, "Details      : %s\n", plan.Details)
	if plan.NewProvider != "" {
		fmt.Fprintf(&sb, "New Provider : %s\n", plan.NewProvider)
	}

	// Show current state summary
	sb.WriteString("\n--- State Summary ---\n")
	if contract != nil {
		fmt.Fprintf(&sb, "Contract  : loaded (pipeline=%s, type=%s)\n", contract.PipelineID, contract.NodeType)
	} else {
		sb.WriteString("Contract  : NOT FOUND\n")
	}
	if manifest != nil {
		fmt.Fprintf(&sb, "Manifest  : status=%s, provider=%s\n", manifest.Status, manifest.Provider)
	} else {
		sb.WriteString("Manifest  : NOT FOUND\n")
	}
	if assessment != nil {
		fmt.Fprintf(&sb, "Assessment: status=%s, score=%.0f%%\n", assessment.Status, assessment.Score*100)
	} else {
		sb.WriteString("Assessment: NOT FOUND\n")
	}

	// Actionable next steps
	sb.WriteString("\n--- Recommended Next Steps ---\n")
	switch plan.Strategy {
	case "retry":
		fmt.Fprintf(&sb, "1. Call ExperimentDispatch with node_id=%q to retry.\n", params.NodeID)
	case "switch_provider":
		fmt.Fprintf(&sb, "1. Call ExperimentDispatch with node_id=%q and provider=%q.\n", params.NodeID, plan.NewProvider)
	case "repair":
		sb.WriteString("1. Review the issues listed in the assessment.\n")
		fmt.Fprintf(&sb, "2. Call ExperimentBrief with node_id=%q (or a new node_id) to regenerate the contract with tighter constraints.\n", params.NodeID)
		fmt.Fprintf(&sb, "3. Call ExperimentDispatch with the repaired node.\n")
	case "rollback":
		sb.WriteString("1. Restore from the node snapshot directory (experiments/nodes/<id>/snapshots/).\n")
		fmt.Fprintf(&sb, "2. Call ExperimentDispatch with node_id=%q to re-dispatch.\n", params.NodeID)
	case "archive":
		sb.WriteString("1. The negative result is scientifically valid — archive the outputs.\n")
		sb.WriteString("2. Record findings in the experimental journal via ExperimentJournal.\n")
		sb.WriteString("3. Consider branching to an alternative hypothesis.\n")
	}

	return NewTextResponse(sb.String()), nil
}
