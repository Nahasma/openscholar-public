package kb

// BatchEventStatus represents the lifecycle status of a batch ingestion event.
type BatchEventStatus string

const (
	BatchEventStarted          BatchEventStatus = "started"
	BatchEventCompleted        BatchEventStatus = "completed"
	BatchEventFailed           BatchEventStatus = "failed"
	BatchEventPermanentlyFailed BatchEventStatus = "permanently_failed"
)

// BatchEvent carries information about a batch job state change.
type BatchEvent struct {
	JobID      string
	PaperTitle string
	Status     BatchEventStatus
	Error      string
	Attempt    int
}
