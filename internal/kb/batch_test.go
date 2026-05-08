package kb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"

	"github.com/google/uuid"
	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- test DB setup (avoids import cycle with testutil) ----------

var batchTestGooseMu sync.Mutex

func setupBatchTestDB(t *testing.T) (*sql.DB, db.Querier) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "batch_test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	if err != nil {
		t.Fatalf("failed to open batch test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			t.Fatalf("failed to set pragma %s: %v", pragma, err)
		}
	}

	batchTestGooseMu.Lock()
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		batchTestGooseMu.Unlock()
		t.Fatalf("failed to set goose dialect: %v", err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		batchTestGooseMu.Unlock()
		t.Fatalf("failed to run migrations: %v", err)
	}
	batchTestGooseMu.Unlock()

	return conn, db.New(conn)
}

// ---------- mocks ----------

type mockIndexer struct {
	result *ParseResult
	err    error
}

func (m *mockIndexer) BuildTree(_ context.Context, _ string, _ IndexOptions) (*IndexResult, error) {
	return nil, errors.New("not used in fast-ingest path")
}

func (m *mockIndexer) ParseDocument(_ context.Context, _ string, _ ParseOptions) (*ParseResult, error) {
	return m.result, m.err
}

type mockKBService struct {
	err error
}

func (m *mockKBService) AddPaper(_ context.Context, _ Paper, _ PaperTree) error  { return m.err }
func (m *mockKBService) GetPaper(_ context.Context, _ string) (*Paper, error)    { return nil, nil }
func (m *mockKBService) GetTree(_ context.Context, _ string) (*PaperTree, error) { return nil, nil }
func (m *mockKBService) ListPapers(_ context.Context, _, _ int) ([]Paper, error) { return nil, nil }
func (m *mockKBService) SearchByTitle(_ context.Context, _ string, _ int) ([]Paper, error) {
	return nil, nil
}
func (m *mockKBService) RemovePaper(_ context.Context, _ string) error { return nil }
func (m *mockKBService) GetNodeSummaries(_ context.Context, _ string) ([]NodeSummary, error) {
	return nil, nil
}
func (m *mockKBService) CountPapers(_ context.Context) (int64, error)              { return 0, nil }
func (m *mockKBService) FindByDOI(_ context.Context, _ string) (*Paper, error)     { return nil, nil }
func (m *mockKBService) FindByArxivID(_ context.Context, _ string) (*Paper, error) { return nil, nil }
func (m *mockKBService) CrossPaperSearch(_ context.Context, _ LLMCaller, _ string, _ int) (*CrossSearchResult, error) {
	return nil, nil
}
func (m *mockKBService) AddParsedPaper(_ context.Context, paper Paper, _ ParseResult, _ string) (PaperIndexState, error) {
	return PaperIndexState{PaperID: paper.PaperID, IndexLevel: IndexLevelSimpleFullText, RawParseStatus: "ready", FTSStatus: "ready"}, m.err
}

// ---------- helper ----------

func insertSimpleJob(t *testing.T, q db.Querier) db.BatchJob {
	t.Helper()
	ctx := context.Background()
	jobID := uuid.New().String()
	now := time.Now().Unix()
	require.NoError(t, q.InsertBatchJob(ctx, db.InsertBatchJobParams{
		ID:        jobID,
		FilePath:  "/nonexistent/test.pdf",
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	job, err := q.GetBatchJob(ctx, jobID)
	require.NoError(t, err)
	return job
}

// ---------- tests ----------

// TestBatchJob_RetryOnFailure verifies that after a job failure the attempt count
// is incremented and the job is reset to pending (ready for next retry cycle).
func TestBatchJob_RetryOnFailure(t *testing.T) {
	_, q := setupBatchTestDB(t)
	ctx := context.Background()

	job := insertSimpleJob(t, q)

	idxr := &mockIndexer{err: errors.New("parse failure")}
	svc := &mockKBService{}
	// maxAttempts=3, so first failure should reset job to pending
	ingester := NewBatchIngester(svc, idxr, q, 1, nil, 3)

	ingester.processJob(ctx, job)

	// Allow the internal sleep to complete (1s for attempt 0)
	time.Sleep(1200 * time.Millisecond)

	updated, err := q.GetBatchJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.AttemptCount, "attempt_count should be 1 after first failure")
	assert.Equal(t, "pending", updated.Status, "job should be pending (ready for retry)")
}

// TestBatchJob_PermanentFailure verifies that after exhausting all retries
// the job ends with status=failed.
func TestBatchJob_PermanentFailure(t *testing.T) {
	conn, q := setupBatchTestDB(t)
	ctx := context.Background()

	job := insertSimpleJob(t, q)

	// Override max_attempts to 1 so one failure is fatal
	_, err := conn.ExecContext(ctx, "UPDATE batch_jobs SET max_attempts = 1 WHERE id = ?", job.ID)
	require.NoError(t, err)
	job.MaxAttempts = 1

	idxr := &mockIndexer{err: errors.New("fatal error")}
	svc := &mockKBService{}
	ingester := NewBatchIngester(svc, idxr, q, 1, nil, 1)

	ingester.processJob(ctx, job)

	updated, err := q.GetBatchJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", updated.Status, "job should be permanently failed")
	assert.Equal(t, int64(1), updated.AttemptCount)
	assert.True(t, updated.Error.Valid)
	assert.Contains(t, updated.Error.String, "fatal error")
}

// TestBatchJob_EventPublishing verifies that started, failed and permanently_failed
// events are published in the correct order when maxAttempts=1.
func TestBatchJob_EventPublishing(t *testing.T) {
	conn, q := setupBatchTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	job := insertSimpleJob(t, q)

	// Override max_attempts to 1 so one failure triggers permanently_failed
	_, err := conn.ExecContext(ctx, "UPDATE batch_jobs SET max_attempts = 1 WHERE id = ?", job.ID)
	require.NoError(t, err)
	job.MaxAttempts = 1

	broker := pubsub.NewBroker[BatchEvent]()
	defer broker.Shutdown()
	events := broker.Subscribe(ctx)

	idxr := &mockIndexer{err: errors.New("event test error")}
	svc := &mockKBService{}
	ingester := NewBatchIngester(svc, idxr, q, 1, broker, 1)

	ingester.processJob(ctx, job)

	// Collect events with a short timeout
	var received []BatchEvent
	timeout := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				break collect
			}
			received = append(received, evt.Payload)
		case <-timeout:
			break collect
		}
	}

	require.GreaterOrEqual(t, len(received), 3, "expected at least: started, failed, permanently_failed")
	assert.Equal(t, BatchEventStarted, received[0].Status, "first event should be started")
	assert.Equal(t, BatchEventFailed, received[1].Status, "second event should be failed")
	assert.Equal(t, BatchEventPermanentlyFailed, received[len(received)-1].Status, "last event should be permanently_failed")
}

// TestBatchIngester_ListFailedJobs verifies that ListFailedJobs returns only failed jobs.
func TestBatchIngester_ListFailedJobs(t *testing.T) {
	_, q := setupBatchTestDB(t)
	ctx := context.Background()

	job := insertSimpleJob(t, q)
	now := time.Now().Unix()
	require.NoError(t, q.UpdateBatchJobStatus(ctx, db.UpdateBatchJobStatusParams{
		Status:    "failed",
		Error:     toNullString("some error"),
		UpdatedAt: now,
		ID:        job.ID,
	}))

	svc := &mockKBService{}
	ingester := NewBatchIngester(svc, nil, q, 1, nil, 3)

	failed, err := ingester.ListFailedJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, job.ID, failed[0].ID)
	assert.Equal(t, "failed", failed[0].Status)
}
