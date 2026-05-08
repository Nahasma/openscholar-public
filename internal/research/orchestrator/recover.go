package orchestrator

import "github.com/openscholar/openscholar/internal/config"

// Failure type constants for ClassifyFailure.
const (
	FailureContract     = "contract_failure"
	FailureExecution    = "execution_failure"
	FailureVerification = "verification_failure"
	FailureDeviation    = "goal_deviation"
	FailureResearch     = "research_failure"
)

// RecoveryPlan describes how to recover from a node failure.
type RecoveryPlan struct {
	FailureType string `json:"failure_type"`
	Strategy    string `json:"strategy"` // "retry" | "switch_provider" | "repair" | "rollback" | "archive"
	Details     string `json:"details"`
	NewProvider string `json:"new_provider,omitempty"`
}

// ClassifyFailure infers the failure type from contract, manifest, and assessment state.
func ClassifyFailure(contract *TaskContract, manifest *Manifest, assessment *Assessment) string {
	if contract == nil {
		return FailureContract
	}
	if manifest != nil {
		switch manifest.Status {
		case "execution_failed", "timeout", "infra_failed":
			return FailureExecution
		}
	}
	if assessment != nil && assessment.Status == "rejected" {
		if len(assessment.Issues) > 0 {
			return FailureVerification
		}
		return FailureDeviation
	}
	return FailureResearch
}

// PlanRecovery returns a RecoveryPlan given a failure type, current manifest, and experiment config.
func PlanRecovery(failureType string, manifest *Manifest, cfg config.ExperimentConfig) *RecoveryPlan {
	switch failureType {
	case FailureContract:
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "repair",
			Details:     "Contract is incomplete. Regenerate with ExperimentBrief.",
		}
	case FailureExecution:
		if manifest != nil && manifest.Provider == "claude" {
			return &RecoveryPlan{
				FailureType: failureType,
				Strategy:    "switch_provider",
				Details:     "Claude execution failed. Trying Codex as fallback.",
				NewProvider: "codex",
			}
		}
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "retry",
			Details:     "Execution failed. Retrying with same provider.",
		}
	case FailureVerification:
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "repair",
			Details:     "Deliverables incomplete or invalid. Re-dispatch with enhanced constraints.",
		}
	case FailureDeviation:
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "rollback",
			Details:     "Agent deviated from goal. Rollback to snapshot and re-dispatch.",
		}
	case FailureResearch:
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "archive",
			Details:     "Experiment ran correctly but did not support hypothesis. Archive as negative result.",
		}
	default:
		return &RecoveryPlan{
			FailureType: failureType,
			Strategy:    "retry",
			Details:     "Unknown failure, attempting retry.",
		}
	}
}
