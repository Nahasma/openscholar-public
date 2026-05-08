package agent

import (
	"sync"
	"testing"
)

func TestCostTrackerV2_EmptyState(t *testing.T) {
	ct := NewCostTrackerV2()

	if ct.TotalCost() != 0 {
		t.Errorf("expected TotalCost=0, got %f", ct.TotalCost())
	}
	if ct.TotalPromptTokens() != 0 {
		t.Errorf("expected TotalPromptTokens=0, got %d", ct.TotalPromptTokens())
	}
	if ct.EntryCount() != 0 {
		t.Errorf("expected EntryCount=0, got %d", ct.EntryCount())
	}
	entries := ct.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestCostTrackerV2_SingleRecord(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("claude-3-5-sonnet", "main", 100, 50, 20, 10, 0.005)

	if ct.EntryCount() != 1 {
		t.Errorf("expected EntryCount=1, got %d", ct.EntryCount())
	}
	if ct.TotalCost() != 0.005 {
		t.Errorf("expected TotalCost=0.005, got %f", ct.TotalCost())
	}

	// PromptEffective = input(100) + cacheRead(20) + cacheWrite(10) = 130
	if ct.TotalPromptTokens() != 130 {
		t.Errorf("expected TotalPromptTokens=130, got %d", ct.TotalPromptTokens())
	}
}

func TestCostTrackerV2_MultipleRecordsSameModel(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("gpt-4o", "main", 200, 100, 0, 0, 0.01)
	ct.Record("gpt-4o", "compact", 300, 150, 50, 0, 0.02)

	if ct.EntryCount() != 2 {
		t.Errorf("expected EntryCount=2, got %d", ct.EntryCount())
	}

	snap := ct.Snapshot("session-abc")
	if snap.SessionID != "session-abc" {
		t.Errorf("unexpected SessionID: %s", snap.SessionID)
	}

	tot := snap.Totals
	if tot.InputTokens != 500 {
		t.Errorf("expected InputTokens=500, got %d", tot.InputTokens)
	}
	if tot.OutputTokens != 250 {
		t.Errorf("expected OutputTokens=250, got %d", tot.OutputTokens)
	}
	if tot.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens=50, got %d", tot.CacheReadTokens)
	}
	// PromptEffective = 200+0+0 + 300+50+0 = 550
	if tot.PromptEffective != 550 {
		t.Errorf("expected PromptEffective=550, got %d", tot.PromptEffective)
	}
	// TotalTokens = 550 + 250 = 800
	if tot.TotalTokens != 800 {
		t.Errorf("expected TotalTokens=800, got %d", tot.TotalTokens)
	}
	if tot.CostUSD != 0.03 {
		t.Errorf("expected CostUSD=0.03, got %f", tot.CostUSD)
	}

	// One model entry
	if len(snap.ByModel) != 1 {
		t.Errorf("expected 1 model in ByModel, got %d", len(snap.ByModel))
	}
	mv := snap.ByModel["gpt-4o"]
	if mv == nil {
		t.Fatal("expected gpt-4o in ByModel")
	}
	if mv.InputTokens != 500 {
		t.Errorf("model InputTokens: expected 500, got %d", mv.InputTokens)
	}
}

func TestCostTrackerV2_MultipleModels(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("model-a", "main", 100, 50, 0, 0, 0.01)
	ct.Record("model-b", "sub-agent", 200, 100, 0, 0, 0.02)

	snap := ct.Snapshot("s1")
	if len(snap.ByModel) != 2 {
		t.Errorf("expected 2 models, got %d", len(snap.ByModel))
	}
	if snap.ByModel["model-a"] == nil || snap.ByModel["model-b"] == nil {
		t.Error("missing model entries in snapshot")
	}
}

func TestCostTrackerV2_SnapshotIsDeepCopy(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("model-x", "main", 100, 50, 0, 0, 0.01)

	snap := ct.Snapshot("s2")

	// Mutate the snapshot — should not affect the tracker
	snap.Totals.CostUSD = 999
	snap.ByModel["model-x"].CostUSD = 999

	if ct.TotalCost() != 0.01 {
		t.Errorf("snapshot mutation affected tracker: TotalCost=%f", ct.TotalCost())
	}
}

func TestCostTrackerV2_LegacySnapshot(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("claude-3-haiku", "main", 100, 80, 10, 5, 0.003)

	legacy := ct.LegacySnapshot()

	if legacy.Total != 0.003 {
		t.Errorf("legacy Total: expected 0.003, got %f", legacy.Total)
	}
	if len(legacy.ByModel) != 1 {
		t.Errorf("expected 1 model in legacy, got %d", len(legacy.ByModel))
	}
	mu := legacy.ByModel["claude-3-haiku"]
	if mu == nil {
		t.Fatal("missing claude-3-haiku in legacy ByModel")
	}
	if mu.InputTokens != 100 {
		t.Errorf("legacy InputTokens: expected 100, got %d", mu.InputTokens)
	}
	if mu.OutputTokens != 80 {
		t.Errorf("legacy OutputTokens: expected 80, got %d", mu.OutputTokens)
	}
	if mu.CacheReadTokens != 10 {
		t.Errorf("legacy CacheReadTokens: expected 10, got %d", mu.CacheReadTokens)
	}
	if mu.CacheWriteTokens != 5 {
		t.Errorf("legacy CacheWriteTokens: expected 5, got %d", mu.CacheWriteTokens)
	}
	if mu.Cost != 0.003 {
		t.Errorf("legacy Cost: expected 0.003, got %f", mu.Cost)
	}
}

func TestCostTrackerV2_EntriesCopy(t *testing.T) {
	ct := NewCostTrackerV2()
	ct.Record("model-z", "main", 50, 25, 0, 0, 0.001)

	entries := ct.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Mutate returned slice — should not affect tracker
	entries[0].CostUSD = 9999
	entries = append(entries, UsageEntry{Model: "extra"})

	if ct.EntryCount() != 1 {
		t.Errorf("entries mutation changed EntryCount: %d", ct.EntryCount())
	}
	inner := ct.Entries()
	if inner[0].CostUSD != 0.001 {
		t.Errorf("entries mutation changed stored entry: %f", inner[0].CostUSD)
	}
}

func TestUsageEntry_PromptEffective(t *testing.T) {
	e := UsageEntry{
		InputTokens:  100,
		CacheRead:    20,
		CacheWrite:   10,
		OutputTokens: 50,
	}
	if e.PromptEffective() != 130 {
		t.Errorf("expected PromptEffective=130, got %d", e.PromptEffective())
	}
}

func TestCostTrackerV2_ConcurrentAccess(t *testing.T) {
	ct := NewCostTrackerV2()
	var wg sync.WaitGroup
	const goroutines = 20
	const recordsEach = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			model := "model-concurrent"
			for j := 0; j < recordsEach; j++ {
				ct.Record(model, "main", 1, 1, 0, 0, 0.0001)
			}
		}(i)
	}
	wg.Wait()

	expected := int64(goroutines * recordsEach)
	if ct.EntryCount() != int(expected) {
		t.Errorf("expected EntryCount=%d, got %d", expected, ct.EntryCount())
	}
	snap := ct.Snapshot("concurrent-session")
	if snap.Totals.InputTokens != expected {
		t.Errorf("expected InputTokens=%d, got %d", expected, snap.Totals.InputTokens)
	}
}
