package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: write a file with given content
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// helper: read file content
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

func TestCheckpoint_BasicCapture(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	filePath := filepath.Join(workDir, "test.txt")
	writeFile(t, filePath, "original content")

	cp, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", filePath)
	if err != nil {
		t.Fatalf("CaptureBeforeWrite: %v", err)
	}

	if !cp.ExistsBefore {
		t.Error("ExistsBefore should be true")
	}
	if cp.SnapshotPath == "" {
		t.Error("SnapshotPath should not be empty")
	}
	if cp.SHA256 == "" {
		t.Error("SHA256 should not be empty")
	}
	if cp.TurnID != "turn1" {
		t.Errorf("TurnID = %q, want turn1", cp.TurnID)
	}
	if cp.SessionID != "sess1" {
		t.Errorf("SessionID = %q, want sess1", cp.SessionID)
	}

	// Verify the snapshot file contains the original content.
	snapContent := readFile(t, cp.SnapshotPath)
	if snapContent != "original content" {
		t.Errorf("snapshot content = %q, want %q", snapContent, "original content")
	}
}

func TestCheckpoint_SameTurnSameFile_OnlyOnce(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	filePath := filepath.Join(workDir, "dup.txt")
	writeFile(t, filePath, "v1")

	cp1, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", filePath)
	if err != nil {
		t.Fatalf("first capture: %v", err)
	}

	// Modify the file.
	writeFile(t, filePath, "v2")

	// Second capture: should return the same checkpoint without updating.
	cp2, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", filePath)
	if err != nil {
		t.Fatalf("second capture: %v", err)
	}

	if cp1.ID != cp2.ID {
		t.Errorf("expected same checkpoint ID: %q vs %q", cp1.ID, cp2.ID)
	}

	// The snapshot should still hold "v1" (the first capture).
	snapContent := readFile(t, cp1.SnapshotPath)
	if snapContent != "v1" {
		t.Errorf("snapshot should hold first version, got %q", snapContent)
	}

	// ListByTurn should return exactly one entry.
	list, err := cs.ListByTurn(ctx, "sess1", "turn1")
	if err != nil {
		t.Fatalf("ListByTurn: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListByTurn count = %d, want 1", len(list))
	}
}

func TestCheckpoint_SHA256_Dedup(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	content := "identical content"
	file1 := filepath.Join(workDir, "file1.txt")
	file2 := filepath.Join(workDir, "file2.txt")
	writeFile(t, file1, content)
	writeFile(t, file2, content)

	cp1, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", file1)
	if err != nil {
		t.Fatalf("capture file1: %v", err)
	}
	cp2, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn2", file2)
	if err != nil {
		t.Fatalf("capture file2: %v", err)
	}

	if cp1.SHA256 != cp2.SHA256 {
		t.Errorf("SHA256 mismatch: %q vs %q", cp1.SHA256, cp2.SHA256)
	}
	// Both should point to the same .bak file (dedup).
	if cp1.SnapshotPath != cp2.SnapshotPath {
		t.Errorf("expected same snapshot path due to dedup: %q vs %q", cp1.SnapshotPath, cp2.SnapshotPath)
	}
}

func TestCheckpoint_FileNotExist(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	// File does not exist.
	filePath := filepath.Join(workDir, "nonexistent.txt")

	cp, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", filePath)
	if err != nil {
		t.Fatalf("CaptureBeforeWrite on non-existent file: %v", err)
	}

	if cp.ExistsBefore {
		t.Error("ExistsBefore should be false for non-existent file")
	}
	if cp.SnapshotPath != "" {
		t.Errorf("SnapshotPath should be empty, got %q", cp.SnapshotPath)
	}
}

func TestCheckpoint_RestoreTurn(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	// Capture existing file.
	existingFile := filepath.Join(workDir, "existing.txt")
	writeFile(t, existingFile, "before")

	// Capture non-existing file (will be created later).
	newFile := filepath.Join(workDir, "newfile.txt")

	_, err := cs.CaptureBeforeWrite(ctx, "sess1", "turn1", existingFile)
	if err != nil {
		t.Fatalf("capture existing: %v", err)
	}
	_, err = cs.CaptureBeforeWrite(ctx, "sess1", "turn1", newFile)
	if err != nil {
		t.Fatalf("capture new file: %v", err)
	}

	// Simulate writes.
	writeFile(t, existingFile, "after")
	writeFile(t, newFile, "created by agent")

	// Restore.
	if err := cs.RestoreTurn(ctx, "sess1", "turn1"); err != nil {
		t.Fatalf("RestoreTurn: %v", err)
	}

	// existingFile should be back to "before".
	got := readFile(t, existingFile)
	if got != "before" {
		t.Errorf("existingFile = %q, want %q", got, "before")
	}

	// newFile should not exist.
	if _, statErr := os.Stat(newFile); !os.IsNotExist(statErr) {
		t.Errorf("newFile should have been deleted after restore")
	}
}

func TestCheckpoint_LatestTurn(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	_, ok := cs.LatestTurn(ctx, "sess1")
	if ok {
		t.Error("LatestTurn should return false for empty session")
	}

	for i, turn := range []string{"turn-a", "turn-b", "turn-c"} {
		_ = i
		file := filepath.Join(workDir, turn+".txt")
		writeFile(t, file, "data")
		_, err := cs.CaptureBeforeWrite(ctx, "sess1", turn, file)
		if err != nil {
			t.Fatalf("capture %s: %v", turn, err)
		}
	}

	latest, ok := cs.LatestTurn(ctx, "sess1")
	if !ok {
		t.Fatal("LatestTurn should return true")
	}
	if latest != "turn-c" {
		t.Errorf("LatestTurn = %q, want turn-c", latest)
	}
}

func TestCheckpoint_Prune(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()
	cs := NewCheckpointStore(rootDir)
	ctx := context.Background()

	// Create 5 turns.
	turns := []string{"t1", "t2", "t3", "t4", "t5"}
	for _, turn := range turns {
		file := filepath.Join(workDir, turn+".txt")
		writeFile(t, file, "data for "+turn)
		_, err := cs.CaptureBeforeWrite(ctx, "sess1", turn, file)
		if err != nil {
			t.Fatalf("capture %s: %v", turn, err)
		}
		time.Sleep(1 * time.Millisecond) // ensure ordering
	}

	// Prune: keep only last 3.
	if err := cs.Prune(ctx, "sess1", 3); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// t1 and t2 should be gone from ListByTurn.
	for _, pruned := range []string{"t1", "t2"} {
		list, err := cs.ListByTurn(ctx, "sess1", pruned)
		if err != nil {
			t.Fatalf("ListByTurn %s: %v", pruned, err)
		}
		if len(list) != 0 {
			t.Errorf("turn %s should be pruned, got %d entries", pruned, len(list))
		}
	}

	// t3, t4, t5 should still exist.
	for _, kept := range []string{"t3", "t4", "t5"} {
		list, err := cs.ListByTurn(ctx, "sess1", kept)
		if err != nil {
			t.Fatalf("ListByTurn %s: %v", kept, err)
		}
		if len(list) == 0 {
			t.Errorf("turn %s should be kept", kept)
		}
	}
}

func TestCheckpoint_ManifestReload(t *testing.T) {
	rootDir := t.TempDir()
	workDir := t.TempDir()

	// Store 1: create checkpoints.
	cs1 := NewCheckpointStore(rootDir)
	ctx := context.Background()

	filePath := filepath.Join(workDir, "persist.txt")
	writeFile(t, filePath, "persisted content")

	cp1, err := cs1.CaptureBeforeWrite(ctx, "sess1", "turn1", filePath)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Store 2: loaded from same rootDir (simulates restart).
	cs2 := NewCheckpointStore(rootDir)

	list, err := cs2.ListByTurn(ctx, "sess1", "turn1")
	if err != nil {
		t.Fatalf("ListByTurn after reload: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("after reload: expected 1 checkpoint, got %d", len(list))
	}
	if list[0].SHA256 != cp1.SHA256 {
		t.Errorf("reloaded SHA256 = %q, want %q", list[0].SHA256, cp1.SHA256)
	}

	latest, ok := cs2.LatestTurn(ctx, "sess1")
	if !ok || latest != "turn1" {
		t.Errorf("LatestTurn after reload = %q, %v", latest, ok)
	}
}

func TestCheckpoint_ContextHelpers(t *testing.T) {
	ctx := context.Background()

	// TurnID helpers.
	if CurrentTurnID(ctx) != "" {
		t.Error("empty context should have no turn ID")
	}
	ctx2 := WithTurnID(ctx, "my-turn")
	if CurrentTurnID(ctx2) != "my-turn" {
		t.Errorf("CurrentTurnID = %q, want my-turn", CurrentTurnID(ctx2))
	}

	// CheckpointStore helpers.
	if CheckpointStoreFromContext(ctx) != nil {
		t.Error("empty context should have nil store")
	}
	cs := NewCheckpointStore(t.TempDir())
	ctx3 := WithCheckpointStore(ctx, cs)
	if CheckpointStoreFromContext(ctx3) == nil {
		t.Error("store should be retrievable from context")
	}
}
