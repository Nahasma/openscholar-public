package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/db"
)

type store struct {
	q db.Querier
}

func newStore(q db.Querier) *store {
	return &store{q: q}
}

func (s *store) Insert(ctx context.Context, item MemoryItem) error {
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	now := time.Now().Unix()
	metadata, _ := json.Marshal(item.Metadata)
	if item.Metadata == nil {
		metadata = []byte("{}")
	}
	history, _ := json.Marshal(item.OperationHistory)
	if item.OperationHistory == nil {
		history = []byte("[]")
	}

	return s.q.InsertMemoryItem(ctx, db.InsertMemoryItemParams{
		ID:               item.ID,
		SessionID:        sql.NullString{String: item.SessionID, Valid: item.SessionID != ""},
		Content:          item.Content,
		Metadata:         string(metadata),
		AccessCount:      0,
		OperationHistory: string(history),
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (s *store) Update(ctx context.Context, id string, content string, metadata map[string]any) error {
	metadataJSON, _ := json.Marshal(metadata)
	if metadata == nil {
		metadataJSON = []byte("{}")
	}
	return s.q.UpdateMemoryItem(ctx, db.UpdateMemoryItemParams{
		Content:  content,
		Metadata: string(metadataJSON),
		ID:       id,
	})
}

func (s *store) Delete(ctx context.Context, id string) error {
	return s.q.DeleteMemoryItem(ctx, id)
}

func (s *store) RetrieveForSession(ctx context.Context, sessionID string, limit int) ([]MemoryItem, error) {
	rows, err := s.q.ListMemoryItemsForSession(ctx, db.ListMemoryItemsForSessionParams{
		SessionID: sql.NullString{String: sessionID, Valid: sessionID != ""},
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, err
	}
	return dbRowsToMemoryItems(rows), nil
}

func (s *store) RetrieveRecent(ctx context.Context, limit int) ([]MemoryItem, error) {
	rows, err := s.q.ListRecentMemoryItems(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	return dbRowsToMemoryItems(rows), nil
}

func (s *store) RetrieveSessionMemories(ctx context.Context, sessionID string, limit int) ([]MemoryItem, error) {
	rows, err := s.q.ListSessionMemoryItems(ctx, db.ListSessionMemoryItemsParams{
		SessionID: sql.NullString{String: sessionID, Valid: sessionID != ""},
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, err
	}
	return dbRowsToMemoryItems(rows), nil
}

func (s *store) RetrieveGlobalMemories(ctx context.Context, limit int) ([]MemoryItem, error) {
	rows, err := s.q.ListGlobalMemoryItems(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	return dbRowsToMemoryItems(rows), nil
}

func dbRowsToMemoryItems(rows []db.MemoryItem) []MemoryItem {
	items := make([]MemoryItem, len(rows))
	for i, row := range rows {
		var metadata map[string]any
		_ = json.Unmarshal([]byte(row.Metadata), &metadata)
		var history []string
		_ = json.Unmarshal([]byte(row.OperationHistory), &history)

		items[i] = MemoryItem{
			ID:               row.ID,
			SessionID:        row.SessionID.String,
			Content:          row.Content,
			Metadata:         metadata,
			AccessCount:      int(row.AccessCount),
			OperationHistory: history,
			CreatedAt:        time.Unix(row.CreatedAt, 0),
			UpdatedAt:        time.Unix(row.UpdatedAt, 0),
		}
	}
	return items
}
