package kb_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/db"
)

func TestMigration030BackfillsLegacyContentWithoutDuplicateFTSRows(t *testing.T) {
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "legacy.db"))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	goose.SetBaseFS(db.FS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 29))

	_, err = conn.Exec(`
		INSERT INTO papers (paper_id, title, indexed_at) VALUES ('legacy-p', 'Legacy', 1);
		INSERT INTO paper_trees (paper_id, tree_json, model_used, index_metadata)
		VALUES ('legacy-p', '{"doc_name":"Legacy","structure":[{"node_id":"page_1","title":"Page 1","start_index":1,"end_index":1}]}', 'simple_extract', '{"extractor":"simple_extract"}');
		INSERT INTO node_summaries (paper_id, node_id, title, start_page, end_page, summary)
		VALUES ('legacy-p', 'page_1', 'Page 1', 1, 1, 'legacy summary');
		INSERT INTO node_contents (paper_id, node_id, content, token_count, source)
		VALUES ('legacy-p', 'page_1', 'legacy raw content', 4, 'simple_extract');
	`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 30))

	var chunkCount, ftsCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM paper_chunks WHERE paper_id = 'legacy-p'`).Scan(&chunkCount))
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM paper_chunks_fts WHERE paper_id = 'legacy-p'`).Scan(&ftsCount))
	require.Equal(t, 1, chunkCount)
	require.Equal(t, 1, ftsCount)
}
