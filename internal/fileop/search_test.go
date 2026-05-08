package fileop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunGoSearchCountMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "todo.txt")
	if err := os.WriteFile(file, []byte("TODO one\nskip\nTODO two\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := runGoSearch(SearchOptions{
		Pattern:    "TODO",
		Path:       dir,
		OutputMode: "count",
		HeadLimit:  10,
	})
	if err != nil {
		t.Fatalf("runGoSearch: %v", err)
	}
	if len(res.Lines) != 1 {
		t.Fatalf("count lines = %d, want 1: %#v", len(res.Lines), res.Lines)
	}
	if got, want := res.Lines[0], file+":2"; got != want {
		t.Fatalf("count line = %q, want %q", got, want)
	}
}

func TestLimitedBufferCapsOutput(t *testing.T) {
	buf := &limitedBuffer{limit: 4}
	n, err := buf.Write([]byte("abcdef"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 6 {
		t.Fatalf("Write count = %d, want 6", n)
	}
	if got := buf.String(); got != "abcd" {
		t.Fatalf("buffer = %q, want %q", got, "abcd")
	}
	if !buf.truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestRunGoSearchStopsAtHeadLimitAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, string(rune('a'+i))+".txt")
		if err := os.WriteFile(name, []byte("TODO\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	res, err := runGoSearch(SearchOptions{
		Pattern:    "TODO",
		Path:       dir,
		OutputMode: "content",
		HeadLimit:  2,
	})
	if err != nil {
		t.Fatalf("runGoSearch content: %v", err)
	}
	if len(res.Lines) != 2 {
		t.Fatalf("content lines = %d, want 2: %#v", len(res.Lines), res.Lines)
	}
	if !res.Truncated {
		t.Fatal("expected content search to mark truncated")
	}

	res, err = runGoSearch(SearchOptions{
		Pattern:    "TODO",
		Path:       dir,
		OutputMode: "count",
		HeadLimit:  2,
	})
	if err != nil {
		t.Fatalf("runGoSearch count: %v", err)
	}
	if len(res.Lines) != 2 {
		t.Fatalf("count lines = %d, want 2: %#v", len(res.Lines), res.Lines)
	}
	if !res.Truncated {
		t.Fatal("expected count search to mark truncated")
	}
}
