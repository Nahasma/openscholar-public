package kb_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

// newServiceWithRawDB returns the service and the underlying *sql.DB so tests
// can inspect table contents directly.
func newServiceWithRawDB(t *testing.T) (kb.Service, *sql.DB) {
	t.Helper()
	conn, q := testutil.SetupTestDB(t)
	return kb.NewService(q, conn), conn
}

// countRows is a helper that counts rows in a table for a given paper_id.
func countRows(t *testing.T, conn *sql.DB, table, column string, paperID string) int {
	t.Helper()
	var n int
	row := conn.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+" = ?", paperID)
	require.NoError(t, row.Scan(&n))
	return n
}

// TestAddPaper_Success verifies that all three tables receive rows after a
// successful AddPaper call (atomic commit).
func TestAddPaper_Success(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	paper := kb.Paper{
		PaperID:  "tx-success-001",
		Title:    "Transaction Success Paper",
		Authors:  []kb.Author{{Name: "Alice"}, {Name: "Bob"}},
		Year:     2024,
		Venue:    "ICML",
		Abstract: "Testing atomic commit.",
	}
	tree := kb.PaperTree{
		DocName:        "tx_success",
		DocDescription: "A paper for transaction success test",
		Structure: []kb.TreeNode{
			{NodeID: "1", Title: "Introduction", StartIndex: 1, EndIndex: 3, Summary: "Intro"},
			{NodeID: "2", Title: "Methods", StartIndex: 4, EndIndex: 8, Summary: "Methods"},
		},
	}

	err := svc.AddPaper(ctx, paper, tree)
	require.NoError(t, err)

	// All three tables must have the corresponding row.
	assert.Equal(t, 1, countRows(t, conn, "papers", "paper_id", "tx-success-001"),
		"papers table should have 1 row")
	assert.Equal(t, 1, countRows(t, conn, "paper_trees", "paper_id", "tx-success-001"),
		"paper_trees table should have 1 row")
	assert.Equal(t, 2, countRows(t, conn, "node_summaries", "paper_id", "tx-success-001"),
		"node_summaries table should have 2 rows (one per top-level node)")
}

// TestAddPaper_RollbackOnFailure verifies that when InsertPaper fails the
// transaction is rolled back and no partial data is written.
//
// Strategy: first insert a paper successfully so its paper_id occupies the PK.
// Then attempt AddPaper with the same paper_id — InsertPaper fails immediately
// (duplicate PK) and the transaction must not leave any new rows.
func TestAddPaper_RollbackOnFailure_DuplicatePaper(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	paper := kb.Paper{
		PaperID: "tx-dup-001",
		Title:   "Original Paper",
	}
	tree := kb.PaperTree{Structure: []kb.TreeNode{
		{NodeID: "1", Title: "Intro", StartIndex: 1, EndIndex: 2, Summary: "Intro"},
	}}

	// First insert succeeds.
	require.NoError(t, svc.AddPaper(ctx, paper, tree))

	// Second insert with same paper_id must fail.
	paper.Title = "Duplicate Attempt"
	err := svc.AddPaper(ctx, paper, tree)
	require.Error(t, err, "duplicate PK should cause an error")

	// The original rows must still be intact; no extra rows from the failed tx.
	assert.Equal(t, 1, countRows(t, conn, "papers", "paper_id", "tx-dup-001"),
		"papers should still have exactly 1 row")
	assert.Equal(t, 1, countRows(t, conn, "paper_trees", "paper_id", "tx-dup-001"),
		"paper_trees should still have exactly 1 row")
	assert.Equal(t, 1, countRows(t, conn, "node_summaries", "paper_id", "tx-dup-001"),
		"node_summaries should still have exactly 1 row")
}

// TestAddPaper_RollbackOnFailure_NodeSummary verifies rollback when the node
// summary step fails mid-transaction.  A tree with duplicate (paper_id,node_id)
// pairs triggers a PK violation on the second InsertNodeSummary, so both the
// papers row and the paper_trees row that were written inside the same
// transaction must be rolled back.
func TestAddPaper_RollbackOnFailure_NodeSummary(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	// Build a tree that will produce duplicate (paper_id, node_id) entries.
	// FlattenNodes uses the NodeID field directly; providing two nodes with the
	// same NodeID will trigger a PRIMARY KEY constraint violation on the second
	// InsertNodeSummary call.
	paper := kb.Paper{
		PaperID: "tx-node-fail-001",
		Title:   "Rollback Test Paper",
	}
	tree := kb.PaperTree{
		Structure: []kb.TreeNode{
			{NodeID: "dup", Title: "Node A", StartIndex: 1, EndIndex: 2, Summary: "A"},
			{NodeID: "dup", Title: "Node B", StartIndex: 3, EndIndex: 4, Summary: "B"},
		},
	}

	err := svc.AddPaper(ctx, paper, tree)
	require.Error(t, err, "duplicate node_id PK should cause an error")

	// The transaction must have been rolled back: no orphan rows in any table.
	assert.Equal(t, 0, countRows(t, conn, "papers", "paper_id", "tx-node-fail-001"),
		"papers should have 0 rows after rollback")
	assert.Equal(t, 0, countRows(t, conn, "paper_trees", "paper_id", "tx-node-fail-001"),
		"paper_trees should have 0 rows after rollback")
	assert.Equal(t, 0, countRows(t, conn, "node_summaries", "paper_id", "tx-node-fail-001"),
		"node_summaries should have 0 rows after rollback")
}
