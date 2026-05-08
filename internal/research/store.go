package research

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Store 提供 pipelines 和 pipeline_phases 的持久化操作
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// --- Pipeline ---

func (s *Store) CreatePipeline(p *Pipeline) error {
	now := time.Now().Unix()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO pipelines (id, session_id, topic, template, mode, status, budget_limit, budget_spent, work_dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.SessionID, p.Topic, p.Template, p.Mode, p.Status,
		p.Budget.Limit, p.Budget.Spent, p.WorkDir, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *Store) GetPipeline(id string) (*Pipeline, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, topic, template, mode, status, budget_limit, budget_spent, work_dir, created_at, updated_at
		FROM pipelines WHERE id = ?`, id)
	return scanPipeline(row)
}

func (s *Store) GetPipelineBySession(sessionID string) (*Pipeline, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, topic, template, mode, status, budget_limit, budget_spent, work_dir, created_at, updated_at
		FROM pipelines WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`, sessionID)
	return scanPipeline(row)
}

func (s *Store) UpdatePipelineStatus(id string, status PipelineStatus) error {
	_, err := s.db.Exec(`UPDATE pipelines SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().Unix(), id)
	return err
}

func (s *Store) UpdatePipelineMode(id string, mode AutomationMode) error {
	_, err := s.db.Exec(`UPDATE pipelines SET mode = ?, updated_at = ? WHERE id = ?`,
		mode, time.Now().Unix(), id)
	return err
}

func (s *Store) UpdatePipelineBudget(id string, spent float64) error {
	_, err := s.db.Exec(`UPDATE pipelines SET budget_spent = ?, updated_at = ? WHERE id = ?`,
		spent, time.Now().Unix(), id)
	return err
}

// --- Phase ---

func (s *Store) CreatePhase(ph *Phase) error {
	ph.CreatedAt = time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO pipeline_phases (id, pipeline_id, name, phase_order, status, checkpoint, max_workers, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ph.ID, ph.PipelineID, ph.Name, ph.Order, ph.Status,
		boolToInt(ph.Checkpoint), ph.MaxWorkers, ph.CreatedAt,
	)
	return err
}

func (s *Store) ListPhases(pipelineID string) ([]*Phase, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_id, name, phase_order, status, checkpoint, max_workers, created_at, completed_at
		FROM pipeline_phases WHERE pipeline_id = ? ORDER BY phase_order`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var phases []*Phase
	for rows.Next() {
		ph := &Phase{}
		var checkpoint int
		var completedAt sql.NullInt64
		err := rows.Scan(&ph.ID, &ph.PipelineID, &ph.Name, &ph.Order, &ph.Status,
			&checkpoint, &ph.MaxWorkers, &ph.CreatedAt, &completedAt)
		if err != nil {
			return nil, err
		}
		ph.Checkpoint = checkpoint == 1
		if completedAt.Valid {
			ph.CompletedAt = completedAt.Int64
		}
		phases = append(phases, ph)
	}
	return phases, rows.Err()
}

func (s *Store) UpdatePhaseStatus(id string, status PhaseStatus) error {
	_, err := s.db.Exec(`UPDATE pipeline_phases SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) FailRunningPhase(id string) (bool, error) {
	res, err := s.db.Exec(`UPDATE pipeline_phases SET status = ? WHERE id = ? AND status = ?`, PhaseFailed, id, PhaseRunning)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) UpdatePhaseOrder(id string, order int) error {
	_, err := s.db.Exec(`UPDATE pipeline_phases SET phase_order = ? WHERE id = ?`, order, id)
	return err
}

func (s *Store) CompletePhase(id string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE pipeline_phases SET status = 'completed', completed_at = ? WHERE id = ?`, now, id)
	return err
}

// --- ExperimentNode (NodeStore implementation) ---

func (s *Store) CreateNode(node *ExperimentNode) error {
	metricsJSON := "{}"
	if node.Metrics != nil {
		if data, err := json.Marshal(node.Metrics); err == nil {
			metricsJSON = string(data)
		}
	}
	_, err := s.db.Exec(`
		INSERT INTO experiment_nodes (id, parent_id, pipeline_id, node_type, status, description, code_path, metrics, error_log, score, depth, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, node.ParentID, node.PipelineID, node.Type, node.Status,
		node.Description, node.CodePath, metricsJSON, node.ErrorLog,
		node.Score, node.Depth, node.CreatedAt,
	)
	return err
}

func (s *Store) UpdateNode(node *ExperimentNode) error {
	metricsJSON := "{}"
	if node.Metrics != nil {
		if data, err := json.Marshal(node.Metrics); err == nil {
			metricsJSON = string(data)
		}
	}
	_, err := s.db.Exec(`
		UPDATE experiment_nodes SET status = ?, metrics = ?, error_log = ?, score = ? WHERE id = ?`,
		node.Status, metricsJSON, node.ErrorLog, node.Score, node.ID,
	)
	return err
}

func (s *Store) GetNodesByPipeline(pipelineID string) ([]*ExperimentNode, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, pipeline_id, node_type, status, description, code_path, metrics, error_log, score, depth, created_at
		FROM experiment_nodes WHERE pipeline_id = ? ORDER BY created_at`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*ExperimentNode
	for rows.Next() {
		n := &ExperimentNode{}
		var parentID sql.NullString
		var metricsJSON sql.NullString
		var description, codePath, errorLog sql.NullString
		err := rows.Scan(&n.ID, &parentID, &n.PipelineID, &n.Type, &n.Status,
			&description, &codePath, &metricsJSON, &errorLog, &n.Score, &n.Depth, &n.CreatedAt)
		if err != nil {
			return nil, err
		}
		if parentID.Valid {
			n.ParentID = parentID.String
		}
		if description.Valid {
			n.Description = description.String
		}
		if codePath.Valid {
			n.CodePath = codePath.String
		}
		if errorLog.Valid {
			n.ErrorLog = errorLog.String
		}
		n.Metrics = make(map[string]float64)
		if metricsJSON.Valid && metricsJSON.String != "" {
			json.Unmarshal([]byte(metricsJSON.String), &n.Metrics)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) GetNodeByID(id string) (*ExperimentNode, error) {
	n := &ExperimentNode{}
	var parentID sql.NullString
	var metricsJSON sql.NullString
	var description, codePath, errorLog sql.NullString
	err := s.db.QueryRow(`
		SELECT id, parent_id, pipeline_id, node_type, status, description, code_path, metrics, error_log, score, depth, created_at
		FROM experiment_nodes WHERE id = ?`, id).
		Scan(&n.ID, &parentID, &n.PipelineID, &n.Type, &n.Status,
			&description, &codePath, &metricsJSON, &errorLog, &n.Score, &n.Depth, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		n.ParentID = parentID.String
	}
	if description.Valid {
		n.Description = description.String
	}
	if codePath.Valid {
		n.CodePath = codePath.String
	}
	if errorLog.Valid {
		n.ErrorLog = errorLog.String
	}
	n.Metrics = make(map[string]float64)
	if metricsJSON.Valid && metricsJSON.String != "" {
		json.Unmarshal([]byte(metricsJSON.String), &n.Metrics)
	}
	return n, nil
}

// --- PipelineEvent ---

func (s *Store) CreatePipelineEvent(evt *PipelineEvent) error {
	if evt.CreatedAt == 0 {
		evt.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`
		INSERT INTO pipeline_events (pipeline_id, event_type, payload, created_at)
		VALUES (?, ?, ?, ?)`,
		evt.PipelineID, evt.EventType, evt.Payload, evt.CreatedAt,
	)
	return err
}

func (s *Store) ListRecentEvents(pipelineID string, limit int) ([]*PipelineEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_id, event_type, payload, created_at
		FROM pipeline_events WHERE pipeline_id = ?
		ORDER BY created_at DESC LIMIT ?`, pipelineID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPipelineEvents(rows)
}

func (s *Store) ListEventsByType(pipelineID, eventType string) ([]*PipelineEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_id, event_type, payload, created_at
		FROM pipeline_events WHERE pipeline_id = ? AND event_type = ?
		ORDER BY created_at`, pipelineID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPipelineEvents(rows)
}

func scanPipelineEvents(rows *sql.Rows) ([]*PipelineEvent, error) {
	var events []*PipelineEvent
	for rows.Next() {
		evt := &PipelineEvent{}
		var payload sql.NullString
		if err := rows.Scan(&evt.ID, &evt.PipelineID, &evt.EventType, &payload, &evt.CreatedAt); err != nil {
			return nil, err
		}
		if payload.Valid {
			evt.Payload = payload.String
		} else {
			evt.Payload = `{}`
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}

// --- helpers ---

func scanPipeline(row *sql.Row) (*Pipeline, error) {
	p := &Pipeline{}
	err := row.Scan(&p.ID, &p.SessionID, &p.Topic, &p.Template, &p.Mode, &p.Status,
		&p.Budget.Limit, &p.Budget.Spent, &p.WorkDir, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
