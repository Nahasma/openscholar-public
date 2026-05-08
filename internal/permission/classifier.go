package permission

import "context"

// ClassifyResult represents the outcome of an AI security classification.
type ClassifyResult string

const (
	// ClassifyAllow indicates the classifier determined the action is safe.
	ClassifyAllow ClassifyResult = "allow"
	// ClassifyDeny indicates the classifier determined the action is unsafe.
	ClassifyDeny ClassifyResult = "deny"
	// ClassifyUnsure indicates the classifier could not make a determination.
	ClassifyUnsure ClassifyResult = "unsure"
)

// SecurityClassifier is an interface for AI-assisted permission classification.
// Implementations should be lightweight and have bounded latency.
// This interface is defined in Phase 3 but implementations are deferred to Phase 4.
type SecurityClassifier interface {
	// Classify evaluates a permission request and returns a classification result.
	// Implementations must respect context cancellation and timeouts.
	Classify(ctx context.Context, req CreatePermissionRequest) (ClassifyResult, error)

	// Name returns a human-readable identifier for this classifier.
	Name() string
}
