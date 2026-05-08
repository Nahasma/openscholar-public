package fileop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeWrite_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "safe.txt")

	if err := SafeWrite(path, []byte("content")); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "content" {
		t.Errorf("got %q, want %q", got, "content")
	}
}

func TestSafeWrite_WithBoundary(t *testing.T) {
	dir := t.TempDir()

	// Inside boundary: ok
	if err := SafeWrite(filepath.Join(dir, "ok.txt"), []byte("ok"), WithBoundary(dir)); err != nil {
		t.Fatal(err)
	}

	// Outside boundary: blocked
	err := SafeWrite("/tmp/outside.txt", []byte("bad"), WithBoundary(dir))
	if err == nil {
		t.Fatal("expected boundary error")
	}
}

func TestSafeWrite_WithMkdir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.txt")

	if err := SafeWrite(path, []byte("deep"), WithMkdir()); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "deep" {
		t.Errorf("got %q", got)
	}
}

func TestSafeWrite_TraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "../../../etc/evil")

	err := SafeWrite(path, []byte("bad"))
	if err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestSafeWrite_WithPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "permed.txt")

	if err := SafeWrite(path, []byte("x"), WithPerm(0o600)); err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestSafeWrite_WithNonAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonatomic.txt")

	if err := SafeWrite(path, []byte("quick"), WithNonAtomic()); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "quick" {
		t.Errorf("got %q", got)
	}
}
