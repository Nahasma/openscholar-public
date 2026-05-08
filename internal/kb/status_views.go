package kb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Nahasma/openscholar-public/internal/db"
)

// PaperListItem is the bulk KB list projection used to avoid per-paper status queries.
type PaperListItem struct {
	Paper Paper
	State PaperIndexState
	Task  *KBTask
}

// ListPapersWithStatusView returns papers with index state and latest/active task in one SQL query.
func (s *service) ListPapersWithStatusView(ctx context.Context, limit, offset int) ([]PaperListItem, error) {
	rows, err := s.q.ListPapersWithIndexStateAndTask(ctx, db.ListPapersWithIndexStateAndTaskParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]PaperListItem, 0, len(rows))
	for _, row := range rows {
		item := PaperListItem{
			Paper: Paper{
				PaperID:   row.PaperID,
				Title:     row.Title,
				Year:      int(row.Year.Int64),
				Venue:     row.Venue.String,
				Abstract:  row.Abstract.String,
				DOI:       row.Doi.String,
				ArxivID:   row.ArxivID.String,
				PDFPath:   row.PdfPath.String,
				FilePath:  row.FilePath.String,
				DocType:   row.DocType.String,
				IndexedAt: row.IndexedAt.Int64,
			},
			State: PaperIndexState{
				PaperID:                row.PaperID,
				IndexLevel:             row.IndexLevel.String,
				RawParseStatus:         row.RawParseStatus.String,
				ContentAvailable:       row.ContentAvailable.Int64 > 0,
				ContentBytes:           int(row.ContentBytes.Int64),
				FlatPageIndexAvailable: row.FlatPageIndexAvailable.Int64 > 0,
				FTSStatus:              row.FtsStatus.String,
				SemanticTreeStatus:     row.SemanticTreeStatus.String,
				SemanticTreeAvailable:  row.SemanticTreeAvailable.Int64 > 0,
				SemanticTreeError:      row.SemanticTreeError.String,
				SemanticTreeTaskID:     row.SemanticTreeTaskID.String,
				Extractor:              row.Extractor.String,
				FallbackReason:         row.FallbackReason.String,
				TotalPages:             int(row.TotalPages.Int64),
				TotalTokens:            int(row.TotalTokens.Int64),
				SourceFileAvailable:    row.SourceFileAvailable.Int64 > 0,
				FileHash:               row.FileHash.String,
				UpdatedAt:              row.StateUpdatedAt.Int64,
			},
		}
		if row.Authors.Valid {
			item.Paper.Authors = parseAuthorsJSON(row.Authors.String)
		}
		if taskID := interfaceString(row.TaskID); taskID != "" {
			item.Task = &KBTask{
				TaskID:    taskID,
				PaperID:   row.PaperID,
				TaskType:  interfaceString(row.TaskType),
				Status:    interfaceString(row.TaskStatus),
				Progress:  interfaceFloat(row.TaskProgress),
				Error:     interfaceString(row.TaskError),
				UpdatedAt: interfaceInt(row.TaskUpdatedAt),
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// GetPaperIndexStateView returns durable index status for a paper.
func (s *service) GetPaperIndexStateView(ctx context.Context, paperID string) (PaperIndexState, error) {
	row, err := s.q.GetPaperIndexState(ctx, paperID)
	if err != nil {
		return PaperIndexState{}, err
	}
	return PaperIndexState{
		PaperID:                row.PaperID,
		IndexLevel:             row.IndexLevel.String,
		RawParseStatus:         row.RawParseStatus.String,
		ContentAvailable:       row.ContentAvailable > 0,
		ContentBytes:           int(row.ContentBytes),
		FlatPageIndexAvailable: row.FlatPageIndexAvailable > 0,
		FTSStatus:              row.FtsStatus.String,
		SemanticTreeStatus:     row.SemanticTreeStatus.String,
		SemanticTreeAvailable:  row.SemanticTreeAvailable > 0,
		SemanticTreeError:      row.SemanticTreeError.String,
		SemanticTreeTaskID:     row.SemanticTreeTaskID.String,
		Extractor:              row.Extractor.String,
		FallbackReason:         row.FallbackReason.String,
		TotalPages:             int(row.TotalPages.Int64),
		TotalTokens:            int(row.TotalTokens.Int64),
		SourceFileAvailable:    row.SourceFileAvailable.Int64 > 0,
		FileHash:               row.FileHash.String,
		UpdatedAt:              row.UpdatedAt,
	}, nil
}

// GetPaperChunksView returns ordered raw chunks, optionally limited.
func (s *service) GetPaperChunksView(ctx context.Context, paperID string, limit int) ([]PaperChunk, error) {
	rows, err := s.q.GetPaperChunks(ctx, paperID)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]PaperChunk, 0, len(rows))
	for _, row := range rows {
		out = append(out, PaperChunk{
			PaperID:    row.PaperID,
			ChunkID:    row.ChunkID,
			Kind:       row.Kind,
			PageStart:  int(row.PageStart.Int64),
			PageEnd:    int(row.PageEnd.Int64),
			Title:      row.Title.String,
			Content:    row.Content,
			TokenCount: int(row.TokenCount.Int64),
			Source:     row.Source.String,
			CreatedAt:  row.CreatedAt,
		})
	}
	return out, nil
}

func parseAuthorsJSON(raw string) []Author {
	var authors []Author
	_ = json.Unmarshal([]byte(raw), &authors)
	return authors
}

func interfaceString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func interfaceFloat(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case []byte:
		f, _ := strconv.ParseFloat(string(x), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

func interfaceInt(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case int64:
		return x
	case int:
		return int64(x)
	case []byte:
		i, _ := strconv.ParseInt(string(x), 10, 64)
		return i
	case string:
		i, _ := strconv.ParseInt(x, 10, 64)
		return i
	default:
		return 0
	}
}

// ListPaperTasksView returns most recent KB tasks linked to a paper.
func (s *service) ListPaperTasksView(ctx context.Context, paperID string, limit int) ([]KBTask, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.q.ListKBTasksByPaper(ctx, db.ListKBTasksByPaperParams{
		PaperID: sql.NullString{String: paperID, Valid: paperID != ""},
		Limit:   int64(limit),
		Offset:  0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]KBTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, KBTask{
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
		})
	}
	return out, nil
}
