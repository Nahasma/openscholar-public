package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSearchState_Reserve(t *testing.T) {
	s := NewSearchState("")

	// No limit → always succeeds
	if !s.Reserve("test", QuotaConfig{}) {
		t.Error("expected true for unlimited quota")
	}

	// Monthly limit
	s2 := NewSearchState("")
	if !s2.Reserve("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected true: 1/3 used")
	}
	if !s2.Reserve("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected true: 2/3 used")
	}
	if !s2.Reserve("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected true: 3/3 used")
	}
	if s2.Reserve("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected false: 3/3 exhausted")
	}

	// Daily limit
	s3 := NewSearchState("")
	s3.Reserve("x", QuotaConfig{DailyLimit: 1})
	if s3.Reserve("x", QuotaConfig{DailyLimit: 1}) {
		t.Error("expected false: daily limit 1 exhausted")
	}
}

func TestSearchState_HasRemaining(t *testing.T) {
	s := NewSearchState("")

	// No limit → always has remaining
	if !s.HasRemaining("test", QuotaConfig{}) {
		t.Error("expected true for unlimited quota")
	}

	// Use Reserve to increment
	s.Reserve("brave", QuotaConfig{MonthlyLimit: 3})
	s.Reserve("brave", QuotaConfig{MonthlyLimit: 3})
	if !s.HasRemaining("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected true: 2 used, limit 3")
	}
	s.Reserve("brave", QuotaConfig{MonthlyLimit: 3})
	if s.HasRemaining("brave", QuotaConfig{MonthlyLimit: 3}) {
		t.Error("expected false: 3 used, limit 3")
	}
}

func TestSearchState_InCooldown(t *testing.T) {
	s := NewSearchState("")

	if s.InCooldown("test") {
		t.Error("new backend should not be in cooldown")
	}

	s.SetCooldown("test", 1*time.Hour)
	if !s.InCooldown("test") {
		t.Error("expected cooldown active")
	}

	// RecordSuccess clears cooldown
	s.RecordSuccess("test")
	if s.InCooldown("test") {
		t.Error("RecordSuccess should clear cooldown")
	}
}

func TestSearchState_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Save
	s1 := NewSearchState(path)
	s1.Reserve("brave", QuotaConfig{MonthlyLimit: 100})
	s1.Reserve("brave", QuotaConfig{MonthlyLimit: 100})
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load
	s2 := NewSearchState(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	stats := s2.Stats()
	if ps, ok := stats["brave"]; !ok || ps.UsedToday != 2 || ps.UsedMonth != 2 {
		t.Errorf("unexpected stats after load: %+v", stats)
	}
}

func TestSearchState_LoadNonExistent(t *testing.T) {
	s := NewSearchState("/tmp/nonexistent-state-file-xxx.json")
	if err := s.Load(); err != nil {
		t.Errorf("Load non-existent file should return nil: %v", err)
	}
}

func TestSearchState_SaveEmpty(t *testing.T) {
	s := NewSearchState("")
	if err := s.Save(); err != nil {
		t.Errorf("Save with empty path should succeed: %v", err)
	}
}

func TestSearchState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := NewSearchState(path)
	s.Reserve("test", QuotaConfig{MonthlyLimit: 100})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify tmp file doesn't linger
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after successful save")
	}

	// Verify main file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file should exist: %v", err)
	}
}

func TestSearchState_Stats(t *testing.T) {
	s := NewSearchState("")
	s.Reserve("a", QuotaConfig{MonthlyLimit: 100})
	s.Reserve("b", QuotaConfig{MonthlyLimit: 100})
	s.Reserve("b", QuotaConfig{MonthlyLimit: 100})

	stats := s.Stats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(stats))
	}
	if stats["a"].UsedToday != 1 {
		t.Errorf("a.UsedToday = %d, want 1", stats["a"].UsedToday)
	}
	if stats["b"].UsedToday != 2 {
		t.Errorf("b.UsedToday = %d, want 2", stats["b"].UsedToday)
	}
}
