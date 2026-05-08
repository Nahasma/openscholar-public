package prompt

import (
	"strings"
	"testing"
)

// --- test helpers ---

type staticMod struct {
	name     string
	content  string
	priority int
}

func (m staticMod) Name() string    { return m.name }
func (m staticMod) Content() string { return m.content }
func (m staticMod) Priority() int   { return m.priority }

type dynamicMod struct {
	staticMod
}

func (m dynamicMod) CacheCategory() int { return CacheCategoryDynamic }

// --- BuildBlocks tests ---

func TestBuildBlocks_EmptyBuilder(t *testing.T) {
	b := NewPromptBuilder()
	blocks := b.BuildBlocks()
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestBuildBlocks_AllStatic(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"a", "Alpha", 1})
	b.Add(staticMod{"b", "Beta", 2})
	b.Add(staticMod{"c", "Gamma", 3})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (all static merged), got %d", len(blocks))
	}
	if blocks[0].IsDynamic {
		t.Error("expected IsDynamic=false for static-only block")
	}
	// All three contents should appear joined by double newline
	if !strings.Contains(blocks[0].Text, "Alpha") || !strings.Contains(blocks[0].Text, "Beta") || !strings.Contains(blocks[0].Text, "Gamma") {
		t.Errorf("unexpected block text: %q", blocks[0].Text)
	}
}

func TestBuildBlocks_AllDynamic(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(dynamicMod{staticMod{"x", "X content", 1}})
	b.Add(dynamicMod{staticMod{"y", "Y content", 2}})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (all dynamic merged), got %d", len(blocks))
	}
	if !blocks[0].IsDynamic {
		t.Error("expected IsDynamic=true for dynamic-only block")
	}
}

func TestBuildBlocks_StaticThenDynamic(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"s1", "Static content", 1})
	b.Add(dynamicMod{staticMod{"d1", "Dynamic content", 2}})

	blocks := b.BuildBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].IsDynamic {
		t.Error("first block should be static")
	}
	if !blocks[1].IsDynamic {
		t.Error("second block should be dynamic")
	}
	if blocks[0].Text != "Static content" {
		t.Errorf("unexpected static block text: %q", blocks[0].Text)
	}
	if blocks[1].Text != "Dynamic content" {
		t.Errorf("unexpected dynamic block text: %q", blocks[1].Text)
	}
}

func TestBuildBlocks_DynamicSandwichedBetweenStatic(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"s1", "Static 1", 1})
	b.Add(dynamicMod{staticMod{"d1", "Dynamic", 2}})
	b.Add(staticMod{"s2", "Static 2", 3})

	blocks := b.BuildBlocks()
	// Expected: [static, dynamic, static]
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(blocks), blocks)
	}
	if blocks[0].IsDynamic || !blocks[1].IsDynamic || blocks[2].IsDynamic {
		t.Errorf("block dynamic flags wrong: %v %v %v",
			blocks[0].IsDynamic, blocks[1].IsDynamic, blocks[2].IsDynamic)
	}
}

func TestBuildBlocks_PriorityOrdering(t *testing.T) {
	b := NewPromptBuilder()
	// Add in reverse priority order
	b.Add(staticMod{"c", "Third", 30})
	b.Add(staticMod{"a", "First", 10})
	b.Add(staticMod{"b", "Second", 20})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 merged block, got %d", len(blocks))
	}
	// Text should be in priority order: First, Second, Third
	idx1 := strings.Index(blocks[0].Text, "First")
	idx2 := strings.Index(blocks[0].Text, "Second")
	idx3 := strings.Index(blocks[0].Text, "Third")
	if !(idx1 < idx2 && idx2 < idx3) {
		t.Errorf("modules not in priority order: %q", blocks[0].Text)
	}
}

func TestBuildBlocks_EmptyModulesSkipped(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"empty", "", 1})
	b.Add(staticMod{"real", "Hello", 2})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Text != "Hello" {
		t.Errorf("expected 'Hello', got %q", blocks[0].Text)
	}
}

// TestBuildBlocks_AdjacentSameCategoryMerged verifies that multiple adjacent
// static (or dynamic) modules are concatenated into a single block.
func TestBuildBlocks_AdjacentSameCategoryMerged(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"s1", "A", 1})
	b.Add(staticMod{"s2", "B", 2})
	b.Add(dynamicMod{staticMod{"d1", "C", 3}})
	b.Add(dynamicMod{staticMod{"d2", "D", 4}})

	blocks := b.BuildBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	// First block: A and B merged
	if !strings.Contains(blocks[0].Text, "A") || !strings.Contains(blocks[0].Text, "B") {
		t.Errorf("static block should contain both A and B, got %q", blocks[0].Text)
	}
	// Second block: C and D merged
	if !strings.Contains(blocks[1].Text, "C") || !strings.Contains(blocks[1].Text, "D") {
		t.Errorf("dynamic block should contain both C and D, got %q", blocks[1].Text)
	}
}

// TestBuildBlocks_CacheKey_Static verifies static blocks get stable cache keys.
func TestBuildBlocks_CacheKey_Static(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"a", "System instructions here", 1})
	b.Add(staticMod{"b", "More instructions", 2})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	if blocks[0].CacheKey == "" {
		t.Error("expected non-empty CacheKey for static block")
	}

	// Same content should produce same key
	b2 := NewPromptBuilder()
	b2.Add(staticMod{"a", "System instructions here", 1})
	b2.Add(staticMod{"b", "More instructions", 2})
	blocks2 := b2.BuildBlocks()

	if blocks[0].CacheKey != blocks2[0].CacheKey {
		t.Errorf("same content should produce same CacheKey: %q != %q",
			blocks[0].CacheKey, blocks2[0].CacheKey)
	}
}

// TestBuildBlocks_CacheKey_Dynamic verifies dynamic blocks have empty cache keys.
func TestBuildBlocks_CacheKey_Dynamic(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(dynamicMod{staticMod{"d", "Dynamic memory content", 1}})

	blocks := b.BuildBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	if blocks[0].CacheKey != "" {
		t.Errorf("expected empty CacheKey for dynamic block, got %q", blocks[0].CacheKey)
	}
}

// TestBuildBlocks_CacheKey_StableAcrossDynamic verifies that changing dynamic
// content does not affect the static block's cache key.
func TestBuildBlocks_CacheKey_StableAcrossDynamic(t *testing.T) {
	// Build 1: static + dynamic A
	b1 := NewPromptBuilder()
	b1.Add(staticMod{"sys", "System prompt", 1})
	b1.Add(dynamicMod{staticMod{"mem", "Memory A", 2}})
	blocks1 := b1.BuildBlocks()

	// Build 2: same static + dynamic B (different content)
	b2 := NewPromptBuilder()
	b2.Add(staticMod{"sys", "System prompt", 1})
	b2.Add(dynamicMod{staticMod{"mem", "Memory B (changed)", 2}})
	blocks2 := b2.BuildBlocks()

	if len(blocks1) != 2 || len(blocks2) != 2 {
		t.Fatalf("expected 2 blocks each, got %d and %d", len(blocks1), len(blocks2))
	}

	// Static block cache keys should be the same
	if blocks1[0].CacheKey != blocks2[0].CacheKey {
		t.Errorf("static CacheKey should be stable: %q != %q",
			blocks1[0].CacheKey, blocks2[0].CacheKey)
	}

	// Dynamic block cache keys should both be empty
	if blocks1[1].CacheKey != "" || blocks2[1].CacheKey != "" {
		t.Error("dynamic blocks should have empty CacheKey")
	}
}

// TestBuild_BackwardCompat verifies that Build() is unaffected by the new code.
func TestBuild_BackwardCompat(t *testing.T) {
	b := NewPromptBuilder()
	b.Add(staticMod{"a", "Hello", 1})
	b.Add(dynamicMod{staticMod{"b", "World", 2}})

	result := b.Build()
	if result != "Hello\n\nWorld" {
		t.Errorf("Build() returned %q, want %q", result, "Hello\n\nWorld")
	}
}
