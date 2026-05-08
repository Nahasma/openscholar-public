package picker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexFiles_DefaultIndexesDeepFiles(t *testing.T) {
	ClearIndexCacheForTest()
	root := t.TempDir()
	deep := filepath.Join(root, "paper", "sections", "appendix", "experiments", "tables")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deep, "results.tex"), []byte("table"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries := IndexFiles(root, DefaultIndexConfig())
	if !hasEntry(entries, "paper/sections/appendix/experiments/tables/results.tex") {
		t.Fatalf("deep file was not indexed: %#v", entries)
	}
}

func TestIndexFiles_RespectsIgnoreFiles(t *testing.T) {
	ClearIndexCacheForTest()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n*.tmp\n"), 0o644); err != nil {
		t.Fatalf("write ignore: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	_ = os.WriteFile(filepath.Join(root, "ignored", "a.txt"), []byte("ignored"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "keep.tmp"), []byte("tmp"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "visible.md"), []byte("visible"), 0o644)

	entries := IndexFiles(root, DefaultIndexConfig())
	if hasEntry(entries, "ignored") || hasEntry(entries, "ignored/a.txt") {
		t.Fatalf("ignored directory appeared in index: %#v", entries)
	}
	if hasEntry(entries, "keep.tmp") {
		t.Fatalf("ignored file pattern appeared in index: %#v", entries)
	}
	if !hasEntry(entries, "visible.md") {
		t.Fatalf("visible file missing from index: %#v", entries)
	}
}

func TestIndexFiles_CacheHitAvoidsImmediateRescan(t *testing.T) {
	ClearIndexCacheForTest()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(sub, "first.md"), []byte("first"), 0o644)

	cfg := DefaultIndexConfig()
	cfg.CacheTTL = time.Minute
	first := IndexFiles(root, cfg)
	_ = os.WriteFile(filepath.Join(sub, "second.md"), []byte("second"), 0o644)
	second := IndexFiles(root, cfg)

	if IndexSignatureForTest(first) != IndexSignatureForTest(second) {
		t.Fatalf("expected second index to come from cache")
	}
	if hasEntry(second, "sub/second.md") {
		t.Fatalf("new nested file should not appear before cache expiry: %#v", second)
	}
}

func TestIndexFiles_DifferentRootsDoNotShareCache(t *testing.T) {
	ClearIndexCacheForTest()
	rootA := t.TempDir()
	rootB := t.TempDir()
	_ = os.WriteFile(filepath.Join(rootA, "a.md"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(rootB, "b.md"), []byte("b"), 0o644)

	entriesA := IndexFiles(rootA, DefaultIndexConfig())
	entriesB := IndexFiles(rootB, DefaultIndexConfig())

	if !hasEntry(entriesA, "a.md") || hasEntry(entriesA, "b.md") {
		t.Fatalf("root A index is wrong: %#v", entriesA)
	}
	if !hasEntry(entriesB, "b.md") || hasEntry(entriesB, "a.md") {
		t.Fatalf("root B index is wrong: %#v", entriesB)
	}
}

func TestIndexFiles_MaxEntriesCapsLargeDirectory(t *testing.T) {
	ClearIndexCacheForTest()
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		name := filepath.Join(root, "file-"+string(rune('a'+i%26))+".txt")
		_ = os.WriteFile(name, []byte("x"), 0o644)
	}

	cfg := DefaultIndexConfig()
	cfg.MaxEntries = 10
	entries := IndexFiles(root, cfg)
	if len(entries) > cfg.MaxEntries {
		t.Fatalf("index length = %d, want at most %d", len(entries), cfg.MaxEntries)
	}
}

func hasEntry(entries []FileEntry, rel string) bool {
	for _, entry := range entries {
		if entry.RelPath == rel {
			return true
		}
	}
	return false
}
