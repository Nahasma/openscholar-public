package kb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/db"
)

type semanticTaskInput struct {
	FilePath        string `json:"file_path,omitempty"`
	GenerateSummary bool   `json:"generate_summary"`
}

// EnqueueSemanticTreeTask creates a durable semantic tree background task and marks paper state as queued.
func (s *service) EnqueueSemanticTreeTask(ctx context.Context, paperID, filePath string, generateSummary bool) (KBTask, error) {
	return s.enqueueSemanticTreeTask(ctx, paperID, filePath, generateSummary, false)
}

func (s *service) requeueSemanticTreeTask(ctx context.Context, paperID, filePath string, generateSummary bool) (KBTask, error) {
	return s.enqueueSemanticTreeTask(ctx, paperID, filePath, generateSummary, true)
}

func (s *service) enqueueSemanticTreeTask(ctx context.Context, paperID, filePath string, generateSummary bool, force bool) (KBTask, error) {
	now := time.Now().Unix()
	taskID := semanticTreeTaskID(paperID)
	in := semanticTaskInput{
		FilePath:        filePath,
		GenerateSummary: generateSummary,
	}
	inputJSON, _ := json.Marshal(in)

	row, err := s.q.EnqueueKBTask(ctx, db.EnqueueKBTaskParams{
		TaskID:    taskID,
		PaperID:   toNullString(paperID),
		TaskType:  KBTaskTypeSemanticTree,
		InputJson: toNullString(string(inputJSON)),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return KBTask{}, fmt.Errorf("enqueue kb task: %w", err)
	}
	task := kbTaskFromDB(row)
	if force && task.Status == KBTaskStatusSucceeded {
		row, err = s.q.ForceRequeueKBTask(ctx, db.ForceRequeueKBTaskParams{
			InputJson: toNullString(string(inputJSON)),
			UpdatedAt: now,
			TaskID:    taskID,
		})
		if err != nil {
			return KBTask{}, fmt.Errorf("force requeue kb task: %w", err)
		}
		task = kbTaskFromDB(row)
	}
	if task.Status == KBTaskStatusSucceeded {
		return task, nil
	}
	if err := s.q.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
		SemanticTreeStatus:    toNullString(task.Status),
		SemanticTreeAvailable: 0,
		SemanticTreeError:     sql.NullString{},
		SemanticTreeTaskID:    toNullString(taskID),
		UpdatedAt:             now,
		PaperID:               paperID,
	}); err != nil {
		return KBTask{}, fmt.Errorf("update semantic state queued: %w", err)
	}
	return task, nil
}

func semanticTreeTaskID(paperID string) string {
	return "semantic-tree:" + strings.TrimSpace(paperID)
}

// ClaimPendingKBTask claims one queued/stale-running task with a fresh lease.
func (s *service) ClaimPendingKBTask(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*KBTask, error) {
	now := time.Now().Unix()
	row, err := s.q.ClaimPendingKBTask(ctx, db.ClaimPendingKBTaskParams{
		LeaseOwner: toNullString(leaseOwner),
		LeaseUntil: sql.NullInt64{Int64: now + int64(leaseDuration.Seconds()), Valid: true},
		UpdatedAt:  now,
		NowEpoch:   sql.NullInt64{Int64: now, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim kb task: %w", err)
	}
	task := kbTaskFromDB(row)
	return &task, nil
}

// ClaimPendingKBTaskByType claims one queued/stale-running task of the requested type.
func (s *service) ClaimPendingKBTaskByType(ctx context.Context, taskType, leaseOwner string, leaseDuration time.Duration) (*KBTask, error) {
	now := time.Now().Unix()
	row, err := s.q.ClaimPendingKBTaskByType(ctx, db.ClaimPendingKBTaskByTypeParams{
		TaskType:   taskType,
		LeaseOwner: toNullString(leaseOwner),
		LeaseUntil: sql.NullInt64{Int64: now + int64(leaseDuration.Seconds()), Valid: true},
		UpdatedAt:  now,
		NowEpoch:   sql.NullInt64{Int64: now, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim kb task by type: %w", err)
	}
	task := kbTaskFromDB(row)
	return &task, nil
}

// HeartbeatKBTask refreshes lease and progress for a running task.
func (s *service) HeartbeatKBTask(ctx context.Context, taskID, leaseOwner string, progress float64, leaseDuration time.Duration) error {
	now := time.Now().Unix()
	return s.q.UpdateKBTaskProgress(ctx, db.UpdateKBTaskProgressParams{
		Progress:   sql.NullFloat64{Float64: progress, Valid: true},
		LeaseOwner: toNullString(leaseOwner),
		LeaseUntil: sql.NullInt64{Int64: now + int64(leaseDuration.Seconds()), Valid: true},
		UpdatedAt:  now,
		TaskID:     taskID,
	})
}

func (s *service) CompleteKBTask(ctx context.Context, taskID, resultJSON string) error {
	return s.q.CompleteKBTask(ctx, db.CompleteKBTaskParams{
		ResultJson: toNullString(resultJSON),
		ResultRef:  sql.NullString{},
		UpdatedAt:  time.Now().Unix(),
		TaskID:     taskID,
	})
}

func (s *service) RetryKBTask(ctx context.Context, taskID, errMsg string) error {
	return s.q.RetryKBTask(ctx, db.RetryKBTaskParams{
		Error:     toNullString(errMsg),
		UpdatedAt: time.Now().Unix(),
		TaskID:    taskID,
	})
}

func (s *service) FailKBTask(ctx context.Context, taskID, errMsg string) error {
	return s.q.FailKBTask(ctx, db.FailKBTaskParams{
		Error:     toNullString(errMsg),
		UpdatedAt: time.Now().Unix(),
		TaskID:    taskID,
	})
}

func (s *service) CancelKBTask(ctx context.Context, taskID, reason string) error {
	return s.q.CancelKBTask(ctx, db.CancelKBTaskParams{
		Error:     toNullString(reason),
		UpdatedAt: time.Now().Unix(),
		TaskID:    taskID,
	})
}

func (s *service) GetKBTask(ctx context.Context, taskID string) (KBTask, error) {
	row, err := s.q.GetKBTask(ctx, taskID)
	if err != nil {
		return KBTask{}, err
	}
	return kbTaskFromDB(row), nil
}

func kbTaskFromDB(row db.KbTask) KBTask {
	return KBTask{
		TaskID:       row.TaskID,
		PaperID:      row.PaperID.String,
		TaskType:     row.TaskType,
		Status:       row.Status,
		Progress:     row.Progress.Float64,
		InputJSON:    row.InputJson.String,
		ResultJSON:   row.ResultJson.String,
		ResultRef:    row.ResultRef.String,
		Error:        row.Error.String,
		LeaseOwner:   row.LeaseOwner.String,
		LeaseUntil:   row.LeaseUntil.Int64,
		AttemptCount: int(row.AttemptCount),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		StartedAt:    row.StartedAt.Int64,
		FinishedAt:   row.FinishedAt.Int64,
	}
}
