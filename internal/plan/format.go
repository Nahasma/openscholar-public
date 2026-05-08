package plan

import "strings"

const (
	proposedOpen  = "<proposed_plan>"
	proposedClose = "</proposed_plan>"
	approvedOpen  = "<approved_plan>"
	approvedClose = "</approved_plan>"
)

// NormalizeBody removes a single outer proposed/approved plan envelope.
func NormalizeBody(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if body, ok := unwrap(trimmed, proposedOpen, proposedClose); ok {
		return body
	}
	if body, ok := unwrap(trimmed, approvedOpen, approvedClose); ok {
		return body
	}
	return raw
}

func WrapProposed(raw string) string {
	body := strings.TrimSpace(NormalizeBody(raw))
	return proposedOpen + "\n" + body + "\n" + proposedClose
}

func WrapApproved(raw string) string {
	body := strings.TrimSpace(NormalizeBody(raw))
	return approvedOpen + "\n" + body + "\n" + approvedClose
}

func DisplayBody(raw string) string {
	return strings.TrimSpace(NormalizeBody(raw))
}

func unwrap(raw, open, close string) (string, bool) {
	if !strings.HasPrefix(raw, open) || !strings.HasSuffix(raw, close) {
		return "", false
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, open), close))
	return body, true
}
