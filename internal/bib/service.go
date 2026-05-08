package bib

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/Nahasma/openscholar-public/internal/db"
)

type Author struct {
	Name string `json:"name"`
}

type Entry struct {
	ID                string
	SessionID         string
	Title             string
	Authors           []Author
	Year              int
	Venue             string
	Abstract          string
	DOI               string
	ArxivID           string
	SemanticScholarID string
	CiteKey           string
	BibType           string // article, inproceedings, misc
}

type Service interface {
	Create(ctx context.Context, entry Entry) error
	Get(ctx context.Context, id string) (*Entry, error)
	FindByDOI(ctx context.Context, sessionID, doi string) (*Entry, error)
	FindByCiteKey(ctx context.Context, sessionID, citeKey string) (*Entry, error)
	SearchByTitle(ctx context.Context, sessionID, query string, limit int) ([]Entry, error)
	ListBySession(ctx context.Context, sessionID string) ([]Entry, error)
	Delete(ctx context.Context, id string) error
	DeleteBySession(ctx context.Context, sessionID string) error
}

type service struct {
	q db.Querier
}

func NewService(q db.Querier) Service {
	return &service{q: q}
}

func (s *service) Create(ctx context.Context, entry Entry) error {
	authorsJSON, _ := json.Marshal(entry.Authors)
	now := time.Now().Unix()
	id := entry.ID
	if id == "" {
		id = uuid.New().String()
	}
	bibType := entry.BibType
	if bibType == "" {
		bibType = "misc"
	}
	return s.q.CreateBibEntry(ctx, db.CreateBibEntryParams{
		ID:                id,
		SessionID:         entry.SessionID,
		Title:             entry.Title,
		Authors:           string(authorsJSON),
		Year:              sql.NullInt64{Int64: int64(entry.Year), Valid: entry.Year > 0},
		Venue:             sql.NullString{String: entry.Venue, Valid: entry.Venue != ""},
		Abstract:          sql.NullString{String: entry.Abstract, Valid: entry.Abstract != ""},
		Doi:               sql.NullString{String: entry.DOI, Valid: entry.DOI != ""},
		ArxivID:           sql.NullString{String: entry.ArxivID, Valid: entry.ArxivID != ""},
		SemanticScholarID: sql.NullString{String: entry.SemanticScholarID, Valid: entry.SemanticScholarID != ""},
		CiteKey:           entry.CiteKey,
		BibType:           bibType,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
}

func (s *service) Get(ctx context.Context, id string) (*Entry, error) {
	row, err := s.q.GetBibEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	return rowToEntry(row), nil
}

func (s *service) FindByDOI(ctx context.Context, sessionID, doi string) (*Entry, error) {
	row, err := s.q.GetBibEntryByDOI(ctx, db.GetBibEntryByDOIParams{
		Doi:       sql.NullString{String: doi, Valid: true},
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	return rowToEntry(row), nil
}

func (s *service) FindByCiteKey(ctx context.Context, sessionID, citeKey string) (*Entry, error) {
	row, err := s.q.GetBibEntryByCiteKey(ctx, db.GetBibEntryByCiteKeyParams{
		CiteKey:   citeKey,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	return rowToEntry(row), nil
}

func (s *service) SearchByTitle(ctx context.Context, sessionID, query string, limit int) ([]Entry, error) {
	rows, err := s.q.SearchBibEntriesByTitle(ctx, db.SearchBibEntriesByTitleParams{
		SessionID: sessionID,
		Column2:   sql.NullString{String: query, Valid: true},
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(rows))
	for i, row := range rows {
		entries[i] = *rowToEntry(row)
	}
	return entries, nil
}

func (s *service) ListBySession(ctx context.Context, sessionID string) ([]Entry, error) {
	rows, err := s.q.ListBibEntriesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(rows))
	for i, row := range rows {
		entries[i] = *rowToEntry(row)
	}
	return entries, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.q.DeleteBibEntry(ctx, id)
}

func (s *service) DeleteBySession(ctx context.Context, sessionID string) error {
	return s.q.DeleteBibEntriesBySession(ctx, sessionID)
}

func rowToEntry(row db.BibEntry) *Entry {
	var authors []Author
	json.Unmarshal([]byte(row.Authors), &authors)

	entry := &Entry{
		ID:        row.ID,
		SessionID: row.SessionID,
		Title:     row.Title,
		Authors:   authors,
		CiteKey:   row.CiteKey,
		BibType:   row.BibType,
	}
	if row.Year.Valid {
		entry.Year = int(row.Year.Int64)
	}
	if row.Venue.Valid {
		entry.Venue = row.Venue.String
	}
	if row.Abstract.Valid {
		entry.Abstract = row.Abstract.String
	}
	if row.Doi.Valid {
		entry.DOI = row.Doi.String
	}
	if row.ArxivID.Valid {
		entry.ArxivID = row.ArxivID.String
	}
	if row.SemanticScholarID.Valid {
		entry.SemanticScholarID = row.SemanticScholarID.String
	}
	return entry
}
