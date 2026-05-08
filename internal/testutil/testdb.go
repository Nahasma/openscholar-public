package testutil

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"

	"github.com/openscholar/openscholar/internal/db"
)

var gooseMu sync.Mutex

// SetupTestDB creates a temporary SQLite database with all migrations applied.
// Returns (*sql.DB, db.Querier) with automatic cleanup via t.Cleanup.
func SetupTestDB(t *testing.T) (*sql.DB, db.Querier) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
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

	// Serialize goose operations — goose uses global state
	gooseMu.Lock()
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		gooseMu.Unlock()
		t.Fatalf("failed to set dialect: %v", err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		gooseMu.Unlock()
		t.Fatalf("failed to run migrations: %v", err)
	}
	gooseMu.Unlock()

	return conn, db.New(conn)
}
