package evolution

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/db"
)

var testGooseMu sync.Mutex

func setupStore(t *testing.T) *Store {
	t.Helper()
	conn := setupTestConn(t)
	return NewStore(conn)
}

func setupTestConn(t *testing.T) *sql.DB {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	conn.Exec("PRAGMA foreign_keys = ON")
	conn.Exec("PRAGMA journal_mode = WAL")

	testGooseMu.Lock()
	goose.SetBaseFS(db.FS)
	goose.SetDialect("sqlite3")
	err = goose.Up(conn, "migrations")
	testGooseMu.Unlock()
	require.NoError(t, err)

	// 插入测试会话（满足 FK 约束）
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-1', 'test', 1, 1)`)
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('s1', 'test', 1, 1)`)
	return conn
}

func TestStoreCreateCase(t *testing.T) {
	s := setupStore(t)

	c := &EvolutionCase{
		SessionID:   "sess-1",
		SkillID:     "writing/format",
		UserRequest: "写 Related Work",
		AgentOutput: "APA 格式的段落",
		Feedback:    "应该用 IEEE 格式",
	}
	err := s.CreateCase(c)
	require.NoError(t, err)
	assert.NotEmpty(t, c.ID)
	assert.Equal(t, "pending", c.Status)
	assert.Equal(t, "explicit", c.FeedbackType)
}

func TestStoreListByStatus(t *testing.T) {
	s := setupStore(t)

	// 创建 2 个 pending + 1 个 resolved
	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r1", Feedback: "f1"})
	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r2", Feedback: "f2"})

	c3 := &EvolutionCase{SessionID: "s1", UserRequest: "r3", Feedback: "f3"}
	s.CreateCase(c3)
	s.UpdateCaseStatus(c3.ID, "resolved", "fixed")

	pending, err := s.ListCasesByStatus("pending", 10)
	require.NoError(t, err)
	assert.Len(t, pending, 2)

	resolved, err := s.ListCasesByStatus("resolved", 10)
	require.NoError(t, err)
	assert.Len(t, resolved, 1)
}

func TestStoreCountPending(t *testing.T) {
	s := setupStore(t)

	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r1", Feedback: "f1"})
	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r2", Feedback: "f2"})

	count, err := s.CountPending()
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStoreDismissAll(t *testing.T) {
	s := setupStore(t)

	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r1", Feedback: "f1"})
	s.CreateCase(&EvolutionCase{SessionID: "s1", UserRequest: "r2", Feedback: "f2"})

	affected, err := s.DismissAll()
	require.NoError(t, err)
	assert.Equal(t, int64(2), affected)

	count, _ := s.CountPending()
	assert.Equal(t, 0, count)
}

func TestStoreSnapshot(t *testing.T) {
	s := setupStore(t)

	snap := &SkillSnapshot{
		SkillID: "writing/format",
		Version: 3,
		Content: "old instruction content",
		Reason:  "evolution",
	}
	err := s.CreateSnapshot(snap)
	require.NoError(t, err)
	assert.NotEmpty(t, snap.ID)

	latest, err := s.LatestSnapshot("writing/format")
	require.NoError(t, err)
	assert.Equal(t, "old instruction content", latest.Content)
	assert.Equal(t, 3, latest.Version)
}

func TestStoreSnapshotCount(t *testing.T) {
	s := setupStore(t)

	s.CreateSnapshot(&SkillSnapshot{SkillID: "s1", Version: 1, Content: "v1"})
	s.CreateSnapshot(&SkillSnapshot{SkillID: "s1", Version: 2, Content: "v2"})
	s.CreateSnapshot(&SkillSnapshot{SkillID: "s2", Version: 1, Content: "v1"})

	count, err := s.SnapshotCount("s1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStoreFeedbackCountBySkill(t *testing.T) {
	s := setupStore(t)

	s.CreateCase(&EvolutionCase{SessionID: "s1", SkillID: "writing/format", UserRequest: "r1", Feedback: "f1"})
	s.CreateCase(&EvolutionCase{SessionID: "s1", SkillID: "writing/format", UserRequest: "r2", Feedback: "f2"})
	s.CreateCase(&EvolutionCase{SessionID: "s1", SkillID: "other/skill", UserRequest: "r3", Feedback: "f3"})

	count, err := s.FeedbackCountBySkill("writing/format")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
