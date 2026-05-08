package kb

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

// BatchIngester manages a worker pool for importing multiple documents.
type BatchIngester struct {
	kbService   Service
	indexer     Indexer
	q           db.Querier
	workers     int
	broker      *pubsub.Broker[BatchEvent]
	maxAttempts int
}

// NewBatchIngester creates a batch ingestion pipeline.
// broker may be nil; when nil, no events are published.
// maxAttempts specifies how many times a failing job is retried (default 3).
func NewBatchIngester(svc Service, indexer Indexer, q db.Querier, workers int, broker *pubsub.Broker[BatchEvent], maxAttempts int) *BatchIngester {
	if workers <= 0 {
		workers = 2
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &BatchIngester{
		kbService:   svc,
		indexer:     indexer,
		q:           q,
		workers:     workers,
		broker:      broker,
		maxAttempts: maxAttempts,
	}
}

// supportedExtensions lists file types that can be ingested.
var supportedExtensions = map[string]string{
	".pdf":  "pdf",
	".docx": "docx",
	".pptx": "pptx",
	".xlsx": "xlsx",
}

// Enqueue scans a directory or file list and adds jobs to the queue.
// Returns the list of created job IDs.
func (b *BatchIngester) Enqueue(ctx context.Context, paths []string) ([]string, error) {
	var filePaths []string

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// Scan directory for supported files
			_ = filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if _, ok := supportedExtensions[ext]; ok {
					filePaths = append(filePaths, path)
				}
				return nil
			})
		} else {
			ext := strings.ToLower(filepath.Ext(p))
			if _, ok := supportedExtensions[ext]; ok {
				filePaths = append(filePaths, p)
			}
		}
	}

	now := time.Now().Unix()
	var jobIDs []string

	for _, fp := range filePaths {
		absPath, _ := filepath.Abs(fp)

		// Dedup: skip if already in KB
		if existing, _ := b.q.GetPaperByFilePath(ctx, toNullString(absPath)); existing.PaperID != "" {
			continue
		}

		jobID := uuid.New().String()
		if err := b.q.InsertBatchJob(ctx, db.InsertBatchJobParams{
			ID:        jobID,
			FilePath:  absPath,
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			continue
		}
		jobIDs = append(jobIDs, jobID)
	}

	return jobIDs, nil
}

// ProcessQueue starts workers to process pending jobs.
func (b *BatchIngester) ProcessQueue(ctx context.Context) error {
	if b.indexer == nil {
		return fmt.Errorf("indexer not available")
	}
	if _, ok := b.indexer.(RawParser); !ok {
		return fmt.Errorf("raw parser not available")
	}

	jobs, err := b.q.ListPendingBatchJobs(ctx, 100)
	if err != nil {
		return fmt.Errorf("list pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, b.workers)

	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}

		go func(j db.BatchJob) {
			defer wg.Done()
			defer func() { <-sem }()
			b.processJob(ctx, j)
		}(job)
	}

	wg.Wait()
	return nil
}

func (b *BatchIngester) processJob(ctx context.Context, job db.BatchJob) {
	now := time.Now().Unix()

	// Publish started event
	b.publishEvent(pubsub.CreatedEvent, BatchEvent{
		JobID:      job.ID,
		PaperTitle: filepath.Base(job.FilePath),
		Status:     BatchEventStarted,
		Attempt:    int(job.AttemptCount) + 1,
	})

	// Mark as processing
	_ = b.q.UpdateBatchJobStatus(ctx, db.UpdateBatchJobStatusParams{
		Status:    "processing",
		UpdatedAt: now,
		ID:        job.ID,
	})

	rawParser := b.indexer.(RawParser)
	parseResult, err := rawParser.ParseDocument(ctx, job.FilePath, ParseOptions{})
	if err != nil {
		b.handleJobFailure(ctx, job, err)
		return
	}

	// Add paper with fast raw ingest
	ext := strings.ToLower(filepath.Ext(job.FilePath))
	paper := Paper{
		Title:    parseResult.PaperTitle,
		FilePath: job.FilePath,
		DocType:  supportedExtensions[ext],
	}
	if ext == ".pdf" {
		paper.PDFPath = job.FilePath
	}

	stateWriter, ok := b.kbService.(interface {
		AddParsedPaper(ctx context.Context, paper Paper, parseResult ParseResult, semanticTreeStatus string) (PaperIndexState, error)
	})
	if !ok {
		b.handleJobFailure(ctx, job, fmt.Errorf("kb service does not support AddParsedPaper"))
		return
	}
	state, err := stateWriter.AddParsedPaper(ctx, paper, *parseResult, "not_requested")
	if err != nil {
		b.handleJobFailure(ctx, job, err)
		return
	}
	paper.PaperID = state.PaperID

	_ = b.q.UpdateBatchJobStatus(ctx, db.UpdateBatchJobStatusParams{
		Status:    "completed",
		PaperID:   toNullString(paper.PaperID),
		UpdatedAt: time.Now().Unix(),
		ID:        job.ID,
	})

	// Publish completed event
	b.publishEvent(pubsub.UpdatedEvent, BatchEvent{
		JobID:      job.ID,
		PaperTitle: filepath.Base(job.FilePath),
		Status:     BatchEventCompleted,
		Attempt:    int(job.AttemptCount) + 1,
	})

	log.Printf("[batch] completed: %s", filepath.Base(job.FilePath))
}

// handleJobFailure increments attempt count and either re-queues or permanently fails the job.
func (b *BatchIngester) handleJobFailure(ctx context.Context, job db.BatchJob, jobErr error) {
	// Increment attempt count
	_ = b.q.IncrementBatchJobAttempt(ctx, job.ID)
	newAttempt := int(job.AttemptCount) + 1

	// Publish transient failed event
	b.publishEvent(pubsub.UpdatedEvent, BatchEvent{
		JobID:      job.ID,
		PaperTitle: filepath.Base(job.FilePath),
		Status:     BatchEventFailed,
		Error:      jobErr.Error(),
		Attempt:    newAttempt,
	})

	log.Printf("[batch] failed (attempt %d/%d) %s: %v", newAttempt, b.maxAttempts, job.FilePath, jobErr)

	if newAttempt < b.maxAttempts {
		// Exponential backoff capped at 4 seconds
		delay := time.Duration(1<<uint(newAttempt-1)) * time.Second
		if delay > 4*time.Second {
			delay = 4 * time.Second
		}
		time.Sleep(delay)

		// Reset to pending so the next ProcessQueue call picks it up
		_ = b.q.ResetBatchJobForRetry(ctx, job.ID)
		return
	}

	// Permanently failed
	_ = b.q.UpdateBatchJobStatus(ctx, db.UpdateBatchJobStatusParams{
		Status:    "failed",
		Error:     toNullString(jobErr.Error()),
		UpdatedAt: time.Now().Unix(),
		ID:        job.ID,
	})

	b.publishEvent(pubsub.DeletedEvent, BatchEvent{
		JobID:      job.ID,
		PaperTitle: filepath.Base(job.FilePath),
		Status:     BatchEventPermanentlyFailed,
		Error:      jobErr.Error(),
		Attempt:    newAttempt,
	})

	log.Printf("[batch] permanently failed after %d attempts: %s", newAttempt, job.FilePath)
}

// publishEvent sends a BatchEvent to the broker if one is configured.
func (b *BatchIngester) publishEvent(t pubsub.EventType, evt BatchEvent) {
	if b.broker != nil {
		b.broker.Publish(t, evt)
	}
}

// Status returns job counts by status.
func (b *BatchIngester) Status(ctx context.Context) (map[string]int64, error) {
	rows, err := b.q.CountBatchJobsByStatus(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]int64)
	for _, row := range rows {
		result[row.Status] = row.Count
	}
	return result, nil
}

// RetryFailed re-queues all failed jobs as pending.
func (b *BatchIngester) RetryFailed(ctx context.Context) error {
	return b.q.ResetFailedBatchJobs(ctx, time.Now().Unix())
}

// ListFailedJobs returns up to limit failed batch jobs ordered by most recent.
func (b *BatchIngester) ListFailedJobs(ctx context.Context, limit int) ([]db.BatchJob, error) {
	return b.q.ListFailedBatchJobs(ctx, int64(limit))
}
