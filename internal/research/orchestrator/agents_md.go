package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// GenerateAgentsMD writes an AGENTS.md file into the node's directory.
// The file contains all context a Worker Agent needs to execute the experiment.
func GenerateAgentsMD(workDir, nodeID string, contract *TaskContract, sharedFiles []string) error {
	var sb strings.Builder

	// Header
	sb.WriteString("# AGENTS.md — Experiment Node Instructions\n\n")
	sb.WriteString(fmt.Sprintf("**Node ID**: `%s`  \n", nodeID))
	sb.WriteString(fmt.Sprintf("**Pipeline ID**: `%s`  \n", contract.PipelineID))
	sb.WriteString(fmt.Sprintf("**Generated**: `%s`\n\n", contract.CreatedAt.Format("2006-01-02T15:04:05Z")))

	// Mission
	sb.WriteString("## Mission\n\n")
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n\n", contract.Goal))
	if contract.Background.Hypothesis != "" {
		sb.WriteString(fmt.Sprintf("**Hypothesis**: %s\n\n", contract.Background.Hypothesis))
	}
	sb.WriteString(fmt.Sprintf("**Node Type**: `%s`\n\n", contract.NodeType))

	// Parent Context
	if contract.Background.ParentNodeID != "" {
		sb.WriteString("## Parent Context\n\n")
		sb.WriteString(fmt.Sprintf("**Parent Node**: `%s`\n\n", contract.Background.ParentNodeID))
		if contract.Background.ParentSummary != "" {
			sb.WriteString(fmt.Sprintf("**Summary**: %s\n\n", contract.Background.ParentSummary))
		}
		if len(contract.Background.ParentMetrics) > 0 {
			sb.WriteString("**Parent Metrics**:\n\n")
			for k, v := range contract.Background.ParentMetrics {
				sb.WriteString(fmt.Sprintf("- `%s`: %.6g\n", k, v))
			}
			sb.WriteString("\n")
		}
	}

	// Directory Structure
	sb.WriteString("## Directory Structure\n\n")
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("experiments/nodes/%s/\n", nodeID))
	sb.WriteString("  work/        ← your primary working directory (read/write)\n")
	sb.WriteString("  input/       ← read-only inputs provided by the orchestrator\n")
	sb.WriteString("  outputs/     ← write all deliverables here\n")
	sb.WriteString("  outputs/figures/  ← save all plots and figures here\n")
	sb.WriteString("  logs/        ← execution logs (stdout/stderr)\n")
	sb.WriteString("  snapshots/   ← checkpoints created by the orchestrator\n")
	sb.WriteString("```\n\n")

	// Output Protocol
	sb.WriteString("## Output Protocol\n\n")
	sb.WriteString("You MUST produce the following files in `outputs/`:\n\n")
	for _, d := range contract.Deliverables {
		sb.WriteString(fmt.Sprintf("- `%s`\n", d))
	}
	sb.WriteString("\n")
	// Metric example from domain profile
	metricExample := `{"primary_metric": 0.85, "secondary_metric": 0.72, "runtime_sec": 300}`
	if contract.Protocol.MetricExample != "" {
		metricExample = contract.Protocol.MetricExample
	}
	metricPath := "outputs/metrics.json"
	if contract.Protocol.MetricArtifactPath != "" {
		metricPath = contract.Protocol.MetricArtifactPath
	}
	sb.WriteString(fmt.Sprintf("**`%s`** must be a flat JSON object mapping metric names to numeric values:\n\n", metricPath))
	sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", metricExample))
	sb.WriteString("**`outputs/run_report.md`** must contain a brief summary of what was done, results, and observations.\n\n")
	sb.WriteString("**`outputs/figures/`** — save any plots or visualizations as `.png` or `.pdf`.\n\n")

	// Constraints
	sb.WriteString("## Constraints\n\n")
	for _, c := range contract.AcceptCriteria {
		sb.WriteString(fmt.Sprintf("- %s\n", c))
	}
	sb.WriteString("\n")
	sb.WriteString("**Hard boundaries** — you MUST NOT:\n\n")
	sb.WriteString("- Modify any file under `input/`\n")
	sb.WriteString("- Access or modify paths outside your node directory\n")
	forbidden := []string{"paper/", "results/final/", ".git/"}
	if len(contract.Scope.ForbiddenPaths) > 0 {
		forbidden = contract.Scope.ForbiddenPaths
	}
	for _, fp := range forbidden {
		sb.WriteString(fmt.Sprintf("- Modify `%s`\n", fp))
	}
	sb.WriteString("\n")

	// Available Shared Code
	if len(sharedFiles) > 0 {
		sb.WriteString("## Available Shared Code\n\n")
		sb.WriteString("The following files from `code/shared/` have been copied into your `work/` directory:\n\n")
		for _, f := range sharedFiles {
			sb.WriteString(fmt.Sprintf("- `work/%s`\n", f))
		}
		sb.WriteString("\n")
	}

	// Retry / Timeout
	sb.WriteString("## Execution Policy\n\n")
	sb.WriteString(fmt.Sprintf("- **Timeout**: %d seconds\n", contract.RetryPolicy.TimeoutSec))
	sb.WriteString(fmt.Sprintf("- **Max replans**: %d\n", contract.RetryPolicy.MaxAgentReplans))
	sb.WriteString(fmt.Sprintf("- **Max execution retries**: %d\n\n", contract.RetryPolicy.MaxExecutionRetries))

	// Write file
	dest := filepath.Join(workDir, "experiments", "nodes", nodeID, "AGENTS.md")
	return fileop.SafeWrite(dest, []byte(sb.String()), fileop.WithMkdir())
}
