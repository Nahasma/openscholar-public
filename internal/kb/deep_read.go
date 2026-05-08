package kb

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/db"
)

const (
	deepReadDefaultBatchChars   = 12000
	deepReadDefaultReduceChars  = 20000
	deepReadDefaultBatchTokens  = 3000
	deepReadDefaultReduceTokens = 5000
)

type DeepReadTaskInput struct {
	PaperID  string `json:"paper_id"`
	Question string `json:"question"`
}

type DeepReadTaskResult struct {
	PaperID      string      `json:"paper_id"`
	Question     string      `json:"question"`
	MapSummaries []string    `json:"map_summaries,omitempty"`
	Answer       string      `json:"answer"`
	Sources      []SourceRef `json:"sources,omitempty"`
}

type DeepReadOptions struct {
	MaxBatchChars   int `json:"max_batch_chars,omitempty"`
	MaxBatchTokens  int `json:"max_batch_tokens,omitempty"`
	MaxReduceChars  int `json:"max_reduce_chars,omitempty"`
	MaxReduceTokens int `json:"max_reduce_tokens,omitempty"`
	MaxAttempts     int `json:"max_attempts,omitempty"`
}

const deepReadMapPrompt = `You are reading part of a paper.
Summarize ONLY information relevant to the question.
Be precise and include page hints when present.

Question: %s

Chunk batch:\n%s`

const deepReadReducePrompt = `You are combining partial analyses from a paper.
Produce a final answer to the question using only the map outputs.
State uncertainty if evidence is insufficient.

Question: %s

Map outputs:\n%s`

func deepReadTaskID(paperID string, question string) string {
	h := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(paperID)) + "\n" + strings.ToLower(strings.TrimSpace(question))))
	return "deep-read:" + paperID + ":" + hex.EncodeToString(h[:8])
}

func (s *service) EnsureDeepReadTask(ctx context.Context, paperID string, question string, allowCreate bool) (KBTask, error) {
	taskID := deepReadTaskID(paperID, question)
	existing, err := s.q.GetKBTask(ctx, taskID)
	if err == nil {
		return kbTaskFromRow(existing), nil
	}
	if err != nil && err != sql.ErrNoRows {
		return KBTask{}, fmt.Errorf("get deep-read task: %w", err)
	}
	if !allowCreate {
		return KBTask{TaskID: taskID, PaperID: paperID, TaskType: KBTaskTypeDeepRead, Status: "not_created"}, nil
	}
	inputJSON, _ := json.Marshal(DeepReadTaskInput{PaperID: paperID, Question: question})
	now := time.Now().Unix()
	row, err := s.q.EnqueueKBTask(ctx, db.EnqueueKBTaskParams{
		TaskID:    taskID,
		PaperID:   sql.NullString{String: paperID, Valid: paperID != ""},
		TaskType:  KBTaskTypeDeepRead,
		InputJson: sql.NullString{String: string(inputJSON), Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return KBTask{}, fmt.Errorf("enqueue deep-read task: %w", err)
	}
	return kbTaskFromRow(row), nil
}

func (s *service) RunDeepRead(ctx context.Context, callLLM LLMCaller, paperID string, question string, opts DeepReadOptions) (*DeepReadTaskResult, error) {
	chunks, err := s.q.GetPaperChunks(ctx, paperID)
	if err != nil {
		return nil, fmt.Errorf("load paper chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no paper chunks found for %s", paperID)
	}
	batchChars := opts.MaxBatchChars
	if batchChars <= 0 {
		batchChars = deepReadDefaultBatchChars
	}
	reduceChars := opts.MaxReduceChars
	if reduceChars <= 0 {
		reduceChars = deepReadDefaultReduceChars
	}
	batchTokens := opts.MaxBatchTokens
	if batchTokens <= 0 {
		batchTokens = deepReadDefaultBatchTokens
	}
	reduceTokens := opts.MaxReduceTokens
	if reduceTokens <= 0 {
		reduceTokens = deepReadDefaultReduceTokens
	}

	batches, sources := buildDeepReadBatches(chunks, batchChars, batchTokens)
	if len(batches) == 0 {
		return nil, fmt.Errorf("no usable chunk content for deep-read")
	}

	maps := make([]string, 0, len(batches))
	for _, batch := range batches {
		out, callErr := callLLM(ctx, fmt.Sprintf(deepReadMapPrompt, question, batch))
		if callErr != nil {
			return nil, fmt.Errorf("deep-read map call failed: %w", callErr)
		}
		maps = append(maps, strings.TrimSpace(out))
	}

	reduceInput := strings.Join(maps, "\n\n---\n\n")
	reduceInput = truncateDeepReadText(reduceInput, reduceChars, reduceTokens)
	answer, err := callLLM(ctx, fmt.Sprintf(deepReadReducePrompt, question, reduceInput))
	if err != nil {
		return nil, fmt.Errorf("deep-read reduce call failed: %w", err)
	}

	return &DeepReadTaskResult{
		PaperID:      paperID,
		Question:     question,
		MapSummaries: maps,
		Answer:       strings.TrimSpace(answer),
		Sources:      sources,
	}, nil
}

func (s *service) CompleteDeepReadTask(ctx context.Context, taskID string, result *DeepReadTaskResult) error {
	if result == nil {
		return fmt.Errorf("deep-read result is nil")
	}
	blob, _ := json.Marshal(result)
	now := time.Now().Unix()
	return s.q.CompleteKBTask(ctx, db.CompleteKBTaskParams{
		TaskID:     taskID,
		UpdatedAt:  now,
		ResultJson: sql.NullString{String: string(blob), Valid: true},
		ResultRef:  sql.NullString{String: "kb_task://" + taskID, Valid: true},
	})
}

// DeepReadWorker processes durable deep-read map/reduce tasks.
type DeepReadWorker struct {
	svc           *service
	callLLM       LLMCaller
	leaseOwner    string
	leaseDuration time.Duration
	opts          DeepReadOptions
}

func NewDeepReadWorker(kbService Service, callLLM LLMCaller, leaseOwner string, leaseDuration time.Duration, opts DeepReadOptions) (*DeepReadWorker, error) {
	svc, ok := kbService.(*service)
	if !ok {
		return nil, fmt.Errorf("kb service does not support deep-read worker")
	}
	if callLLM == nil {
		return nil, fmt.Errorf("deep-read LLM caller is required")
	}
	if leaseOwner == "" {
		leaseOwner = "deep-read-worker"
	}
	if leaseDuration <= 0 {
		leaseDuration = 5 * time.Minute
	}
	return &DeepReadWorker{svc: svc, callLLM: callLLM, leaseOwner: leaseOwner, leaseDuration: leaseDuration, opts: opts}, nil
}

func (w *DeepReadWorker) RunOnce(ctx context.Context) (*KBTask, error) {
	task, err := w.svc.ClaimPendingKBTaskByType(ctx, KBTaskTypeDeepRead, w.leaseOwner, w.leaseDuration)
	if err != nil || task == nil {
		return task, err
	}
	_ = w.svc.HeartbeatKBTask(ctx, task.TaskID, w.leaseOwner, 0.1, w.leaseDuration)
	_, runErr := w.svc.RunDeepReadTask(ctx, w.callLLM, task.TaskID, w.opts)
	return task, runErr
}

// RunDeepReadTask executes a claimed deep-read task and records retry/final status.
func (s *service) RunDeepReadTask(ctx context.Context, callLLM LLMCaller, taskID string, opts DeepReadOptions) (*DeepReadTaskResult, error) {
	if callLLM == nil {
		return nil, fmt.Errorf("deep-read LLM caller is required")
	}
	task, err := s.GetKBTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load deep-read task: %w", err)
	}
	if task.TaskType != KBTaskTypeDeepRead {
		return nil, fmt.Errorf("task %s is %s, not %s", taskID, task.TaskType, KBTaskTypeDeepRead)
	}
	if task.Status == KBTaskStatusSucceeded && strings.TrimSpace(task.ResultJSON) != "" {
		var done DeepReadTaskResult
		if err := json.Unmarshal([]byte(task.ResultJSON), &done); err != nil {
			return nil, fmt.Errorf("decode deep-read result: %w", err)
		}
		return &done, nil
	}

	var input DeepReadTaskInput
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return nil, s.finishDeepReadError(ctx, task, fmt.Sprintf("decode deep-read input: %v", err), opts)
	}
	if strings.TrimSpace(input.PaperID) == "" || strings.TrimSpace(input.Question) == "" {
		return nil, s.finishDeepReadError(ctx, task, "deep-read task input requires paper_id and question", opts)
	}

	result, err := s.RunDeepRead(ctx, callLLM, input.PaperID, input.Question, opts)
	if err != nil {
		return nil, s.finishDeepReadError(ctx, task, err.Error(), opts)
	}
	if err := s.CompleteDeepReadTask(ctx, task.TaskID, result); err != nil {
		return nil, fmt.Errorf("complete deep-read task: %w", err)
	}
	return result, nil
}

func (s *service) finishDeepReadError(ctx context.Context, task KBTask, msg string, opts DeepReadOptions) error {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if task.AttemptCount < maxAttempts {
		_ = s.RetryKBTask(ctx, task.TaskID, msg)
		return fmt.Errorf("deep-read task will retry: %s", msg)
	}
	_ = s.FailKBTask(ctx, task.TaskID, msg)
	return fmt.Errorf("deep-read task failed: %s", msg)
}

func buildDeepReadBatches(chunks []db.PaperChunk, maxChars, maxTokens int) ([]string, []SourceRef) {
	if maxChars <= 0 {
		maxChars = deepReadDefaultBatchChars
	}
	if maxTokens <= 0 {
		maxTokens = deepReadDefaultBatchTokens
	}
	batches := make([]string, 0)
	sources := make([]SourceRef, 0, len(chunks))
	var sb strings.Builder
	remainingChars := maxChars
	remainingTokens := maxTokens
	for _, c := range chunks {
		text := strings.TrimSpace(c.Content)
		if text == "" {
			continue
		}
		title := strings.TrimSpace(c.Title.String)
		if title == "" {
			title = c.ChunkID
		}
		sp := int(c.PageStart.Int64)
		ep := int(c.PageEnd.Int64)
		block := fmt.Sprintf("[%s] %s (pp. %d-%d)\n%s\n\n", c.ChunkID, title, sp, ep, text)
		blockTokens := chunkTokenBudget(c, block)
		block = truncateDeepReadText(block, maxChars, maxTokens)
		if blockTokens > maxTokens {
			blockTokens = maxTokens
		}
		if sb.Len() > 0 && (len(block) > remainingChars || blockTokens > remainingTokens) {
			batches = append(batches, sb.String())
			sb.Reset()
			remainingChars = maxChars
			remainingTokens = maxTokens
		}
		sb.WriteString(block)
		remainingChars -= len(block)
		remainingTokens -= blockTokens
		sources = append(sources, SourceRef{NodeID: c.ChunkID, Title: title, StartPage: sp, EndPage: ep})
	}
	if sb.Len() > 0 {
		batches = append(batches, sb.String())
	}
	return batches, sources
}

func chunkTokenBudget(c db.PaperChunk, block string) int {
	if c.TokenCount.Valid && c.TokenCount.Int64 > 0 {
		return int(c.TokenCount.Int64)
	}
	return approxDeepReadTokens(block)
}

func truncateDeepReadText(s string, maxChars, maxTokens int) string {
	if maxChars <= 0 {
		maxChars = len(s)
	}
	if maxTokens > 0 {
		tokenChars := maxTokens * 4
		if tokenChars < maxChars {
			maxChars = tokenChars
		}
	}
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars]
}

func approxDeepReadTokens(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	tokens := len(s) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func kbTaskFromRow(row db.KbTask) KBTask {
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
