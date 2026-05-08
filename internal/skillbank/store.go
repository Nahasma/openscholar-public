package skillbank

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type skillRow struct {
	ID               string
	Name             string
	Description      string
	Category         string
	Tags             []string
	MetaJSON         string
	ResolvedSourceID string
	Version          int
	Author           string
	UsageCount       int
	SuccessCount     int
	FilePath         string
	CreatedAt        int64
	UpdatedAt        int64
	Content          string // skill body, stored in FTS only
}

type store struct {
	db *sql.DB
}

type skillSourceRow struct {
	ID          string
	SkillID     string
	SourceTier  string
	SourceKind  string
	SourceKey   string
	DisplayName string
	FilePath    string
	MetaJSON    string
	CreatedAt   int64
	UpdatedAt   int64
}

func newStore(db *sql.DB) *store {
	return &store{db: db}
}

// Upsert inserts or updates a skill in the index and FTS tables.
func (s *store) Upsert(ctx context.Context, row skillRow) error {
	tagsJSON, _ := json.Marshal(row.Tags)
	if row.Tags == nil {
		tagsJSON = []byte("[]")
	}

	// Upsert into skills_index (triggers handle skills_fts for name/desc/tags)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO skills_index (id, name, description, category, tags, meta_json, resolved_source_id, version, author, usage_count, success_count, file_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   description = excluded.description,
		   category = excluded.category,
		   tags = excluded.tags,
		   meta_json = excluded.meta_json,
		   resolved_source_id = excluded.resolved_source_id,
		   version = excluded.version,
		   author = excluded.author,
		   file_path = excluded.file_path,
		   updated_at = excluded.updated_at`,
		row.ID, row.Name, row.Description, row.Category, string(tagsJSON),
		row.MetaJSON, row.ResolvedSourceID, row.Version, row.Author, row.FilePath, row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert skills_index: %w", err)
	}

	// Also update FTS content field (triggers only copy name/desc/tags)
	_, err = s.db.ExecContext(ctx,
		`UPDATE skills_fts SET content = ? WHERE id = ?`,
		row.Content, row.ID,
	)
	if err != nil {
		return fmt.Errorf("update skills_fts content: %w", err)
	}

	return nil
}

// Get retrieves a single skill row by ID.
func (s *store) Get(ctx context.Context, id string) (*skillRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, category, tags, meta_json, resolved_source_id, version, author, usage_count, success_count, file_path, created_at, updated_at
		 FROM skills_index WHERE id = ?`, id)

	var r skillRow
	var tagsJSON string
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &tagsJSON, &r.MetaJSON, &r.ResolvedSourceID,
		&r.Version, &r.Author, &r.UsageCount, &r.SuccessCount, &r.FilePath, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
	return &r, nil
}

// ListByCategory lists skills filtered by category, ordered by usage_count DESC.
// If category is empty, returns all skills.
func (s *store) ListByCategory(ctx context.Context, category string, limit int) ([]skillRow, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows *sql.Rows
	var err error
	if category == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, description, category, tags, meta_json, resolved_source_id, version, author, usage_count, success_count, file_path, created_at, updated_at
			 FROM skills_index ORDER BY usage_count DESC LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, description, category, tags, meta_json, resolved_source_id, version, author, usage_count, success_count, file_path, created_at, updated_at
			 FROM skills_index WHERE category = ? ORDER BY usage_count DESC LIMIT ?`, category, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	return scanSkillRows(rows)
}

// Delete removes a skill from both index and FTS tables.
func (s *store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM skills_index WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	// FTS row deleted by trigger
	return nil
}

// RecordUsage increments usage and optionally success counts.
func (s *store) RecordUsage(ctx context.Context, id string, success bool) error {
	now := time.Now().Unix()
	if success {
		_, err := s.db.ExecContext(ctx,
			`UPDATE skills_index SET usage_count = usage_count + 1, success_count = success_count + 1, updated_at = ? WHERE id = ?`,
			now, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE skills_index SET usage_count = usage_count + 1, updated_at = ? WHERE id = ?`,
		now, id)
	return err
}

// DeleteNotIn removes skills whose IDs are not in the given set.
func (s *store) DeleteNotIn(ctx context.Context, activeIDs []string) error {
	if len(activeIDs) == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM skills_index`)
		return err
	}
	placeholders := strings.Repeat("?,", len(activeIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(activeIDs))
	for i, id := range activeIDs {
		args[i] = id
	}
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM skills_index WHERE id NOT IN (%s)", placeholders),
		args...)
	return err
}

func (s *store) ReplaceSkillSources(ctx context.Context, rows []skillSourceRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM skill_sources`); err != nil {
		return fmt.Errorf("clear skill_sources: %w", err)
	}

	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_sources
			(id, skill_id, source_tier, source_kind, source_key, display_name, file_path, meta_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, r.SkillID, r.SourceTier, r.SourceKind, r.SourceKey, r.DisplayName, r.FilePath, r.MetaJSON, r.CreatedAt, r.UpdatedAt); err != nil {
			return fmt.Errorf("insert skill_source %s: %w", r.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *store) ListSkillSources(ctx context.Context, skillID string) ([]skillSourceRow, error) {
	query := `SELECT id, skill_id, source_tier, source_kind, source_key, display_name, file_path, meta_json, created_at, updated_at
		FROM skill_sources`
	var rows *sql.Rows
	var err error
	if skillID == "" {
		query += ` ORDER BY skill_id, source_tier DESC, updated_at DESC`
		rows, err = s.db.QueryContext(ctx, query)
	} else {
		query += ` WHERE skill_id = ? ORDER BY source_tier DESC, updated_at DESC`
		rows, err = s.db.QueryContext(ctx, query, skillID)
	}
	if err != nil {
		return nil, fmt.Errorf("list skill_sources: %w", err)
	}
	defer rows.Close()

	var out []skillSourceRow
	for rows.Next() {
		var r skillSourceRow
		if err := rows.Scan(&r.ID, &r.SkillID, &r.SourceTier, &r.SourceKind, &r.SourceKey, &r.DisplayName, &r.FilePath, &r.MetaJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan skill_source: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanSkillRows(rows *sql.Rows) ([]skillRow, error) {
	var result []skillRow
	for rows.Next() {
		var r skillRow
		var tagsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &tagsJSON, &r.MetaJSON, &r.ResolvedSourceID,
			&r.Version, &r.Author, &r.UsageCount, &r.SuccessCount, &r.FilePath, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan skill row: %w", err)
		}
		_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
		result = append(result, r)
	}
	return result, rows.Err()
}
