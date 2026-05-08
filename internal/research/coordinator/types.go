package coordinator

import "context"

// PhaseExecutor defines the interface for phase execution.
// ADR-3: Coordinator sits above Engine, delegates actual execution here.
type PhaseExecutor interface {
	RunPhase(ctx context.Context, pipeline *PipelineRef, phase *PhaseRef) error
}

// PipelineRef is a lightweight pipeline reference used within the Coordinator.
// This intentionally avoids importing research.Pipeline to prevent a circular
// dependency: research/engine.go already imports coordinator for PhaseExecutor,
// so coordinator cannot import research. Conversion adapters live in app/.
type PipelineRef struct {
	ID        string
	SessionID string
	Topic     string
	WorkDir   string
}

// PhaseRef is a lightweight phase reference used within the Coordinator.
type PhaseRef struct {
	ID         string
	Name       string
	Order      int
	MaxWorkers int
}

// Coordinator orchestrates research pipeline phase execution.
// It sits above Engine and does not mutate the Engine's internal state machine (ADR-3).
type Coordinator struct {
	phaseRunner *PhaseRunner
}

// NewCoordinator creates a new Coordinator with the given PhaseExecutor.
func NewCoordinator(executor PhaseExecutor) *Coordinator {
	return NewCoordinatorWithNotifier(executor, nil)
}

// NewCoordinatorWithNotifier creates a Coordinator and wires phase notifications.
func NewCoordinatorWithNotifier(executor PhaseExecutor, publisher NotificationPublisher) *Coordinator {
	return &Coordinator{
		phaseRunner: NewPhaseRunner(executor, publisher),
	}
}

// RunPhase orchestrates a single phase execution through the PhaseRunner.
func (c *Coordinator) RunPhase(
	ctx context.Context,
	taskID string,
	pipeline *PipelineRef,
	phase *PhaseRef,
	nextPhase *PhaseRef,
) error {
	return c.phaseRunner.Run(ctx, PhaseRunRequest{
		TaskID:    taskID,
		Pipeline:  pipeline,
		Phase:     phase,
		NextPhase: nextPhase,
	})
}

// NotifyCheckpoint publishes a checkpoint notification for the current phase.
func (c *Coordinator) NotifyCheckpoint(
	ctx context.Context,
	taskID string,
	phase *PhaseRef,
	summary string,
) error {
	return c.phaseRunner.NotifyCheckpoint(ctx, taskID, phase, summary)
}
