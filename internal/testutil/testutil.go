package testutil

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TempProjectDir creates a temporary directory with .openscholar/ initialized.
func TempProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".openscholar"), 0o755)
	return dir
}

// InMemoryDB creates an in-memory SQLite database with WAL mode.
// Caller must defer db.Close().
func InMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	db.Exec("PRAGMA journal_mode=WAL")
	return db
}
