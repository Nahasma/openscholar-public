package agent

import "testing"

func TestCostStateTracksUnknownCost(t *testing.T) {
	cs := newCostState()
	cs.RecordWithCostKnown("custom-model", 100, 50, 0, 0, 0, false)

	snap := cs.Snapshot()
	if snap.CostKnown {
		t.Fatal("session cost should be unknown")
	}
	if snap.ByModel["custom-model"].CostKnown {
		t.Fatal("model cost should be unknown")
	}
}
