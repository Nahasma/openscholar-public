package evolution

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Store 提供 evolution_cases 和 skill_snapshots 的持久化操作
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// --- EvolutionCase ---

func (s *Store) CreateCase(c *EvolutionCase) error {
	c.ID = uuid.NewString()
	now := time.Now().Unix()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.FeedbackType == "" {
		c.FeedbackType = "explicit"
	}
	if c.Status == "" {
		c.Status = "pending"
	}
	_, err := s.db.Exec(`
		INSERT INTO evolution_cases
		(id, session_id, skill_id, user_request, agent_output, feedback, feedback_type, status, resolution, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.SessionID, c.SkillID, c.UserRequest, c.AgentOutput,
		c.Feedback, c.FeedbackType, c.Status, c.Resolution, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *Store) ListCasesByStatus(status string, limit int) ([]EvolutionCase, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, session_id, skill_id, user_request, agent_output, feedback, feedback_type, status, resolution, created_at, updated_at
		FROM evolution_cases WHERE status = ? ORDER BY created_at ASC LIMIT ?`
	rows, err := s.db.Query(query, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCases(rows)
}

func (s *Store) ListCasesBySkill(skillID string, limit int) ([]EvolutionCase, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, session_id, skill_id, user_request, agent_output, feedback, feedback_type, status, resolution, created_at, updated_at
		FROM evolution_cases WHERE skill_id = ? ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.Query(query, skillID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCases(rows)
}

func (s *Store) UpdateCaseStatus(id, status, resolution string) error {
	_, err := s.db.Exec(
		`UPDATE evolution_cases SET status = ?, resolution = ?, updated_at = ? WHERE id = ?`,
		status, resolution, time.Now().Unix(), id,
	)
	return err
}

func (s *Store) CountPending() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM evolution_cases WHERE status = 'pending'`).Scan(&count)
	return count, err
}

func (s *Store) DismissAll() (int64, error) {
	result, err := s.db.Exec(
		`UPDATE evolution_cases SET status = 'dismissed', updated_at = ? WHERE status = 'pending'`,
		time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- SkillSnapshot ---

func (s *Store) CreateSnapshot(snap *SkillSnapshot) error {
	snap.ID = uuid.NewString()
	snap.CreatedAt = time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO skill_snapshots (id, skill_id, version, content, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.SkillID, snap.Version, snap.Content, snap.Reason, snap.CreatedAt,
	)
	return err
}

func (s *Store) LatestSnapshot(skillID string) (*SkillSnapshot, error) {
	row := s.db.QueryRow(
		`SELECT id, skill_id, version, content, reason, created_at
		FROM skill_snapshots WHERE skill_id = ? ORDER BY created_at DESC LIMIT 1`,
		skillID,
	)
	var snap SkillSnapshot
	var reason sql.NullString
	err := row.Scan(&snap.ID, &snap.SkillID, &snap.Version, &snap.Content, &reason, &snap.CreatedAt)
	if err != nil {
		return nil, err
	}
	snap.Reason = reason.String
	return &snap, nil
}

func (s *Store) SnapshotCount(skillID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM skill_snapshots WHERE skill_id = ?`, skillID).Scan(&count)
	return count, err
}

// --- 统计 ---

func (s *Store) FeedbackCountBySkill(skillID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM evolution_cases WHERE skill_id = ?`, skillID).Scan(&count)
	return count, err
}

// --- helpers ---

func scanCases(rows *sql.Rows) ([]EvolutionCase, error) {
	var cases []EvolutionCase
	for rows.Next() {
		var c EvolutionCase
		var skillID, agentOutput, resolution sql.NullString
		err := rows.Scan(
			&c.ID, &c.SessionID, &skillID, &c.UserRequest, &agentOutput,
			&c.Feedback, &c.FeedbackType, &c.Status, &resolution,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		c.SkillID = skillID.String
		c.AgentOutput = agentOutput.String
		c.Resolution = resolution.String
		cases = append(cases, c)
	}
	return cases, rows.Err()
}
