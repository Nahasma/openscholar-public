package fileop

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	if err := WriteFileAtomic(path, data, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("got %q, want %q", got, data)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("perm = %o, want 0644", info.Mode().Perm())
	}
}

func TestWriteFileAtomic_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := WriteFileAtomic(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

func TestWriteFileAtomic_NoTempLeftOnError(t *testing.T) {
	dir := t.TempDir()
	// Write to a non-existent subdirectory → should fail at CreateTemp
	path := filepath.Join(dir, "nonexistent", "sub", "file.txt")

	err := WriteFileAtomic(path, []byte("data"), 0o644)
	if err == nil {
		t.Fatal("expected error for non-existent dir")
	}

	// No temp files should be left in dir
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "nonexistent" {
			t.Errorf("unexpected file left: %s", e.Name())
		}
	}
}

func TestWriteFileAtomic_Concurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.txt")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte("goroutine write")
			_ = WriteFileAtomic(path, data, 0o644)
		}(i)
	}
	wg.Wait()

	// File should exist and be valid
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after concurrent writes: %v", err)
	}
	if string(got) != "goroutine write" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestCreateFileAtomic_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.bin")

	w, err := CreateFileAtomic(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("streaming data")
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestCreateFileAtomic_Abort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aborted.bin")

	w, err := CreateFileAtomic(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("some data"))
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}

	// Final file should NOT exist
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("aborted file should not exist")
	}
}

func TestCreateFileAtomic_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "double.bin")

	w, err := CreateFileAtomic(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("data"))
	w.Close()

	// Second close should be no-op
	if err := w.Close(); err != nil {
		t.Errorf("second Close should be nil, got %v", err)
	}
}
