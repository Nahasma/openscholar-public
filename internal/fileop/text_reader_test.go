package fileop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTextRangeAllowsRangeFromLargeFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "large.txt")
	content := strings.Repeat("x\n", 1024)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := ReadTextRange(file, 10, 3, 2000, 128)
	if err != nil {
		t.Fatalf("ReadTextRange returned error for ranged read: %v", err)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(res.Lines))
	}
	if res.FullHash != "" {
		t.Fatal("large ranged read should not force a full-file hash")
	}
	if !res.IsPartial {
		t.Fatal("large ranged read should be marked partial")
	}
}
