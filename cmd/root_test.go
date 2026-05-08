package cmd

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestSplitRootProviderModel(t *testing.T) {
	providerName, modelID, ok := splitRootProviderModel("openai:future-model")
	if !ok {
		t.Fatal("expected provider:model to parse")
	}
	if providerName != models.ProviderOpenAI || modelID != "future-model" {
		t.Fatalf("unexpected parse result: provider=%s model=%s", providerName, modelID)
	}
}

func TestSplitRootProviderModelRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"future-model", "unknown:future-model", "openai:"} {
		if _, _, ok := splitRootProviderModel(input); ok {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}
