package task

// RegistryEvent is published on the event bus whenever a task is registered,
// updated, or removed from the Registry.
type RegistryEvent struct {
	// Action is one of "registered", "updated", or "removed".
	Action string `json:"action"`
	TaskID string `json:"task_id"`
	Kind   Kind   `json:"kind"`
	Status Status `json:"status"`
}
