package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/openscholar/openscholar/internal/fileop"
)

// Assessment records the evaluation result of a single node execution.
type Assessment struct {
	NodeID              string            `json:"node_id"`
	Status              string            `json:"status"` // "accepted" | "rejected" | "repair_needed"
	Score               float64           `json:"score"`
	Checks              []AssessmentCheck `json:"checks"`
	PromotableArtifacts []string          `json:"promotable_artifacts"`
	Issues              []ValidationIssue `json:"issues"`
}

// AssessmentCheck represents a single named boolean check in the assessment.
type AssessmentCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

// Assess evaluates a node execution based on contract, manifest, and artifact report.
func Assess(contract *TaskContract, manifest *Manifest, report *ArtifactReport) *Assessment {
	a := &Assessment{NodeID: contract.NodeID}
	var checks []AssessmentCheck
	issues := ValidateArtifacts(report)

	// Check 1: deliverables exist
	deliverablesOK := len(report.Missing) == 0
	checks = append(checks, AssessmentCheck{"deliverables_exist", deliverablesOK})

	// Check 2: metrics valid
	metricsOK := len(report.Metrics) > 0
	for _, issue := range issues {
		if issue.Check == "metrics_valid" || issue.Check == "empty_metrics" || issue.Check == "invalid_metric" {
			metricsOK = false
		}
	}
	checks = append(checks, AssessmentCheck{"metrics_valid", metricsOK})

	// Check 3: validation pass
	validationOK := manifest.ValidationPass
	checks = append(checks, AssessmentCheck{"validation_pass", validationOK})

	a.Checks = checks
	a.Issues = issues

	passCount := 0
	for _, c := range checks {
		if c.Passed {
			passCount++
		}
	}
	a.Score = float64(passCount) / float64(len(checks))

	switch {
	case a.Score >= 0.8:
		a.Status = "accepted"
	case a.Score >= 0.5:
		a.Status = "repair_needed"
	default:
		a.Status = "rejected"
	}

	for _, entry := range report.Found {
		a.PromotableArtifacts = append(a.PromotableArtifacts, entry.Path)
	}

	return a
}

// Save writes assessment.json to the node directory.
func (a *Assessment) Save(workDir string) error {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return fileop.SafeWrite(filepath.Join(nodeDir(workDir, a.NodeID), "assessment.json"), data, fileop.WithMkdir())
}

// LoadAssessment reads assessment.json from a node directory.
func LoadAssessment(workDir, nodeID string) (*Assessment, error) {
	path := filepath.Join(nodeDir(workDir, nodeID), "assessment.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a Assessment
	return &a, json.Unmarshal(data, &a)
}
