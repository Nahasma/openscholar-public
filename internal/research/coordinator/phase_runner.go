package coordinator

import (
	"context"
	"errors"
	"fmt"
)

// NotificationPublisher delivers serialized XML notifications.
// It is intentionally minimal so callers can adapt to hooks/task/tui layers later.
type NotificationPublisher func(ctx context.Context, payload string) error

// PhaseRunRequest contains the minimal inputs to execute one phase.
type PhaseRunRequest struct {
	TaskID         string
	Pipeline       *PipelineRef
	Phase          *PhaseRef
	NextPhase      *PhaseRef
	StartSummary   string
	SuccessSummary string
}

// PhaseRunner orchestrates one phase: start notification -> execute -> result notification.
type PhaseRunner struct {
	executor  PhaseExecutor
	publisher NotificationPublisher
}

// NewPhaseRunner creates a runner with a phase executor and optional publisher.
func NewPhaseRunner(executor PhaseExecutor, publisher NotificationPublisher) *PhaseRunner {
	if publisher == nil {
		publisher = func(context.Context, string) error { return nil }
	}
	return &PhaseRunner{
		executor:  executor,
		publisher: publisher,
	}
}

// Run executes a phase and emits task-notification XML events.
func (r *PhaseRunner) Run(ctx context.Context, req PhaseRunRequest) error {
	if err := r.validate(req); err != nil {
		return err
	}

	taskID := req.TaskID
	if taskID == "" {
		taskID = defaultTaskID(req.Pipeline, req.Phase)
	}

	startSummary := req.StartSummary
	if startSummary == "" {
		startSummary = fmt.Sprintf("Phase %d (%s) started.", req.Phase.Order, req.Phase.Name)
	}
	if err := r.publish(ctx, FormatPhaseNotification(taskID, NotifyPhaseStarted, *req.Phase, nil, startSummary), NotifyPhaseStarted); err != nil {
		return err
	}

	execErr := r.executor.RunPhase(ctx, req.Pipeline, req.Phase)
	if execErr != nil {
		failSummary := fmt.Sprintf("Phase %d (%s) failed: %v", req.Phase.Order, req.Phase.Name, execErr)
		notifyErr := r.publish(
			ctx,
			FormatPhaseNotification(taskID, NotifyPhaseFailed, *req.Phase, nil, failSummary),
			NotifyPhaseFailed,
		)
		if notifyErr != nil {
			return errors.Join(execErr, notifyErr)
		}
		return execErr
	}

	successSummary := req.SuccessSummary
	if successSummary == "" {
		successSummary = fmt.Sprintf("Phase %d (%s) completed.", req.Phase.Order, req.Phase.Name)
	}

	if err := r.publish(
		ctx,
		FormatPhaseNotification(taskID, NotifyPhaseCompleted, *req.Phase, req.NextPhase, successSummary),
		NotifyPhaseCompleted,
	); err != nil {
		return err
	}

	return nil
}

// NotifyCheckpoint emits a checkpoint task-notification XML event.
func (r *PhaseRunner) NotifyCheckpoint(ctx context.Context, taskID string, phase *PhaseRef, summary string) error {
	if phase == nil {
		return errors.New("phase must not be nil")
	}
	if taskID == "" {
		taskID = fmt.Sprintf("phase-%s", phase.ID)
	}
	if summary == "" {
		summary = fmt.Sprintf("Phase %d (%s) waiting for checkpoint approval.", phase.Order, phase.Name)
	}
	return r.publish(
		ctx,
		FormatPhaseNotification(taskID, NotifyCheckpoint, *phase, nil, summary),
		NotifyCheckpoint,
	)
}

func (r *PhaseRunner) validate(req PhaseRunRequest) error {
	if r.executor == nil {
		return errors.New("phase executor is required")
	}
	if req.Pipeline == nil {
		return errors.New("pipeline must not be nil")
	}
	if req.Phase == nil {
		return errors.New("phase must not be nil")
	}
	return nil
}

func (r *PhaseRunner) publish(ctx context.Context, payload string, notifType NotificationType) error {
	if err := r.publisher(ctx, payload); err != nil {
		return fmt.Errorf("publish %s notification: %w", notifType, err)
	}
	return nil
}

func defaultTaskID(pipeline *PipelineRef, phase *PhaseRef) string {
	if pipeline.ID != "" && phase.ID != "" {
		return fmt.Sprintf("pipeline-%s-phase-%s", pipeline.ID, phase.ID)
	}
	if phase.ID != "" {
		return fmt.Sprintf("phase-%s", phase.ID)
	}
	return "phase"
}
