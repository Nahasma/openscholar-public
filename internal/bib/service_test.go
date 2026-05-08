package bib_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/bib"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func setup(t *testing.T) (bib.Service, session.Service) {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	return bib.NewService(q), session.NewService(q)
}

func TestBibService_CreateAndGet(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	entry := bib.Entry{
		SessionID: sess.ID,
		Title:     "Attention Is All You Need",
		Authors:   []bib.Author{{Name: "Vaswani"}, {Name: "Shazeer"}},
		Year:      2017,
		Venue:     "NeurIPS",
		DOI:       "10.5555/3295222.3295349",
		CiteKey:   "vaswani2017attention",
		BibType:   "inproceedings",
	}
	err := bibSvc.Create(ctx, entry)
	require.NoError(t, err)

	// Get by DOI to find the entry
	got, err := bibSvc.FindByDOI(ctx, sess.ID, "10.5555/3295222.3295349")
	require.NoError(t, err)
	assert.Equal(t, "Attention Is All You Need", got.Title)
	assert.Len(t, got.Authors, 2)
	assert.Equal(t, 2017, got.Year)
	assert.Equal(t, "NeurIPS", got.Venue)
}

func TestBibService_FindByDOI(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Paper A", "10.1234/a", "paperA")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Paper B", "10.1234/b", "paperB")

	got, err := bibSvc.FindByDOI(ctx, sess.ID, "10.1234/b")
	require.NoError(t, err)
	assert.Equal(t, "Paper B", got.Title)
}

func TestBibService_FindByDOI_NotFound(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	_, err := bibSvc.FindByDOI(ctx, sess.ID, "nonexistent")
	assert.Error(t, err)
}

func TestBibService_FindByCiteKey(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "My Paper", "10.1234/x", "mykey2024")

	got, err := bibSvc.FindByCiteKey(ctx, sess.ID, "mykey2024")
	require.NoError(t, err)
	assert.Equal(t, "My Paper", got.Title)
}

func TestBibService_SearchByTitle(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Deep Learning Basics", "10.1/dl", "dl2024")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Transformer Architecture", "10.1/tf", "tf2024")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Deep Reinforcement Learning", "10.1/drl", "drl2024")

	results, err := bibSvc.SearchByTitle(ctx, sess.ID, "Deep", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestBibService_ListBySession(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess1 := testutil.CreateTestSession(t, sessions)
	sess2 := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess1.ID, "Paper 1", "10.1/p1", "p1")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess1.ID, "Paper 2", "10.1/p2", "p2")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess2.ID, "Paper 3", "10.1/p3", "p3")

	list1, err := bibSvc.ListBySession(ctx, sess1.ID)
	require.NoError(t, err)
	assert.Len(t, list1, 2)

	list2, _ := bibSvc.ListBySession(ctx, sess2.ID)
	assert.Len(t, list2, 1)
}

func TestBibService_Delete(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "To Delete", "10.1/del", "del")

	entry, _ := bibSvc.FindByCiteKey(ctx, sess.ID, "del")
	err := bibSvc.Delete(ctx, entry.ID)
	require.NoError(t, err)

	_, err = bibSvc.Get(ctx, entry.ID)
	assert.Error(t, err)
}

func TestBibService_DeleteBySession(t *testing.T) {
	bibSvc, sessions := setup(t)
	ctx := context.Background()
	sess := testutil.CreateTestSession(t, sessions)

	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Entry 1", "10.1/e1", "e1")
	testutil.CreateTestBibEntryCustom(t, bibSvc, sess.ID, "Entry 2", "10.1/e2", "e2")

	err := bibSvc.DeleteBySession(ctx, sess.ID)
	require.NoError(t, err)

	list, _ := bibSvc.ListBySession(ctx, sess.ID)
	assert.Empty(t, list)
}
