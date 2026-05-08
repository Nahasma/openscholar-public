package research

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceSaveLoad(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".citations"), 0o755)

	evidence := []Evidence{
		{Claim: "Transformer outperforms GNN", PaperID: "paper-1", Page: 5, Quote: "our model...", Verified: true},
		{Claim: "Attention is all you need", PaperID: "paper-2", Page: 1, Quote: "we propose...", Verified: false},
	}

	err := SaveEvidence(workDir, evidence)
	require.NoError(t, err)

	loaded, err := LoadEvidence(workDir)
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
	assert.Equal(t, "Transformer outperforms GNN", loaded[0].Claim)
	assert.True(t, loaded[0].Verified)
}

func TestEvidenceAppend(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".citations"), 0o755)

	// 写入第一批
	SaveEvidence(workDir, []Evidence{{Claim: "claim1", PaperID: "p1", Verified: true}})
	// 追加第二批
	SaveEvidence(workDir, []Evidence{{Claim: "claim2", PaperID: "p2", Verified: false}})

	loaded, _ := LoadEvidence(workDir)
	assert.Len(t, loaded, 2)
}

func TestCoverageRate(t *testing.T) {
	evidence := []Evidence{
		{Verified: true}, {Verified: true}, {Verified: true}, {Verified: true},
		{Verified: true}, {Verified: true}, {Verified: true}, {Verified: true},
		{Verified: false}, {Verified: false},
	}
	rate := CoverageRate(evidence)
	assert.InDelta(t, 0.8, rate, 0.001)
}

func TestCoverageRate_Empty(t *testing.T) {
	rate := CoverageRate(nil)
	assert.Equal(t, float64(0), rate)
}

func TestLoadEvidence_NotExist(t *testing.T) {
	evidence, err := LoadEvidence(t.TempDir())
	assert.NoError(t, err)
	assert.Nil(t, evidence)
}
