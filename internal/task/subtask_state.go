package task

// SubtaskState tracks TaskV2 delegated sub-agents.
type SubtaskState struct {
	BaseState
	ParentSessionID string       `json:"parent_session_id"`
	ChildSessionID  string       `json:"child_session_id"`
	FanoutGroup     string       `json:"fanout_group,omitempty"`
	WriteSet        []string     `json:"write_set,omitempty"`
	AgentType       string       `json:"agent_type"`
	Summary         string       `json:"summary,omitempty"`
	Model           string       `json:"model,omitempty"`
	ResultMaxChars  int          `json:"result_max_chars,omitempty"`
	ResultTruncated bool         `json:"result_truncated,omitempty"`
	MaxTurns        int          `json:"max_turns,omitempty"`
	VerifyPolicy    string       `json:"verify_policy,omitempty"`
	VerifyStatus    VerifyStatus `json:"verify_status,omitempty"`
	VerifyVerdict   string       `json:"verify_verdict,omitempty"`
	VerifyResult    string       `json:"verify_result,omitempty"`
	VerifyError     string       `json:"verify_error,omitempty"`
	Result          string       `json:"result,omitempty"`
	LastError       string       `json:"last_error,omitempty"`
}

// NewSubtaskState creates a task state with KindAgent metadata.
func NewSubtaskState(meta Meta, parentSessionID, childSessionID, agentType, summary, model, verifyPolicy string) *SubtaskState {
	meta.Kind = KindAgent
	verifyStatus := VerifyStatusSkipped
	if verifyPolicy != "" && verifyPolicy != "none" {
		verifyStatus = VerifyStatusPending
	}
	return &SubtaskState{
		BaseState:       BaseState{M: meta},
		ParentSessionID: parentSessionID,
		ChildSessionID:  childSessionID,
		AgentType:       agentType,
		Summary:         summary,
		Model:           model,
		VerifyPolicy:    verifyPolicy,
		VerifyStatus:    verifyStatus,
	}
}
