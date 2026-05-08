package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/openscholar/openscholar/internal/llm/tools/codeagent"
	"github.com/openscholar/openscholar/internal/research/orchestrator"
)

type experimentDispatchTool struct {
	registry *codeagent.Registry
}

// NewExperimentDispatchTool creates an ExperimentDispatch tool backed by the given provider registry.
func NewExperimentDispatchTool(registry *codeagent.Registry) BaseTool {
	return &experimentDispatchTool{registry: registry}
}

func (t *experimentDispatchTool) Info() ToolInfo {
	available := t.registry.Available()

	desc := `Dispatch an experiment node to an external coding agent for execution.
Loads the node's TaskContract, builds the agent prompt, executes the coding agent, then collects and validates artifacts.
Call ExperimentBrief first to create the contract and workspace.`

	if len(available) > 0 {
		desc += fmt.Sprintf("\n\nCurrently available code agents: %v", available)
	} else {
		desc += "\n\nNo code agent CLI currently installed."
	}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"node_id": map[string]any{
				"type":        "string",
				"description": "The experiment node ID to dispatch (must have an existing contract).",
			},
			"provider": map[string]any{
				"type":        "string",
				"description": "Code agent provider to use. If omitted or 'auto', the best available provider is selected.",
			},
		},
		"required": []string{"node_id"},
	}

	return ToolInfo{
		Name:        "ExperimentDispatch",
		Description: desc,
		Parameters:  params,
		Required:    []string{"node_id"},
	}
}

type experimentDispatchParams struct {
	NodeID   string `json:"node_id"`
	Provider string `json:"provider,omitempty"`
}

func (t *experimentDispatchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params experimentDispatchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if params.NodeID == "" {
		return NewTextErrorResponse("node_id is required"), nil
	}

	workDir := ResearchWorkDir(ctx)
	if workDir == "" {
		return NewTextErrorResponse("ExperimentDispatch requires a research workspace context"), nil
	}

	nodeDir := filepath.Join(workDir, "experiments", "nodes", params.NodeID)

	// Load contract
	contract, err := orchestrator.LoadContract(nodeDir)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to load contract for node %q: %s", params.NodeID, err)), nil
	}

	// Build instruction and prompt
	inst := orchestrator.InstructionFromContract(contract, workDir, nil)
	prompt := inst.BuildPrompt()

	// Determine provider
	providerName := params.Provider
	if providerName == "auto" {
		providerName = ""
	}

	provider, err := t.registry.Get(providerName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("code agent unavailable: %s", err)), nil
	}

	// Create initial manifest
	manifest := &orchestrator.Manifest{
		NodeID:     contract.NodeID,
		PipelineID: contract.PipelineID,
		Provider:   provider.Name(),
		Status:     "dispatched",
		StartedAt:  time.Now().UTC(),
	}
	if saveErr := manifest.Save(workDir); saveErr != nil {
		// Non-fatal — log and continue
		_ = saveErr
	}

	// Apply provider-specific hint as system prompt append
	hint := orchestrator.ProviderHint(provider.Name())

	timeout := time.Duration(contract.RetryPolicy.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	// Execute
	startedAt := time.Now()
	resp, execErr := provider.Execute(ctx, codeagent.CodeRequest{
		Prompt:            prompt,
		WorkDir:           nodeDir,
		Timeout:           timeout,
		AppendSystemPrompt: hint,
		Bare:              true,
	})
	elapsed := time.Since(startedAt)

	// Update manifest with outcome
	manifest.EndedAt = time.Now().UTC()
	manifest.DurationSec = elapsed.Seconds()

	if execErr != nil {
		manifest.Status = "execution_failed"
		manifest.ExitReason = execErr.Error()
		_ = manifest.Save(workDir)
		return NewTextErrorResponse(fmt.Sprintf("[%s] execution failed: %s", provider.Name(), execErr)), nil
	}

	if resp != nil {
		manifest.SessionID = resp.SessionID
		manifest.ExitReason = resp.ExitReason
		if resp.Success {
			manifest.Status = "completed"
		} else {
			manifest.Status = "completed_with_issues"
		}
	}

	// Collect and validate artifacts
	report, collectErr := orchestrator.CollectArtifacts(workDir, params.NodeID, contract.Deliverables, contract.Protocol.MetricArtifactPath)
	if collectErr == nil && report != nil {
		issues := orchestrator.ValidateArtifacts(report)
		manifest.Artifacts = report.Found
		manifest.Missing = report.Missing
		manifest.Metrics = report.Metrics
		manifest.Warnings = report.Warnings
		manifest.ValidationPass = len(issues) == 0
	}

	_ = manifest.Save(workDir)

	// Build summary
	var result string
	if resp != nil {
		result = resp.Result
	}

	summary := fmt.Sprintf(
		"[%s] Node %q dispatched and completed.\n\nStatus: %s\nDuration: %.1fs\nArtifacts found: %d\nMissing: %d\nMetrics: %d entries\nValidation pass: %v\n\n--- Agent Output ---\n%s",
		provider.Name(),
		params.NodeID,
		manifest.Status,
		manifest.DurationSec,
		len(manifest.Artifacts),
		len(manifest.Missing),
		len(manifest.Metrics),
		manifest.ValidationPass,
		result,
	)

	return NewTextResponse(summary), nil
}
