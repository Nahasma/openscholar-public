package kb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/db"
)

type KBHealthOptions struct {
	LeaseStaleSeconds int64 `json:"lease_stale_seconds,omitempty"`
}

type KBHealthCheck struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Count   int      `json:"count"`
	Details []string `json:"details,omitempty"`
}

type KBHealthReport struct {
	GeneratedAt int64           `json:"generated_at"`
	Checks      []KBHealthCheck `json:"checks"`
}

func (s *service) KBHealth(ctx context.Context, opts KBHealthOptions) (*KBHealthReport, error) {
	if s.db == nil {
		return nil, fmt.Errorf("raw db is not available")
	}
	leaseSec := opts.LeaseStaleSeconds
	if leaseSec <= 0 {
		leaseSec = 900
	}
	now := time.Now().Unix()
	report := &KBHealthReport{GeneratedAt: now, Checks: make([]KBHealthCheck, 0, 5)}

	papers, err := s.q.ListPapers(ctx, db.ListPapersParams{Limit: 100000, Offset: 0})
	if err != nil {
		return nil, fmt.Errorf("list papers: %w", err)
	}

	var noChunks []string
	var missingStates []string
	var ftsStale []string
	var semanticMismatch []string
	for _, paper := range papers {
		chunkCount, cErr := s.q.CountPaperChunks(ctx, paper.PaperID)
		if cErr != nil {
			return nil, fmt.Errorf("count chunks for %s: %w", paper.PaperID, cErr)
		}
		if chunkCount == 0 {
			noChunks = append(noChunks, paper.PaperID)
		}

		st, stateErr := s.q.GetPaperIndexState(ctx, paper.PaperID)
		if stateErr != nil {
			if stateErr == sql.ErrNoRows {
				missingStates = append(missingStates, paper.PaperID)
				continue
			}
			return nil, fmt.Errorf("load paper index state for %s: %w", paper.PaperID, stateErr)
		}
		if chunkCount > 0 && !strings.EqualFold(st.FtsStatus.String, "ready") {
			ftsStale = append(ftsStale, paper.PaperID+":status="+st.FtsStatus.String)
		} else if strings.EqualFold(st.FtsStatus.String, "ready") && chunkCount > 0 {
			var chunkBytes int64
			if bErr := s.db.QueryRowContext(ctx, `SELECT CAST(COALESCE(SUM(LENGTH(COALESCE(content,''))),0) AS INTEGER) FROM paper_chunks WHERE paper_id = ?`, paper.PaperID).Scan(&chunkBytes); bErr != nil {
				ftsStale = append(ftsStale, paper.PaperID+":chunk-bytes-error="+bErr.Error())
			} else if chunkBytes > 0 {
				var ftsBytes int64
				if qErr := s.db.QueryRowContext(ctx, `SELECT CAST(COALESCE(SUM(LENGTH(COALESCE(content,''))),0) AS INTEGER) FROM paper_chunks_fts WHERE paper_id = ?`, paper.PaperID).Scan(&ftsBytes); qErr != nil {
					ftsStale = append(ftsStale, paper.PaperID+":fts-bytes-error="+qErr.Error())
				} else {
					if ftsBytes == 0 || ftsBytes+2048 < chunkBytes {
						ftsStale = append(ftsStale, paper.PaperID)
					}
				}
			}
		}
		if strings.EqualFold(st.SemanticTreeStatus.String, "ready") {
			_, tErr := s.GetTree(ctx, paper.PaperID)
			if tErr != nil {
				semanticMismatch = append(semanticMismatch, paper.PaperID)
			}
		}
	}

	report.Checks = append(report.Checks,
		healthCheck("papers_without_chunks", noChunks),
		healthCheck("paper_index_states_missing", missingStates),
		healthCheck("chunks_fts_missing_or_stale", ftsStale),
		healthCheck("semantic_tree_state_mismatch", semanticMismatch),
	)

	stuck, stale, err := s.scanStaleTasks(ctx, now, leaseSec)
	if err != nil {
		report.Checks = append(report.Checks, KBHealthCheck{Name: "stuck_or_stale_tasks", Status: "error", Details: []string{err.Error()}})
	} else {
		details := make([]string, 0, len(stuck)+len(stale))
		details = append(details, stuck...)
		details = append(details, stale...)
		report.Checks = append(report.Checks, KBHealthCheck{Name: "stuck_or_stale_tasks", Status: healthStatus(len(details)), Count: len(details), Details: details})
	}

	return report, nil
}

func healthCheck(name string, details []string) KBHealthCheck {
	return KBHealthCheck{Name: name, Status: healthStatus(len(details)), Count: len(details), Details: details}
}

func healthStatus(count int) string {
	if count == 0 {
		return "ok"
	}
	return "warn"
}

func (s *service) scanStaleTasks(ctx context.Context, now int64, leaseStaleSeconds int64) ([]string, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT task_id, status, lease_until, updated_at FROM kb_tasks WHERE status IN ('running','queued')`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	stuck := make([]string, 0)
	stale := make([]string, 0)
	for rows.Next() {
		var taskID, status string
		var leaseUntil, updatedAt sql.NullInt64
		if err := rows.Scan(&taskID, &status, &leaseUntil, &updatedAt); err != nil {
			return nil, nil, err
		}
		if status == "running" {
			if leaseUntil.Valid && leaseUntil.Int64 < now {
				stale = append(stale, taskID)
			} else if updatedAt.Valid && updatedAt.Int64 < now-leaseStaleSeconds {
				stuck = append(stuck, taskID)
			}
		}
	}
	return stuck, stale, rows.Err()
}
