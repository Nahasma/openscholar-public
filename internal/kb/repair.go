package kb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/openscholar/openscholar/internal/db"
)

type KBRepairOptions struct {
	PaperID string `json:"paper_id,omitempty"`
	Apply   bool   `json:"apply,omitempty"`
}

type KBReindexOptions struct {
	PaperID string `json:"paper_id,omitempty"`
	Apply   bool   `json:"apply,omitempty"`
}

type KBMaintenanceReport struct {
	Operation string   `json:"operation"`
	DryRun    bool     `json:"dry_run"`
	Changed   []string `json:"changed,omitempty"`
	Skipped   []string `json:"skipped,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

func (s *service) KBRepair(ctx context.Context, opts KBRepairOptions) (*KBMaintenanceReport, error) {
	report := &KBMaintenanceReport{Operation: "repair", DryRun: !opts.Apply}
	if s.db == nil {
		return nil, fmt.Errorf("raw db is not available")
	}
	if opts.PaperID != "" {
		report.Skipped = append(report.Skipped, "repair with paper_id applies only targeted FTS/state fixes")
	}

	if !opts.Apply {
		report.Skipped = append(report.Skipped, "dry-run: no changes applied")
		return report, nil
	}
	now := time.Now().Unix()
	if opts.PaperID != "" {
		if err := s.q.RebuildPaperChunksFTSForPaper(ctx, opts.PaperID); err != nil {
			report.Errors = append(report.Errors, "rebuild paper_chunks_fts for "+opts.PaperID+": "+err.Error())
		} else {
			report.Changed = append(report.Changed, "rebuild paper_chunks_fts:"+opts.PaperID)
			_ = s.q.UpdateFTSState(ctx, db.UpdateFTSStateParams{PaperID: opts.PaperID, FtsStatus: sql.NullString{String: "ready", Valid: true}, UpdatedAt: now})
		}
	} else {
		if err := s.q.RebuildPaperChunksFTS(ctx); err != nil {
			report.Errors = append(report.Errors, "rebuild paper_chunks_fts: "+err.Error())
		} else {
			report.Changed = append(report.Changed, "rebuild paper_chunks_fts:all")
		}
	}

	if count, err := s.backfillMissingIndexStates(ctx, now); err != nil {
		report.Errors = append(report.Errors, "backfill paper_index_states: "+err.Error())
	} else if count > 0 {
		report.Changed = append(report.Changed, fmt.Sprintf("backfill paper_index_states:%d", count))
	} else {
		report.Skipped = append(report.Skipped, "paper_index_states already present")
	}

	if count, err := s.requeueStaleRunningTasks(ctx, now); err != nil {
		report.Errors = append(report.Errors, "requeue stale running tasks: "+err.Error())
	} else if count > 0 {
		report.Changed = append(report.Changed, fmt.Sprintf("requeue stale running tasks:%d", count))
	} else {
		report.Skipped = append(report.Skipped, "no stale running tasks")
	}

	return report, nil
}

func (s *service) KBReindex(ctx context.Context, opts KBReindexOptions) (*KBMaintenanceReport, error) {
	report := &KBMaintenanceReport{Operation: "reindex", DryRun: !opts.Apply}
	if s.db == nil {
		return nil, fmt.Errorf("raw db is not available")
	}
	if !opts.Apply {
		report.Skipped = append(report.Skipped, "dry-run: no changes applied")
		if opts.PaperID == "" {
			report.Skipped = append(report.Skipped, "would rebuild paper_chunks_fts for all papers and enqueue semantic-tree intents")
		} else {
			report.Skipped = append(report.Skipped, "would rebuild paper_chunks_fts and enqueue semantic-tree intent for paper "+opts.PaperID)
		}
		return report, nil
	}
	if opts.PaperID == "" {
		if err := s.q.RebuildPaperChunksFTS(ctx); err != nil {
			report.Errors = append(report.Errors, "rebuild paper_chunks_fts: "+err.Error())
		} else {
			report.Changed = append(report.Changed, "rebuild paper_chunks_fts:all")
		}
	} else {
		if err := s.q.RebuildPaperChunksFTSForPaper(ctx, opts.PaperID); err != nil {
			report.Errors = append(report.Errors, "rebuild paper_chunks_fts for "+opts.PaperID+": "+err.Error())
		} else {
			report.Changed = append(report.Changed, "rebuild paper_chunks_fts:"+opts.PaperID)
		}
	}
	if err := s.enqueueSemanticReindexIntent(ctx, opts.PaperID); err != nil {
		report.Errors = append(report.Errors, "enqueue semantic-tree intent: "+err.Error())
	} else {
		report.Changed = append(report.Changed, "enqueue semantic-tree intent")
	}
	return report, nil
}

func (s *service) backfillMissingIndexStates(ctx context.Context, now int64) (int, error) {
	rows, err := s.q.ListPapers(ctx, db.ListPapersParams{Limit: 100000, Offset: 0})
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, p := range rows {
		if _, err := s.q.GetPaperIndexState(ctx, p.PaperID); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return changed, err
		}
		chunks, err := s.q.CountPaperChunks(ctx, p.PaperID)
		if err != nil {
			return changed, err
		}
		contentAvailable := chunks > 0
		level := IndexLevelSummaryOnly
		rawStatus := "failed"
		ftsStatus := "missing"
		if contentAvailable {
			level = IndexLevelSimpleFullText
			rawStatus = "ready"
			ftsStatus = "ready"
		}
		if err := s.q.UpsertPaperIndexState(ctx, db.UpsertPaperIndexStateParams{
			PaperID:                p.PaperID,
			IndexLevel:             sql.NullString{String: level, Valid: true},
			RawParseStatus:         sql.NullString{String: rawStatus, Valid: true},
			ContentAvailable:       boolToInt64(contentAvailable),
			ContentBytes:           0,
			FlatPageIndexAvailable: 0,
			FtsStatus:              sql.NullString{String: ftsStatus, Valid: true},
			SemanticTreeStatus:     sql.NullString{String: "not_requested", Valid: true},
			SemanticTreeAvailable:  0,
			SemanticTreeError:      sql.NullString{},
			SemanticTreeTaskID:     sql.NullString{},
			Extractor:              sql.NullString{},
			FallbackReason:         sql.NullString{},
			TotalPages:             sql.NullInt64{},
			TotalTokens:            sql.NullInt64{},
			SourceFileAvailable:    sql.NullInt64{Int64: boolToInt64(p.FilePath.Valid || p.PdfPath.Valid), Valid: true},
			FileHash:               sql.NullString{},
			UpdatedAt:              now,
		}); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

func (s *service) requeueStaleRunningTasks(ctx context.Context, now int64) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT task_id FROM kb_tasks WHERE status = 'running' AND lease_until IS NOT NULL AND lease_until < ?`, now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	changed := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return changed, err
		}
		if err := s.q.RetryKBTask(ctx, db.RetryKBTaskParams{TaskID: id, UpdatedAt: now, Error: sql.NullString{String: "stale lease requeued", Valid: true}}); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, rows.Err()
}

func (s *service) enqueueSemanticReindexIntent(ctx context.Context, paperID string) error {
	if paperID != "" {
		return s.enqueueSemanticIntentForPaper(ctx, paperID)
	}
	papers, err := s.q.ListPapers(ctx, db.ListPapersParams{Limit: 100000, Offset: 0})
	if err != nil {
		return err
	}
	for _, p := range papers {
		if err := s.enqueueSemanticIntentForPaper(ctx, p.PaperID); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) enqueueSemanticIntentForPaper(ctx context.Context, paperID string) error {
	paper, err := s.GetPaper(ctx, paperID)
	if err != nil {
		return err
	}
	filePath := paper.FilePath
	if filePath == "" {
		filePath = paper.PDFPath
	}
	_, err = s.requeueSemanticTreeTask(ctx, paperID, filePath, true)
	return err
}
