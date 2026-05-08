package fileop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path atomically using a temp-file + rename strategy.
// The temp file is created in the same directory to ensure same-filesystem rename.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-atomic-*")
	if err != nil {
		return fmt.Errorf("atomic write: create temp: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write: close: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("atomic write: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic write: rename: %w", err)
	}
	success = true
	return nil
}

// AtomicWriter supports streaming atomic writes (e.g. HTTP downloads).
// Data is written to a temp file; Close() triggers fsync + rename.
type AtomicWriter struct {
	tmp       *os.File
	finalPath string
	perm      os.FileMode
	done      bool
}

// CreateFileAtomic creates a streaming atomic writer.
// The caller must call Close() to finalize or Abort() to discard.
func CreateFileAtomic(path string, perm os.FileMode) (*AtomicWriter, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-atomic-*")
	if err != nil {
		return nil, fmt.Errorf("atomic create: %w", err)
	}
	return &AtomicWriter{tmp: tmp, finalPath: path, perm: perm}, nil
}

func (w *AtomicWriter) Write(p []byte) (int, error) { return w.tmp.Write(p) }

// ReadFrom implements io.ReaderFrom for io.Copy optimization.
func (w *AtomicWriter) ReadFrom(r io.Reader) (int64, error) { return io.Copy(w.tmp, r) }

// Close finalizes the atomic write: sync → chmod → rename.
func (w *AtomicWriter) Close() error {
	if w.done {
		return nil
	}
	w.done = true
	tmpName := w.tmp.Name()

	if err := w.tmp.Sync(); err != nil {
		w.tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("atomic close: sync: %w", err)
	}
	if err := w.tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic close: close: %w", err)
	}
	if err := os.Chmod(tmpName, w.perm); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic close: chmod: %w", err)
	}
	if err := os.Rename(tmpName, w.finalPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic close: rename: %w", err)
	}
	return nil
}

// Abort discards the write and removes the temp file.
func (w *AtomicWriter) Abort() error {
	if w.done {
		return nil
	}
	w.done = true
	tmpName := w.tmp.Name()
	w.tmp.Close()
	return os.Remove(tmpName)
}
