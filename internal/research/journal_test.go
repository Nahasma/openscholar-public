package research

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalAppendRead(t *testing.T) {
	dir := t.TempDir()

	entry1 := &JournalEntry{
		Stage:      "preliminary",
		Hypothesis: "baseline should reach 85% accuracy",
		Metrics:    map[string]float64{"accuracy": 0.832, "loss": 0.45},
		Status:     "completed",
	}
	err := AppendEntry(dir, entry1)
	require.NoError(t, err)
	assert.Equal(t, "exp-001", entry1.ID)

	entry2 := &JournalEntry{
		Stage:      "hyperparameter",
		Hypothesis: "cosine annealing should improve convergence",
		Metrics:    map[string]float64{"accuracy": 0.867},
		Status:     "completed",
	}
	err = AppendEntry(dir, entry2)
	require.NoError(t, err)
	assert.Equal(t, "exp-002", entry2.ID)

	entries, err := ReadJournal(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "preliminary", entries[0].Stage)
	assert.Equal(t, "hyperparameter", entries[1].Stage)
	assert.InDelta(t, 0.832, entries[0].Metrics["accuracy"], 0.001)
}

func TestJournalEmpty(t *testing.T) {
	dir := t.TempDir()
	entries, err := ReadJournal(dir)
	require.NoError(t, err)
	assert.Nil(t, entries)
}

func TestJournalSummarize(t *testing.T) {
	dir := t.TempDir()

	AppendEntry(dir, &JournalEntry{
		Stage:        "preliminary",
		Hypothesis:   "test hypothesis",
		Metrics:      map[string]float64{"acc": 0.9},
		Observations: "looks good",
		Status:       "completed",
	})

	summary, err := SummarizeJournal(dir)
	require.NoError(t, err)
	assert.Contains(t, summary, "1 条实验记录")
	assert.Contains(t, summary, "test hypothesis")
	assert.Contains(t, summary, "looks good")
}

func TestJournalSummarize_Empty(t *testing.T) {
	dir := t.TempDir()
	summary, err := SummarizeJournal(dir)
	require.NoError(t, err)
	assert.Equal(t, "（无实验日志）", summary)
}

func TestAppendDebugEntry(t *testing.T) {
	dir := t.TempDir()
	err := AppendDebugEntry(dir, "node-001", 1, "ImportError: no module named torch", "pip install torch")
	require.NoError(t, err)

	entries, err := ReadJournal(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "debug", entries[0].Stage)
	assert.Equal(t, "buggy", entries[0].Status)
	assert.Contains(t, entries[0].Observations, "ImportError")
}
