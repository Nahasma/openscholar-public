package provider

import (
	"testing"
)

func TestCacheMonitor_FirstCall(t *testing.T) {
	cm := NewCacheMonitor()
	cm.PreCall([]string{"system"}, []string{"tool1"}, "claude-3-5-sonnet")
	cm.PostCall(1000)
	// First call: no break detected (no baseline), lastCacheRead should be set
	if cm.lastCacheRead != 1000 {
		t.Errorf("expected lastCacheRead=1000, got %d", cm.lastCacheRead)
	}
	if cm.callCount != 1 {
		t.Errorf("expected callCount=1, got %d", cm.callCount)
	}
}

func TestCacheMonitor_NormalHit(t *testing.T) {
	cm := NewCacheMonitor()
	cm.PreCall([]string{"system"}, []string{"tool1"}, "claude-3-5-sonnet")
	cm.PostCall(1000)
	cm.PreCall([]string{"system"}, []string{"tool1"}, "claude-3-5-sonnet")
	cm.PostCall(950)
	// Small 5% drop — should not log (no panic/error expected)
	if cm.lastCacheRead != 950 {
		t.Errorf("expected lastCacheRead=950, got %d", cm.lastCacheRead)
	}
	if cm.callCount != 2 {
		t.Errorf("expected callCount=2, got %d", cm.callCount)
	}
}

func TestCacheMonitor_SignificantDrop(t *testing.T) {
	cm := NewCacheMonitor()
	cm.PreCall([]string{"system"}, []string{"tool1"}, "claude-3-5-sonnet")
	cm.PostCall(5000)
	cm.PreCall([]string{"system"}, []string{"tool1"}, "claude-3-5-sonnet")
	// Drop from 5000 to 100: >5% and >2000 absolute — should log warning (no panic)
	cm.PostCall(100)
	if cm.lastCacheRead != 100 {
		t.Errorf("expected lastCacheRead=100, got %d", cm.lastCacheRead)
	}
}

func TestCacheMonitor_ZeroPreviousRead(t *testing.T) {
	cm := NewCacheMonitor()
	// First call with zero cache read
	cm.PreCall([]string{"system"}, []string{}, "model")
	cm.PostCall(0)
	// Second call — prev is 0, division guard should prevent panic
	cm.PreCall([]string{"system"}, []string{}, "model")
	cm.PostCall(0)
	if cm.callCount != 2 {
		t.Errorf("expected callCount=2, got %d", cm.callCount)
	}
}

func TestCacheMonitor_MultipleCallsStable(t *testing.T) {
	cm := NewCacheMonitor()
	// Simulate a stable session — cache should remain consistent
	for i := 0; i < 5; i++ {
		cm.PreCall([]string{"static system prompt"}, []string{"ToolA", "ToolB"}, "model-x")
		cm.PostCall(8000)
	}
	if cm.callCount != 5 {
		t.Errorf("expected callCount=5, got %d", cm.callCount)
	}
	if cm.lastCacheRead != 8000 {
		t.Errorf("expected lastCacheRead=8000, got %d", cm.lastCacheRead)
	}
}

func TestCacheMonitor_UnknownCacheReadSkipsBaseline(t *testing.T) {
	cm := NewCacheMonitorWithProvider("minimax", "MiniMax-M2.7", false)
	cm.PreCall([]string{"system"}, []string{"tool1"}, "MiniMax-M2.7")
	cm.PostCall(0)
	cm.PreCall([]string{"system"}, []string{"tool1"}, "MiniMax-M2.7")
	cm.PostCall(0)

	if cm.callCount != 0 {
		t.Errorf("unknown cache reads should not participate in drop detection, got callCount=%d", cm.callCount)
	}
	if cm.lastCacheRead != 0 {
		t.Errorf("unknown cache reads should not establish a baseline, got lastCacheRead=%d", cm.lastCacheRead)
	}
	if cm.baselineValid {
		t.Error("unknown cache reads should leave the baseline invalid")
	}
}

func TestHashStrings_Consistency(t *testing.T) {
	h1 := hashStrings([]string{"a", "b", "c"})
	h2 := hashStrings([]string{"a", "b", "c"})
	if h1 != h2 {
		t.Errorf("hashStrings not deterministic: %q vs %q", h1, h2)
	}
}

func TestHashStrings_OrderSensitive(t *testing.T) {
	h1 := hashStrings([]string{"a", "b"})
	h2 := hashStrings([]string{"b", "a"})
	if h1 == h2 {
		t.Error("hashStrings should be order-sensitive but produced same hash for different orders")
	}
}
