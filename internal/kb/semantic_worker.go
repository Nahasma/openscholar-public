package kb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/Nahasma/openscholar-public/internal/db"
)

const semanticSyncLeaseDuration = 5 * time.Minute

// SemanticWorker processes durable semantic tree build tasks.
type SemanticWorker struct {
	svc           *service
	indexer       Indexer
	leaseOwner    string
	leaseDuration time.Duration
}

func NewSemanticWorker(kbService Service, indexer Indexer, leaseOwner string, leaseDuration time.Duration) (*SemanticWorker, error) {
	svc, ok := kbService.(*service)
	if !ok {
		return nil, fmt.Errorf("kb service does not support semantic worker")
	}
	if indexer == nil {
		return nil, fmt.Errorf("semantic indexer is required")
	}
	if leaseOwner == "" {
		leaseOwner = "semantic-worker"
	}
	if leaseDuration <= 0 {
		leaseDuration = 5 * time.Minute
	}
	return &SemanticWorker{svc: svc, indexer: indexer, leaseOwner: leaseOwner, leaseDuration: leaseDuration}, nil
}

// RunOnce claims and handles a single pending task.
func (w *SemanticWorker) RunOnce(ctx context.Context) (*KBTask, error) {
	task, err := w.svc.ClaimPendingKBTaskByType(ctx, KBTaskTypeSemanticTree, w.leaseOwner, w.leaseDuration)
	if err != nil || task == nil {
		return task, err
	}
	_ = w.svc.HeartbeatKBTask(ctx, task.TaskID, w.leaseOwner, 0.1, w.leaseDuration)
	err = w.svc.runSemanticTreeTask(ctx, w.indexer, *task)
	return task, err
}

// RunSemanticTreeSync executes semantic build in current request with durable task lifecycle.
func (s *service) RunSemanticTreeSync(ctx context.Context, indexer Indexer, paperID, filePath string, generateSummary bool) (KBTask, error) {
	if indexer == nil {
		return KBTask{}, fmt.Errorf("semantic indexer is required")
	}
	now := time.Now().Unix()
	taskID := uuid.New().String()
	input, _ := json.Marshal(semanticTaskInput{
		FilePath:        filePath,
		GenerateSummary: generateSummary,
	})
	if err := s.q.InsertKBTask(ctx, db.InsertKBTaskParams{
		TaskID:       taskID,
		PaperID:      toNullString(paperID),
		TaskType:     KBTaskTypeSemanticTree,
		Status:       KBTaskStatusRunning,
		Progress:     sql.NullFloat64{Float64: 0.1, Valid: true},
		InputJson:    toNullString(string(input)),
		ResultJson:   sql.NullString{},
		ResultRef:    sql.NullString{},
		Error:        sql.NullString{},
		LeaseOwner:   toNullString("sync"),
		LeaseUntil:   sql.NullInt64{Int64: now + int64(semanticSyncLeaseDuration.Seconds()), Valid: true},
		AttemptCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
		StartedAt:    sql.NullInt64{Int64: now, Valid: true},
		FinishedAt:   sql.NullInt64{},
	}); err != nil {
		return KBTask{}, fmt.Errorf("insert sync task: %w", err)
	}
	if err := s.q.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
		SemanticTreeStatus:    toNullString(KBTaskStatusRunning),
		SemanticTreeAvailable: 0,
		SemanticTreeError:     sql.NullString{},
		SemanticTreeTaskID:    toNullString(taskID),
		UpdatedAt:             now,
		PaperID:               paperID,
	}); err != nil {
		return KBTask{}, fmt.Errorf("update semantic state running: %w", err)
	}

	task := KBTask{
		TaskID:    taskID,
		PaperID:   paperID,
		TaskType:  KBTaskTypeSemanticTree,
		Status:    KBTaskStatusRunning,
		InputJSON: string(input),
	}
	stopHeartbeat := s.startKBTaskHeartbeat(ctx, taskID, "sync", semanticSyncLeaseDuration)
	defer stopHeartbeat()
	if err := s.runSemanticTreeTask(ctx, indexer, task); err != nil {
		failed, getErr := s.GetKBTask(ctx, taskID)
		if getErr == nil {
			return failed, err
		}
		return task, err
	}
	return s.GetKBTask(ctx, taskID)
}

func (s *service) startKBTaskHeartbeat(ctx context.Context, taskID, leaseOwner string, leaseDuration time.Duration) func() {
	if leaseDuration <= 0 {
		leaseDuration = 5 * time.Minute
	}
	interval := leaseDuration / 3
	if interval < time.Second {
		interval = time.Second
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.HeartbeatKBTask(heartbeatCtx, taskID, leaseOwner, 0.1, leaseDuration)
			case <-heartbeatCtx.Done():
				return
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func (s *service) runSemanticTreeTask(ctx context.Context, indexer Indexer, task KBTask) error {
	var in semanticTaskInput
	_ = json.Unmarshal([]byte(task.InputJSON), &in)
	paperID := task.PaperID
	if paperID == "" {
		_ = s.FailKBTask(ctx, task.TaskID, "missing paper_id")
		return fmt.Errorf("missing paper_id")
	}

	state, err := s.GetPaperIndexStateView(ctx, paperID)
	if err == nil && state.SemanticTreeStatus == "ready" && state.SemanticTreeAvailable {
		_ = s.CompleteKBTask(ctx, task.TaskID, `{"status":"skipped_already_ready"}`)
		_ = s.q.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
			SemanticTreeStatus:    toNullString("ready"),
			SemanticTreeAvailable: 1,
			SemanticTreeError:     sql.NullString{},
			SemanticTreeTaskID:    toNullString(task.TaskID),
			UpdatedAt:             time.Now().Unix(),
			PaperID:               paperID,
		})
		return nil
	}

	filePath := in.FilePath
	if filePath == "" {
		paper, paperErr := s.GetPaper(ctx, paperID)
		if paperErr != nil {
			msg := fmt.Sprintf("load paper failed: %v", paperErr)
			_ = s.failSemanticTask(ctx, task, msg)
			return fmt.Errorf("%s", msg)
		}
		filePath = paper.FilePath
		if filePath == "" {
			filePath = paper.PDFPath
		}
	}
	if filePath == "" {
		msg := "semantic build source file is missing"
		_ = s.failSemanticTask(ctx, task, msg)
		return fmt.Errorf("%s", msg)
	}

	opts := DefaultIndexOptions()
	opts.GenerateSummary = in.GenerateSummary
	result, buildErr := indexer.BuildTree(ctx, filePath, opts)
	if buildErr != nil {
		msg := fmt.Sprintf("semantic build failed: %v", buildErr)
		_ = s.failSemanticTask(ctx, task, msg)
		return fmt.Errorf("%s", msg)
	}
	if err := s.AttachSemanticTree(ctx, paperID, *result); err != nil {
		msg := fmt.Sprintf("attach semantic tree failed: %v", err)
		_ = s.failSemanticTask(ctx, task, msg)
		return fmt.Errorf("%s", msg)
	}

	resultJSON, _ := json.Marshal(map[string]any{
		"paper_id":     paperID,
		"status":       "succeeded",
		"total_pages":  result.TotalPages,
		"total_tokens": result.TotalTokens,
		"index_level":  result.IndexLevel,
	})
	if err := s.CompleteKBTask(ctx, task.TaskID, string(resultJSON)); err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	return s.q.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
		SemanticTreeStatus:    toNullString("ready"),
		SemanticTreeAvailable: 1,
		SemanticTreeError:     sql.NullString{},
		SemanticTreeTaskID:    toNullString(task.TaskID),
		UpdatedAt:             time.Now().Unix(),
		PaperID:               paperID,
	})
}

func (s *service) failSemanticTask(ctx context.Context, task KBTask, msg string) error {
	_ = s.FailKBTask(ctx, task.TaskID, msg)
	return s.q.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
		SemanticTreeStatus:    toNullString(KBTaskStatusFailed),
		SemanticTreeAvailable: 0,
		SemanticTreeError:     toNullString(msg),
		SemanticTreeTaskID:    toNullString(task.TaskID),
		UpdatedAt:             time.Now().Unix(),
		PaperID:               task.PaperID,
	})
}
