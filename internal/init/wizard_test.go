package initwizard

import (
	"testing"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestModelsForProviderUsesStableMiniMaxOrder(t *testing.T) {
	first := modelsForProvider(models.ProviderMiniMax)
	if len(first) < 2 {
		t.Fatalf("expected MiniMax models, got %d", len(first))
	}
	for i := 0; i < 100; i++ {
		next := modelsForProvider(models.ProviderMiniMax)
		if len(next) != len(first) {
			t.Fatalf("iteration %d length = %d, want %d", i, len(next), len(first))
		}
		for j := range first {
			if next[j].ID != first[j].ID {
				t.Fatalf("iteration %d index %d = %s, want %s", i, j, next[j].ID, first[j].ID)
			}
		}
	}
}

func TestWizardBuildConfigIncludesLayeredPathsRoot(t *testing.T) {
	m := newWizardModel(t.TempDir())
	m.defaultProvider = models.ProviderOpenAI
	m.coderModel = models.GPT41
	m.apiKeys[models.ProviderOpenAI] = "sk-test"

	m.buildConfig()

	if m.result == nil {
		t.Fatal("result config is nil")
	}
	if got := m.result.Paths.Root; got != config.DefaultDataDir() {
		t.Fatalf("paths.root = %q, want %q", got, config.DefaultDataDir())
	}
}
