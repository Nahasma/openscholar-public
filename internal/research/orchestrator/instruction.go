package orchestrator

import (
	"fmt"
	"strings"
)

// ExperimentInstruction is the structured instruction derived from a TaskContract,
// designed as input for external coding agents.
type ExperimentInstruction struct {
	Objective     string   `json:"objective"`
	Hypothesis    string   `json:"hypothesis"`
	NodeType      string   `json:"node_type"`
	WorkDir       string   `json:"work_dir"`
	Timeout       int      `json:"timeout_sec"`
	Language      string   `json:"language"`
	ParentSummary string   `json:"parent_summary"`
	SharedCode    []string `json:"shared_code"`
	ExpectedFiles []string `json:"expected_files"`
	MetricNames   []string `json:"metric_names"`
	Constraints   []string `json:"constraints"`
}

// InstructionFromContract creates an ExperimentInstruction from a TaskContract.
// Language and file references are derived from contract.Protocol (domain profile snapshot).
func InstructionFromContract(c *TaskContract, workDir string, sharedFiles []string) *ExperimentInstruction {
	primaryLang := "python"
	if len(c.Protocol.Languages) > 0 {
		primaryLang = c.Protocol.Languages[0]
	}
	return &ExperimentInstruction{
		Objective:     c.Goal,
		Hypothesis:    c.Background.Hypothesis,
		NodeType:      c.NodeType,
		WorkDir:       workDir,
		Timeout:       c.RetryPolicy.TimeoutSec,
		Language:      primaryLang,
		ParentSummary: c.Background.ParentSummary,
		SharedCode:    sharedFiles,
		ExpectedFiles: c.Deliverables,
		Constraints:   c.AcceptCriteria,
	}
}

// BuildPrompt renders the instruction into a self-contained prompt for external agents.
func (inst *ExperimentInstruction) BuildPrompt() string {
	var sb strings.Builder

	// Mission
	sb.WriteString("## Mission\n\n")
	sb.WriteString(fmt.Sprintf("**Objective**: %s\n\n", inst.Objective))
	if inst.Hypothesis != "" {
		sb.WriteString(fmt.Sprintf("**Hypothesis**: %s\n\n", inst.Hypothesis))
	}
	sb.WriteString(fmt.Sprintf("**Node Type**: `%s`  \n", inst.NodeType))
	sb.WriteString(fmt.Sprintf("**Language**: `%s`\n\n", inst.Language))

	// Parent Context
	if inst.ParentSummary != "" {
		sb.WriteString("## Parent Context\n\n")
		sb.WriteString(inst.ParentSummary)
		sb.WriteString("\n\n")
	}

	// Available Code
	if len(inst.SharedCode) > 0 {
		sb.WriteString("## Available Code in work/\n\n")
		for _, f := range inst.SharedCode {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	// Execution Protocol
	sb.WriteString("## Execution Protocol\n\n")
	sb.WriteString("1. Read `AGENTS.md` in your working directory for full context and directory layout.\n")
	sb.WriteString("2. Implement your solution in the `work/` subdirectory.\n")
	sb.WriteString("3. Run your code and capture all output.\n")
	sb.WriteString("4. Write all results to the `outputs/` directory:\n")
	if len(inst.ExpectedFiles) > 0 {
		for _, f := range inst.ExpectedFiles {
			sb.WriteString(fmt.Sprintf("   - `%s`\n", f))
		}
	}
	sb.WriteString("5. Ensure the primary results file is a structured data file with all numeric metrics.\n")
	sb.WriteString("6. Write a brief results report summarising your findings.\n\n")

	// Definition of Done
	if len(inst.Constraints) > 0 {
		sb.WriteString("## Definition of Done\n\n")
		for _, c := range inst.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sb.WriteString("\n")
	}

	// Hard Boundaries
	sb.WriteString("## Hard Boundaries\n\n")
	sb.WriteString("- Only write files inside `work/` and `outputs/`.\n")
	sb.WriteString("- Do NOT modify `input/`, `paper/`, `.git/`, or any path outside your node directory.\n")
	if inst.Timeout > 0 {
		sb.WriteString(fmt.Sprintf("- Your total execution time must not exceed **%d seconds**.\n", inst.Timeout))
	}
	sb.WriteString("\n")

	return sb.String()
}

// ProviderHint returns a lightweight hint for a specific provider.
func ProviderHint(provider string) string {
	switch provider {
	case "claude":
		return "Submit a final text summary when done. Prefer structured file output over terminal print."
	case "codex":
		return "Follow the AGENTS.md in the working directory. Use --full-auto sandbox mode."
	case "gemini":
		return "Write all results to files. Do not rely on terminal output for final metrics."
	default:
		return ""
	}
}
